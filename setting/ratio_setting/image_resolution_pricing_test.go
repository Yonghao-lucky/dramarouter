package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupImageResolutionPricing(t *testing.T, jsonString string) {
	t.Helper()
	original := ImageResolutionPricing2JSONString()
	require.NoError(t, UpdateImageResolutionPricingByJSONString(jsonString))
	t.Cleanup(func() {
		require.NoError(t, UpdateImageResolutionPricingByJSONString(original))
	})
}

const testImageResolutionSetting = `{
	"enabled": true,
	"models": {
		"gpt-image-2.5": {
			"tiers": [
				{"name": "1K", "max_pixels": 2000000, "price": 0.0039},
				{"name": "2K", "max_pixels": 6000000, "price": 0.0166},
				{"name": "4K", "max_pixels": 0, "price": 0.0292}
			],
			"default_price": 0.0039
		}
	}
}`

func TestMatchImageResolutionPriceUsesPixelArea(t *testing.T) {
	setupImageResolutionPricing(t, testImageResolutionSetting)

	cases := []struct {
		name     string
		size     string
		wantTier string
		wantOK   bool
	}{
		{name: "square 1K", size: "1024x1024", wantTier: "1K", wantOK: true},
		{name: "landscape 1K", size: "1536x1024", wantTier: "1K", wantOK: true},
		{name: "portrait 1K", size: "1024x1536", wantTier: "1K", wantOK: true},
		{name: "2K landscape", size: "2048x1152", wantTier: "2K", wantOK: true},
		{name: "2K square", size: "2048x2048", wantTier: "2K", wantOK: true},
		{name: "4K landscape", size: "3840x2160", wantTier: "4K", wantOK: true},
		{name: "4K portrait", size: "2160x3840", wantTier: "4K", wantOK: true},
		{name: "fallback for auto", size: "auto", wantTier: "default", wantOK: true},
		{name: "fallback for empty", size: "", wantTier: "default", wantOK: true},
		{name: "fallback for garbage", size: "huge", wantTier: "default", wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, ok := MatchImageResolutionPrice("gpt-image-2.5", tc.size)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantTier, matched.TierName)
			assert.Greater(t, matched.Price, 0.0)
		})
	}
}

func TestMatchImageResolutionPriceKeepsTiersDistinct(t *testing.T) {
	setupImageResolutionPricing(t, testImageResolutionSetting)

	small, ok := MatchImageResolutionPrice("gpt-image-2.5", "1024x1024")
	require.True(t, ok)
	large, ok := MatchImageResolutionPrice("gpt-image-2.5", "3840x2160")
	require.True(t, ok)

	assert.Greater(t, large.Price, small.Price)
	assert.Equal(t, 1024, small.Width)
	assert.Equal(t, 3840, large.Width)
}

func TestMatchImageResolutionPriceIgnoresUnconfiguredModels(t *testing.T) {
	setupImageResolutionPricing(t, testImageResolutionSetting)

	_, ok := MatchImageResolutionPrice("gpt-4o", "1024x1024")
	assert.False(t, ok)
	assert.False(t, HasImageResolutionPricing("gpt-4o"))
	assert.True(t, HasImageResolutionPricing("gpt-image-2.5"))
}

func TestMatchImageResolutionPriceDisabledSetting(t *testing.T) {
	setupImageResolutionPricing(t, `{
		"enabled": false,
		"models": {
			"gpt-image-2.5": {
				"tiers": [{"name": "1K", "max_pixels": 2000000, "price": 0.0039}]
			}
		}
	}`)

	_, ok := MatchImageResolutionPrice("gpt-image-2.5", "1024x1024")
	assert.False(t, ok)
	assert.False(t, HasImageResolutionPricing("gpt-image-2.5"))
}

func TestMatchImageResolutionPriceWildcardModel(t *testing.T) {
	setupImageResolutionPricing(t, `{
		"enabled": true,
		"models": {
			"gpt-image-*": {
				"tiers": [{"name": "1K", "max_pixels": 2000000, "price": 0.0039}]
			}
		}
	}`)

	matched, ok := MatchImageResolutionPrice("gpt-image-2.5", "1024x1024")
	require.True(t, ok)
	assert.Equal(t, "1K", matched.TierName)
}

func TestMatchImageResolutionPriceFallsBackWhenNoDefaultConfigured(t *testing.T) {
	// 没有兜底档也没有默认价时不应静默变免费，交给普通模型价处理。
	setupImageResolutionPricing(t, `{
		"enabled": true,
		"models": {
			"gpt-image-2.5": {
				"tiers": [{"name": "1K", "max_pixels": 2000000, "price": 0.0039}]
			}
		}
	}`)

	_, ok := MatchImageResolutionPrice("gpt-image-2.5", "3840x2160")
	assert.False(t, ok)
	_, ok = MatchImageResolutionPrice("gpt-image-2.5", "auto")
	assert.False(t, ok)
}

func TestValidateImageResolutionPricingSetting(t *testing.T) {
	valid := `{
		"enabled": true,
		"models": {
			"gpt-image-2.5": {
				"tiers": [
					{"name": "1K", "max_pixels": 2000000, "price": 0.0039},
					{"name": "4K", "max_pixels": 0, "price": 0.0292}
				],
				"default_price": 0.0039
			}
		}
	}`
	require.NoError(t, UpdateImageResolutionPricingByJSONString(valid))

	invalid := map[string]string{
		"non-positive price": `{"enabled":true,"models":{"m":{"tiers":[{"name":"1K","max_pixels":2000000,"price":0}]}}}`,
		"duplicate tier name": `{"enabled":true,"models":{"m":{"tiers":[
			{"name":"1K","max_pixels":2000000,"price":0.01},
			{"name":"1K","max_pixels":3000000,"price":0.02}]}}}`,
		"descending limits": `{"enabled":true,"models":{"m":{"tiers":[
			{"name":"2K","max_pixels":6000000,"price":0.02},
			{"name":"1K","max_pixels":2000000,"price":0.01}]}}}`,
		"catch-all before sized tier": `{"enabled":true,"models":{"m":{"tiers":[
			{"name":"4K","max_pixels":0,"price":0.02},
			{"name":"1K","max_pixels":2000000,"price":0.01}]}}}`,
		"empty model name": `{"enabled":true,"models":{"  ":{"tiers":[{"name":"1K","max_pixels":1,"price":0.01}]}}}`,
	}
	for name, raw := range invalid {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, UpdateImageResolutionPricingByJSONString(raw))
		})
	}
}

func TestParseImageSize(t *testing.T) {
	cases := []struct {
		input      string
		wantWidth  int
		wantHeight int
		wantOK     bool
	}{
		{input: "1024x1024", wantWidth: 1024, wantHeight: 1024, wantOK: true},
		{input: " 2048X1152 ", wantWidth: 2048, wantHeight: 1152, wantOK: true},
		{input: "3840*2160", wantWidth: 3840, wantHeight: 2160, wantOK: true},
		{input: "auto", wantOK: false},
		{input: "", wantOK: false},
		{input: "1024", wantOK: false},
		{input: "0x1024", wantOK: false},
		{input: "-1024x1024", wantOK: false},
		{input: "200000x200000", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			width, height, ok := ParseImageSize(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantWidth, width)
			assert.Equal(t, tc.wantHeight, height)
		})
	}
}
