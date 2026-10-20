package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatStreamPartialFailureDoesNotSendSuccessfulTerminal(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini} {
		for _, readError := range []bool{false, true} {
			name := string(format) + "/EOF"
			if readError {
				name = string(format) + "/broken HTTP body"
			}
			t.Run(name, func(t *testing.T) {
				tail := `{"choices":[{"index":0,"delta":{"content":"partial tail"}}]}`
				if readError {
					tail = `{"choices":[{"index":0,"delta":{"content":"partial tail"},"finish_reason":"stop"}]}`
				}
				payload := "data: " + streamUsageTextChunk + "\n\ndata: " + streamUsageFinalChunk + "\n\ndata: " + streamUsageZeroChunk + "\n\n" +
					"data: " + tail + "\n\n"
				c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(payload)))
				info.RelayMode = relayconstant.RelayModeChatCompletions
				info.RelayFormat = format
				info.ShouldIncludeUsage = true
				info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{}
				if readError {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							return
						}
						defer conn.Close()
						// Send complete SSE data inside HTTP chunks, then disconnect
						// without the zero chunk that completes the HTTP message.
						_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(payload), payload)
					}))
					t.Cleanup(upstream.Close)
					var err error
					resp, err = upstream.Client().Get(upstream.URL)
					require.NoError(t, err)
				}

				usage, apiErr := OaiStreamHandler(c, info, resp)
				require.Nil(t, apiErr, "partial output must not enter retry/refund handling")
				require.NotNil(t, usage)
				assert.Equal(t, 5000, usage.PromptTokens)
				assert.Equal(t, 60, usage.CompletionTokens)
				assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
				got := w.Body.String()
				assert.Contains(t, got, "hello world")
				assert.Equal(t, 1, strings.Count(got, "partial tail"), "the final buffered content must be delivered exactly once")
				assert.Contains(t, got, "The upstream stream ended before the response was complete.")
				assert.Less(t, strings.Index(got, "partial tail"), strings.Index(got, "The upstream stream ended before the response was complete."))
				assert.NotContains(t, got, "data: [DONE]")
				assert.NotContains(t, got, "event: message_stop\n")
				assert.NotContains(t, got, `"finishReason":"STOP"`)
				assert.True(t, info.StreamStatus.HasErrors())
				if format == types.RelayFormatOpenAI {
					assert.Contains(t, got, "data: "+tail+"\n\n", "native Chat retains the original partial payload")
				}
				if readError {
					assert.Equal(t, relaycommon.StreamEndReasonScannerErr, info.StreamStatus.EndReason)
					assert.ErrorIs(t, info.StreamStatus.EndError, io.ErrUnexpectedEOF)
				}
			})
		}
	}
}

func TestChatStreamFailureBeforeOutputRemainsRetryable(t *testing.T) {
	for _, body := range []string{"", "data: " + streamUsageTextChunk + "\n\n"} {
		c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(body)))
		info.RelayMode = relayconstant.RelayModeChatCompletions
		info.RelayFormat = types.RelayFormatOpenAI
		usage, apiErr := OaiStreamHandler(c, info, resp)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		assert.Nil(t, usage)
		assert.Empty(t, w.Body.String())
		assert.Empty(t, w.Header().Get("Content-Type"))
	}
}

func TestChatStreamCompletionBoundary(t *testing.T) {
	second := `{"choices":[{"index":1,"delta":{"content":"partial"}}]}`
	bothFinished := `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{},"finish_reason":"length"}]}`
	for _, tc := range []struct {
		name     string
		events   []string
		n        int
		complete bool
		partial  bool
	}{
		{"DONE with usage", []string{streamUsageFinalChunk, `[DONE]`}, 1, true, false},
		{"finish and usage before EOF", []string{streamUsageEmptyChunk, streamUsageFinalChunk}, 1, true, false},
		{"missing second choice", []string{streamUsageEmptyChunk}, 2, false, false},
		{"unfinished second choice", []string{streamUsageEmptyChunk, second}, 2, false, true},
		{"both choices finished", []string{second, bothFinished, streamUsageFinalChunk}, 2, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := streamTestBody(append([]string{streamUsageTextChunk}, tc.events...)...)
			c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(body)))
			info.RelayMode, info.RelayFormat, info.ShouldIncludeUsage = relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI, true
			info.Request = &dto.GeneralOpenAIRequest{N: &tc.n}
			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, !tc.complete, info.StreamStatus.HasErrors())
			assert.Contains(t, w.Body.String(), "hello world")
			if tc.complete {
				assert.Equal(t, 5060, usage.TotalTokens)
				assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
				assert.Equal(t, 1, strings.Count(w.Body.String(), "data: [DONE]"))
			} else {
				assert.Contains(t, w.Body.String(), "upstream_stream_incomplete")
				assert.NotContains(t, w.Body.String(), "data: [DONE]")
			}
			if tc.partial {
				assert.Contains(t, w.Body.String(), "partial", "the buffered candidate must survive EOF")
			}
		})
	}
}

func TestLegacyCompletionStreamPreservesExplicitFinish(t *testing.T) {
	body := "data: {\"id\":\"cmpl-test\",\"choices\":[{\"index\":0,\"text\":\"hello\",\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"cmpl-test\",\"choices\":[{\"index\":0,\"text\":\"\",\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"
	c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(body)))
	c.Request.URL.Path = "/v1/completions"
	info.RelayMode = relayconstant.RelayModeCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 18, usage.TotalTokens)
	assert.Contains(t, w.Body.String(), `"text":"hello"`)
	assert.Contains(t, w.Body.String(), "data: [DONE]")
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamTimeoutReportsProtocolError(t *testing.T) {
	tool := `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"{"}}]}}]}`
	for _, tc := range []struct {
		name, body, tail string
		handler          streamTestHandler
	}{
		{"Chat partial tool", streamTestBody(streamUsageTextChunk, tool), "lookup", OaiStreamHandler},
		{"Responses partial text", streamTestBody(`{"type":"response.output_text.delta","delta":"partial output"}`), "partial output", OaiResponsesStreamHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
			c, w, info, resp := setupResponsesStreamReliabilityTest(t, reader)
			constant.StreamingTimeout = 1
			info.RelayMode, info.RelayFormat, info.ShouldIncludeUsage = relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI, true
			done := make(chan struct{})
			go func() {
				_, apiErr := tc.handler(c, info, resp)
				assert.Nil(t, apiErr)
				close(done)
			}()
			_, err := io.WriteString(writer, tc.body)
			require.NoError(t, err)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = writer.Close()
				<-done
				t.Fatal("stream ignored its idle timeout")
			}
			assert.Contains(t, w.Body.String(), tc.tail)
			assert.Contains(t, w.Body.String(), "upstream_stream_timeout")
			assert.NotContains(t, w.Body.String(), `"usage":`)
			assert.NotContains(t, w.Body.String(), "data: [DONE]")
			assert.NotContains(t, w.Body.String(), "response.completed")
			assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
		})
	}
}

func TestChatStreamRejectsErrorAndMalformedFrames(t *testing.T) {
	for _, tail := range []string{`{"error":{"type":"server_error","message":"private upstream details"}}`, `{not valid JSON}`} {
		c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader("data: "+streamUsageTextChunk+"\n\ndata: "+tail+"\n\ndata: [DONE]\n\n")))
		info.RelayMode = relayconstant.RelayModeChatCompletions
		info.RelayFormat = types.RelayFormatOpenAI
		_, apiErr := OaiStreamHandler(c, info, resp)
		require.Nil(t, apiErr)
		assert.Contains(t, w.Body.String(), "hello world")
		assert.Contains(t, w.Body.String(), "upstream_stream_incomplete")
		assert.NotContains(t, w.Body.String(), "private upstream details")
		assert.NotContains(t, w.Body.String(), "data: [DONE]")
	}
}

type streamTerminalFailingWriter struct {
	gin.ResponseWriter
	failOn             string
	err                error
	failed             bool
	writesAfterFailure int
}

func (w *streamTerminalFailingWriter) Write(data []byte) (int, error) {
	if w.failed {
		w.writesAfterFailure++
		return 0, w.err
	}
	if strings.Contains(string(data), w.failOn) {
		w.failed = true
		return 0, w.err
	}
	return w.ResponseWriter.Write(data)
}

func TestChatStreamFinalWriteFailureDoesNotSendLaterTerminal(t *testing.T) {
	for _, tc := range []struct {
		format   types.RelayFormat
		failOn   string
		terminal string
	}{
		{types.RelayFormatOpenAI, `"usage":`, "data: [DONE]"},
		{types.RelayFormatClaude, "event: message_delta", "event: message_stop"},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			body := "data: " + streamUsageTextChunk + "\n\ndata: " + streamUsageEmptyChunk + "\n\ndata: [DONE]\n\n"
			c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(body)))
			info.RelayMode = relayconstant.RelayModeChatCompletions
			info.RelayFormat = tc.format
			info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{}
			info.ShouldIncludeUsage = true
			writer := &streamTerminalFailingWriter{ResponseWriter: c.Writer, failOn: tc.failOn, err: errors.New("client socket disconnected")}
			c.Writer = writer

			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.True(t, writer.failed, "the test must reach the terminal write")
			assert.Zero(t, writer.writesAfterFailure)
			assert.Contains(t, w.Body.String(), "hello world")
			assert.NotContains(t, w.Body.String(), tc.terminal)
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}

func TestChatStreamInterruptedTailWriteFailurePreservesUsage(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			body := "data: " + streamUsageTextChunk + "\n\ndata: " + streamUsageFinalChunk + "\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial tail\"}}]}\n\n"
			c, recorder, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(body)))
			info.RelayMode = relayconstant.RelayModeChatCompletions
			info.RelayFormat = format
			info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{}
			writer := &streamTerminalFailingWriter{ResponseWriter: c.Writer, failOn: "partial tail", err: errors.New("client socket disconnected")}
			c.Writer = writer

			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr, "the already-started attempt must settle instead of retrying or refunding all usage")
			require.NotNil(t, usage)
			assert.Equal(t, 5000, usage.PromptTokens)
			assert.Equal(t, 60, usage.CompletionTokens)
			assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
			assert.True(t, writer.failed, "the interrupted stream must attempt to flush its buffered content")
			assert.Zero(t, writer.writesAfterFailure)
			assert.Contains(t, recorder.Body.String(), "hello world")
			assert.NotContains(t, recorder.Body.String(), "The upstream stream ended before the response was complete.")
			assert.NotContains(t, recorder.Body.String(), "data: [DONE]")
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}

func TestChatStreamKeepsFinishAndToolArgumentsInUsageBearingTail(t *testing.T) {
	tail := `{"id":"stream-usage","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`
	c, w, info, resp := setupResponsesStreamReliabilityTest(t, io.NopCloser(strings.NewReader(
		"data: "+streamUsageTextChunk+"\n\ndata: "+tail+"\n\ndata: [DONE]\n\n")))
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.RelayFormat = types.RelayFormatOpenAI
	info.ShouldIncludeUsage = false
	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 18, usage.TotalTokens)
	assert.Contains(t, w.Body.String(), "data: "+tail+"\n\n", "disabling usage must not drop finish_reason or final tool arguments")
	assert.Equal(t, 1, strings.Count(w.Body.String(), "data: [DONE]"))
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamClientFailuresStopWritesAndRetainUsage(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		handler    streamTestHandler
		format     types.RelayFormat
		chat       bool
	}{
		{"Chat write", "write", OaiStreamHandler, types.RelayFormatOpenAI, true},
		{"Chat to Claude write", "write", OaiStreamHandler, types.RelayFormatClaude, true},
		{"Chat to Gemini write", "write", OaiStreamHandler, types.RelayFormatGemini, true},
		{"Responses write", "terminal write", OaiResponsesStreamHandler, types.RelayFormatOpenAIResponses, false},
		{"Chat to Responses write", "write", OaiChatToResponsesStreamHandler, types.RelayFormatOpenAIResponses, true},
		{"Responses to Chat write", "write", OaiResponsesToChatStreamHandler, types.RelayFormatOpenAI, false},
		{"Responses to Claude write", "write", OaiResponsesToChatStreamHandler, types.RelayFormatClaude, false},
		{"Responses to Gemini write", "write", OaiResponsesToChatStreamHandler, types.RelayFormatGemini, false},
		{"Chat cancel on write", "cancel write", OaiStreamHandler, types.RelayFormatOpenAI, true},
		{"Chat cancel on flush", "cancel flush", OaiStreamHandler, types.RelayFormatOpenAI, true},
		{"Responses cancel on write", "cancel write", OaiResponsesStreamHandler, types.RelayFormatOpenAIResponses, false},
		{"Responses cancel terminal flush", "terminal flush", OaiResponsesStreamHandler, types.RelayFormatOpenAIResponses, false},
		{"Chat to Responses already cancelled", "cancel before", OaiChatToResponsesStreamHandler, types.RelayFormatOpenAIResponses, true},
		{"Responses to Chat already cancelled", "cancel before", OaiResponsesToChatStreamHandler, types.RelayFormatOpenAI, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := streamTestBody(convertedResponseUsage, convertedResponseText, `{"type":"response.completed"}`)
			if tc.chat {
				withUsage := strings.Replace(streamUsageFinalChunk, `"choices":[]`, `"choices":[{"index":0,"delta":{"content":"answer"}}]`, 1)
				body = streamTestBody(withUsage, streamUsageFinalChunk, streamUsageEmptyChunk, `[DONE]`)
			} else if tc.mode == "terminal flush" || tc.mode == "terminal write" {
				body = streamTestBody(strings.Replace(convertedResponseUsage, "response.created", "response.completed", 1))
			}
			c, recorder, resp, info := setupConvertedStreamTest(t, body, true)
			info.RelayFormat, info.RelayMode = tc.format, relayconstant.RelayModeChatCompletions
			info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{}
			ctx, cancel := context.WithCancel(c.Request.Context())
			t.Cleanup(cancel)
			c.Request = c.Request.WithContext(ctx)
			failedWriter := &streamTerminalFailingWriter{ResponseWriter: c.Writer, err: errors.New("client disconnected")}
			switch tc.mode {
			case "write", "terminal write":
				c.Writer = failedWriter
			case "cancel write":
				c.Writer = &responsesCancelWriter{ResponseWriter: c.Writer, cancel: cancel}
			case "cancel flush", "terminal flush":
				c.Writer = &responsesCancelOnFlushWriter{ResponseWriter: c.Writer, cancel: cancel}
			case "cancel before":
				cancel()
			}
			usage, apiErr := tc.handler(c, info, resp)
			require.Nil(t, apiErr, "a disconnected client must not enter retry or whole-request refund")
			require.NotNil(t, usage)
			if tc.mode != "cancel before" {
				assert.Equal(t, 5000, usage.PromptTokens)
				assert.Equal(t, 60, usage.CompletionTokens)
				if tc.chat && tc.format == types.RelayFormatOpenAIResponses {
					require.NotNil(t, usage.BillingUsage)
					require.NotNil(t, usage.BillingUsage.OpenAIUsage)
					assert.Equal(t, 4800, usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens)
				} else {
					assert.Equal(t, 4800, usage.PromptTokensDetails.CachedTokens)
				}
			}
			if tc.mode == "write" || tc.mode == "terminal write" {
				assert.True(t, failedWriter.failed)
				assert.Zero(t, failedWriter.writesAfterFailure)
				assert.Empty(t, recorder.Body.String())
				assert.True(t, info.StreamStatus.HasErrors())
			} else {
				assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
				assert.ErrorIs(t, info.StreamStatus.EndError, context.Canceled)
			}
			assert.NotContains(t, recorder.Body.String(), "data: [DONE]")
			assert.NotContains(t, recorder.Body.String(), "event: error\n")
			assert.NotContains(t, recorder.Body.String(), "upstream_stream_incomplete")
		})
	}
}
