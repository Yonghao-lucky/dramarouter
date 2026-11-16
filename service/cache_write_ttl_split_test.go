package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCacheWriteTTLSplitFromHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		usage     dto.Usage
		want5m    int
		want1h    int
		wantSplit bool
	}{
		{
			name:      "all writes are 1h",
			header:    "2816",
			usage:     dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816}},
			want5m:    0,
			want1h:    2816,
			wantSplit: true,
		},
		{
			name:      "partial 1h share leaves the remainder as 5m",
			header:    "1000",
			usage:     dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816}},
			want5m:    1816,
			want1h:    1000,
			wantSplit: true,
		},
		{
			name:      "header larger than the body total is bounded by the total",
			header:    "999999999",
			usage:     dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816}},
			want5m:    0,
			want1h:    2816,
			wantSplit: true,
		},
		{
			name:   "zero 1h header keeps the legacy total-only usage",
			header: "0",
			usage:  dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816}},
		},
		{
			name:   "missing header keeps the legacy total-only usage",
			header: "",
			usage:  dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816}},
		},
		{
			name:   "malformed header is ignored",
			header: "-5",
			usage:  dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816}},
		},
		{
			name:   "no cache writes in the body",
			header: "2816",
			usage:  dto.Usage{},
		},
		{
			name:   "anthropic usage already carries its own split",
			header: "2816",
			usage: dto.Usage{
				UsageSemantic:               dto.BillingUsageSemanticAnthropic,
				ClaudeCacheCreation5mTokens: 2816,
				PromptTokensDetails:         dto.InputTokenDetails{CachedCreationTokens: 2816},
			},
			want5m: 2816,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.header != "" {
				header.Set(MoonshotCacheWrite1hHeader, tt.header)
			}
			usage := tt.usage

			ApplyCacheWriteTTLSplitFromHeader(header, &usage)

			assert.Equal(t, tt.want5m, usage.ClaudeCacheCreation5mTokens)
			assert.Equal(t, tt.want1h, usage.ClaudeCacheCreation1hTokens)
			if tt.wantSplit {
				assert.Equal(t, dto.BillingUsageSemanticOpenAI, usage.UsageSemantic)
			}
		})
	}
}

func TestApplyCacheWriteTTLSplitFromHeaderUpdatesOpenAIBillingUsage(t *testing.T) {
	header := http.Header{}
	header.Set(MoonshotCacheWrite1hHeader, "500")
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 800},
		BillingUsage: &dto.BillingUsage{
			Source: dto.BillingUsageSourceOAIChat,
			OpenAIUsage: &dto.Usage{
				PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 800},
			},
		},
	}

	ApplyCacheWriteTTLSplitFromHeader(header, usage)

	billingUsage := effectiveBillingUsage(usage)
	require.NotNil(t, billingUsage)
	assert.Equal(t, 300, billingUsage.ClaudeCacheCreation5mTokens)
	assert.Equal(t, 500, billingUsage.ClaudeCacheCreation1hTokens)
}

// Chat-completions cache writes reported as 1h by the upstream header must be
// priced with cc1h, matching what the same writes cost via /v1/messages.
func TestBuildTieredTokenParamsPricesOpenAICacheWriteTTLSplit(t *testing.T) {
	expr := `tier("base", p * 13 + c * 68 + cr * 1.3 + cc * 13 + cc1h * 26)`
	tests := []struct {
		name     string
		header1h string
		wantCost float64
	}{
		// p = 2818 - 2816 = 2: (2*13 + 16*68 + 2816*26) * 0.5
		{name: "all 1h", header1h: "2816", wantCost: 37165},
		// (2*13 + 16*68 + 1816*13 + 1000*26) * 0.5
		{name: "mixed 5m and 1h", header1h: "1000", wantCost: 25361},
		// (2*13 + 16*68 + 2816*13) * 0.5
		{name: "no header keeps 5m pricing", header1h: "0", wantCost: 18861},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &dto.Usage{
				PromptTokens:        2818,
				CompletionTokens:    16,
				PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 2816},
			}
			header := http.Header{}
			header.Set(MoonshotCacheWrite1hHeader, tt.header1h)
			ApplyCacheWriteTTLSplitFromHeader(header, usage)

			assert.InDelta(t, tt.wantCost, tieredQuota(expr, usage, false, 1.0), 0.01)
		})
	}
}

func TestCalculateTextQuotaSummaryPricesOpenAICacheWriteTTLSplit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "kimi-k3.1-pro",
		PriceData: hosttypes.PriceData{
			ModelRatio:           1,
			CompletionRatio:      2,
			CacheRatio:           0.1,
			CacheCreationRatio:   1.25,
			CacheCreation5mRatio: 1.25,
			CacheCreation1hRatio: 2,
			GroupRatioInfo:       hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:        1473,
		CompletionTokens:    19,
		PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 1470},
	}
	header := http.Header{}
	header.Set(MoonshotCacheWrite1hHeader, "470")
	ApplyCacheWriteTTLSplitFromHeader(header, usage)

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.Equal(t, 1470, summary.CacheCreationTokens)
	assert.Equal(t, 1000, summary.CacheCreationTokens5m)
	assert.Equal(t, 470, summary.CacheCreationTokens1h)
	assert.Equal(t, 1470, cacheWriteTokensTotal(summary))
	// (1473-1470) + 1000*1.25 + 470*2 + 19*2 = 3 + 1250 + 940 + 38 = 2231
	assert.Equal(t, 2231, summary.Quota)
}
