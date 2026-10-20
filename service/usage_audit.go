package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

type UsageAuditTokens struct {
	Input      int64 `json:"input_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
	Output     int64 `json:"output_tokens"`
	TotalInput int64 `json:"total_input_tokens"`
}

type UsageAuditCost struct {
	Input      string `json:"input"`
	CacheRead  string `json:"cache_read"`
	CacheWrite string `json:"cache_write"`
	Output     string `json:"output"`
	Rounding   string `json:"rounding"`
	Adjustment string `json:"adjustment"`
	Total      string `json:"total"`
}

type UsageAuditComparison struct {
	Platform   UsageAuditCost `json:"platform"`
	List       UsageAuditCost `json:"list"`
	Difference UsageAuditCost `json:"difference"`
}

type UsageAuditRate struct {
	Model string `json:"model"`
	Group string `json:"group"`
	Ratio string `json:"ratio"`
}

// Preserve the historical billing inputs needed to explain each CSV charge.
// This is a whitelist of usage/pricing evidence, not the log's arbitrary other data.
type UsageAuditEvidence struct {
	RequestPath     string   `json:"request_path"`
	Conversion      []string `json:"request_conversion"`
	BillingPath     string   `json:"billing_path"`
	ModelRatio      *float64 `json:"model_ratio"`
	CompletionRatio *float64 `json:"completion_ratio"`
	CacheRatio      *float64 `json:"cache_ratio"`
	WriteRatio      *float64 `json:"cache_creation_ratio"`
	WriteRatio5m    *float64 `json:"cache_creation_ratio_5m"`
	WriteRatio1h    *float64 `json:"cache_creation_ratio_1h"`
	Write5m         int64    `json:"cache_creation_tokens_5m"`
	Write1h         int64    `json:"cache_creation_tokens_1h"`
}

// Values the candidate duplicate tokens at the historical ordinary-input rate.
// It is separate from settled charges and never changes a wallet or quota.
type UsageAuditInputRecalculation struct {
	RequestCount              int    `json:"request_count"`
	ExcludedWriteRequestCount int    `json:"excluded_cache_write_request_count"`
	Tokens                    int64  `json:"tokens"`
	CacheRead                 int64  `json:"cache_read_input_tokens"`
	CacheWrite                int64  `json:"cache_creation_input_tokens"`
	UnpricedTokens            int64  `json:"unpriced_tokens"`
	PlatformAmount            string `json:"platform_amount"`
	ListAmount                string `json:"list_amount"`
}

type UsageAuditRow struct {
	InputRecalculation      *UsageAuditInputRecalculation `json:"input_recalculation"`
	ID                      int                           `json:"id"`
	UserID                  int                           `json:"user_id"`
	Username                string                        `json:"username"`
	TokenID                 int                           `json:"token_id"`
	TokenName               string                        `json:"token_name"`
	CreatedAt               int64                         `json:"created_at"`
	RequestID               string                        `json:"request_id"`
	UpstreamRequestID       string                        `json:"upstream_request_id"`
	Model                   string                        `json:"model"`
	Group                   string                        `json:"group"`
	ChannelID               int                           `json:"channel_id"`
	IsStream                bool                          `json:"is_stream"`
	Kind                    string                        `json:"kind"`
	Protocol                string                        `json:"protocol"`
	UsageSemantic           string                        `json:"usage_semantic"`
	GroupRatio              *float64                      `json:"group_ratio"`
	Tokens                  *UsageAuditTokens             `json:"tokens"`
	ActualAmount            string                        `json:"actual_amount"`
	ListPrices              *UsageAuditCost               `json:"list_prices_per_million"`
	ListCost                *UsageAuditCost               `json:"list_cost"`
	DiscountedCost          *UsageAuditCost               `json:"discounted_cost"`
	RecalculationDifference *string                       `json:"recalculation_difference"`
	PossibleDuplicateInput  int64                         `json:"possible_duplicate_input_tokens"`
	DuplicateStatus         string                        `json:"duplicate_status"`
	Issue                   string                        `json:"issue"`
	Evidence                *UsageAuditEvidence           `json:"evidence"`
}

type UsageAuditSummary struct {
	InputRecalculation      UsageAuditInputRecalculation `json:"input_recalculation"`
	Count                   int                          `json:"count"`
	PricedCount             int                          `json:"priced_count"`
	UnknownUsageCount       int                          `json:"unknown_usage_count"`
	DuplicateReviewCount    int                          `json:"duplicate_review_count"`
	Tokens                  UsageAuditTokens             `json:"tokens"`
	PossibleDuplicateInput  int64                        `json:"possible_duplicate_input_tokens"`
	ActualAmount            string                       `json:"actual_amount"`
	PricedActualAmount      string                       `json:"priced_actual_amount"`
	ListCost                UsageAuditCost               `json:"list_cost"`
	DiscountedCost          UsageAuditCost               `json:"discounted_cost"`
	RecalculationDifference string                       `json:"recalculation_difference"`
	Comparison              *UsageAuditComparison        `json:"comparison"`
}

type UsageAuditGroup struct {
	Label string `json:"label"`
	UsageAuditSummary
}

type UsageAuditReport struct {
	RecalculationBasis string                 `json:"recalculation_basis"`
	Filter             model.UsageAuditFilter `json:"filter"`
	GeneratedAt        int64                  `json:"generated_at"`
	Timezone           string                 `json:"timezone"`
	PriceSource        string                 `json:"price_source"`
	Currency           BillingCurrency        `json:"currency"`
	SHA256             string                 `json:"sha256"`
	Summary            UsageAuditSummary      `json:"summary"`
	Days               []UsageAuditGroup      `json:"days"`
	Models             []UsageAuditGroup      `json:"models"`
	Users              []UsageAuditGroup      `json:"users"`
	Keys               []UsageAuditGroup      `json:"keys"`
	Rows               []UsageAuditRow        `json:"rows"`
	Rates              []UsageAuditRate       `json:"rates"`
}

type usageAuditMetadata struct {
	Semantic        string             `json:"usage_semantic"`
	Claude          bool               `json:"claude"`
	CacheRead       *int64             `json:"cache_tokens"`
	CacheWrite      int64              `json:"cache_creation_tokens"`
	WriteTotal      *int64             `json:"cache_write_tokens"`
	Write5m         int64              `json:"cache_creation_tokens_5m"`
	Write1h         int64              `json:"cache_creation_tokens_1h"`
	InputTotal      *int64             `json:"input_tokens_total"`
	ModelRatio      *float64           `json:"model_ratio"`
	CompletionRatio *float64           `json:"completion_ratio"`
	GroupRatio      *float64           `json:"group_ratio"`
	CacheRatio      *float64           `json:"cache_ratio"`
	WriteRatio      *float64           `json:"cache_creation_ratio"`
	WriteRatio5m    *float64           `json:"cache_creation_ratio_5m"`
	WriteRatio1h    *float64           `json:"cache_creation_ratio_1h"`
	ModelPrice      float64            `json:"model_price"`
	BillingMode     string             `json:"billing_mode"`
	Path            string             `json:"request_path"`
	Conversion      []string           `json:"request_conversion"`
	Task            bool               `json:"is_task"`
	Audio           bool               `json:"audio"`
	Image           bool               `json:"image"`
	WS              bool               `json:"ws"`
	PerCall         bool               `json:"per_call_billing"`
	OtherRatios     map[string]float64 `json:"other_ratios"`
	Tools           []any              `json:"tool_surcharges"`
	Admin           struct {
		BillingPath string `json:"usage_billing_path"`
		Local       bool   `json:"local_count_tokens"`
	} `json:"admin_info"`
}

// Rows always retain the settled amount. Missing evidence makes reference
// prices unknown rather than falling back to today's model/group settings.
func buildUsageAuditRow(log *model.Log, currency BillingCurrency) (UsageAuditRow, error) {
	row := UsageAuditRow{ID: log.Id, UserID: log.UserId, Username: log.Username, TokenID: log.TokenId, TokenName: log.TokenName,
		CreatedAt: log.CreatedAt, RequestID: log.RequestId, UpstreamRequestID: log.UpstreamRequestId, Model: log.ModelName,
		Group: log.Group, ChannelID: log.ChannelId, IsStream: log.IsStream, Kind: "consumption", DuplicateStatus: "not_identified"}
	if log.Quota < 0 || int64(log.Quota) > common.MaxWalletQuota {
		return row, errors.New("invalid recorded usage quota")
	}
	sign := int64(1)
	if log.Type == model.LogTypeRefund {
		row.Kind, sign = "refund", -1
	}
	amountRate, rateErr := decimal.NewFromString(currency.Rate)
	amountUnit, unitErr := decimal.NewFromString(currency.QuotaPerUnit)
	if rateErr != nil || unitErr != nil || !amountRate.IsPositive() || !amountUnit.IsPositive() {
		return row, errors.New("invalid usage audit currency")
	}
	row.ActualAmount = decimal.NewFromInt(int64(log.Quota) * sign).Mul(amountRate).Div(amountUnit).StringFixed(9)
	var meta usageAuditMetadata
	if common.UnmarshalJsonStr(log.Other, &meta) != nil {
		row.Issue = "missing_usage_evidence"
		return row, nil
	}
	row.Evidence = &UsageAuditEvidence{RequestPath: meta.Path, Conversion: meta.Conversion, BillingPath: meta.Admin.BillingPath,
		ModelRatio: meta.ModelRatio, CompletionRatio: meta.CompletionRatio, CacheRatio: meta.CacheRatio, WriteRatio: meta.WriteRatio,
		WriteRatio5m: meta.WriteRatio5m, WriteRatio1h: meta.WriteRatio1h, Write5m: meta.Write5m, Write1h: meta.Write1h}
	row.GroupRatio = meta.GroupRatio
	switch meta.Path {
	case "/v1/messages":
		row.Protocol = "anthropic"
	case "/v1/chat/completions", "/v1/responses":
		row.Protocol = "openai"
	default:
		row.Protocol = "other"
	}
	if meta.Admin.Local || strings.Contains(meta.Admin.BillingPath, "estimated") {
		row.Issue = "missing_usage_evidence"
		return row, nil
	}
	if log.Type == model.LogTypeRefund {
		// A refund is another financial event for an existing request, not a
		// second model invocation. Do not count its copied usage again.
		row.Issue = "unsupported_pricing"
		return row, nil
	}
	if meta.CacheRead == nil || *meta.CacheRead < 0 || meta.CacheWrite < 0 || meta.Write5m < 0 || meta.Write1h < 0 ||
		*meta.CacheRead > common.MaxQuota || meta.CacheWrite > common.MaxQuota || meta.Write5m > common.MaxQuota || meta.Write1h > common.MaxQuota ||
		log.PromptTokens < 0 || log.CompletionTokens < 0 || log.PromptTokens > common.MaxQuota || log.CompletionTokens > common.MaxQuota {
		row.Issue = "missing_usage_evidence"
		return row, nil
	}
	write := meta.CacheWrite
	if meta.WriteTotal != nil {
		write = *meta.WriteTotal
	}
	if write == 0 {
		write = meta.Write5m + meta.Write1h
	}
	if write < 0 || write > common.MaxQuota || meta.Write5m+meta.Write1h > write {
		row.Issue = "invalid_usage_evidence"
		return row, nil
	}
	semantic := meta.Semantic
	if semantic == "" {
		switch {
		case strings.HasPrefix(meta.Admin.BillingPath, "billing-usage-anthropic"):
			semantic = "anthropic"
		case strings.HasPrefix(meta.Admin.BillingPath, "billing-usage-openai"):
			semantic = "openai"
		case strings.HasPrefix(meta.Admin.BillingPath, "billing-usage-gemini"):
			semantic = "gemini"
		case meta.Claude:
			semantic = "anthropic"
		case meta.Write5m > 0 || meta.Write1h > 0:
			// Legacy Claude-derived OpenAI usage can retain uncached input.
			// The conversion path alone cannot establish its token semantics.
		case len(meta.Conversion) > 0 && strings.HasPrefix(meta.Conversion[len(meta.Conversion)-1], "OpenAI"):
			semantic = "openai"
		case *meta.CacheRead == 0 && write == 0:
			semantic = "openai"
		}
	}
	row.UsageSemantic = semantic
	tokens := UsageAuditTokens{CacheRead: *meta.CacheRead, CacheWrite: write, Output: int64(log.CompletionTokens)}
	switch {
	case meta.InputTotal != nil:
		tokens.TotalInput = *meta.InputTotal
		tokens.Input = tokens.TotalInput - tokens.CacheRead - tokens.CacheWrite
	case semantic == "anthropic":
		tokens.Input = int64(log.PromptTokens)
		tokens.TotalInput = tokens.Input + tokens.CacheRead + tokens.CacheWrite
	case semantic == "openai", semantic == "gemini":
		tokens.TotalInput = int64(log.PromptTokens)
		tokens.Input = tokens.TotalInput - tokens.CacheRead - tokens.CacheWrite
	default:
		row.Issue = "missing_usage_evidence"
		return row, nil
	}
	if tokens.Input < 0 || tokens.TotalInput < 0 || tokens.TotalInput > 3*int64(common.MaxQuota) {
		row.Issue = "invalid_usage_evidence"
		return row, nil
	}
	row.Tokens = &tokens
	// This report evaluates the legacy OpenAI cache_write_tokens branch. The
	// historical source field is a stated audit assumption, not recovered wire
	// evidence. With a positive native write count the old converter subtracted
	// both reads and writes; only read-only requests skipped that subtraction.
	if row.Protocol == "anthropic" && semantic == "openai" && len(meta.Conversion) > 1 &&
		strings.HasPrefix(meta.Conversion[len(meta.Conversion)-1], "OpenAI") {
		if tokens.CacheWrite > 0 {
			row.DuplicateStatus = "excluded_cache_write_present"
		} else if tokens.CacheRead > 0 {
			row.PossibleDuplicateInput = tokens.CacheRead
			row.DuplicateStatus = "needs_response_evidence"
			row.InputRecalculation = &UsageAuditInputRecalculation{RequestCount: 1, Tokens: tokens.CacheRead, CacheRead: tokens.CacheRead, UnpricedTokens: tokens.CacheRead}
		}
	}

	if log.Type == model.LogTypeRefund || meta.Task || meta.Audio || meta.Image || meta.WS || meta.PerCall || meta.ModelPrice > 0 ||
		meta.BillingMode != "" && meta.BillingMode != "ratio" || len(meta.OtherRatios) > 0 || len(meta.Tools) > 0 || meta.Admin.Local || strings.Contains(meta.Admin.BillingPath, "estimated") {
		row.Issue = "unsupported_pricing"
		return row, nil
	}
	if semantic != "anthropic" && meta.Write5m+meta.Write1h > 0 {
		row.Issue = "unsupported_pricing"
		return row, nil
	}
	for _, ratio := range []*float64{meta.ModelRatio, meta.GroupRatio} {
		if ratio == nil || *ratio < 0 || *ratio > common.MaxQuota || math.IsNaN(*ratio) || math.IsInf(*ratio, 0) {
			row.Issue = "missing_price_evidence"
			return row, nil
		}
	}
	if row.InputRecalculation != nil {
		rate, _ := decimal.NewFromString(currency.Rate)
		unit, _ := decimal.NewFromString(currency.QuotaPerUnit)
		amount := decimal.NewFromInt(row.PossibleDuplicateInput).Mul(decimal.NewFromFloat(*meta.ModelRatio)).Mul(rate).Div(unit)
		row.InputRecalculation.ListAmount = amount.StringFixed(9)
		row.InputRecalculation.PlatformAmount = amount.Mul(decimal.NewFromFloat(*meta.GroupRatio)).StringFixed(9)
		row.InputRecalculation.UnpricedTokens = 0
	}
	if meta.CompletionRatio == nil || *meta.CompletionRatio < 0 || *meta.CompletionRatio > common.MaxQuota || math.IsNaN(*meta.CompletionRatio) || math.IsInf(*meta.CompletionRatio, 0) {
		row.Issue = "missing_price_evidence"
		return row, nil
	}
	readRatio, writeRatio := decimal.Zero, decimal.Zero
	for _, component := range []struct {
		count  int64
		ratio  *float64
		target *decimal.Decimal
	}{
		{tokens.CacheRead, meta.CacheRatio, &readRatio}, {tokens.CacheWrite - meta.Write5m - meta.Write1h, meta.WriteRatio, &writeRatio},
	} {
		if component.count == 0 {
			continue
		}
		if component.ratio == nil || *component.ratio < 0 || *component.ratio > common.MaxQuota || math.IsNaN(*component.ratio) || math.IsInf(*component.ratio, 0) {
			row.Issue = "missing_price_evidence"
			return row, nil
		}
		*component.target = decimal.NewFromInt(component.count).Mul(decimal.NewFromFloat(*component.ratio))
	}
	for _, split := range []struct {
		count int64
		ratio *float64
	}{{meta.Write5m, meta.WriteRatio5m}, {meta.Write1h, meta.WriteRatio1h}} {
		if split.count == 0 {
			continue
		}
		if split.ratio == nil || *split.ratio < 0 || *split.ratio > common.MaxQuota || math.IsNaN(*split.ratio) || math.IsInf(*split.ratio, 0) {
			row.Issue = "missing_price_evidence"
			return row, nil
		}
		writeRatio = writeRatio.Add(decimal.NewFromInt(split.count).Mul(decimal.NewFromFloat(*split.ratio)))
	}
	modelRatio := decimal.NewFromFloat(*meta.ModelRatio)
	parts := [4]decimal.Decimal{decimal.NewFromInt(tokens.Input).Mul(modelRatio), readRatio.Mul(modelRatio), writeRatio.Mul(modelRatio), decimal.NewFromInt(tokens.Output).Mul(modelRatio).Mul(decimal.NewFromFloat(*meta.CompletionRatio))}
	minimumCharge := modelRatio.IsPositive() && tokens.TotalInput+tokens.Output > 0
	list, err := usageAuditCost(parts, decimal.NewFromInt(1), currency, minimumCharge)
	if err != nil {
		row.Issue = "invalid_price_evidence"
		return row, nil
	}
	discounted, err := usageAuditCost(parts, decimal.NewFromFloat(*meta.GroupRatio), currency, minimumCharge)
	if err != nil {
		row.Issue = "invalid_price_evidence"
		return row, nil
	}
	row.ListCost, row.DiscountedCost = &list, &discounted
	difference, _ := decimal.NewFromString(row.ActualAmount)
	recomputed, _ := decimal.NewFromString(discounted.Total)
	value := difference.Sub(recomputed).StringFixed(9)
	row.RecalculationDifference = &value
	rate, _ := decimal.NewFromString(currency.Rate)
	unit, _ := decimal.NewFromString(currency.QuotaPerUnit)
	inputPrice := modelRatio.Mul(decimal.NewFromInt(1000000)).Mul(rate).Div(unit)
	prices := UsageAuditCost{Input: inputPrice.String(), Output: inputPrice.Mul(decimal.NewFromFloat(*meta.CompletionRatio)).String()}
	if meta.CacheRatio != nil {
		prices.CacheRead = inputPrice.Mul(decimal.NewFromFloat(*meta.CacheRatio)).String()
	}
	if tokens.CacheWrite > 0 {
		prices.CacheWrite = writeRatio.Div(decimal.NewFromInt(tokens.CacheWrite)).Mul(inputPrice).String()
	}
	row.ListPrices = &prices
	return row, nil
}

// Round each request using the existing quota boundary, then sum the rounded
// charges. Component amounts retain a separate rounding reconciliation.
func usageAuditCost(parts [4]decimal.Decimal, group decimal.Decimal, currency BillingCurrency, minimumCharge bool) (UsageAuditCost, error) {
	rate, err := decimal.NewFromString(currency.Rate)
	if err != nil {
		return UsageAuditCost{}, err
	}
	unit, err := decimal.NewFromString(currency.QuotaPerUnit)
	if err != nil || !unit.IsPositive() {
		return UsageAuditCost{}, errors.New("invalid currency unit")
	}
	total := decimal.Zero
	amounts := [4]string{}
	for i, part := range parts {
		part = part.Mul(group)
		total = total.Add(part)
		amounts[i] = part.Mul(rate).Div(unit).StringFixed(9)
	}
	quota, err := common.QuotaFromDecimalStrict(total)
	if err != nil {
		return UsageAuditCost{}, err
	}
	if group.IsPositive() && minimumCharge && quota == 0 {
		quota = 1
	}
	rounded := decimal.NewFromInt(int64(quota)).Mul(rate).Div(unit)
	return UsageAuditCost{Input: amounts[0], CacheRead: amounts[1], CacheWrite: amounts[2], Output: amounts[3], Rounding: rounded.Sub(total.Mul(rate).Div(unit)).StringFixed(9), Total: rounded.StringFixed(9)}, nil
}

func addUsageAuditRow(summary *UsageAuditSummary, row UsageAuditRow) {
	summary.Count++
	repricing := UsageAuditInputRecalculation{}
	if row.InputRecalculation != nil {
		repricing = *row.InputRecalculation
	}
	summary.InputRecalculation.RequestCount += repricing.RequestCount
	if row.DuplicateStatus == "excluded_cache_write_present" {
		summary.InputRecalculation.ExcludedWriteRequestCount++
	}
	summary.InputRecalculation.Tokens += repricing.Tokens
	summary.InputRecalculation.CacheRead += repricing.CacheRead
	summary.InputRecalculation.CacheWrite += repricing.CacheWrite
	summary.InputRecalculation.UnpricedTokens += repricing.UnpricedTokens
	for _, amounts := range []struct {
		target *string
		source string
	}{
		{&summary.InputRecalculation.PlatformAmount, repricing.PlatformAmount},
		{&summary.InputRecalculation.ListAmount, repricing.ListAmount},
	} {
		previous, _ := decimal.NewFromString(*amounts.target)
		value, _ := decimal.NewFromString(amounts.source)
		*amounts.target = previous.Add(value).StringFixed(9)
	}
	actual, _ := decimal.NewFromString(summary.ActualAmount)
	amount, _ := decimal.NewFromString(row.ActualAmount)
	summary.ActualAmount = actual.Add(amount).StringFixed(9)
	if row.Tokens == nil && row.Kind != "refund" {
		summary.UnknownUsageCount++
	} else if row.Tokens != nil {
		summary.Tokens.Input += row.Tokens.Input
		summary.Tokens.CacheRead += row.Tokens.CacheRead
		summary.Tokens.CacheWrite += row.Tokens.CacheWrite
		summary.Tokens.Output += row.Tokens.Output
		summary.Tokens.TotalInput += row.Tokens.TotalInput
	}
	if row.DuplicateStatus == "needs_response_evidence" {
		summary.DuplicateReviewCount++
		summary.PossibleDuplicateInput += row.PossibleDuplicateInput
	}
	if row.ListCost == nil || row.DiscountedCost == nil {
		return
	}
	summary.PricedCount++
	priced, _ := decimal.NewFromString(summary.PricedActualAmount)
	summary.PricedActualAmount = priced.Add(amount).StringFixed(9)
	for _, pair := range []struct {
		target *UsageAuditCost
		source *UsageAuditCost
	}{{&summary.ListCost, row.ListCost}, {&summary.DiscountedCost, row.DiscountedCost}} {
		for _, field := range []struct {
			target *string
			value  string
		}{{&pair.target.Input, pair.source.Input}, {&pair.target.CacheRead, pair.source.CacheRead}, {&pair.target.CacheWrite, pair.source.CacheWrite}, {&pair.target.Output, pair.source.Output}, {&pair.target.Rounding, pair.source.Rounding}, {&pair.target.Total, pair.source.Total}} {
			previous, _ := decimal.NewFromString(*field.target)
			value, _ := decimal.NewFromString(field.value)
			*field.target = previous.Add(value).StringFixed(9)
		}
	}
	difference, _ := decimal.NewFromString(summary.PricedActualAmount)
	recomputed, _ := decimal.NewFromString(summary.DiscountedCost.Total)
	summary.RecalculationDifference = difference.Sub(recomputed).StringFixed(9)
	comparison := &UsageAuditComparison{Platform: summary.DiscountedCost, List: summary.ListCost}
	comparison.Platform.Total = summary.PricedActualAmount
	comparison.Platform.Adjustment = summary.RecalculationDifference
	comparison.List.Adjustment = "0.000000000"
	for _, field := range []struct {
		target         *string
		list, platform string
	}{
		{&comparison.Difference.Input, comparison.List.Input, comparison.Platform.Input},
		{&comparison.Difference.CacheRead, comparison.List.CacheRead, comparison.Platform.CacheRead},
		{&comparison.Difference.CacheWrite, comparison.List.CacheWrite, comparison.Platform.CacheWrite},
		{&comparison.Difference.Output, comparison.List.Output, comparison.Platform.Output},
		{&comparison.Difference.Rounding, comparison.List.Rounding, comparison.Platform.Rounding},
		{&comparison.Difference.Adjustment, comparison.List.Adjustment, comparison.Platform.Adjustment},
		{&comparison.Difference.Total, comparison.List.Total, comparison.Platform.Total},
	} {
		list, _ := decimal.NewFromString(field.list)
		platform, _ := decimal.NewFromString(field.platform)
		*field.target = list.Sub(platform).StringFixed(9)
	}
	summary.Comparison = comparison
}

func GetUsageAuditReport(ctx context.Context, filter model.UsageAuditFilter) (*UsageAuditReport, error) {
	now := common.GetTimestamp()
	if filter.UserID < 0 || filter.TokenID < 0 || filter.ChannelID < 0 || filter.StartAt <= 0 || filter.StartAt > now || filter.EndAt <= filter.StartAt ||
		filter.EndAt-filter.StartAt > 93*86400 || len(filter.Model) > 255 || len(filter.Group) > 64 ||
		filter.Protocol != "" && filter.Protocol != "anthropic" && filter.Protocol != "openai" {
		return nil, errors.New("invalid usage audit filters; select at most 93 days")
	}
	if filter.EndAt > now {
		filter.EndAt = now
	}
	if filter.EndAt <= filter.StartAt {
		return nil, errors.New("usage audit period has not started")
	}
	currency, err := currentBillingCurrency(false)
	if err != nil {
		return nil, err
	}
	logs, err := model.GetUsageAuditLogs(ctx, filter)
	if err != nil {
		return nil, err
	}
	report := &UsageAuditReport{RecalculationBasis: "legacy_openai_cache_write_tokens", Filter: filter, GeneratedAt: now, Timezone: "Asia/Shanghai", PriceSource: "historical_model_base_without_group_ratio", Currency: currency, Rows: make([]UsageAuditRow, 0, len(logs))}
	report.Summary.ActualAmount, report.Summary.PricedActualAmount = "0.000000000", "0.000000000"
	groups := [4]map[string]*UsageAuditGroup{}
	rates := make(map[UsageAuditRate]bool)
	for i := range groups {
		groups[i] = make(map[string]*UsageAuditGroup)
	}
	for _, log := range logs {
		row, err := buildUsageAuditRow(log, currency)
		if err != nil {
			return nil, err
		}
		if filter.Protocol != "" && row.Protocol != filter.Protocol {
			continue
		}
		report.Rows = append(report.Rows, row)
		if row.ListCost != nil && row.GroupRatio != nil {
			rates[UsageAuditRate{Model: row.Model, Group: row.Group, Ratio: decimal.NewFromFloat(*row.GroupRatio).String()}] = true
		}
		addUsageAuditRow(&report.Summary, row)
		labels := [4]string{time.Unix(row.CreatedAt, 0).In(billingLocation).Format("2006-01-02"), row.Model, strconv.Itoa(row.UserID) + " / " + row.Username, strconv.Itoa(row.TokenID) + " / " + row.TokenName}
		identities := [4]string{labels[0], row.Model, strconv.Itoa(row.UserID), strconv.Itoa(row.UserID) + "/" + strconv.Itoa(row.TokenID)}
		for i, label := range labels {
			group := groups[i][identities[i]]
			if group == nil {
				group = &UsageAuditGroup{Label: label}
				groups[i][identities[i]] = group
			}
			addUsageAuditRow(&group.UsageAuditSummary, row)
		}
	}
	for i, target := range []*[]UsageAuditGroup{&report.Days, &report.Models, &report.Users, &report.Keys} {
		*target = make([]UsageAuditGroup, 0, len(groups[i]))
		for _, group := range groups[i] {
			*target = append(*target, *group)
		}
		sort.Slice(*target, func(a, b int) bool { return (*target)[a].Label < (*target)[b].Label })
	}
	report.Rates = make([]UsageAuditRate, 0, len(rates))
	for rate := range rates {
		report.Rates = append(report.Rates, rate)
	}
	sort.Slice(report.Rates, func(i, j int) bool {
		a, b := report.Rates[i], report.Rates[j]
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Ratio < b.Ratio
	})
	report.SHA256, err = usageAuditReportDigest(report)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func usageAuditReportDigest(report *UsageAuditReport) (string, error) {
	snapshot := *report
	snapshot.SHA256 = ""
	body, err := common.Marshal(&snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

var ErrUsageAuditChanged = errors.New("usage audit data changed; run the audit again before exporting")

type UsageAuditExportInput struct {
	Filter      model.UsageAuditFilter `json:"filter"`
	GeneratedAt int64                  `json:"generated_at"`
	SHA256      string                 `json:"sha256"`
}

// Re-read the original frozen time range and compare its digest. PDF amounts
// always come from server evidence, never from client-supplied report rows.
func GetUsageAuditExportReport(ctx context.Context, input UsageAuditExportInput) (*UsageAuditReport, error) {
	if input.GeneratedAt <= 0 || input.GeneratedAt > common.GetTimestamp() || input.Filter.EndAt > input.GeneratedAt || len(input.SHA256) != 64 {
		return nil, ErrUsageAuditChanged
	}
	report, err := GetUsageAuditReport(ctx, input.Filter)
	if err != nil {
		return nil, err
	}
	report.GeneratedAt = input.GeneratedAt
	report.SHA256, err = usageAuditReportDigest(report)
	if err != nil {
		return nil, err
	}
	if report.SHA256 != input.SHA256 {
		return nil, ErrUsageAuditChanged
	}
	return report, nil
}
