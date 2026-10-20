package ratio_setting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPeakPricing(t *testing.T, jsonString string) {
	t.Helper()
	original := PeakPricing2JSONString()
	require.NoError(t, UpdatePeakPricingByJSONString(jsonString))
	t.Cleanup(func() {
		require.NoError(t, UpdatePeakPricingByJSONString(original))
	})
}

func shanghaiTime(t *testing.T, value string) time.Time {
	t.Helper()
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, location)
	require.NoError(t, err)
	return parsed
}

func TestPeakFactorsUseIndependentInputOutputAndCacheMultipliers(t *testing.T) {
	setupPeakPricing(t, `{
		"enabled": true,
		"timezone": "Asia/Shanghai",
		"models": ["claude-*"],
		"rules": [
			{
				"name": "day",
				"start": "08:00",
				"end": "22:00",
				"input_factor": "1.2",
				"output_factor": "1.5",
				"cache_factor": "1",
				"cache_write_factor": "1.1"
			}
		]
	}`)

	factors := GetPeakFactors("claude-sonnet-4-5", shanghaiTime(t, "2026-10-19 10:00"))
	assert.Equal(t, "day", factors.RuleName)
	assert.Equal(t, 1.2, factors.Input)
	assert.Equal(t, 1.5, factors.Output)
	assert.Equal(t, 1.0, factors.Cache)
	assert.Equal(t, 1.1, factors.CacheWrite)
	assert.False(t, factors.IsNeutral())
}

func TestPeakFactorsFallBackToOffPeakOutsideWindows(t *testing.T) {
	setupPeakPricing(t, `{
		"enabled": true,
		"timezone": "Asia/Shanghai",
		"models": [],
		"rules": [
			{"name": "day", "start": "08:00", "end": "22:00", "input_factor": "1.5", "output_factor": "1.5"}
		],
		"off_peak": {"input_factor": "0.5", "output_factor": "0.6", "cache_factor": "0.4"}
	}`)

	night := GetPeakFactors("any-model", shanghaiTime(t, "2026-10-19 03:00"))
	assert.Empty(t, night.RuleName)
	assert.Equal(t, 0.5, night.Input)
	assert.Equal(t, 0.6, night.Output)
	assert.Equal(t, 0.4, night.Cache)
	assert.Equal(t, 1.0, night.CacheWrite, "unset factors default to 1")
}

func TestPeakFactorsHandleCrossMidnightWindowsAndWeekdays(t *testing.T) {
	setupPeakPricing(t, `{
		"enabled": true,
		"timezone": "Asia/Shanghai",
		"rules": [
			{"name": "night", "start": "22:00", "end": "06:00", "input_factor": "0.8"}
		],
		"off_peak": {"input_factor": "1.3"}
	}`)

	assert.Equal(t, 0.8, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 23:30")).Input)
	assert.Equal(t, 0.8, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 05:59")).Input)
	assert.Equal(t, 1.3, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 06:00")).Input)

	setupPeakPricing(t, `{
		"enabled": true,
		"timezone": "Asia/Shanghai",
		"rules": [
			{"name": "weekday-peak", "days": [1, 2, 3, 4, 5], "start": "09:00", "end": "18:00", "input_factor": "2"}
		]
	}`)
	// 2026-10-19 is a Monday, 2026-10-18 a Sunday.
	assert.Equal(t, 2.0, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 10:00")).Input)
	assert.Equal(t, 1.0, GetPeakFactors("m", shanghaiTime(t, "2026-10-18 10:00")).Input)
}

func TestPeakFactorExpressionsSeeBothZones(t *testing.T) {
	setupPeakPricing(t, `{
		"enabled": true,
		"timezone": "Asia/Shanghai",
		"rules": [
			{"name": "expr", "start": "00:00", "end": "00:00", "input_factor": "hour >= 22 || utc_hour < 6 ? 0.5 : 1.5"}
		]
	}`)

	// 23:30 Beijing == 15:30 UTC -> hour branch matches.
	assert.Equal(t, 0.5, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 23:30")).Input)
	// 10:00 Beijing == 02:00 UTC -> utc_hour branch matches.
	assert.Equal(t, 0.5, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 10:00")).Input)
	// 14:00 Beijing == 06:00 UTC -> neither branch matches.
	assert.Equal(t, 1.5, GetPeakFactors("m", shanghaiTime(t, "2026-10-19 14:00")).Input)
}

func TestPeakPricingRejectsInvalidConfiguration(t *testing.T) {
	original := PeakPricing2JSONString()
	defer func() {
		require.NoError(t, UpdatePeakPricingByJSONString(original))
	}()

	assert.Error(t, UpdatePeakPricingByJSONString(`{"timezone":"Not/AZone"}`))
	assert.Error(t, UpdatePeakPricingByJSONString(`{"rules":[{"start":"25:00","end":"10:00"}]}`))
	assert.Error(t, UpdatePeakPricingByJSONString(`{"rules":[{"days":[7]}]}`))
	assert.Error(t, UpdatePeakPricingByJSONString(`{"rules":[{"input_factor":"0"}]}`))
	assert.Error(t, UpdatePeakPricingByJSONString(`{"rules":[{"input_factor":"hour +"}]}`))
	assert.Error(t, UpdatePeakPricingByJSONString(`{"rules":[{"input_factor":"unknown_var"}]}`))
}

func TestPeakPricingDisabledReturnsNeutralFactors(t *testing.T) {
	setupPeakPricing(t, `{
		"enabled": false,
		"models": [],
		"rules": [{"start": "00:00", "end": "00:00", "input_factor": "3"}]
	}`)

	factors := GetPeakFactors("m", shanghaiTime(t, "2026-10-19 10:00"))
	assert.True(t, factors.IsNeutral())
	assert.Equal(t, 1.0, factors.Input)
}
