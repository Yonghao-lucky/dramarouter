package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderTierCalculationSubstitutesUsageFacts(t *testing.T) {
	const h3 = `u("resolution") == "768P" ? tier("768P", max(0, u("input_images") - 5) * 0.03 + u("input_video_seconds") * 0.075 + u("seconds") * 0.075) : tier("2K", max(0, u("input_images") - 5) * 0.03 + u("input_video_seconds") * 0.12 + u("seconds") * 0.12)`

	rendered, ok := RenderTierCalculation(h3, "768P", map[string]any{
		"resolution":         "768P",
		"input_images":       float64(0),
		"input_video_seconds": float64(0),
		"seconds":            float64(4),
	})

	assert.True(t, ok)
	assert.Equal(t, "max(0, 0 - 5) * 0.03 + 0 * 0.075 + 4 * 0.075", rendered)
}

func TestRenderTierCalculationKeepsUnrecordedUsageSymbolic(t *testing.T) {
	const expression = `tier("standard", u("seconds") * 0.5 + u("input_images") * 0.03)`

	rendered, ok := RenderTierCalculation(expression, "standard", map[string]any{"seconds": 4})

	assert.True(t, ok)
	assert.Equal(t, `4 * 0.5 + u("input_images") * 0.03`, rendered)
}

func TestRenderTierCalculationWithoutMatchingTierReportsMiss(t *testing.T) {
	const expression = `tier("768P", u("seconds") * 0.075)`

	_, ok := RenderTierCalculation(expression, "1080P", map[string]any{"seconds": 4})

	assert.False(t, ok)
}

func TestRenderTierCalculationRejectsUnparseableExpression(t *testing.T) {
	_, ok := RenderTierCalculation(`tier("768P", u("seconds" * 0.075)`, "768P", map[string]any{"seconds": 4})

	assert.False(t, ok)
}
