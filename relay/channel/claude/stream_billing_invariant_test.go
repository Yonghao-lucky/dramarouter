package claude

import (
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 计费快照只允许包含上游真实上报的字段。message_delta 未到达时输出计数不可信，
// 必须置零；上游什么都没上报时必须保持全零，使结算判为无计费量。
func TestHandleStreamFinalResponseBillingSnapshotOnlyUsesUpstreamReport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name             string
		usage            *dto.Usage
		done             bool
		outputReported   bool
		responseText     string
		wantInput        int
		wantOutput       int
		wantCacheRead    int
		wantEstimated    bool
		wantDisplaysText bool
	}{
		{
			name: "upstream reported input and output",
			usage: &dto.Usage{
				PromptTokens:        1000,
				CompletionTokens:    50,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 800},
			},
			done:           true,
			outputReported: true,
			wantInput:      1000,
			wantOutput:     50,
			wantCacheRead:  800,
			wantEstimated:  false,
		},
		{
			name: "message_delta never arrived keeps input drops placeholder output",
			usage: &dto.Usage{
				PromptTokens:        7796,
				CompletionTokens:    1,
				PromptTokensDetails: dto.InputTokenDetails{},
			},
			done:           false,
			outputReported: false,
			responseText:   "短正文",
			wantInput:      7796,
			wantOutput:     0,
			wantEstimated:  true,
		},
		{
			name: "upstream reported nothing bills nothing",
			usage: &dto.Usage{
				PromptTokens:     0,
				CompletionTokens: 0,
			},
			done:           false,
			outputReported: false,
			wantInput:      0,
			wantOutput:     0,
			wantEstimated:  true,
		},
		{
			name: "fully cached prompt keeps cache read billing",
			usage: &dto.Usage{
				PromptTokens:        0,
				CompletionTokens:    0,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 90000},
			},
			done:           true,
			outputReported: false,
			responseText:   "正文",
			wantInput:      0,
			wantOutput:     0,
			wantCacheRead:  90000,
			wantEstimated:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			info := &relaycommon.RelayInfo{
				OriginModelName: "claude-opus-4-8",
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-4-8"},
				RelayFormat:     types.RelayFormatClaude,
			}
			claudeInfo := &ClaudeResponseInfo{
				Usage:                  tc.usage,
				Done:                   tc.done,
				UpstreamOutputReported: tc.outputReported,
			}
			if tc.responseText != "" {
				claudeInfo.ResponseText = strings.Builder{}
				claudeInfo.ResponseText.WriteString(tc.responseText)
			}

			HandleStreamFinalResponse(c, info, claudeInfo)

			require.NotNil(t, claudeInfo.Usage.BillingUsage, "计费快照不能为 nil，否则结算会回退到展示用估算值")
			billing := claudeInfo.Usage.BillingUsage
			require.NotNil(t, billing.ClaudeUsage)
			assert.Equal(t, dto.BillingUsageSemanticAnthropic, billing.Semantic)

			assert.Equal(t, tc.wantInput, billing.ClaudeUsage.InputTokens, "输入必须来自上游上报")
			assert.Equal(t, tc.wantOutput, billing.ClaudeUsage.OutputTokens, "输出未上报时必须为 0")
			assert.Equal(t, tc.wantCacheRead, billing.ClaudeUsage.CacheReadInputTokens)
			assert.Equal(t, tc.wantEstimated, billing.Estimated)
		})
	}
}

// 展示用的 usage 可以带估算值，但计费快照不能跟着变，否则又变成伪造计费。
func TestHandleStreamFinalResponseEstimateDoesNotLeakIntoBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-4-8"},
		RelayFormat: types.RelayFormatClaude,
	}
	claudeInfo := &ClaudeResponseInfo{
		Usage: &dto.Usage{PromptTokens: 500},
		Done:  false,
	}
	claudeInfo.ResponseText.WriteString(strings.Repeat("流式正文内容", 100))

	HandleStreamFinalResponse(c, info, claudeInfo)

	assert.Greater(t, claudeInfo.Usage.CompletionTokens, 0, "展示值应补齐，客户端需要看到 usage")
	require.NotNil(t, claudeInfo.Usage.BillingUsage)
	assert.Equal(t, 0, claudeInfo.Usage.BillingUsage.ClaudeUsage.OutputTokens, "计费快照不能包含估算输出")
	assert.Equal(t, 500, claudeInfo.Usage.BillingUsage.ClaudeUsage.InputTokens)
	assert.True(t, claudeInfo.Usage.BillingUsage.Estimated)
}
