package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Reseller wallet grant kinds. A recharge is money the reseller actually paid
// the platform, a precredit is credit the platform granted ahead of payment.
// Both land in the reseller wallet, but they are accounted separately so the
// administrator can see how much of a reseller's balance is unpaid credit.
const (
	ResellerGrantRecharge  = "recharge"
	ResellerGrantPrecredit = "precredit"
)

var (
	ErrResellerGrantInvalid  = errors.New("invalid reseller grant")
	ErrResellerGrantReplay   = errors.New("this event key belongs to another reseller's grant")
	ErrResellerGrantConflict = errors.New("this event key was already used with a different amount or type")
)

// ResellerWalletGrant is one administrator top-up of a reseller wallet. It is
// append-only evidence: the wallet movement it caused lives in the billing
// ledger, and this row explains why it happened.
type ResellerWalletGrant struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	EventKey   string `json:"event_key" gorm:"type:varchar(128);uniqueIndex"`
	ResellerId int    `json:"reseller_id" gorm:"index:idx_reseller_grant_owner,priority:1"`
	Amount     int64  `json:"amount" gorm:"bigint"`
	Kind       string `json:"kind" gorm:"type:varchar(16);index"`
	Remark     string `json:"remark" gorm:"type:varchar(255)"`
	OperatorId int    `json:"operator_id"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index:idx_reseller_grant_time,priority:1"`
}

func (*ResellerWalletGrant) BeforeUpdate(*gorm.DB) error { return ErrBillingEvidenceImmutable }
func (*ResellerWalletGrant) BeforeDelete(*gorm.DB) error { return ErrBillingEvidenceImmutable }

// NormalizeResellerGrantKind validates the grant type, defaulting to a plain
// recharge because that is what an unlabelled top-up actually is.
func NormalizeResellerGrantKind(kind string) (string, error) {
	switch strings.TrimSpace(kind) {
	case "":
		return ResellerGrantRecharge, nil
	case ResellerGrantRecharge:
		return ResellerGrantRecharge, nil
	case ResellerGrantPrecredit:
		return ResellerGrantPrecredit, nil
	}
	return "", ErrResellerGrantInvalid
}

// GrantResellerWallet adds quota to a reseller wallet and records why. The
// wallet movement and the grant row commit together, so a failure never leaves
// a grant without money or money without a grant. The event key is caller-owned
// and idempotent: replaying it returns the original grant instead of paying
// twice.
func GrantResellerWallet(resellerId, operatorId int, amount int64, kind, remark, eventKey string) (*ResellerWalletGrant, error) {
	if resellerId <= 0 || amount <= 0 || amount > int64(common.MaxWalletQuota) || eventKey == "" {
		return nil, ErrResellerGrantInvalid
	}
	normalized, err := NormalizeResellerGrantKind(kind)
	if err != nil {
		return nil, err
	}
	profile, err := GetResellerProfileByUser(resellerId)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrResellerNotEnabled
	}
	// A disabled profile may still be topped up: suspension freezes selling, not
	// the administrator's ability to settle or pre-fund the account.
	remark = truncateResellerText(remark, 255)

	grant := &ResellerWalletGrant{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing ResellerWalletGrant
		err := tx.Where("event_key = ?", eventKey).First(&existing).Error
		if err == nil {
			if existing.ResellerId != resellerId {
				return ErrResellerGrantReplay
			}
			if existing.Amount != amount || existing.Kind != normalized {
				return ErrResellerGrantConflict
			}
			*grant = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		granted, err := grantResellerWalletTx(tx, resellerId, operatorId, amount, normalized, remark, eventKey)
		if err != nil {
			return err
		}
		*grant = *granted
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = invalidateUserCache(resellerId)
	return grant, nil
}

// grantResellerWalletTx credits a reseller wallet inside an existing
// transaction. It performs no idempotency lookup: callers that need replay
// protection resolve their own event key first, so a settlement confirmation
// can share one transaction with the grant it produces.
func grantResellerWalletTx(tx *gorm.DB, resellerId, operatorId int, amount int64, kind, remark, eventKey string) (*ResellerWalletGrant, error) {
	if resellerId <= 0 || amount <= 0 || amount > int64(common.MaxWalletQuota) || eventKey == "" {
		return nil, ErrResellerGrantInvalid
	}
	normalized, err := NormalizeResellerGrantKind(kind)
	if err != nil {
		return nil, err
	}
	var user User
	if err := lockForUpdate(tx).First(&user, resellerId).Error; err != nil {
		return nil, err
	}
	if err := changeBillingWallet(tx, &user, amount); err != nil {
		return nil, err
	}
	entry := BillingEntry{
		EventKey:    "reseller:grant:" + strconv.Itoa(resellerId) + ":" + eventKey,
		Kind:        "funding",
		WalletDelta: amount,
		ActorID:     operatorId,
	}
	if err := appendBillingEntry(tx, &user, &entry); err != nil {
		return nil, err
	}
	grant := &ResellerWalletGrant{
		EventKey:   eventKey,
		ResellerId: resellerId,
		Amount:     amount,
		Kind:       normalized,
		Remark:     truncateResellerText(remark, 255),
		OperatorId: operatorId,
		CreatedAt:  common.GetTimestamp(),
	}
	if err := tx.Create(grant).Error; err != nil {
		return nil, err
	}
	return grant, nil
}

// ListResellerWalletGrants pages one reseller's top-up history, newest first.
func ListResellerWalletGrants(resellerId int, limit int) ([]ResellerWalletGrant, error) {
	if resellerId <= 0 {
		return nil, ErrResellerGrantInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerWalletGrant, 0, limit)
	err := DB.Where("reseller_id = ?", resellerId).Order("id desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ResellerGrantTotals is how much an administrator put into one reseller wallet,
// split by whether the reseller paid for it.
type ResellerGrantTotals struct {
	Recharge  int64 `json:"granted_recharge_quota"`
	Precredit int64 `json:"granted_precredit_quota"`
}
