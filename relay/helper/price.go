package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// defaultTieredPreConsumeMaxTokens is the fallback completion-token estimate
// used for tiered expression pre-consume when the client omits max_tokens, so
// the pre-consumed quota still reflects a plausible output cost in paid groups.
const defaultTieredPreConsumeMaxTokens = 8192

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) hosttypes.GroupRatioInfo {
	groupRatioInfo := hosttypes.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	// Reseller layer: a customer-specific or reseller-default retail price wins
	// over both group ratios, and carries the wholesale ratio downstream as
	// margin evidence. Money moves only when the reseller transfers quota.
	applyResellerPricing(relayInfo, &groupRatioInfo)

	return groupRatioInfo
}

// applyResellerPricing overrides the resolved group ratio when the customer has
// a configured reseller price for this billing group, and always attaches the
// reseller evidence the margin rollup needs. A customer whose group has no
// reseller retail price is still charged the platform ratio, but the spread
// between that ratio and the reseller's wholesale cost is the reseller's margin
// and must be recorded when a wholesale cost exists.
func applyResellerPricing(relayInfo *relaycommon.RelayInfo, groupRatioInfo *hosttypes.GroupRatioInfo) {
	relayInfo.ResellerOwnerID = 0
	relayInfo.ResellerCostRatio = 0
	relayInfo.ResellerRetailRatio = 0
	if relayInfo.UserId <= 0 || relayInfo.UsingGroup == "" {
		return
	}
	pricing, err := model.ResolveResellerPricing(relayInfo.UserId, relayInfo.UsingGroup)
	if err != nil {
		logger.LogError(nil, "failed to resolve reseller pricing: "+err.Error())
		return
	}
	if pricing.OwnerID <= 0 || pricing.CostRatio <= 0 {
		return
	}
	if pricing.Override {
		groupRatioInfo.GroupSpecialRatio = pricing.Retail
		groupRatioInfo.GroupRatio = pricing.Retail
		groupRatioInfo.HasSpecialRatio = true
		relayInfo.ResellerRetailRatio = pricing.Retail
	} else {
		relayInfo.ResellerRetailRatio = groupRatioInfo.GroupRatio
	}
	relayInfo.ResellerOwnerID = pricing.OwnerID
	relayInfo.ResellerCostRatio = pricing.CostRatio
}

// imageResolutionSize 取出本次请求的出图尺寸。
// 图片接口走 ImageRequest；chat/completions 生图把 size 放在通用请求里；
// 两者都没拿到时回落到原始请求体的 size 字段。
func imageResolutionSize(c *gin.Context, info *relaycommon.RelayInfo, meta *types.TokenCountMeta) string {
	if meta != nil && strings.TrimSpace(meta.ImageSize) != "" {
		return meta.ImageSize
	}
	if info != nil && info.Request != nil {
		switch request := info.Request.(type) {
		case *dto.ImageRequest:
			if strings.TrimSpace(request.Size) != "" {
				return request.Size
			}
		case *dto.GeneralOpenAIRequest:
			if strings.TrimSpace(request.Size) != "" {
				return request.Size
			}
		}
	}
	// chat/completions 生图场景下请求体里才有 size；仅在模型配置了分辨率档位时读取。
	body, err := readIncomingBillingExprBody(c)
	if err != nil || len(body) == 0 {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(body, "size").String())
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (hosttypes.PriceData, error) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	groupRatioInfo := HandleGroupRatio(c, info)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
	}

	// 图片按分辨率计费：命中档位时以档位价作为本次按次单价。
	// 该价格随后与 n、分组倍率、峰谷系数叠加，预扣与结算读同一份 PriceData。
	var imageResolutionInfo *hosttypes.ImageResolutionInfo
	if ratio_setting.HasImageResolutionPricing(info.OriginModelName) {
		if matched, ok := ratio_setting.MatchImageResolutionPrice(info.OriginModelName, imageResolutionSize(c, info, meta)); ok {
			modelPrice = matched.Price
			usePrice = true
			imageResolutionInfo = &hosttypes.ImageResolutionInfo{
				TierName: matched.TierName,
				Width:    matched.Width,
				Height:   matched.Height,
				Size:     matched.Size,
				Price:    matched.Price,
			}
		}
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	var peakPricingInfo *hosttypes.PeakPricingInfo
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return hosttypes.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)

		// 峰谷系数在请求开始时求值一次：输入倍率乘在模型倍率上，
		// 输出 / 缓存以「相对输入价」的形式折算，保证三类 token 各自独立定价。
		peakFactors := ratio_setting.GetPeakFactors(info.OriginModelName, info.StartTime)
		if !peakFactors.IsNeutral() {
			modelRatio = modelRatio * peakFactors.Input
			completionRatio = completionRatio * peakFactors.Output / peakFactors.Input
			cacheRatio = cacheRatio * peakFactors.Cache / peakFactors.Input
			cacheCreationRatio = cacheCreationRatio * peakFactors.CacheWrite / peakFactors.Input
			cacheCreationRatio5m = cacheCreationRatio5m * peakFactors.CacheWrite5m / peakFactors.Input
			cacheCreationRatio1h = cacheCreationRatio1h * peakFactors.CacheWrite1h / peakFactors.Input
			audioCompletionRatio = audioCompletionRatio * peakFactors.Output / peakFactors.Input
		}
		// 峰时系数可能正好是 1（等价于基础价），此时倍率无需折算，
		// 但日志仍要记录本次是峰时还是谷时，便于用户核对优惠。
		if peakFactors.Scheduled {
			peakPricingInfo = &hosttypes.PeakPricingInfo{
				RuleName:     peakFactors.RuleName,
				IsPeak:       peakFactors.IsPeak,
				Holiday:      peakFactors.Holiday,
				Input:        peakFactors.Input,
				Output:       peakFactors.Output,
				Cache:        peakFactors.Cache,
				CacheWrite:   peakFactors.CacheWrite,
				CacheWrite5m: peakFactors.CacheWrite5m,
				CacheWrite1h: peakFactors.CacheWrite1h,
			}
		}

		ratio := modelRatio * groupRatioInfo.GroupRatio
		quota, err := common.QuotaFromFloatStrict(float64(preConsumedTokens) * ratio)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		preConsumedQuota = quota
	} else {
		// 分辨率档位价已经是最终按次单价，不再叠加 dall-e 的尺寸倍率。
		if meta.ImagePriceRatio != 0 && imageResolutionInfo == nil {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := hosttypes.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
		PeakPricing:          peakPricingInfo,
		ImageResolution:      imageResolutionInfo,
	}
	if usePrice {
		for name, ratio := range meta.BillingRatios {
			priceData.AddOtherRatio(name, ratio)
		}
		quotaToPreConsume := priceData.ApplyOtherRatiosToFloat(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		quota, err := common.QuotaFromFloatStrict(quotaToPreConsume)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "model_price_helper result: %s", priceData.ToSetting())
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次/按量计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (hosttypes.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return hosttypes.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		var err error
		quota, err = common.QuotaFromFloatStrict(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		var err error
		quota, err = common.QuotaFromFloatStrict(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := hosttypes.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		ModelRatio:     modelRatio,
		UsePrice:       usePrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData, nil
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo hosttypes.GroupRatioInfo) (hosttypes.PriceData, error) {
	exprStr, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return hosttypes.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
	}

	estimatedCompletionTokens := meta.MaxTokens
	if estimatedCompletionTokens == 0 && groupRatioInfo.GroupRatio != 0 {
		estimatedCompletionTokens = defaultTieredPreConsumeMaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return hosttypes.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		// expr-lang reports the offending source line with the whole expression,
		// so the raw error would hand the administrator's pricing rules to the
		// caller. Keep the detail in the server log and answer generically.
		common.SysError(fmt.Sprintf("model %s tiered expr run failed: %v", info.OriginModelName, err))
		return hosttypes.PriceData{}, fmt.Errorf("model %s billing expression failed; contact the administrator", info.OriginModelName)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	if err != nil {
		return hosttypes.PriceData{}, err
	}

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := hosttypes.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	logger.LogDebug(c, "model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier)

	info.PriceData = priceData
	return priceData, nil
}
