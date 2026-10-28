package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestApplyResellerPricingKeepsAPlatformDirectOverride locks the rule that a
// per-user group price belongs to the customer, not to the reseller layer. An
// administrator can set one on an ordinary platform account, which has no
// reseller owner and therefore no wholesale cost; the price must still be
// charged instead of falling back to the platform group ratio.
func TestApplyResellerPricingKeepsAPlatformDirectOverride(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CustomerGroupPrice{}, &model.ResellerCustomerBinding{}, &model.ResellerGroupCost{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, model.InvalidateResellerPricingCache(1866))
	})

	const (
		userID = 1866
		group  = "Claude-AWS Bedrock"
	)
	require.NoError(t, db.Create(&model.CustomerGroupPrice{
		UserId: userID, GroupName: group, Ratio: "0.55", Source: "admin",
	}).Error)
	require.NoError(t, model.InvalidateResellerPricingCache(userID))

	relayInfo := &relaycommon.RelayInfo{UserId: userID, UsingGroup: group}
	groupRatioInfo := &hosttypes.GroupRatioInfo{GroupRatio: 0.6}
	applyResellerPricing(relayInfo, groupRatioInfo)

	assert.Equal(t, 0.55, groupRatioInfo.GroupRatio, "the per-user price must win over the platform group ratio")
	assert.Equal(t, 0.55, groupRatioInfo.GroupSpecialRatio)
	assert.True(t, groupRatioInfo.HasSpecialRatio)
	assert.Equal(t, 0.0, relayInfo.ResellerCostRatio, "a platform-direct customer records no wholesale cost")
	assert.Equal(t, 0, relayInfo.ResellerOwnerID)
}

// TestApplyResellerPricingKeepsOverrideWithoutWholesaleCost covers the same
// guard from the other side: a customer owned by a reseller, priced for a group
// whose wholesale cost is not configured, is still charged the configured price.
func TestApplyResellerPricingKeepsOverrideWithoutWholesaleCost(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CustomerGroupPrice{}, &model.ResellerCustomerBinding{}, &model.ResellerGroupCost{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, model.InvalidateResellerPricingCache(1867))
	})

	const (
		userID     = 1867
		resellerID = 1868
		group      = "unpriced-group"
	)
	require.NoError(t, db.Create(&model.CustomerGroupPrice{
		UserId: userID, GroupName: group, Ratio: "0.9", Source: "reseller",
	}).Error)
	require.NoError(t, db.Create(&model.ResellerCustomerBinding{
		CustomerUserId: userID, ResellerId: resellerID,
	}).Error)
	require.NoError(t, model.InvalidateResellerPricingCache(userID))

	relayInfo := &relaycommon.RelayInfo{UserId: userID, UsingGroup: group}
	groupRatioInfo := &hosttypes.GroupRatioInfo{GroupRatio: 0.6}
	applyResellerPricing(relayInfo, groupRatioInfo)

	assert.Equal(t, 0.9, groupRatioInfo.GroupRatio)
	assert.Equal(t, 0.0, relayInfo.ResellerCostRatio, "no wholesale row means no margin evidence")
}
