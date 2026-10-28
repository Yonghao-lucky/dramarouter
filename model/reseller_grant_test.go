package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGrantResellerWalletRecordsKindAndIsIdempotent locks the admin top-up
// contract: the wallet grows by the exact amount, the reason and type are
// stored, pre-credit is accounted separately from paid recharge, and replaying
// the same event key pays nothing a second time.
func TestGrantResellerWalletRecordsKindAndIsIdempotent(t *testing.T) {
	truncateTables(t)

	const (
		resellerID = 9511
		operatorID = 1
	)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "grant_reseller", Group: "default", AffCode: "grt1", Quota: 1000}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "g", Status: ResellerStatusEnabled}).Error)

	grant, err := GrantResellerWallet(resellerID, operatorID, 5000, ResellerGrantPrecredit, "opening credit", "grant-evt-1")
	require.NoError(t, err)
	assert.Equal(t, int64(5000), grant.Amount)
	assert.Equal(t, ResellerGrantPrecredit, grant.Kind)
	assert.Equal(t, "opening credit", grant.Remark)

	quota, err := GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 6000, quota)

	again, err := GrantResellerWallet(resellerID, operatorID, 5000, ResellerGrantPrecredit, "opening credit", "grant-evt-1")
	require.NoError(t, err)
	assert.Equal(t, grant.Id, again.Id)
	quota, err = GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 6000, quota, "a replayed grant must not pay twice")

	// The same key bound to a different amount is a conflict, not a replay.
	_, err = GrantResellerWallet(resellerID, operatorID, 9000, ResellerGrantPrecredit, "opening credit", "grant-evt-1")
	assert.ErrorIs(t, err, ErrResellerGrantReplay)

	rows, err := ListResellerWalletGrants(resellerID, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, ResellerGrantPrecredit, rows[0].Kind)

	overview, err := GetResellerOverview(resellerID)
	require.NoError(t, err)
	require.NotNil(t, overview)
	assert.Equal(t, int64(5000), overview.GrantedPrecredit)
	assert.Equal(t, int64(0), overview.GrantedRecharge)
	assert.Equal(t, int64(6000), overview.WalletQuota)

	_, err = GrantResellerWallet(resellerID, operatorID, 100, "bogus", "", "grant-evt-2")
	assert.ErrorIs(t, err, ErrResellerGrantInvalid)

	// A user without a reseller profile cannot receive a grant.
	require.NoError(t, DB.Create(&User{Id: 9512, Username: "plain_grant", Group: "default", AffCode: "grt2"}).Error)
	_, err = GrantResellerWallet(9512, operatorID, 100, ResellerGrantRecharge, "", "grant-evt-3")
	assert.ErrorIs(t, err, ErrResellerNotEnabled)

	_ = common.GetTimestamp()
}
