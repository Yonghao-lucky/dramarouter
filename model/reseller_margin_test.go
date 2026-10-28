package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestClaimResellerMarginIsExplicitAndIdempotent locks the profit payout
// contract: earned margin sits pending until the reseller credits it, the credit
// lands in the wallet exactly once, and a replayed claim cannot pay twice.
func TestClaimResellerMarginIsExplicitAndIdempotent(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&User{},
		&BillingAccount{},
		&BillingEntry{},
		&CostAccountingSnapshot{},
		&ResellerMarginCreditCursor{},
	))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	reseller := &User{Username: "margin-owner", Password: "unused", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "margin-owner", Quota: 1000}
	require.NoError(t, db.Create(reseller).Error)

	for _, margin := range []int64{120, 80} {
		require.NoError(t, db.Create(&CostAccountingSnapshot{
			EventKey:            "snapshot:" + reseller.Username + ":" + common.GetUUID(),
			UserID:              reseller.Id,
			OwnerID:             reseller.Id,
			GroupName:           "default",
			OccurredAt:          common.GetTimestamp(),
			RevenueQuota:        600,
			ResellerMarginQuota: margin,
			ResellerSettlement:  "self",
		}).Error)
	}
	// Margin recorded against another owner must never be claimed here.
	require.NoError(t, db.Create(&CostAccountingSnapshot{
		EventKey:            "snapshot:other:" + common.GetUUID(),
		UserID:              999,
		OwnerID:             999,
		GroupName:           "default",
		OccurredAt:          common.GetTimestamp(),
		RevenueQuota:        300,
		ResellerMarginQuota: 45,
		ResellerSettlement:  "self",
	}).Error)

	pending, err := PendingResellerMarginQuota(reseller.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(200), pending, "both batches are pending before the claim")

	credited, err := ClaimResellerMargin(reseller.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(200), credited)

	var afterUser User
	require.NoError(t, db.First(&afterUser, reseller.Id).Error)
	assert.Equal(t, 1200, afterUser.Quota, "margin moved into the wallet 1:1")

	pending, err = PendingResellerMarginQuota(reseller.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending, "nothing stays pending after the claim")

	credited, err = ClaimResellerMargin(reseller.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(0), credited, "a replayed claim pays nothing")
	require.NoError(t, db.First(&afterUser, reseller.Id).Error)
	assert.Equal(t, 1200, afterUser.Quota)

	var otherPending int64
	otherPending, err = PendingResellerMarginQuota(999)
	require.NoError(t, err)
	assert.Equal(t, int64(45), otherPending, "another owner's margin is untouched")
}

// TestResellerMarginClaimNeverDebitsTheWallet locks the loss guard: an
// administrator may price a customer below the reseller's wholesale cost, which
// yields a negative margin. Claiming must not turn that loss into a debit on the
// reseller wallet, and the loss must still settle once instead of retrying.
func TestResellerMarginClaimNeverDebitsTheWallet(t *testing.T) {
	truncateTables(t)

	const ownerID = 9421
	require.NoError(t, DB.Create(&User{Id: ownerID, Username: "loss_owner", Group: "default", AffCode: "los1", Quota: 5000}).Error)
	require.NoError(t, DB.Create(&CostAccountingSnapshot{
		EventKey:            "snapshot:loss:" + common.GetUUID(),
		UserID:              9422,
		OwnerID:             ownerID,
		GroupName:           "default",
		OccurredAt:          common.GetTimestamp(),
		RevenueQuota:        100,
		ResellerMarginQuota: -113,
		ResellerSettlement:  "self",
	}).Error)

	pending, err := PendingResellerMarginQuota(ownerID)
	require.NoError(t, err)
	assert.Equal(t, int64(-113), pending, "a loss is reported as negative pending margin")

	credited, err := ClaimResellerMargin(ownerID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), credited)

	quota, err := GetUserQuota(ownerID, true)
	require.NoError(t, err)
	assert.Equal(t, 5000, quota, "claiming must never debit the reseller wallet")

	pending, err = PendingResellerMarginQuota(ownerID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending, "the loss settles once instead of retrying forever")
}
