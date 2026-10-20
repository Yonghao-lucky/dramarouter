package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	moonshotfacade "github.com/QuantumNous/new-api/relay/facade/moonshot"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const streamUsageTextChunk = `{"id":"stream-usage","created":1,"model":"kimi-k3","choices":[{"index":0,"delta":{"role":"assistant","content":"hello world"}}]}`
const streamUsageEmptyChunk = `{"id":"stream-usage","created":1,"model":"kimi-k3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
const streamUsageZeroChunk = `{"id":"stream-usage","created":1,"model":"kimi-k3","choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
const streamUsageFinalChunk = `{"id":"stream-usage","created":1,"model":"kimi-k3","choices":[],"usage":{"prompt_tokens":5000,"completion_tokens":60,"total_tokens":5060,"prompt_tokens_details":{"cached_tokens":4800}}}`
const streamUsageProgressChunk = `{"id":"stream-usage","created":1,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"progress"}}],"usage":{"prompt_tokens":4700,"completion_tokens":10,"total_tokens":4710,"prompt_tokens_details":{"cached_tokens":4500}}}`

type streamUsageFixture struct {
	format      types.RelayFormat
	channelType int
	model       string
	chunks      []string
	passthrough bool
}

func runStreamUsageFixture(t *testing.T, fixture streamUsageFixture) (*gin.Context, *httptest.ResponseRecorder, *dto.Usage) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	path := "/v1/chat/completions"
	if fixture.format == types.RelayFormatClaude {
		path = "/v1/messages"
	}
	if fixture.passthrough {
		path = "/moonshot/v1/chat/completions"
	}
	ctx.Request = httptest.NewRequest(http.MethodPost, path, nil)
	setting := dto.ChannelSettings{}
	if fixture.passthrough {
		setting.MoonshotFacadeMode = dto.MoonshotFacadeModeKimiPassthrough
		setting.ForceFormat = true
		common.SetContextKey(ctx, constant.ContextKeyChannelSetting, setting)
		moonshotfacade.Middleware()(ctx)
		require.True(t, moonshotfacade.KimiPassthroughEnabled(ctx))
	}
	model := fixture.model
	if model == "" {
		model = "kimi-k3"
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: fixture.format,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       fixture.channelType,
			ChannelSetting:    setting,
			UpstreamModelName: model,
		},
		ClaudeConvertInfo:  &relaycommon.ClaudeConvertInfo{},
		StartTime:          time.Now(),
		ShouldIncludeUsage: true,
	}
	info.SetEstimatePromptTokens(12345)
	var body strings.Builder
	for _, chunk := range fixture.chunks {
		body.WriteString("data: " + chunk + "\n\n")
	}
	body.WriteString("data: [DONE]\n\n")
	usage, apiErr := OaiStreamHandler(ctx, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body.String())),
	})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	return ctx, recorder, usage
}

func streamUsageData(t *testing.T, body string) []string {
	t.Helper()
	var payloads []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload != "[DONE]" {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

func claudeStreamFinalUsage(t *testing.T, body string) *dto.ClaudeUsage {
	t.Helper()
	var usage *dto.ClaudeUsage
	var deltaCount, stopCount int
	for _, payload := range streamUsageData(t, body) {
		var event dto.ClaudeResponse
		require.NoError(t, common.UnmarshalJsonStr(payload, &event))
		if event.Type == "message_delta" {
			deltaCount++
			usage = event.Usage
		}
		if event.Type == "message_stop" {
			stopCount++
		}
	}
	require.Equal(t, 1, deltaCount)
	require.Equal(t, 1, stopCount)
	require.NotNil(t, usage)
	return usage
}

func TestOaiStreamHandlerRetainsLatestAuthoritativeUsage(t *testing.T) {
	usageWithContent := strings.Replace(streamUsageFinalChunk, `"choices":[]`, `"choices":[{"index":0,"delta":{"content":"final content"}}]`, 1)
	tests := []struct {
		name   string
		model  string
		chunks []string
	}{
		{name: "usage is the last JSON frame", chunks: []string{streamUsageTextChunk, streamUsageEmptyChunk, streamUsageFinalChunk}},
		{name: "usage followed by multiple absent and zero usages", chunks: []string{streamUsageTextChunk, usageWithContent, streamUsageEmptyChunk, streamUsageZeroChunk, streamUsageEmptyChunk}},
		{name: "final zero usage cannot override retained counts", chunks: []string{streamUsageTextChunk, usageWithContent, streamUsageEmptyChunk, streamUsageZeroChunk}},
		{name: "later authoritative usage replaces progress snapshot", chunks: []string{streamUsageTextChunk, streamUsageProgressChunk, streamUsageFinalChunk, streamUsageEmptyChunk}},
		{name: "audio usage survives more than one trailing frame", model: "gpt-audio", chunks: []string{streamUsageTextChunk, streamUsageFinalChunk, streamUsageEmptyChunk, streamUsageZeroChunk, streamUsageEmptyChunk}},
	}
	for _, test := range tests {
		for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
			t.Run(test.name+"/"+string(format), func(t *testing.T) {
				ctx, recorder, usage := runStreamUsageFixture(t, streamUsageFixture{format: format, channelType: constant.ChannelTypeOpenAI, model: test.model, chunks: test.chunks})
				assert.Equal(t, 5000, usage.PromptTokens)
				assert.Equal(t, 60, usage.CompletionTokens)
				assert.Equal(t, 5060, usage.TotalTokens)
				assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
				assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
				if format == types.RelayFormatClaude {
					final := claudeStreamFinalUsage(t, recorder.Body.String())
					assert.Equal(t, 200, final.InputTokens)
					assert.Equal(t, 4800, final.CacheReadInputTokens)
					assert.Zero(t, final.CacheCreationInputTokens)
					assert.Equal(t, 60, final.OutputTokens)
					require.NotNil(t, final.BillingUsage)
					assert.Equal(t, usage, final.BillingUsage.OpenAIUsage)
					return
				}
				payloads := streamUsageData(t, recorder.Body.String())
				assert.Equal(t, test.chunks, payloads, "upstream frames must be preserved without invented usage")
				assert.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
			})
		}
	}
}

func TestOaiStreamHandlerPreservesProviderCacheAndBillingSnapshot(t *testing.T) {
	for _, writeField := range []string{"cache_write_tokens", "cached_creation_tokens"} {
		t.Run(writeField, func(t *testing.T) {
			snapshotUsage := &dto.Usage{PromptTokens: 5000, CompletionTokens: 60, TotalTokens: 5060, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 4800, CacheWriteTokens: 50}}
			snapshot := dto.NewOpenAIChatBillingUsage(snapshotUsage)
			frame := map[string]any{
				"id": "stream-usage", "created": 1, "model": "kimi-k3",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop", "usage": map[string]any{"cached_tokens": 4800}}},
				"usage":   map[string]any{"prompt_tokens": 5000, "completion_tokens": 60, "total_tokens": 5060, "prompt_tokens_details": map[string]any{writeField: 50}, "billing_usage": snapshot},
			}
			encoded, err := common.Marshal(frame)
			require.NoError(t, err)
			ctx, recorder, usage := runStreamUsageFixture(t, streamUsageFixture{
				format: types.RelayFormatClaude, channelType: constant.ChannelTypeMoonshot,
				chunks: []string{streamUsageTextChunk, string(encoded), streamUsageEmptyChunk, streamUsageZeroChunk, streamUsageEmptyChunk},
			})
			assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
			assert.Equal(t, 5000, usage.PromptTokens)
			assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, 50, usage.PromptTokensDetails.CacheCreationTokensTotal())
			assert.Equal(t, snapshot, usage.BillingUsage)
			final := claudeStreamFinalUsage(t, recorder.Body.String())
			assert.Equal(t, 150, final.InputTokens)
			assert.Equal(t, 4800, final.CacheReadInputTokens)
			assert.Equal(t, 50, final.CacheCreationInputTokens)
			assert.Equal(t, 60, final.OutputTokens)
			assert.Equal(t, snapshot, final.BillingUsage)
		})
	}
}

func TestOaiStreamHandlerStillEstimatesWhenUsageIsAbsent(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		t.Run(string(format), func(t *testing.T) {
			ctx, recorder, usage := runStreamUsageFixture(t, streamUsageFixture{format: format, channelType: constant.ChannelTypeOpenAI, chunks: []string{streamUsageTextChunk, streamUsageEmptyChunk}})
			assert.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
			assert.Equal(t, 12345, usage.PromptTokens)
			assert.Positive(t, usage.CompletionTokens)
			assert.Zero(t, usage.PromptTokensDetails.CachedTokens)
			if format == types.RelayFormatClaude {
				final := claudeStreamFinalUsage(t, recorder.Body.String())
				assert.Equal(t, usage.PromptTokens, final.InputTokens)
				assert.Equal(t, usage.CompletionTokens, final.OutputTokens)
				return
			}
			payloads := streamUsageData(t, recorder.Body.String())
			require.Len(t, payloads, 3)
			var final dto.ChatCompletionsStreamResponse
			require.NoError(t, common.UnmarshalJsonStr(payloads[2], &final))
			assert.Equal(t, usage, final.Usage)
		})
	}
}

func TestOaiStreamHandlerKimiPassthroughPreservesOriginalSSE(t *testing.T) {
	chunks := []string{streamUsageTextChunk, streamUsageFinalChunk, streamUsageEmptyChunk, streamUsageZeroChunk}
	ctx, recorder, usage := runStreamUsageFixture(t, streamUsageFixture{format: types.RelayFormatOpenAI, channelType: constant.ChannelTypeOpenAI, chunks: chunks, passthrough: true})
	assert.Equal(t, 5000, usage.PromptTokens)
	assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 60, usage.CompletionTokens)
	assert.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens))
	assert.Equal(t, chunks, streamUsageData(t, recorder.Body.String()))
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
}
