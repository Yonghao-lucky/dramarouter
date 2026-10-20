package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveResellerPricingLayering pins the additive contract of the pricing
// layer: it must only override when a configured ratio exists, and the
// customer-specific price must win over the reseller default.
func TestResolveResellerPricingLayering(t *testing.T) {
	truncateTables(t)

	const (
		customerID = 9101
		resellerID = 9102
	)

	require.NoError(t, DB.Create(&User{Id: customerID, Username: "pricing_customer", Group: "default", AffCode: "rpc1"}).Error)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "pricing_reseller", Group: "default", AffCode: "rpr1"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerID, resellerID, 1, "test"))
	require.NoError(t, DB.Create(&ResellerGroupCost{ResellerId: resellerID, GroupName: "vip", CostRatio: "0.8", Enabled: true}).Error)
	require.NoError(t, DB.Create(&ResellerDefaultPrice{ResellerId: resellerID, GroupName: "vip", Ratio: "1.2"}).Error)

	// Platform-direct customer with no configured price: no override.
	require.NoError(t, DB.Create(&User{Id: 9103, Username: "direct_customer", Group: "default", AffCode: "rpd1"}).Error)
	direct, err := ResolveResellerPricing(9103, "vip")
	require.NoError(t, err)
	assert.False(t, direct.Override, "no configured price must leave the existing chain untouched")
	assert.Zero(t, direct.OwnerID)

	// Reseller default applies to an owned customer without a specific price.
	defaulted, err := ResolveResellerPricing(customerID, "vip")
	require.NoError(t, err)
	assert.True(t, defaulted.Override)
	assert.Equal(t, "reseller_default", defaulted.Source)
	assert.Equal(t, 1.2, defaulted.Retail)
	assert.Equal(t, 0.8, defaulted.CostRatio)
	assert.Equal(t, resellerID, defaulted.OwnerID)

	// A customer-specific price wins over the reseller default.
	require.NoError(t, DB.Create(&CustomerGroupPrice{UserId: customerID, GroupName: "vip", Ratio: "1.5", Source: "admin"}).Error)
	require.NoError(t, InvalidateResellerPricingCache(customerID))
	specific, err := ResolveResellerPricing(customerID, "vip")
	require.NoError(t, err)
	assert.True(t, specific.Override)
	assert.Equal(t, "customer", specific.Source)
	assert.Equal(t, 1.5, specific.Retail)
	assert.Equal(t, 0.8, specific.CostRatio, "cost ratio still comes from the reseller profile")

	// A group without any configured layer falls through.
	unpriced, err := ResolveResellerPricing(customerID, "default")
	require.NoError(t, err)
	assert.False(t, unpriced.Override)
}

func TestResellerMarginQuotaArithmetic(t *testing.T) {
	// retail 1000 at retail ratio 1.25 means base 800; cost ratio 1.0 means the
	// wholesale charge is 800, so the reseller keeps 200.
	assert.Equal(t, int64(200), ResellerMarginQuota(1000, 1.25, 1.0))
	// No reseller cost configured: no margin can be derived.
	assert.Zero(t, ResellerMarginQuota(1000, 1.25, 0))
	// Refunds carry the negative sign.
	assert.Equal(t, int64(-200), ResellerMarginQuota(-1000, 1.25, 1.0))
}

func TestResellerCustomerAllocationAvailability(t *testing.T) {
	allocation := ResellerCustomerAllocation{AllocatedQuota: 1000, ReservedQuota: 300, UsedQuota: 500}
	assert.Equal(t, int64(200), allocation.AvailableQuota())
	allocation = ResellerCustomerAllocation{AllocatedQuota: 100, ReservedQuota: 300, UsedQuota: 500}
	assert.Zero(t, allocation.AvailableQuota(), "available quota must never be negative")
}

func TestResellerOwnershipHistoryIsAppendOnly(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 9201, Username: "hist_customer", Group: "default", AffCode: "rph1"}).Error)
	require.NoError(t, SetResellerCustomerOwner(9201, 9202, 1, "assign"))
	require.NoError(t, SetResellerCustomerOwner(9201, 0, 1, "reclaim"))

	var events []ResellerCustomerEvent
	require.NoError(t, DB.Where("customer_user_id = ?", 9201).Order("id asc").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, 0, events[0].FromResellerId)
	assert.Equal(t, 9202, events[0].ToResellerId)
	assert.Equal(t, 9202, events[1].FromResellerId)
	assert.Equal(t, 0, events[1].ToResellerId)

	owner, err := GetResellerCustomerOwner(9201)
	require.NoError(t, err)
	assert.Zero(t, owner)
}
