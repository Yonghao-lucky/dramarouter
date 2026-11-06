package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Precredit reconciliation.
//
// A precredit funding (授信) credits the customer wallet before the reseller has
// actually been paid. The transfer row itself is immutable evidence, so
// "this batch of precredits was paid for" cannot be a column on the transfer.
// It is recorded separately: one receipt groups any number of precredit
// transfers and carries the payment method and the bank/invoice reference an
// operator needs to match against a statement.
const (
	ResellerReceiptPending   = "pending"
	ResellerReceiptConfirmed = "confirmed"
	ResellerReceiptVoid      = "void"
)

var (
	ErrResellerReceiptInvalid  = errors.New("invalid funding receipt")
	ErrResellerReceiptConflict = errors.New("funding receipt is not pending")
	ErrResellerReceiptTransfer = errors.New("transfer cannot be reconciled")
)

// ResellerFundingReceipt is one reconciliation record covering one or more
// precredit transfers that the same payment settled.
type ResellerFundingReceipt struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	EventKey       string `json:"event_key" gorm:"type:varchar(128);uniqueIndex"`
	ResellerId     int    `json:"reseller_id" gorm:"index:idx_reseller_receipt_owner,priority:1"`
	CustomerUserId int    `json:"customer_user_id" gorm:"index:idx_reseller_receipt_customer,priority:1"`
	Status         string `json:"status" gorm:"type:varchar(16);index:idx_reseller_receipt_status,priority:1"`
	Method         string `json:"method" gorm:"type:varchar(32)"`
	ExternalRef    string `json:"external_ref" gorm:"type:varchar(128);index"`
	Remark         string `json:"remark" gorm:"type:varchar(255)"`
	QuotaTotal     int64  `json:"quota_total" gorm:"bigint"`
	TransferCount  int    `json:"transfer_count"`
	CreatedBy      int    `json:"created_by"`
	ConfirmedBy    int    `json:"confirmed_by"`
	ConfirmedAt    int64  `json:"confirmed_at" gorm:"bigint"`
	VoidedBy       int    `json:"voided_by"`
	VoidedAt       int64  `json:"voided_at" gorm:"bigint"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index:idx_reseller_receipt_time,priority:1"`
}

// Receipts are evidence: they are never deleted, and a covered transfer cannot
// silently move to another receipt.
func (*ResellerFundingReceipt) BeforeDelete(*gorm.DB) error { return ErrBillingEvidenceImmutable }

// ResellerFundingReceiptItem links one precredit transfer to the receipt that
// settled it. TransferId is a plain index rather than a unique one because a
// voided receipt releases its transfers for a corrected re-batch; "at most one
// live receipt per transfer" is enforced inside the creating transaction, which
// locks the transfer rows first.
type ResellerFundingReceiptItem struct {
	Id         int64 `json:"id" gorm:"primaryKey"`
	ReceiptId  int64 `json:"receipt_id" gorm:"index:idx_reseller_receipt_item_receipt,priority:1"`
	TransferId int64 `json:"transfer_id" gorm:"index"`
	CreatedAt  int64 `json:"created_at" gorm:"bigint"`
}

func (*ResellerFundingReceiptItem) BeforeDelete(*gorm.DB) error { return ErrBillingEvidenceImmutable }

type ResellerFundingReceiptRequest struct {
	EventKey       string
	ResellerId     int
	CustomerUserId int
	TransferIds    []int64
	Method         string
	ExternalRef    string
	Remark         string
	ActorId        int
	// Confirm records the payment as already received. When false the receipt
	// stays pending so an operator can attach the statement later.
	Confirm bool
}

// CreateResellerFundingReceipt reconciles a batch of precredit transfers into
// one receipt. Every selected transfer must belong to the same reseller and
// customer, be a funding movement, be a precredit, and not already be covered
// by another live receipt — anything else would let one payment settle another
// tenant's credit.
func CreateResellerFundingReceipt(req ResellerFundingReceiptRequest) (*ResellerFundingReceipt, error) {
	if req.ResellerId <= 0 || req.CustomerUserId <= 0 || len(req.TransferIds) == 0 {
		return nil, ErrResellerReceiptInvalid
	}
	if req.EventKey == "" {
		return nil, ErrResellerReceiptInvalid
	}
	ids := make([]int64, 0, len(req.TransferIds))
	seen := make(map[int64]struct{}, len(req.TransferIds))
	for _, id := range req.TransferIds {
		if id <= 0 {
			return nil, ErrResellerReceiptInvalid
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, ErrResellerReceiptInvalid
	}

	receipt := &ResellerFundingReceipt{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing ResellerFundingReceipt
		err := tx.Where("event_key = ?", req.EventKey).First(&existing).Error
		if err == nil {
			if existing.ResellerId != req.ResellerId {
				return ErrResellerGrantReplay
			}
			*receipt = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		transfers := make([]ResellerTransfer, 0, len(ids))
		if err := lockForUpdate(tx).Where("id IN ?", ids).Find(&transfers).Error; err != nil {
			return err
		}
		if len(transfers) != len(ids) {
			return ErrResellerReceiptTransfer
		}

		var total int64
		for i := range transfers {
			transfer := &transfers[i]
			if transfer.ResellerId != req.ResellerId || transfer.CustomerUserId != req.CustomerUserId {
				return ErrResellerReceiptTransfer
			}
			if transfer.Direction != ResellerTransferFund {
				return ErrResellerReceiptTransfer
			}
			if NormalizeResellerFundingKind(transfer.Kind) != ResellerFundingPrecredit {
				return ErrResellerReceiptTransfer
			}
			total += transfer.RetailQuota
		}

		// A transfer can be covered by at most one live receipt. Voided receipts
		// release their transfers so a corrected statement can re-cover them.
		var taken int64
		err = tx.Model(&ResellerFundingReceiptItem{}).
			Joins("JOIN reseller_funding_receipts ON reseller_funding_receipts.id = reseller_funding_receipt_items.receipt_id").
			Where("reseller_funding_receipt_items.transfer_id IN ? AND reseller_funding_receipts.status <> ?", ids, ResellerReceiptVoid).
			Count(&taken).Error
		if err != nil {
			return err
		}
		if taken > 0 {
			return ErrResellerReceiptTransfer
		}

		now := common.GetTimestamp()
		receipt.EventKey = req.EventKey
		receipt.ResellerId = req.ResellerId
		receipt.CustomerUserId = req.CustomerUserId
		receipt.Status = ResellerReceiptPending
		receipt.Method = truncateResellerText(req.Method, 32)
		receipt.ExternalRef = truncateResellerText(req.ExternalRef, 128)
		receipt.Remark = truncateResellerText(req.Remark, 255)
		receipt.QuotaTotal = total
		receipt.TransferCount = len(transfers)
		receipt.CreatedBy = req.ActorId
		receipt.CreatedAt = now
		if req.Confirm {
			receipt.Status = ResellerReceiptConfirmed
			receipt.ConfirmedBy = req.ActorId
			receipt.ConfirmedAt = now
		}
		if err := tx.Create(receipt).Error; err != nil {
			return err
		}

		items := make([]ResellerFundingReceiptItem, 0, len(transfers))
		for _, transfer := range transfers {
			items = append(items, ResellerFundingReceiptItem{
				ReceiptId:  receipt.Id,
				TransferId: transfer.Id,
				CreatedAt:  now,
			})
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

// ConfirmResellerFundingReceipt marks a pending receipt as paid. Confirming
// twice is a no-op so a double click cannot double-count the payment.
func ConfirmResellerFundingReceipt(receiptId int64, actorId int) (*ResellerFundingReceipt, error) {
	if receiptId <= 0 {
		return nil, ErrResellerReceiptInvalid
	}
	var result ResellerFundingReceipt
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row ResellerFundingReceipt
		if err := lockForUpdate(tx).First(&row, receiptId).Error; err != nil {
			return err
		}
		if row.Status == ResellerReceiptConfirmed {
			result = row
			return nil
		}
		if row.Status != ResellerReceiptPending {
			return ErrResellerReceiptConflict
		}
		now := common.GetTimestamp()
		if err := tx.Model(&ResellerFundingReceipt{}).Where("id = ?", row.Id).Updates(map[string]any{
			"status":       ResellerReceiptConfirmed,
			"confirmed_by": actorId,
			"confirmed_at": now,
		}).Error; err != nil {
			return err
		}
		row.Status, row.ConfirmedBy, row.ConfirmedAt = ResellerReceiptConfirmed, actorId, now
		result = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// VoidResellerFundingReceipt releases a pending receipt. A confirmed receipt
// already settled money, so voiding it is refused; record a corrective
// transfer instead.
func VoidResellerFundingReceipt(receiptId int64, actorId int) (*ResellerFundingReceipt, error) {
	if receiptId <= 0 {
		return nil, ErrResellerReceiptInvalid
	}
	var result ResellerFundingReceipt
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row ResellerFundingReceipt
		if err := lockForUpdate(tx).First(&row, receiptId).Error; err != nil {
			return err
		}
		if row.Status == ResellerReceiptVoid {
			result = row
			return nil
		}
		if row.Status != ResellerReceiptPending {
			return ErrResellerReceiptConflict
		}
		now := common.GetTimestamp()
		if err := tx.Model(&ResellerFundingReceipt{}).Where("id = ?", row.Id).Updates(map[string]any{
			"status":    ResellerReceiptVoid,
			"voided_by": actorId,
			"voided_at": now,
		}).Error; err != nil {
			return err
		}
		row.Status, row.VoidedBy, row.VoidedAt = ResellerReceiptVoid, actorId, now
		result = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ResellerFundingReceiptForReseller loads one receipt scoped to its owner.
// Returning nil for a foreign receipt keeps "not found" and "not yours"
// indistinguishable, so a console user cannot probe other resellers' ids.
func ResellerFundingReceiptForReseller(receiptId int64, resellerId int) (*ResellerFundingReceipt, error) {
	if receiptId <= 0 || resellerId <= 0 {
		return nil, ErrResellerReceiptInvalid
	}
	var row ResellerFundingReceipt
	err := DB.Where("id = ? AND reseller_id = ?", receiptId, resellerId).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListResellerFundingReceipts pages receipts newest first for one reseller,
// optionally narrowed to a single customer.
func ListResellerFundingReceipts(resellerId, customerUserId int, limit int) ([]ResellerFundingReceipt, error) {
	if resellerId <= 0 {
		return nil, ErrResellerReceiptInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerFundingReceipt, 0, limit)
	query := DB.Where("reseller_id = ?", resellerId)
	if customerUserId > 0 {
		query = query.Where("customer_user_id = ?", customerUserId)
	}
	err := query.Order("id desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ListUnreconciledPrecredits returns the precredit transfers of one customer
// that no live receipt has covered yet — the candidates an operator picks from
// when consolidating a batch.
func ListUnreconciledPrecredits(resellerId, customerUserId int, limit int) ([]ResellerTransfer, error) {
	if resellerId <= 0 || customerUserId <= 0 {
		return nil, ErrResellerReceiptInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows := make([]ResellerTransfer, 0, limit)
	err := DB.Where(
		"reseller_id = ? AND customer_user_id = ? AND direction = ? AND kind = ?",
		resellerId, customerUserId, ResellerTransferFund, ResellerFundingPrecredit,
	).
		Where("id NOT IN (?)",
			DB.Model(&ResellerFundingReceiptItem{}).
				Select("reseller_funding_receipt_items.transfer_id").
				Joins("JOIN reseller_funding_receipts ON reseller_funding_receipts.id = reseller_funding_receipt_items.receipt_id").
				Where("reseller_funding_receipts.status <> ?", ResellerReceiptVoid),
		).
		Order("id desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}
