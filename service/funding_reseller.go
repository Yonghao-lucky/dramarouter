package service

import (
	"github.com/QuantumNous/new-api/model"
)

// BillingSourceReseller bills one customer request through a reseller-owned
// pool: the customer's allocation is consumed at retail and the reseller wallet
// is charged the wholesale amount computed from the frozen ratios.
const BillingSourceReseller = "reseller"

// ResellerFunding is the pool funding source. It is not a second wallet: every
// movement lands in the owner's normal users.quota wallet and in the append-only
// billing ledger.
type ResellerFunding struct {
	customerUserId int
	ownerId        int
	requestId      string
	retailRatio    float64
	costRatio      float64
	reservedRetail int64
}

func (r *ResellerFunding) Source() string { return BillingSourceReseller }

func (r *ResellerFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	_, ok, err := model.ReserveResellerAllocation(r.customerUserId, r.ownerId, r.requestId, int64(amount), r.retailRatio, r.costRatio)
	if err != nil {
		return err
	}
	if !ok {
		return model.ErrResellerAllocationExhausted
	}
	r.reservedRetail = int64(amount)
	return nil
}

// Extend grows the in-flight reservation, used by streaming reserve calls.
func (r *ResellerFunding) Extend(delta int) error {
	if delta <= 0 {
		return nil
	}
	if err := model.ExtendResellerAllocation(r.requestId, int64(delta), r.retailRatio, r.costRatio); err != nil {
		return err
	}
	r.reservedRetail += int64(delta)
	return nil
}

func (r *ResellerFunding) Settle(delta int) error {
	total := r.reservedRetail + int64(delta)
	if total < 0 {
		total = 0
	}
	return model.SettleResellerAllocation(r.requestId, total, r.retailRatio, r.costRatio)
}

func (r *ResellerFunding) Refund() error {
	return model.ReleaseResellerAllocation(r.requestId)
}
