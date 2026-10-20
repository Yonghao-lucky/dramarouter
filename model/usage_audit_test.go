package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAuditUsesEffectiveAmountsAcrossSeparateDatabases(t *testing.T) {
	fixture := seedBillingEffectiveViews(t)
	appendBillingEffectiveFixtureEvents(t, fixture, false)
	rows, err := GetUsageAuditLogs(context.Background(), UsageAuditFilter{UserID: 50, TokenID: 4, Group: "new", StartAt: fixture.Hour, EndAt: fixture.Hour + 3600})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, 1200, rows[0].Quota)
	assert.Equal(t, 480, rows[1].Quota)
	assert.Equal(t, "new", rows[0].Group)
	var original Log
	require.NoError(t, LOG_DB.First(&original, fixture.Charge.Id).Error)
	assert.Equal(t, 1000, original.Quota)
	assert.Equal(t, "old", original.Group)
	keys, err := SearchUsageAuditKeys(context.Background(), 50, "4")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	body, err := common.Marshal(keys)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "test-key")
	require.NoError(t, DB.Delete(&Token{}, 4).Error)
	keys, err = SearchUsageAuditKeys(context.Background(), 50, "4")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	keys, err = SearchUsageAuditKeys(context.Background(), 51, "4")
	require.NoError(t, err)
	assert.Empty(t, keys)
}
