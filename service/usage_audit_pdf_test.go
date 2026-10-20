package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAuditPDFFocusesOnTokenOverlapAndPreservesLetterhead(t *testing.T) {
	statement, branding := frozenBillingExportFixture(t)
	branding.Issuer = "new-api 聚合平台"
	branding.CompanyTitle = "示例客户有限公司"
	report := &UsageAuditReport{RecalculationBasis: "legacy_openai_cache_write_tokens", Filter: model.UsageAuditFilter{UserID: 42, TokenID: 7, StartAt: statement.StartAt, EndAt: statement.EndAt}, GeneratedAt: statement.CreatedAt, Currency: branding.Currency, Timezone: "Asia/Shanghai"}
	for _, source := range []model.Log{
		{ModelName: "kimi-k3", Type: model.LogTypeConsume, PromptTokens: 4921, CompletionTokens: 86, Quota: 951, Group: "kimi", Other: `{"request_path":"/v1/messages","request_conversion":["Claude Messages","OpenAI Compatible"],"usage_semantic":"openai","cache_tokens":4825,"model_ratio":1.4285715,"group_ratio":0.66,"completion_ratio":4.99999965,"cache_ratio":0.1}`},
		{ModelName: "claude-test", Type: model.LogTypeConsume, PromptTokens: 100, CompletionTokens: 20, Quota: 405, Group: "standard", Other: `{"usage_semantic":"anthropic","cache_tokens":800,"cache_creation_tokens":100,"model_ratio":2,"group_ratio":0.5,"completion_ratio":5,"cache_ratio":0.1,"cache_creation_ratio":1.25}`},
	} {
		row, err := buildUsageAuditRow(&source, report.Currency)
		require.NoError(t, err)
		report.Rows = append(report.Rows, row)
		addUsageAuditRow(&report.Summary, row)
		group := UsageAuditGroup{Label: source.ModelName}
		addUsageAuditRow(&group.UsageAuditSummary, row)
		report.Models = append(report.Models, group)
	}
	report.Rates = []UsageAuditRate{{Model: "kimi-k3", Group: "kimi", Ratio: "0.66"}, {Model: "claude-test", Group: "standard", Ratio: "0.5"}}
	var err error
	report.SHA256, err = usageAuditReportDigest(report)
	require.NoError(t, err)
	body, err := RenderUsageAuditPDF(report, branding)
	require.NoError(t, err)
	cover := billingPDFPageText(t, body, 0)

	for _, text := range []string{"new-api 聚合平台", "Token 用量与重复输入核算单", "示例客户有限公司", "需重算的缓存读取 Token", "额外费用参考 / 平台输入价", "额外费用参考 / 历史基准价", "4825", "cache_write_tokens", "有缓存写入的请求排除", "旧 input_tokens - 正确普通输入", "正常缓存读取费用保留"} {
		assert.Contains(t, cover, text)
	}
	assert.NotContains(t, cover, "费用差额 / DIFFERENCE")
	assert.NotContains(t, cover, "0.018984")
	assert.Less(t, strings.Index(cover, "需重算的缓存读取 Token"), strings.Index(cover, "额外费用参考"))
	models := billingPDFPageText(t, body, 1)
	for _, text := range []string{"kimi-k3", "claude-test", "4825", "800", "100", "输入（不含缓存）", "缓存", "输出", "正常用量 Token", "额外计入输入 Token"} {
		assert.Contains(t, models, text)
	}
	assert.NotContains(t, models, "平台费用")
	assert.NotContains(t, models, "官价费用")
	if output := os.Getenv("USAGE_AUDIT_PREVIEW_DIR"); output != "" {
		require.NoError(t, os.MkdirAll(output, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(output, "usage-audit-preview.pdf"), body, 0600))
	}
}

func TestUsageAuditPDFPaginatesModelDetailsAndDoesNotInventUnknownPrices(t *testing.T) {
	statement, branding := frozenBillingExportFixture(t)
	report := &UsageAuditReport{Filter: model.UsageAuditFilter{StartAt: statement.StartAt, EndAt: statement.EndAt, Group: "kimi", ChannelID: 40, Protocol: "anthropic"}, GeneratedAt: statement.CreatedAt, Currency: branding.Currency, SHA256: strings.Repeat("a", 64), Summary: UsageAuditSummary{Count: 3, UnknownUsageCount: 3}}
	labels := []string{"model-first", "model-second", strings.Repeat("long-model-name-", 12)}
	for _, label := range labels {
		report.Models = append(report.Models, UsageAuditGroup{Label: label, UsageAuditSummary: UsageAuditSummary{Count: 1, UnknownUsageCount: 1}})
	}
	body, err := RenderUsageAuditPDF(report, branding)
	require.NoError(t, err)
	cover := billingPDFPageText(t, body, 0)
	assert.Contains(t, cover, "未核价")
	assert.Contains(t, cover, "分组 kimi / 渠道 #40 / 协议 anthropic")
	assert.Contains(t, cover, "01 / 03")
	first := billingPDFPageText(t, body, 1)
	assert.Contains(t, first, labels[0])
	assert.Contains(t, first, labels[1])
	last := billingPDFPageText(t, body, 2)
	assert.Contains(t, strings.ReplaceAll(last, "\n", ""), labels[len(labels)-1])
	assert.Contains(t, last, "03 / 03")
	assert.Contains(t, last, "缓存读取")
	assert.Contains(t, last, "缺少可信用量 1")
	assert.NotContains(t, last, "平台费用")
}

func TestUsageAuditExportRejectsChangedEvidenceInsteadOfExportingDifferentAmounts(t *testing.T) {
	truncate(t)
	const at = int64(1700000000)
	log := model.Log{UserId: 812, TokenId: 29, Type: model.LogTypeConsume, CreatedAt: at, Quota: 10, PromptTokens: 10, Other: `{"usage_semantic":"openai","cache_tokens":0,"model_ratio":1,"group_ratio":1,"completion_ratio":5}`}
	require.NoError(t, model.LOG_DB.Create(&log).Error)
	report, err := GetUsageAuditReport(context.Background(), model.UsageAuditFilter{UserID: 812, StartAt: at, EndAt: at + 1})
	require.NoError(t, err)
	input := UsageAuditExportInput{Filter: report.Filter, GeneratedAt: report.GeneratedAt, SHA256: report.SHA256}
	export, err := GetUsageAuditExportReport(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, report.SHA256, export.SHA256)
	require.NoError(t, model.LOG_DB.Model(&log).Update("quota", 11).Error)
	_, err = GetUsageAuditExportReport(context.Background(), input)
	assert.ErrorIs(t, err, ErrUsageAuditChanged)
	input.SHA256 = strings.Repeat("a", 64)
	_, err = GetUsageAuditExportReport(context.Background(), input)
	assert.ErrorIs(t, err, ErrUsageAuditChanged)
}
