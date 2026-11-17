package relayconvert

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Anthropic search_result blocks put a bare URL string in `source`. Decoding
// used to fail with "cannot unmarshal string into ... ClaudeMessageSource",
// which surfaced to clients as a 500 on an otherwise valid request.
func TestClaudeSearchResultSourceStringIsAccepted(t *testing.T) {
	raw := mustRawMessage(t, []map[string]any{
		{
			"type":    "search_result",
			"title":   "Weather",
			"source":  "https://example.com/weather",
			"content": "Sunny.",
		},
	})

	var content []dto.ClaudeMediaMessage
	require.NoError(t, kitutil.Unmarshal(raw, &content))
	require.Len(t, content, 1)

	require.NotNil(t, content[0].Source)
	assert.Equal(t, "https://example.com/weather", content[0].Source.Url)
	assert.Equal(t, "Weather", content[0].Title)
}

// The chat-completions converter has no search_result block type, so the block
// must be folded into text with its title and source preserved. Dropping it
// silently would send the upstream a request missing the retrieved context.
func TestClaudeSearchResultFoldsIntoText(t *testing.T) {
	maxTokens := uint(64)
	req := dto.ClaudeRequest{
		Model:     "kimi-k3.1-pro",
		MaxTokens: &maxTokens,
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []map[string]any{
					{
						"type":    "search_result",
						"title":   "Weather",
						"source":  "https://example.com/weather",
						"content": "Sunny.",
					},
				},
			},
		},
	}

	result, err := ConvertRequest(nil, &convmeta.Values{}, types.RelayFormatOpenAI, &req)
	require.NoError(t, err)

	chatReq, ok := result.Value.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chatReq.Messages, 1)

	parts := chatReq.Messages[0].ParseContent()
	require.Len(t, parts, 1)
	require.Equal(t, "text", parts[0].Type)

	// Title and source become a header above the retrieved content.
	assert.Contains(t, parts[0].Text, "Title: Weather")
	assert.Contains(t, parts[0].Text, "Source: https://example.com/weather")
	assert.Contains(t, parts[0].Text, "Sunny.")
}
