package ratio_setting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// 图片按分辨率计费：同一个模型按出图尺寸落在不同价格档。
//
// 设计要点：
//   - 按像素面积分档，而不是枚举尺寸字符串。上游新增等比例尺寸时无需改配置，
//     1024x1024 与 1536x1024 分别落在 1M 与 2M 线两侧，语义仍然正确。
//   - 价格单位为美元/次，与 ModelPrice 一致；由 GroupRatio、经销商定价与
//     BillingRatios("n") 在其上继续折算，因此与按次计费链路完全兼容。
//   - 未配置档位的模型完全不走这里，dall-e 原有的 ImagePriceRatio 逻辑不变。

// ImageResolutionTier 是一个价格档。MaxPixels 为 0 表示兜底档，必须排在最后。
type ImageResolutionTier struct {
	Name      string  `json:"name"`
	MaxPixels int64   `json:"max_pixels"`
	Price     float64 `json:"price"`
}

// ImageResolutionModelPricing 描述单个模型的分辨率档位表。
// DefaultPrice 用于尺寸缺失、为 auto 或无法解析时的兜底价格；为 0 表示不兜底。
type ImageResolutionModelPricing struct {
	Tiers        []ImageResolutionTier `json:"tiers"`
	DefaultPrice float64               `json:"default_price"`
}

type ImageResolutionPricingSetting struct {
	Enabled bool `json:"enabled"`
	// Models 的键为模型名，支持结尾 * 通配。
	Models map[string]ImageResolutionModelPricing `json:"models"`
}

// ImageResolutionPrice 是模型某次请求最终命中的档位。
type ImageResolutionPrice struct {
	TierName string  `json:"tier"`
	MaxPixels int64  `json:"max_pixels,omitempty"`
	Price    float64 `json:"price"`
	// Width / Height 为解析出的尺寸；尺寸缺失或无法解析时为 0。
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// Size 为原始尺寸字符串，便于日志核对。
	Size string `json:"size,omitempty"`
}

var (
	imageResolutionPricingSetting = ImageResolutionPricingSetting{Models: map[string]ImageResolutionModelPricing{}}
	imageResolutionPricingLock    = sync.RWMutex{}
)

func GetImageResolutionPricingSetting() ImageResolutionPricingSetting {
	imageResolutionPricingLock.RLock()
	defer imageResolutionPricingLock.RUnlock()
	models := make(map[string]ImageResolutionModelPricing, len(imageResolutionPricingSetting.Models))
	for name, pricing := range imageResolutionPricingSetting.Models {
		tiers := make([]ImageResolutionTier, len(pricing.Tiers))
		copy(tiers, pricing.Tiers)
		pricing.Tiers = tiers
		models[name] = pricing
	}
	return ImageResolutionPricingSetting{
		Enabled: imageResolutionPricingSetting.Enabled,
		Models:  models,
	}
}

func ImageResolutionPricing2JSONString() string {
	data, err := common.Marshal(GetImageResolutionPricingSetting())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func UpdateImageResolutionPricingByJSONString(jsonString string) error {
	var setting ImageResolutionPricingSetting
	jsonString = strings.TrimSpace(jsonString)
	if jsonString == "" {
		jsonString = "{}"
	}
	if err := common.Unmarshal([]byte(jsonString), &setting); err != nil {
		return err
	}
	if err := ValidateImageResolutionPricingSetting(setting); err != nil {
		return err
	}
	if setting.Models == nil {
		setting.Models = map[string]ImageResolutionModelPricing{}
	}
	for name, pricing := range setting.Models {
		trimmed := make([]ImageResolutionTier, 0, len(pricing.Tiers))
		for _, tier := range pricing.Tiers {
			tier.Name = strings.TrimSpace(tier.Name)
			trimmed = append(trimmed, tier)
		}
		pricing.Tiers = trimmed
		setting.Models[name] = pricing
	}
	imageResolutionPricingLock.Lock()
	imageResolutionPricingSetting = setting
	imageResolutionPricingLock.Unlock()
	return nil
}

func ValidateImageResolutionPricingSetting(setting ImageResolutionPricingSetting) error {
	for name, pricing := range setting.Models {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("模型名称不能为空")
		}
		if math.IsNaN(pricing.DefaultPrice) || math.IsInf(pricing.DefaultPrice, 0) || pricing.DefaultPrice < 0 {
			return fmt.Errorf("模型 %s 的默认价格必须是非负有限数", name)
		}
		lastPixels := int64(-1)
		fallbackSeen := false
		names := make(map[string]struct{}, len(pricing.Tiers))
		for _, tier := range pricing.Tiers {
			tierName := strings.TrimSpace(tier.Name)
			if tierName == "" {
				return fmt.Errorf("模型 %s 存在未命名的价格档", name)
			}
			if _, ok := names[tierName]; ok {
				return fmt.Errorf("模型 %s 的价格档 %s 重复", name, tierName)
			}
			names[tierName] = struct{}{}
			if math.IsNaN(tier.Price) || math.IsInf(tier.Price, 0) || tier.Price <= 0 {
				return fmt.Errorf("模型 %s 的价格档 %s 价格必须大于 0", name, tierName)
			}
			if tier.MaxPixels < 0 {
				return fmt.Errorf("模型 %s 的价格档 %s 像素上限不能为负", name, tierName)
			}
			if fallbackSeen {
				return fmt.Errorf("模型 %s 的兜底档必须排在最后", name)
			}
			if tier.MaxPixels == 0 {
				fallbackSeen = true
				continue
			}
			// 升序保证「命中的是第一个够大的档」，配置顺序即优先级。
			if tier.MaxPixels <= lastPixels {
				return fmt.Errorf("模型 %s 的价格档 %s 像素上限必须按从低到高排列且不能重复", name, tierName)
			}
			lastPixels = tier.MaxPixels
		}
	}
	return nil
}

// HasImageResolutionPricing 报告模型是否配置了分辨率档位表，用于跳过不必要的尺寸解析。
func HasImageResolutionPricing(modelName string) bool {
	imageResolutionPricingLock.RLock()
	defer imageResolutionPricingLock.RUnlock()
	if !imageResolutionPricingSetting.Enabled {
		return false
	}
	_, ok := findImageResolutionPricing(imageResolutionPricingSetting.Models, modelName)
	return ok
}

func findImageResolutionPricing(models map[string]ImageResolutionModelPricing, modelName string) (ImageResolutionModelPricing, bool) {
	if len(models) == 0 || modelName == "" {
		return ImageResolutionModelPricing{}, false
	}
	// 先精确匹配，再按配置顺序试通配，与峰谷定价的覆盖规则一致。
	if pricing, ok := models[modelName]; ok {
		return pricing, true
	}
	for pattern, pricing := range models {
		if !strings.HasSuffix(pattern, "*") {
			continue
		}
		if strings.HasPrefix(modelName, strings.TrimSuffix(pattern, "*")) {
			return pricing, true
		}
	}
	return ImageResolutionModelPricing{}, false
}

// ParseImageSize 解析 "1024x1024" 形式的尺寸；auto 或非法值返回 false。
func ParseImageSize(size string) (width int, height int, ok bool) {
	trimmed := strings.ToLower(strings.TrimSpace(size))
	if trimmed == "" || trimmed == "auto" {
		return 0, 0, false
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == 'x' || r == '*' || r == '×'
	})
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	// 尺寸来自客户端，先做上界保护再相乘，避免溢出成负数后误命中兜底档。
	if width > maxImageResolutionSide || height > maxImageResolutionSide {
		return 0, 0, false
	}
	return width, height, true
}

// maxImageResolutionSide 是单边像素上限，远高于任何真实出图尺寸。
const maxImageResolutionSide = 100_000

// MatchImageResolutionPrice 按出图尺寸返回命中的档位。
// 尺寸无法解析时使用 DefaultPrice；既无匹配档也无默认价时返回 false，交由普通模型价处理。
func MatchImageResolutionPrice(modelName string, size string) (ImageResolutionPrice, bool) {
	imageResolutionPricingLock.RLock()
	defer imageResolutionPricingLock.RUnlock()
	if !imageResolutionPricingSetting.Enabled {
		return ImageResolutionPrice{}, false
	}
	pricing, ok := findImageResolutionPricing(imageResolutionPricingSetting.Models, modelName)
	if !ok {
		return ImageResolutionPrice{}, false
	}

	width, height, parsed := ParseImageSize(size)
	if !parsed {
		if pricing.DefaultPrice <= 0 {
			return ImageResolutionPrice{}, false
		}
		return ImageResolutionPrice{TierName: "default", Price: pricing.DefaultPrice, Size: size}, true
	}

	pixels := int64(width) * int64(height)
	var fallback *ImageResolutionTier
	for i := range pricing.Tiers {
		tier := pricing.Tiers[i]
		if tier.MaxPixels == 0 {
			fallback = &tier
			continue
		}
		if pixels <= tier.MaxPixels {
			return ImageResolutionPrice{
				TierName:  tier.Name,
				MaxPixels: tier.MaxPixels,
				Price:     tier.Price,
				Width:     width,
				Height:    height,
				Size:      size,
			}, true
		}
	}
	if fallback != nil {
		return ImageResolutionPrice{
			TierName:  fallback.Name,
			MaxPixels: fallback.MaxPixels,
			Price:     fallback.Price,
			Width:     width,
			Height:    height,
			Size:      size,
		}, true
	}
	if pricing.DefaultPrice > 0 {
		return ImageResolutionPrice{
			TierName: "default",
			Price:    pricing.DefaultPrice,
			Width:    width,
			Height:   height,
			Size:     size,
		}, true
	}
	return ImageResolutionPrice{}, false
}

// ImageResolutionPricingView 是给模型广场展示用的档位说明。
type ImageResolutionPricingView struct {
	Tiers        []ImageResolutionTier `json:"tiers"`
	DefaultPrice float64               `json:"default_price,omitempty"`
}

// GetModelImageResolutionPricingView 返回模型的分辨率档位；未配置时返回 nil。
func GetModelImageResolutionPricingView(modelName string) *ImageResolutionPricingView {
	imageResolutionPricingLock.RLock()
	defer imageResolutionPricingLock.RUnlock()
	if !imageResolutionPricingSetting.Enabled {
		return nil
	}
	pricing, ok := findImageResolutionPricing(imageResolutionPricingSetting.Models, modelName)
	if !ok || (len(pricing.Tiers) == 0 && pricing.DefaultPrice <= 0) {
		return nil
	}
	view := &ImageResolutionPricingView{
		Tiers:        make([]ImageResolutionTier, len(pricing.Tiers)),
		DefaultPrice: pricing.DefaultPrice,
	}
	copy(view.Tiers, pricing.Tiers)
	return view
}
