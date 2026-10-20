package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenaiHandlerSeparatesClaudeCacheUsageWithoutChangingBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const responseBody = `{"id":"chatcmpl-cache-test","model":"kimi-k3","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4921,"completion_tokens":68,"total_tokens":4989,"prompt_tokens_details":{"cached_tokens":4825}}}`

	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			requestPath := "/v1/messages"
			if format == types.RelayFormatOpenAI {
				requestPath = "/v1/chat/completions"
			}
			ctx.Request = httptest.NewRequest(http.MethodPost, requestPath, nil)
			info := &relaycommon.RelayInfo{
				RelayFormat: format,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeMoonshot,
					UpstreamModelName: "kimi-k3",
				},
			}
			usage, apiErr := OpenaiHandler(ctx, info, &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 4921, usage.PromptTokens)
			assert.Equal(t, 4825, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 68, usage.CompletionTokens)
			assert.Equal(t, 4989, usage.TotalTokens)

			if format == types.RelayFormatOpenAI {
				assert.JSONEq(t, responseBody, recorder.Body.String())
				return
			}
			var response dto.ClaudeResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.NotNil(t, response.Usage)
			assert.Equal(t, 96, response.Usage.InputTokens)
			assert.Equal(t, 4825, response.Usage.CacheReadInputTokens)
			assert.Zero(t, response.Usage.CacheCreationInputTokens)
			require.NotNil(t, response.Usage.BillingUsage)
			require.NotNil(t, response.Usage.BillingUsage.OpenAIUsage)
			assert.Equal(t, *usage, *response.Usage.BillingUsage.OpenAIUsage)
		})
	}
}
