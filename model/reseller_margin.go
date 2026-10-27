package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ResellerMarginCreditCursor is the per-owner watermark of margin that has been
// credited into the reseller wallet. The rollup reads snapshots with
// id > LastSnapshotId and only advances after the credit succeeds, so a crash
// re-runs the same range and the deterministic event key absorbs the retry.
type ResellerMarginCreditCursor struct {
	OwnerId        int   `json:"owner_id" gorm:"primaryKey;autoIncrement:false"`
	LastSnapshotId int64 `json:"last_snapshot_id" gorm:"bigint"`
	UpdatedAt      int64 `json:"updated_at" gorm:"bigint"`
}

// ResellerMarginCreditBatch is one page of snapshots awaiting credit.
type ResellerMarginCreditBatch struct {
	LastSnapshotId int64
	MarginQuota    int64
}

// ListResellerOwnersPendingMarginCredit returns owners that still have
// uncredited reseller snapshots after their cursor. The comparison uses each
// owner's own watermark, so it stays a single indexed range scan per owner
// rather than a full scan.
func ListResellerOwnersPendingMarginCredit(limit int) ([]int, error) {
	if limit <= 0 {
		limit = 200
	}
	owners := make([]int, 0, limit)
	err := DB.Model(&CostAccountingSnapshot{}).
		Joins("LEFT JOIN reseller_margin_credit_cursors c ON c.owner_id = cost_accounting_snapshots.owner_id").
		Where("cost_accounting_snapshots.owner_id > 0").
		Where("cost_accounting_snapshots.reseller_settlement = ?", "self").
		Where("cost_accounting_snapshots.id > COALESCE(c.last_snapshot_id, 0)").
		Distinct("cost_accounting_snapshots.owner_id").
		Order("cost_accounting_snapshots.owner_id asc").
		Limit(limit).
		Pluck("cost_accounting_snapshots.owner_id", &owners).Error
	return owners, err
}

// NextResellerMarginCreditBatch reads the next page of uncredited margin for one
// owner and reports the resulting watermark.
func NextResellerMarginCreditBatch(ownerId int, batchSize int) (ResellerMarginCreditBatch, error) {
	result := ResellerMarginCreditBatch{}
	if ownerId <= 0 {
		return result, errors.New("invalid reseller owner")
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	cursor := ResellerMarginCreditCursor{OwnerId: ownerId}
	err := DB.Where("owner_id = ?", ownerId).First(&cursor).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return result, err
	}

	rows := make([]CostAccountingSnapshot, 0, batchSize)
	err = DB.Select("id", "reseller_margin_quota").
		Where("owner_id = ? AND reseller_settlement = ? AND id > ?", ownerId, "self", cursor.LastSnapshotId).
		Order("id asc").
		Limit(batchSize).
		Find(&rows).Error
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		result.MarginQuota += row.ResellerMarginQuota
		result.LastSnapshotId = row.ID
	}
	return result, nil
}

// CreditResellerMargin posts the accumulated margin into the reseller wallet and
// advances the cursor. The entry key is derived from the exact watermark range,
// making a retried batch a no-op instead of a double credit.
func CreditResellerMargin(ownerId int, batch ResellerMarginCreditBatch) (bool, error) {
	if ownerId <= 0 || batch.LastSnapshotId <= 0 {
		return false, nil
	}
	eventKey := "reseller:margin:" + strconv.Itoa(ownerId) + ":" + strconv.FormatInt(batch.LastSnapshotId, 10)
	applied, err := PostBillingAdjustment(&BillingEntry{
		EventKey:    eventKey,
		UserID:      ownerId,
		Kind:        "funding",
		WalletDelta: batch.MarginQuota,
	}, nil)
	if err != nil {
		return false, err
	}
	now := common.GetTimestamp()
	err = DB.Transaction(func(tx *gorm.DB) error {
		var cursor ResellerMarginCreditCursor
		err := lockForUpdate(tx).Where("owner_id = ?", ownerId).First(&cursor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&ResellerMarginCreditCursor{OwnerId: ownerId, LastSnapshotId: batch.LastSnapshotId, UpdatedAt: now}).Error
		}
		if err != nil {
			return err
		}
		if cursor.LastSnapshotId >= batch.LastSnapshotId {
			return nil
		}
		cursor.LastSnapshotId = batch.LastSnapshotId
		cursor.UpdatedAt = now
		return tx.Model(&cursor).Updates(map[string]any{"last_snapshot_id": cursor.LastSnapshotId, "updated_at": now}).Error
	})
	return applied, err
}
