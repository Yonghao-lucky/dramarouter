package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicResponseCacheSeparationPreservesBillingAndLogUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatClaude,
		FinalRequestRelayFormat: types.RelayFormatOpenAI,
		OriginModelName:         "kimi-k3",
		StartTime:               time.Now(),
		PriceData: hosttypes.PriceData{
			ModelRatio:      1.4285715,
			CompletionRatio: 4.99999965,
			CacheRatio:      0.1,
			GroupRatioInfo:  hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	upstream := dto.Usage{
		PromptTokens:        4921,
		CompletionTokens:    86,
		TotalTokens:         5007,
		PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 4825},
	}
	response := relayconvert.ResponseOpenAI2Claude(&dto.OpenAITextResponse{Usage: upstream}, info)
	require.NotNil(t, response.Usage)
	require.Equal(t, 96, response.Usage.InputTokens)
	convertedUsage := relayconvert.UsageFromClaudeAPIUsage(response.Usage)
	effectiveUsage := effectiveBillingUsage(convertedUsage)
	before := calculateTextQuotaSummary(ctx, info, &upstream)
	after := calculateTextQuotaSummary(ctx, info, effectiveUsage)

	assert.Equal(t, 4921, effectiveUsage.PromptTokens)
	assert.Equal(t, 4825, effectiveUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, dto.BillingUsageSemanticOpenAI, after.UsageSemantic)
	assert.Equal(t, before.PromptTokens, after.PromptTokens)
	assert.Equal(t, before.CompletionTokens, after.CompletionTokens)
	assert.Equal(t, before.CacheTokens, after.CacheTokens)
	assert.Equal(t, before.CacheCreationTokens, after.CacheCreationTokens)
	assert.Equal(t, before.Quota, after.Quota)
	assert.Equal(t, 1441, after.Quota)

	// A consumer can ignore the OpenAI billing snapshot and use only the
	// Anthropic fields, or normalize those fields to OpenAI totals first.
	clientWireUsage := *response.Usage
	clientWireUsage.BillingUsage = nil
	clientNormalizedUsage := relayconvert.UsageFromClaudeAPIUsage(&clientWireUsage)
	clientAnthropicUsage := effectiveBillingUsage(clientNormalizedUsage)
	assert.Equal(t, 96, clientAnthropicUsage.PromptTokens)
	assert.Equal(t, 4825, clientAnthropicUsage.PromptTokensDetails.CachedTokens)
	anthropicSummary := calculateTextQuotaSummary(ctx, info, clientAnthropicUsage)
	assert.Equal(t, 1441, anthropicSummary.Quota)

	clientNormalizedUsage.BillingUsage = nil
	assert.Equal(t, 4921, clientNormalizedUsage.PromptTokens)
	assert.Equal(t, 4825, clientNormalizedUsage.PromptTokensDetails.CachedTokens)
	openAISummary := calculateTextQuotaSummary(ctx, info, clientNormalizedUsage)
	assert.Equal(t, 1441, openAISummary.Quota)
	assert.Equal(t, anthropicSummary.Quota, openAISummary.Quota)
}
