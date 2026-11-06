package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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

	// A group without a reseller retail price still resolves the reseller's cost
	// for it, because the customer is an ordinary account that may call that
	// group and the spread is the reseller's margin.
	unpriced, err := ResolveResellerPricing(customerID, "default")
	require.NoError(t, err)
	assert.False(t, unpriced.Override, "no retail price must leave the platform ratio in charge")
	assert.Equal(t, resellerID, unpriced.OwnerID)
	assert.Greater(t, unpriced.CostRatio, 0.0, "the platform group ratio is the fallback cost")
}

// TestResellerPlatformGroupsListsEveryPlatformGroup pins the contract that a
// reseller customer may be placed in any group the platform serves, not only
// the ones in the reseller's price book, and that a group outside the book
// still reports a cost and an effective price.
func TestResellerPlatformGroupsListsEveryPlatformGroup(t *testing.T) {
	truncateTables(t)

	const (
		customerID = 9201
		resellerID = 9202
	)
	require.NoError(t, DB.Create(&User{Id: customerID, Username: "groups_customer", Group: "default", AffCode: "rpg1"}).Error)
	require.NoError(t, DB.Create(&ResellerGroupCost{ResellerId: resellerID, GroupName: "vip", CostRatio: "0.4", Enabled: true}).Error)
	require.NoError(t, DB.Create(&CustomerGroupPrice{UserId: customerID, GroupName: "vip", Ratio: "0.9", Source: "reseller"}).Error)
	// The book lists only groups an enabled channel still serves, so both groups
	// under test need a live route.
	require.NoError(t, DB.Create(&Channel{Id: 9301, Name: "groups-channel", Status: common.ChannelStatusEnabled}).Error)
	for _, group := range []string{"vip", "default"} {
		require.NoError(t, DB.Create(&Ability{Group: group, Model: "groups-model", ChannelId: 9301, Enabled: true}).Error)
	}

	rows, err := ResellerPlatformGroups(resellerID, customerID)
	require.NoError(t, err)

	byGroup := make(map[string]ResellerPlatformGroupRow, len(rows))
	for _, row := range rows {
		byGroup[row.Group] = row
	}

	vip, ok := byGroup["vip"]
	require.True(t, ok, "the price-book group must be listed")
	assert.True(t, vip.Sellable)
	assert.Equal(t, "0.4", vip.CostRatio)
	assert.Equal(t, "0.9", vip.Ratio)
	assert.Equal(t, "customer", vip.Source)

	platform, ok := byGroup["default"]
	require.True(t, ok, "a platform group outside the price book must still be listed")
	assert.True(t, platform.Sellable, "a platform group has a platform cost, so it is sellable")
	assert.NotEmpty(t, platform.CostRatio)
	// The standing customer price is the platform's own group price, never the
	// reseller's wholesale cost: prefilling the cost would show the customer the
	// reseller's purchase price and leave the reseller no spread to sell on.
	assert.Equal(t, "platform", platform.Source)
	assert.NotEmpty(t, platform.PlatformRatio, "the platform ratio is the standing customer price")
	assert.Equal(t, platform.PlatformRatio, platform.Ratio)
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

// TestResellerTransferMarginsAndReclaim pins the single-wallet contract: a fund
// moves quota 1:1 between the two wallets and a reclaim moves it back. The
// reseller's profit is not taken here — the customer pays retail like any normal
// account and the spread is credited from the billing records afterwards.
func TestResellerTransferMarginsAndReclaim(t *testing.T) {
	truncateTables(t)

	const (
		resellerID = 9401
		customerID = 9402
	)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "transfer_reseller", Group: "default", AffCode: "trs1", Quota: 100000}).Error)
	require.NoError(t, DB.Create(&User{Id: customerID, Username: "transfer_customer", Group: "vip", AffCode: "trs2"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerID, resellerID, resellerID, "seed"))
	require.NoError(t, DB.Create(&ResellerGroupCost{ResellerId: resellerID, GroupName: "vip", CostRatio: "0.8", Enabled: true}).Error)
	require.NoError(t, DB.Create(&ResellerDefaultPrice{ResellerId: resellerID, GroupName: "vip", Ratio: "1.0"}).Error)

	// Funding 10000 moves 10000 from the reseller and credits the customer 10000.
	// The customer's group and the wholesale table play no part in the amount.
	transfer, err := TransferResellerQuota(resellerID, customerID, resellerID, ResellerTransferFund, 10000, "evt-fund-1", "seed")
	require.NoError(t, err)
	assert.Equal(t, int64(10000), transfer.CostQuota)
	assert.Equal(t, int64(10000), transfer.RetailQuota)

	resellerQuota, err := GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 100000-10000, resellerQuota)
	customerQuota, err := GetUserQuota(customerID, true)
	require.NoError(t, err)
	assert.Equal(t, 10000, customerQuota)

	// The same event key is idempotent: no second movement.
	again, err := TransferResellerQuota(resellerID, customerID, resellerID, ResellerTransferFund, 10000, "evt-fund-1", "seed")
	require.NoError(t, err)
	assert.Equal(t, transfer.Id, again.Id)
	resellerQuota, err = GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 100000-10000, resellerQuota)

	// Reclaiming returns exactly what was moved back.
	reclaim, err := TransferResellerQuota(resellerID, customerID, resellerID, ResellerTransferReclaim, 4000, "evt-reclaim-1", "unused")
	require.NoError(t, err)
	assert.Equal(t, int64(4000), reclaim.CostQuota)
	resellerQuota, err = GetUserQuota(resellerID, true)
	require.NoError(t, err)
	assert.Equal(t, 100000-10000+4000, resellerQuota)
	customerQuota, err = GetUserQuota(customerID, true)
	require.NoError(t, err)
	assert.Equal(t, 6000, customerQuota)

	// A reclaim cannot exceed the outstanding retail position.
	_, err = TransferResellerQuota(resellerID, customerID, resellerID, ResellerTransferReclaim, 7000, "evt-reclaim-2", "too much")
	assert.ErrorIs(t, err, ErrResellerTransferInvalid)

	reclaimable, err := ResellerReclaimableQuota(resellerID, customerID)
	require.NoError(t, err)
	assert.Equal(t, int64(6000), reclaimable)
}

// TestResellerTransferGuards pins the three refusals that protect the platform:
// an unauthorized group, a reseller wallet that cannot cover the wholesale, and
// a customer that is not owned by the caller.
func TestResellerTransferGuards(t *testing.T) {
	truncateTables(t)

	const (
		resellerID  = 9411
		customerID  = 9412
		otherID     = 9413
		ungroupedID = 9414
	)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "guard_reseller", Group: "default", AffCode: "trg1", Quota: 100}).Error)
	require.NoError(t, DB.Create(&User{Id: customerID, Username: "guard_customer", Group: "vip", AffCode: "trg2"}).Error)
	require.NoError(t, DB.Create(&User{Id: otherID, Username: "guard_other", Group: "default", AffCode: "trg3"}).Error)
	// A customer sitting in a group the reseller has no cost row for.
	require.NoError(t, DB.Create(&User{Id: ungroupedID, Username: "guard_ungrouped", Group: "default", AffCode: "trg4"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerID, resellerID, resellerID, "seed"))
	require.NoError(t, SetResellerCustomerOwner(ungroupedID, resellerID, resellerID, "seed"))
	require.NoError(t, DB.Create(&ResellerGroupCost{ResellerId: resellerID, GroupName: "vip", CostRatio: "0.8", Enabled: true}).Error)

	_, err := TransferResellerQuota(resellerID, otherID, resellerID, ResellerTransferFund, 1000, "evt-guard-own", "")
	assert.ErrorIs(t, err, ErrResellerCustomerNotFound, "an unowned customer must never be funded")

	// A customer is an ordinary account: funding works no matter which group it
	// sits in, because the transfer is 1:1 and does not price the request.
	funded, err := TransferResellerQuota(resellerID, ungroupedID, resellerID, ResellerTransferFund, 50, "evt-guard-group", "")
	require.NoError(t, err)
	assert.Equal(t, int64(50), funded.CostQuota)

	_, err = TransferResellerQuota(resellerID, customerID, resellerID, ResellerTransferFund, 1000, "evt-guard-wallet", "")
	assert.ErrorIs(t, err, ErrResellerWalletShort, "the reseller must prepay the wholesale amount")
}

// TestResellerPriceFloor pins "客户价格不得低于成本": a reseller-scoped price below
// its own wholesale ratio is refused, while the administrator is not bound.
func TestResellerPriceFloor(t *testing.T) {
	truncateTables(t)

	const (
		resellerID = 9421
		customerID = 9422
	)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "floor_reseller", Group: "default", AffCode: "trf1"}).Error)
	require.NoError(t, DB.Create(&User{Id: customerID, Username: "floor_customer", Group: "vip", AffCode: "trf2"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerID, resellerID, resellerID, "seed"))
	require.NoError(t, DB.Create(&ResellerGroupCost{ResellerId: resellerID, GroupName: "vip", CostRatio: "0.8", Enabled: true}).Error)

	err := SetCustomerGroupPrice(customerID, "vip", "0.5", "reseller", resellerID, true)
	assert.ErrorIs(t, err, ErrResellerPriceBelowCost)

	require.NoError(t, SetCustomerGroupPrice(customerID, "vip", "1.2", "reseller", resellerID, true))
	prices, err := ListCustomerGroupPrices(customerID)
	require.NoError(t, err)
	require.Len(t, prices, 1)
	assert.Equal(t, "1.2", prices[0].Ratio)

	// The administrator may price below cost for a platform promotion.
	require.NoError(t, SetCustomerGroupPrice(customerID, "vip", "0.5", "admin", 1, false))
}

// TestResellerCustomerLogsHideChannel pins that a reseller never learns the
// channel identity of its customers' usage.
func TestResellerCustomerLogsHideChannel(t *testing.T) {
	truncateTables(t)

	const (
		resellerID = 9431
		customerID = 9432
	)
	require.NoError(t, DB.Create(&User{Id: resellerID, Username: "log_reseller", Group: "default", AffCode: "trl1"}).Error)
	require.NoError(t, DB.Create(&User{Id: customerID, Username: "log_customer", Group: "default", AffCode: "trl2"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerID, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerID, resellerID, resellerID, "seed"))
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: customerID, Type: LogTypeConsume, Content: "charge", ChannelId: 77, ChannelName: "secret-channel",
		ModelName: "gpt-image-2.5", CreatedAt: common.GetTimestamp(),
	}).Error)

	logs, total, err := ResellerCustomerLogs(resellerID, 0, LogTypeUnknown, 0, 0, "", 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Zero(t, logs[0].ChannelId, "channel id must never reach a reseller")
	assert.Empty(t, logs[0].ChannelName, "channel name must never reach a reseller")

	// A customer outside the book is refused outright.
	otherLogs, _, err := ResellerCustomerLogs(resellerID, 999999, LogTypeUnknown, 0, 0, "", 0, 20)
	assert.ErrorIs(t, err, ErrResellerCustomerNotFound)
	assert.Empty(t, otherLogs)
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

// TestResellerTransferEventKeyCannotCrossTenants locks the replay guard: the
// event key is globally unique, so a key another reseller already used must be
// a refusal rather than a success that returns someone else's transfer record.
func TestResellerTransferEventKeyCannotCrossTenants(t *testing.T) {
	truncateTables(t)

	const (
		resellerA = 9431
		customerA = 9432
		resellerB = 9433
		customerB = 9434
	)
	require.NoError(t, DB.Create(&User{Id: resellerA, Username: "evt_reseller_a", Group: "default", AffCode: "eva1", Quota: 10000}).Error)
	require.NoError(t, DB.Create(&User{Id: customerA, Username: "evt_customer_a", Group: "default", AffCode: "ecc1"}).Error)
	require.NoError(t, DB.Create(&User{Id: resellerB, Username: "evt_reseller_b", Group: "default", AffCode: "evb1", Quota: 10000}).Error)
	require.NoError(t, DB.Create(&User{Id: customerB, Username: "evt_customer_b", Group: "default", AffCode: "ecc2"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerA, Name: "a", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerB, Name: "b", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(customerA, resellerA, resellerA, "seed"))
	require.NoError(t, SetResellerCustomerOwner(customerB, resellerB, resellerB, "seed"))

	_, err := TransferResellerQuota(resellerA, customerA, resellerA, ResellerTransferFund, 5000, "evt-shared", "")
	require.NoError(t, err)

	_, err = TransferResellerQuota(resellerB, customerB, resellerB, ResellerTransferFund, 7000, "evt-shared", "")
	assert.ErrorIs(t, err, ErrResellerTransferInvalid, "another tenant's event key must not replay")

	quotaB, err := GetUserQuota(resellerB, true)
	require.NoError(t, err)
	assert.Equal(t, 10000, quotaB, "the refused replay moves no money")
}
