package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedResellerFixture(t *testing.T, resellerID, customerID int, ownerQuota, allocated int64) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "rs_owner", Group: "default", AffCode: "rsa1", Quota: int(ownerQuota)}).Error)
	require.NoError(t, DB.Create(&User{Id: customerID, Username: "rs_customer", Group: "default", AffCode: "rsa2"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerID, resellerID, 1, "seed"))
	require.NoError(t, DB.Create(&ResellerGroupCost{ResellerId: resellerID, GroupName: "vip", CostRatio: "1", Enabled: true}).Error)
	require.NoError(t, AdjustResellerAllocation(customerID, resellerID, allocated, 1))
}

// TestResellerPoolReservationChargesWholesale pins the money flow: the customer's
// cap is consumed at retail, while the reseller wallet pays wholesale.
func TestResellerPoolReservationChargesWholesale(t *testing.T) {
	const (
		resellerID = 9301
		customerID = 9302
	)
	seedResellerFixture(t, resellerID, customerID, 10000, 1000)

	wholesale, ok, err := ReserveResellerAllocation(customerID, resellerID, "req-pool-1", 400, 1.25, 1.0)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(320), wholesale, "400 retail at 1.25 with wholesale 1.0 costs the reseller 320")

	var allocation ResellerCustomerAllocation
	require.NoError(t, DB.Where("customer_user_id = ?", customerID).First(&allocation).Error)
	assert.Equal(t, int64(400), allocation.ReservedQuota)
	owner, err := GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000-320, owner)

	// Same request id is idempotent: no second reservation and no second debit.
	again, ok, err := ReserveResellerAllocation(customerID, resellerID, "req-pool-1", 400, 1.25, 1.0)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(320), again)
	require.NoError(t, DB.Where("customer_user_id = ?", customerID).First(&allocation).Error)
	assert.Equal(t, int64(400), allocation.ReservedQuota, "a retried pre-consume must not reserve twice")
	owner, err = GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000-320, owner)
}

// TestResellerAllocationNeverOverCredits pins "不允许超额授信": a reservation
// beyond the remaining cap is refused and falls back, never negative.
func TestResellerAllocationNeverOverCredits(t *testing.T) {
	const (
		resellerID = 9311
		customerID = 9312
	)
	seedResellerFixture(t, resellerID, customerID, 10000, 100)

	_, ok, err := ReserveResellerAllocation(customerID, resellerID, "req-over-1", 150, 1.25, 1.0)
	require.NoError(t, err)
	assert.False(t, ok, "a charge above the allocation must not be reserved")

	var allocation ResellerCustomerAllocation
	require.NoError(t, DB.Where("customer_user_id = ?", customerID).First(&allocation).Error)
	assert.Equal(t, int64(0), allocation.ReservedQuota)
	assert.Equal(t, int64(100), allocation.AvailableQuota())

	_, reserved, err := ReserveResellerAllocation(customerID, resellerID, "req-over-2", 80, 1.25, 1.0)
	require.NoError(t, err)
	require.True(t, reserved)
	require.Error(t, AdjustResellerAllocation(customerID, resellerID, 50, 1), "the cap cannot drop below what is already reserved")
}

// TestResellerEmptyWalletBlocksCustomers pins the shared-pool rule: when the
// reseller wallet cannot fund the wholesale charge, the request fails instead
// of quietly borrowing from the platform.
func TestResellerEmptyWalletBlocksCustomers(t *testing.T) {
	const (
		resellerID = 9321
		customerID = 9322
	)
	seedResellerFixture(t, resellerID, customerID, 10, 1000)

	_, ok, err := ReserveResellerAllocation(customerID, resellerID, "req-broke-1", 400, 1.25, 1.0)
	assert.ErrorIs(t, err, ErrResellerWalletInsufficient)
	assert.False(t, ok)

	var allocation ResellerCustomerAllocation
	require.NoError(t, DB.Where("customer_user_id = ?", customerID).First(&allocation).Error)
	assert.Equal(t, int64(0), allocation.ReservedQuota, "a failed wholesale charge must not consume the cap")
}

// TestResellerSettleMovesReservedToUsed pins settle arithmetic: the reservation
// converts to consumption at the actual retail charge and the wallet is trued up.
func TestResellerSettleMovesReservedToUsed(t *testing.T) {
	const (
		resellerID = 9331
		customerID = 9332
	)
	seedResellerFixture(t, resellerID, customerID, 10000, 1000)

	_, ok, err := ReserveResellerAllocation(customerID, resellerID, "req-settle-1", 400, 1.25, 1.0)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, SettleResellerAllocation("req-settle-1", 500, 1.25, 1.0))
	var allocation ResellerCustomerAllocation
	require.NoError(t, DB.Where("customer_user_id = ?", customerID).First(&allocation).Error)
	assert.Zero(t, allocation.ReservedQuota)
	assert.Equal(t, int64(500), allocation.UsedQuota)
	assert.Equal(t, int64(500), allocation.AvailableQuota())

	owner, err := GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000-400, owner, "500 retail at wholesale 1.0 over retail 1.25 costs 400")

	// Settling twice must not charge twice.
	require.NoError(t, SettleResellerAllocation("req-settle-1", 500, 1.25, 1.0))
	owner, err = GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000-400, owner)
}

// TestResellerReleaseRefundsWholesale pins the refund path.
func TestResellerReleaseRefundsWholesale(t *testing.T) {
	const (
		resellerID = 9341
		customerID = 9342
	)
	seedResellerFixture(t, resellerID, customerID, 10000, 1000)

	_, ok, err := ReserveResellerAllocation(customerID, resellerID, "req-release-1", 400, 1.25, 1.0)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, ReleaseResellerAllocation("req-release-1"))

	var allocation ResellerCustomerAllocation
	require.NoError(t, DB.Where("customer_user_id = ?", customerID).First(&allocation).Error)
	assert.Zero(t, allocation.ReservedQuota)
	assert.Zero(t, allocation.UsedQuota)
	owner, err := GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000, owner, "a released reservation returns the entire wholesale charge")

	// Releasing twice must not credit twice.
	require.NoError(t, ReleaseResellerAllocation("req-release-1"))
	owner, err = GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000, owner)
}
