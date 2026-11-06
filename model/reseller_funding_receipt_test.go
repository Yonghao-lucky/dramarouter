package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPrecreditOwner creates a reseller with one owned customer and a funded
// wallet, so a precredit transfer can move quota.
func seedPrecreditOwner(t *testing.T) (resellerID, customerID int) {
	t.Helper()
	const (
		ownerID    = 9301
		resellerId = 9302
	)
	require.NoError(t, DB.Create(&User{Id: ownerID, Username: "receipt_customer", Group: "default", AffCode: "rrc1"}).Error)
	require.NoError(t, DB.Create(&User{Id: resellerId, Username: "receipt_reseller", Group: "default", AffCode: "rrr1"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: resellerId, Name: "r", Status: ResellerStatusEnabled}).Error)
	require.NoError(t, SetResellerCustomerOwner(ownerID, resellerId, 1, "test"))

	// The reseller must hold quota to fund from.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", resellerId).Update("quota", 100000).Error)
	return resellerId, ownerID
}

func precredit(t *testing.T, resellerID, customerID int, quota int64, eventKey string) *ResellerTransfer {
	t.Helper()
	transfer, err := TransferResellerQuota(resellerID, customerID, resellerID,
		ResellerTransferFund, quota, eventKey, ResellerFundingPrecredit, "")
	require.NoError(t, err)
	return transfer
}

// TestFundingKindDefaultsToRecharge pins that an unlabelled transfer is money
// already collected, not credit — the safe default when a caller forgets.
func TestFundingKindDefaultsToRecharge(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	transfer, err := TransferResellerQuota(resellerID, customerID, resellerID,
		ResellerTransferFund, 1000, "evt-kind-default", "", "")
	require.NoError(t, err)
	assert.Equal(t, ResellerFundingRecharge, transfer.Kind)
}

// TestCreateFundingReceiptMergesPrecredits pins the operator's batch case: one
// payment covers several precredits, and the receipt records the sum plus the
// statement reference so it can be matched line by line.
func TestCreateFundingReceiptMergesPrecredits(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	first := precredit(t, resellerID, customerID, 3000, "evt-pc-1")
	second := precredit(t, resellerID, customerID, 7000, "evt-pc-2")

	receipt, err := CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-1",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{first.Id, second.Id},
		Method:         "bank",
		ExternalRef:    "流水-20261107-001",
		ActorId:        resellerID,
		Confirm:        true,
	})
	require.NoError(t, err)

	assert.Equal(t, ResellerReceiptConfirmed, receipt.Status)
	assert.Equal(t, int64(10000), receipt.QuotaTotal, "the receipt sums the covered transfers")
	assert.Equal(t, 2, receipt.TransferCount)
	assert.Equal(t, "流水-20261107-001", receipt.ExternalRef)

	// Both transfers are now settled, so neither is offered again.
	open, err := ListUnreconciledPrecredits(resellerID, customerID, 50)
	require.NoError(t, err)
	assert.Empty(t, open)
}

// TestPrecreditCannotBeReconciledTwice protects the accounts: the same transfer
// must not be counted against two payments.
func TestPrecreditCannotBeReconciledTwice(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	transfer := precredit(t, resellerID, customerID, 5000, "evt-pc-once")
	_, err := CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-a",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{transfer.Id},
		Method:         "bank",
		ActorId:        resellerID,
		Confirm:        true,
	})
	require.NoError(t, err)

	_, err = CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-b",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{transfer.Id},
		Method:         "alipay",
		ActorId:        resellerID,
		Confirm:        true,
	})
	assert.ErrorIs(t, err, ErrResellerReceiptTransfer)
}

// TestRechargeIsNotReconcilable pins the boundary: a recharge is already paid
// for, so listing it as an outstanding precredit would invent a debt.
func TestRechargeIsNotReconcilable(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	recharge, err := TransferResellerQuota(resellerID, customerID, resellerID,
		ResellerTransferFund, 4000, "evt-plain-recharge", ResellerFundingRecharge, "")
	require.NoError(t, err)

	open, err := ListUnreconciledPrecredits(resellerID, customerID, 50)
	require.NoError(t, err)
	assert.Empty(t, open, "only precredits await a receipt")

	_, err = CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-recharge",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{recharge.Id},
		Method:         "bank",
		ActorId:        resellerID,
		Confirm:        true,
	})
	assert.ErrorIs(t, err, ErrResellerReceiptTransfer)
}

// TestVoidReleasesPrecredits pins that a wrong receipt can be undone and its
// transfers re-batched, which is the whole point of a two-step status.
func TestVoidReleasesPrecredits(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	transfer := precredit(t, resellerID, customerID, 2000, "evt-pc-void")
	receipt, err := CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-void",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{transfer.Id},
		Method:         "bank",
		ActorId:        resellerID,
	})
	require.NoError(t, err)
	require.Equal(t, ResellerReceiptPending, receipt.Status)

	voided, err := VoidResellerFundingReceipt(receipt.Id, resellerID)
	require.NoError(t, err)
	assert.Equal(t, ResellerReceiptVoid, voided.Status)

	open, err := ListUnreconciledPrecredits(resellerID, customerID, 50)
	require.NoError(t, err)
	require.Len(t, open, 1, "voiding releases the transfer for a corrected receipt")

	_, err = CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-retry",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{transfer.Id},
		Method:         "bank",
		ExternalRef:    "corrected",
		ActorId:        resellerID,
		Confirm:        true,
	})
	require.NoError(t, err)
}

// TestConfirmedReceiptCannotBeVoided pins that money already matched to a
// statement is not silently un-matched; an operator records a corrective
// transfer instead.
func TestConfirmedReceiptCannotBeVoided(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	transfer := precredit(t, resellerID, customerID, 1500, "evt-pc-confirmed")
	receipt, err := CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-confirmed",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{transfer.Id},
		Method:         "bank",
		ActorId:        resellerID,
		Confirm:        true,
	})
	require.NoError(t, err)

	_, err = VoidResellerFundingReceipt(receipt.Id, resellerID)
	assert.ErrorIs(t, err, ErrResellerReceiptConflict)
}

// TestReceiptIsScopedToItsOwner pins that another reseller cannot read or act
// on a receipt by guessing its id.
func TestReceiptIsScopedToItsOwner(t *testing.T) {
	truncateTables(t)
	resellerID, customerID := seedPrecreditOwner(t)

	transfer := precredit(t, resellerID, customerID, 1200, "evt-pc-scope")
	receipt, err := CreateResellerFundingReceipt(ResellerFundingReceiptRequest{
		EventKey:       "evt-receipt-scope",
		ResellerId:     resellerID,
		CustomerUserId: customerID,
		TransferIds:    []int64{transfer.Id},
		Method:         "bank",
		ActorId:        resellerID,
	})
	require.NoError(t, err)

	foreign, err := ResellerFundingReceiptForReseller(receipt.Id, resellerID+999)
	require.NoError(t, err)
	assert.Nil(t, foreign)

	own, err := ResellerFundingReceiptForReseller(receipt.Id, resellerID)
	require.NoError(t, err)
	require.NotNil(t, own)
	assert.Equal(t, receipt.Id, own.Id)
}
