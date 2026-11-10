package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Vertex Gemini can return a totalTokenCount smaller than promptTokenCount when
// the stream is truncated. Deriving completion tokens as total-minus-prompt then
// produced a negative output count that reached the consume log and usage
// metrics. Such metadata must settle as zero completion instead.
func TestGeminiUsageDoesNotDeriveNegativeCompletionTokens(t *testing.T) {
	tests := []struct {
		name               string
		metadata           dto.GeminiUsageMetadata
		wantPromptTokens   int
		wantCompletionTok  int
		wantTotalTokenZero bool
	}{
		{
			name: "total below prompt settles zero completion",
			metadata: dto.GeminiUsageMetadata{
				PromptTokenCount: 23627,
				TotalTokenCount:  -254,
			},
			wantPromptTokens:  23627,
			wantCompletionTok: 0,
		},
		{
			name: "total equal to prompt settles zero completion",
			metadata: dto.GeminiUsageMetadata{
				PromptTokenCount: 18313,
				TotalTokenCount:  18313,
			},
			wantPromptTokens:  18313,
			wantCompletionTok: 0,
		},
		{
			name: "total above prompt still derives completion",
			metadata: dto.GeminiUsageMetadata{
				PromptTokenCount:     100,
				TotalTokenCount:      150,
				CandidatesTokenCount: 0,
			},
			wantPromptTokens:  100,
			wantCompletionTok: 50,
		},
		{
			name: "reported candidates win over the derived value",
			metadata: dto.GeminiUsageMetadata{
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
			billingUsage := dto.NewGeminiChatBillingUsage(&tc.metadata)
			require.NotNil(t, billingUsage)

			usage := usageFromGeminiBillingUsage(billingUsage)
			require.NotNil(t, usage)
			assert.Equal(t, tc.wantPromptTokens, usage.PromptTokens)
			assert.Equal(t, tc.wantCompletionTok, usage.CompletionTokens)
			assert.GreaterOrEqual(t, usage.CompletionTokens, 0)
		})
	}
}
