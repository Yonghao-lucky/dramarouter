package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAuditReportFiltersExactKeyAndSummarizesIncompleteEvidence(t *testing.T) {
	truncate(t)
	const at = int64(1700000000)
	metadata := `{"usage_semantic":"openai","model_ratio":2,"completion_ratio":5,"group_ratio":0.5,"cache_ratio":0.1,"cache_tokens":800,"request_path":"/v1/messages","request_conversion":["Claude Messages","OpenAI Compatible"]}`
	logs := []model.Log{
		{UserId: 801, TokenId: 91, TokenName: "same-name", CreatedAt: at, Type: model.LogTypeConsume, ModelName: "kimi-k3", PromptTokens: 1000, CompletionTokens: 20, Quota: 380, RequestId: "selected", Other: metadata},
		{UserId: 801, TokenId: 91, CreatedAt: at + 1, Type: model.LogTypeConsume, Quota: 500, RequestId: "missing", Other: `{}`},
		{UserId: 801, TokenId: 92, TokenName: "same-name", CreatedAt: at, Type: model.LogTypeConsume, Quota: 999999},
		{UserId: 802, TokenId: 91, CreatedAt: at, Type: model.LogTypeConsume, Quota: 999999},
		{UserId: 801, TokenId: 91, CreatedAt: at + 2, Type: model.LogTypeConsume, Quota: 999999},
		{UserId: 801, TokenId: 91, CreatedAt: at, Type: model.LogTypeTopup, Quota: 999999},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)
	report, err := GetUsageAuditReport(context.Background(), model.UsageAuditFilter{UserID: 801, TokenID: 91, StartAt: at, EndAt: at + 2})
	require.NoError(t, err)
	require.Len(t, report.Rows, 2)
	assert.Equal(t, 2, report.Summary.Count)
	assert.Equal(t, 1, report.Summary.PricedCount)
	assert.Equal(t, 1, report.Summary.UnknownUsageCount)
	assert.Equal(t, int64(200), report.Summary.Tokens.Input)
	assert.Equal(t, int64(800), report.Summary.Tokens.CacheRead)
	assert.Len(t, report.SHA256, 64)
	assert.Len(t, report.Days, 1)
	assert.Len(t, report.Users, 1)
	filtered, err := GetUsageAuditReport(context.Background(), model.UsageAuditFilter{UserID: 801, TokenID: 91, StartAt: at, EndAt: at + 2, Model: "kimi-k3", Protocol: "anthropic"})
	require.NoError(t, err)
	require.Len(t, filtered.Rows, 1)
	assert.Equal(t, "selected", filtered.Rows[0].RequestID)
	var original model.Log
	require.NoError(t, model.LOG_DB.First(&original, logs[0].Id).Error)
	assert.Equal(t, logs[0].Quota, original.Quota)
	assert.Equal(t, metadata, original.Other)
}

func TestUsageAuditSeparatesCacheAndUsesHistoricalPrices(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	for _, scenario := range []struct {
		name, metadata string
		prompt         int
	}{
		{"OpenAI total input", `"usage_semantic":"openai"`, 1000},
		{"Anthropic uncached input", `"usage_semantic":"anthropic"`, 100},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			log := &model.Log{Type: model.LogTypeConsume, Quota: 405, PromptTokens: scenario.prompt, CompletionTokens: 20,
				Other: `{` + scenario.metadata + `,"model_ratio":2,"completion_ratio":5,"group_ratio":0.5,"cache_ratio":0.1,"cache_creation_ratio":1.25,"cache_tokens":800,"cache_creation_tokens":100}`}
			row, err := buildUsageAuditRow(log, currency)
			require.NoError(t, err)
			require.NotNil(t, row.Tokens)
			assert.Equal(t, int64(100), row.Tokens.Input)
			assert.Equal(t, int64(1000), row.Tokens.TotalInput)
			assert.Equal(t, int64(800), row.Tokens.CacheRead)
			assert.Equal(t, int64(100), row.Tokens.CacheWrite)
			require.NotNil(t, row.ListCost)
			require.NotNil(t, row.DiscountedCost)
			assert.Equal(t, "0.001620000", row.ListCost.Total)
			assert.Equal(t, "0.000810000", row.DiscountedCost.Total)
			assert.Equal(t, "0.000160000", row.DiscountedCost.CacheRead)
			assert.Equal(t, "0.000250000", row.DiscountedCost.CacheWrite)
			assert.Equal(t, row.ActualAmount, row.DiscountedCost.Total)
			assert.Empty(t, row.Issue)
			var summary UsageAuditSummary
			addUsageAuditRow(&summary, row)
			require.NotNil(t, summary.Comparison)
			assert.Equal(t, "0.000810000", summary.Comparison.Platform.Total)
			assert.Equal(t, "0.001620000", summary.Comparison.List.Total)
			assert.Equal(t, "0.000810000", summary.Comparison.Difference.Total)
			assert.Equal(t, "0.000160000", summary.Comparison.Difference.CacheRead)
		})
	}
}

func TestUsageAuditPreservesActualChargesWhenEvidenceIsMissing(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	for _, metadata := range []string{
		`{}`, // No historical prices or usage semantics.
		`{"usage_semantic":"openai","cache_tokens":1100,"model_ratio":2,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1}`,
		`{"usage_semantic":"openai","cache_tokens":800,"model_ratio":2,"group_ratio":1,"completion_ratio":5}`, // Missing cache price.
		`{"usage_semantic":"openai","cache_tokens":800,"model_ratio":2,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"billing_mode":"tiered_expr"}`,
	} {
		row, err := buildUsageAuditRow(&model.Log{Type: model.LogTypeConsume, Quota: 405, PromptTokens: 1000, Other: metadata}, currency)
		require.NoError(t, err)
		assert.Equal(t, "0.000810000", row.ActualAmount)
		assert.NotEmpty(t, row.Issue)
		assert.Nil(t, row.ListCost)
		assert.Nil(t, row.DiscountedCost)
	}
}

func TestUsageAuditDoesNotDoubleCountCacheWriteBreakdownOrAssumeHistoricalWireUsage(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	row, err := buildUsageAuditRow(&model.Log{Type: model.LogTypeConsume, PromptTokens: 10, Other: `{"usage_semantic":"anthropic","cache_tokens":100,"cache_creation_tokens":30,"cache_write_tokens":30,"cache_creation_tokens_5m":20,"cache_creation_tokens_1h":10,"model_ratio":1,"completion_ratio":5,"group_ratio":0,"cache_ratio":0.1,"cache_creation_ratio_5m":1.25,"cache_creation_ratio_1h":2,"request_path":"/v1/messages","request_conversion":["Claude Messages","OpenAI Compatible"]}`}, currency)
	require.NoError(t, err)
	require.NotNil(t, row.Tokens)
	assert.Equal(t, int64(140), row.Tokens.TotalInput)
	assert.Equal(t, int64(30), row.Tokens.CacheWrite)
	require.NotNil(t, row.Evidence)
	assert.Equal(t, int64(20), row.Evidence.Write5m)
	assert.Equal(t, int64(10), row.Evidence.Write1h)
	assert.Equal(t, []string{"Claude Messages", "OpenAI Compatible"}, row.Evidence.Conversion)
	require.NotNil(t, row.Evidence.WriteRatio1h)
	assert.Equal(t, 2.0, *row.Evidence.WriteRatio1h)
	require.NotNil(t, row.DiscountedCost)
	assert.Equal(t, "0.000000000", row.DiscountedCost.Total)
	assert.Equal(t, "0.000130000", row.ListCost.Total)
	assert.Zero(t, row.PossibleDuplicateInput)
	assert.Equal(t, "not_identified", row.DuplicateStatus)
	row, err = buildUsageAuditRow(&model.Log{Type: model.LogTypeConsume, PromptTokens: 1000, Other: `{"usage_semantic":"openai","cache_tokens":800,"request_path":"/v1/messages","request_conversion":["Claude Messages","OpenAI Compatible"]}`}, currency)
	require.NoError(t, err)
	assert.Equal(t, int64(800), row.PossibleDuplicateInput)
	assert.Equal(t, "needs_response_evidence", row.DuplicateStatus)
}

func TestUsageAuditPreservesMinimumChargeAndDoesNotCountRefundUsageTwice(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	log := &model.Log{Type: model.LogTypeConsume, Quota: 1, PromptTokens: 1, Other: `{"usage_semantic":"openai","cache_tokens":1,"model_ratio":1,"group_ratio":1,"completion_ratio":0,"cache_ratio":0}`}
	row, err := buildUsageAuditRow(log, currency)
	require.NoError(t, err)
	require.NotNil(t, row.DiscountedCost)
	assert.Equal(t, "0.000002000", row.DiscountedCost.Total)
	var summary UsageAuditSummary
	addUsageAuditRow(&summary, row)
	log.Type = model.LogTypeRefund
	refund, err := buildUsageAuditRow(log, currency)
	require.NoError(t, err)
	addUsageAuditRow(&summary, refund)
	assert.Equal(t, "0.000000000", summary.ActualAmount)
	assert.Equal(t, int64(1), summary.Tokens.CacheRead)
	assert.Zero(t, summary.UnknownUsageCount)
}

func TestUsageAuditPrefersBillingSemanticEvidenceOverDisplayFlags(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	row, err := buildUsageAuditRow(&model.Log{Type: model.LogTypeConsume, PromptTokens: 1000, Other: `{"claude":true,"admin_info":{"usage_billing_path":"billing-usage-openai"},"cache_tokens":800}`}, currency)
	require.NoError(t, err)
	require.NotNil(t, row.Tokens)
	assert.Equal(t, int64(200), row.Tokens.Input)
	assert.Equal(t, "openai", row.UsageSemantic)
}

func TestUsageAuditKeepsEstimatedCountsOutOfVerifiedTokenTotals(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	row, err := buildUsageAuditRow(&model.Log{Type: model.LogTypeConsume, Quota: 405, PromptTokens: 1000, Other: `{"request_path":"/v1/messages","admin_info":{"usage_billing_path":"billing-usage-openai-estimated"},"cache_tokens":800,"model_ratio":2,"group_ratio":1,"completion_ratio":5,"cache_ratio":0.1}`}, currency)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", row.Protocol)
	assert.Nil(t, row.Tokens)
	assert.Nil(t, row.ListCost)
	var summary UsageAuditSummary
	addUsageAuditRow(&summary, row)
	assert.Equal(t, 1, summary.UnknownUsageCount)
	assert.Zero(t, summary.Tokens.TotalInput)
	assert.Equal(t, "0.000810000", summary.ActualAmount)
}

func TestUsageAuditRepricesDuplicateCacheAtHistoricalInputRates(t *testing.T) {
	currency := BillingCurrency{Code: "USD", Symbol: "$", Rate: "1", QuotaPerUnit: "500000"}
	for _, tc := range []struct {
		name, path, extra, platform, list string
		tokens, unpriced                  int64
	}{
		{name: "read-only cache uses ordinary input price", path: "/v1/messages", extra: `,"model_ratio":2,"group_ratio":0.5`, tokens: 800, platform: "0.001600000", list: "0.003200000"},
		{name: "missing historical input price retains tokens", path: "/v1/messages", extra: `,"group_ratio":0.5`, tokens: 800, unpriced: 800},
		{name: "zero group rate remains free", path: "/v1/messages", extra: `,"model_ratio":2,"group_ratio":0`, tokens: 800, platform: "0.000000000", list: "0.003200000"},
		{name: "OpenAI cache total needs no correction", path: "/v1/chat/completions", extra: `,"model_ratio":2,"group_ratio":0.5`, tokens: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &model.Log{Type: model.LogTypeConsume, PromptTokens: 1000, Other: `{"usage_semantic":"openai","cache_tokens":800,"cache_creation_tokens":0,"request_path":"` + tc.path + `","request_conversion":["Claude Messages","OpenAI Compatible"]` + tc.extra + `}`}
			row, err := buildUsageAuditRow(log, currency)
			require.NoError(t, err)
			if tc.tokens == 0 {
				assert.Nil(t, row.InputRecalculation)
				return
			}
			require.NotNil(t, row.InputRecalculation)
			assert.Equal(t, tc.tokens, row.InputRecalculation.Tokens)
			assert.Equal(t, int64(800), row.InputRecalculation.CacheRead)
			assert.Equal(t, int64(0), row.InputRecalculation.CacheWrite)
			assert.Equal(t, tc.unpriced, row.InputRecalculation.UnpricedTokens)
			assert.Equal(t, tc.platform, row.InputRecalculation.PlatformAmount)
			assert.Equal(t, tc.list, row.InputRecalculation.ListAmount)
			var summary UsageAuditSummary
			addUsageAuditRow(&summary, row)
			addUsageAuditRow(&summary, row)
			assert.Equal(t, 2, summary.InputRecalculation.RequestCount)
			assert.Equal(t, 2*tc.tokens, summary.InputRecalculation.Tokens)
			assert.Equal(t, 2*tc.unpriced, summary.InputRecalculation.UnpricedTokens)
			if tc.name == "read-only cache uses ordinary input price" {
				assert.Equal(t, "0.003200000", summary.InputRecalculation.PlatformAmount)
				assert.Equal(t, "0.006400000", summary.InputRecalculation.ListAmount)
			}
			log.Type = model.LogTypeRefund
			refund, err := buildUsageAuditRow(log, currency)
			require.NoError(t, err)
			assert.Nil(t, refund.InputRecalculation)
		})
	}
}

func TestUsageAuditExcludesCacheReadsWhenLegacyWriteBranchAlreadySeparatedInput(t *testing.T) {
	currency := BillingCurrency{Code: "CNY", Symbol: "¥", Rate: "7", QuotaPerUnit: "500000"}
	log := model.Log{Type: model.LogTypeConsume, PromptTokens: 89713, Other: `{"usage_semantic":"openai","cache_tokens":85977,"cache_creation_tokens":3545,"model_ratio":1.4285715,"completion_ratio":5,"group_ratio":0.53,"cache_ratio":0.1,"cache_creation_ratio":1,"request_path":"/v1/messages","request_conversion":["Claude Messages","OpenAI Compatible"]}`}
	row, err := buildUsageAuditRow(&log, currency)
	require.NoError(t, err)
	require.NotNil(t, row.Tokens)
	assert.Equal(t, int64(191), row.Tokens.Input)
	assert.Equal(t, int64(85977), row.Tokens.CacheRead)
	assert.Equal(t, int64(3545), row.Tokens.CacheWrite)
	assert.Nil(t, row.InputRecalculation)
	assert.Zero(t, row.PossibleDuplicateInput)
	assert.Equal(t, "excluded_cache_write_present", row.DuplicateStatus)
	var summary UsageAuditSummary
	addUsageAuditRow(&summary, row)
	assert.Zero(t, summary.DuplicateReviewCount)
	assert.Equal(t, 1, summary.InputRecalculation.ExcludedWriteRequestCount)
	assert.Zero(t, summary.InputRecalculation.RequestCount)
	assert.Zero(t, summary.InputRecalculation.Tokens)
	assert.Equal(t, int64(85977), summary.Tokens.CacheRead)
}
