package openai

import (
	"io"
	"net/http"
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

type streamTestHandler func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)

func streamTestBody(events ...string) string {
	if len(events) == 0 {
		return ""
	}
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
}

func setupConvertedStreamTest(t *testing.T, body string, stream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	c, recorder, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(body)))
	info.IsStream, info.ShouldIncludeUsage, info.RelayFormat = stream, true, types.RelayFormatOpenAI
	return c, recorder, resp, info
}

const convertedResponseText = `{"type":"response.output_text.delta","delta":"partial answer"}`
const convertedResponseUsage = `{"type":"response.created","response":{"usage":{"input_tokens":5000,"output_tokens":60,"total_tokens":5060,"input_tokens_details":{"cached_tokens":4800}}}}`

func TestConvertedStreamFailures(t *testing.T) {
	failedUsage := `"status":"failed","error":{"type":"server_error","message":"private upstream failure"},"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18,"input_tokens_details":{"cached_tokens":3}}`
	chatError := strings.Replace(streamUsageFinalChunk, `"choices":[]`, `"error":{"type":"server_error","message":"private upstream failure"},"choices":[]`, 1)
	for _, tc := range []struct {
		name                       string
		handler                    streamTestHandler
		body                       string
		format                     types.RelayFormat
		retry                      bool
		prompt, completion, cache  int
		buffered, cachedInSnapshot bool
	}{
		{name: "Chat EOF", handler: OaiChatToResponsesStreamHandler, body: streamTestBody(streamUsageTextChunk)},
		{name: "Chat malformed after finish", handler: OaiChatToResponsesStreamHandler, body: streamTestBody(streamUsageEmptyChunk, `{invalid JSON`)},
		{name: "Chat error carries usage", handler: OaiChatToResponsesStreamHandler, body: streamTestBody(streamUsageTextChunk, chatError), prompt: 5000, completion: 60, cache: 4800, cachedInSnapshot: true},
		{name: "Responses EOF", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText)},
		{name: "Responses EOF to Claude", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText), format: types.RelayFormatClaude},
		{name: "Responses EOF to Gemini", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText), format: types.RelayFormatGemini},
		{name: "Responses error", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText, `{"type":"error","message":"private upstream failure"}`)},
		{name: "Responses failed usage", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText, `{"type":"response.failed","response":{`+failedUsage+`}}`), prompt: 11, completion: 7, cache: 3},
		{name: "nested completed failure", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText, `{"type":"response.completed","response":{`+failedUsage+`}}`), prompt: 11, completion: 7, cache: 3},
		{name: "nested done failure", handler: OaiResponsesToChatStreamHandler, body: streamTestBody(convertedResponseText, `{"type":"response.done","response":{`+failedUsage+`}}`), prompt: 11, completion: 7, cache: 3},
		{name: "Chat before output", handler: OaiChatToResponsesStreamHandler, retry: true},
		{name: "Responses before output", handler: OaiResponsesToChatStreamHandler, retry: true},
		{name: "buffered EOF", handler: OaiResponsesToChatBufferedStreamHandler, buffered: true, body: streamTestBody(convertedResponseText), retry: true},
		{name: "buffered DONE without terminal", handler: OaiResponsesToChatBufferedStreamHandler, buffered: true, body: streamTestBody(convertedResponseText, `[DONE]`), retry: true},
		{name: "buffered nested failure", handler: OaiResponsesToChatBufferedStreamHandler, buffered: true, body: streamTestBody(convertedResponseText, `{"type":"response.completed","response":{`+failedUsage+`}}`), retry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w, resp, info := setupConvertedStreamTest(t, tc.body, !tc.buffered)
			info.SetEstimatePromptTokens(11)
			if tc.format != "" {
				info.RelayFormat = tc.format
			}
			usage, apiErr := tc.handler(c, info, resp)
			if tc.retry {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				assert.Nil(t, usage)
				assert.Empty(t, w.Body.String())
				assert.Empty(t, w.Header().Get("Content-Type"))
				return
			}
			require.Nil(t, apiErr, "partial streams settle without controller retry or whole-request refund")
			require.NotNil(t, usage)
			if tc.prompt > 0 {
				assert.Equal(t, tc.prompt, usage.PromptTokens)
				assert.Equal(t, tc.completion, usage.CompletionTokens)
				require.NotNil(t, usage.BillingUsage)
				cached := usage.PromptTokensDetails.CachedTokens
				if tc.cachedInSnapshot {
					require.NotNil(t, usage.BillingUsage.OpenAIUsage)
					cached = usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens
				}
				assert.Equal(t, tc.cache, cached)
			}
			assert.True(t, info.StreamStatus.HasErrors())
			assert.Contains(t, w.Body.String(), "The upstream stream ended before the response was complete.")
			for _, terminal := range []string{"data: [DONE]", "event: message_stop", "event: response.completed", `"finishReason":"STOP"`, "private upstream failure"} {
				assert.NotContains(t, w.Body.String(), terminal)
			}
		})
	}
}

func TestConvertedStreamTerminalsRetainBillingSnapshot(t *testing.T) {
	for _, chatUpstream := range []bool{true, false} {
		terminals := []string{"stop", "length"}
		if !chatUpstream {
			terminals = []string{"response.completed", "response.done", "response.incomplete"}
		}
		for _, terminal := range terminals {
			t.Run(terminal, func(t *testing.T) {
				body := streamTestBody(streamUsageTextChunk, strings.Replace(streamUsageEmptyChunk, `"stop"`, `"`+terminal+`"`, 1), streamUsageFinalChunk, streamUsageZeroChunk)
				handler := streamTestHandler(OaiChatToResponsesStreamHandler)
				if !chatUpstream {
					body = streamTestBody(convertedResponseUsage, convertedResponseText, `{"type":"`+terminal+`","response":{"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)
					handler = OaiResponsesToChatStreamHandler
				}
				c, w, resp, info := setupConvertedStreamTest(t, body, true)
				resp.Body = io.NopCloser(io.MultiReader(resp.Body, convertedStreamErrorReader{}))
				usage, apiErr := handler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, 5000, usage.PromptTokens)
				assert.Equal(t, 60, usage.CompletionTokens)
				require.NotNil(t, usage.BillingUsage)
				require.NotNil(t, usage.BillingUsage.OpenAIUsage)
				if chatUpstream {
					assert.Equal(t, 4800, usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens)
					expected := "response.completed"
					if terminal == "length" {
						expected = "response.incomplete"
					}
					assert.Contains(t, w.Body.String(), "event: "+expected+"\n")
				} else {
					require.NotNil(t, usage.BillingUsage.OpenAIUsage.InputTokensDetails)
					assert.Equal(t, 4800, usage.BillingUsage.OpenAIUsage.InputTokensDetails.CachedTokens)
					assert.Equal(t, 1, strings.Count(w.Body.String(), "data: [DONE]"))
				}
				assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
				assert.False(t, info.StreamStatus.HasErrors())
				assert.NotContains(t, w.Body.String(), "event: error\n")
			})
		}
	}
}

type convertedStreamErrorReader struct{}

func (convertedStreamErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
