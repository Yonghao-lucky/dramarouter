package geminichat

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The conversion layer keeps its own copy of the completion-from-total
// derivation used by the billing path. An incomplete usageMetadata (total at or
// below prompt, which Vertex emits for truncated streams) must not surface
// negative completion tokens to callers.
func TestUsageFromGeminiMetadataNeverReturnsNegativeCompletion(t *testing.T) {
	tests := []struct {
		name              string
		metadata          *dto.GeminiUsageMetadata
		wantPromptTokens  int
		wantCompletionTok int
	}{
		{
			name: "total below prompt",
			metadata: &dto.GeminiUsageMetadata{
				PromptTokenCount: 23627,
				TotalTokenCount:  -254,
			},
			wantPromptTokens:  23627,
			wantCompletionTok: 0,
		},
		{
			name: "total equal to prompt",
			metadata: &dto.GeminiUsageMetadata{
				PromptTokenCount: 18313,
				TotalTokenCount:  18313,
			},
			wantPromptTokens:  18313,
			wantCompletionTok: 0,
		},
		{
			name: "total above prompt derives completion",
			metadata: &dto.GeminiUsageMetadata{
				PromptTokenCount: 100,
				TotalTokenCount:  150,
			},
			wantPromptTokens:  100,
			wantCompletionTok: 50,
		},
		{
			name: "explicit candidates are preserved",
			metadata: &dto.GeminiUsageMetadata{
				PromptTokenCount:     100,
				CandidatesTokenCount: 40,
				TotalTokenCount:      150,
			},
			wantPromptTokens:  100,
			wantCompletionTok: 40,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			usage := UsageFromGeminiMetadata(tc.metadata, 0)
			require.NotNil(t, usage)
			assert.Equal(t, tc.wantPromptTokens, usage.PromptTokens)
			assert.Equal(t, tc.wantCompletionTok, usage.CompletionTokens)
			assert.GreaterOrEqual(t, usage.CompletionTokens, 0)
		})
	}
}
