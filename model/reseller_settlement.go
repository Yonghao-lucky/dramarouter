package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Reseller settlement ledger.
//
// A settlement row is the operator's record of money moving between the
// platform and a reseller. It is deliberately separate from
// ResellerWalletGrant: the settlement describes what was received (bank
// transfer, amount, reference, proof) and the grant describes what was credited
// to the wallet. Keeping them apart means "we got paid" and "we added balance"
// can be reconciled against each other instead of being one indistinguishable
// action, and an operator can record an incoming payment before it is confirmed.
const (
	ResellerSettlementIn  = "in"  // reseller pays the platform
	ResellerSettlementOut = "out" // platform pays the reseller

	ResellerSettlementPending   = "pending"
	ResellerSettlementConfirmed = "confirmed"
	ResellerSettlementVoid      = "void"

	ResellerCurrencyCNY = "CNY"
	ResellerCurrencyUSD = "USD"
)

var (
	ErrResellerSettlementInvalid  = errors.New("invalid settlement")
	ErrResellerSettlementConflict = errors.New("settlement is not pending")
)

// ResellerSettlement is one ledger entry for money between the platform and a
// reseller. AmountMinor is the cash actually received, in the currency's minor
// unit (fen for CNY, cents for USD); QuotaAmount is the wallet quota credited
// when the settlement is confirmed. ExchangeRate is recorded only when the two
// currencies differ, so the original payment stays auditable after the wallet
// moves on.
type ResellerSettlement struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	EventKey     string `json:"event_key" gorm:"type:varchar(128);uniqueIndex"`
	ResellerId   int    `json:"reseller_id" gorm:"index:idx_reseller_settlement_owner,priority:1"`
	Direction    string `json:"direction" gorm:"type:varchar(16);index"`
	Status       string `json:"status" gorm:"type:varchar(16);index:idx_reseller_settlement_status,priority:1"`
	Currency     string `json:"currency" gorm:"type:varchar(8)"`
	AmountMinor  int64  `json:"amount_minor" gorm:"bigint"`
	ExchangeRate string `json:"exchange_rate" gorm:"type:varchar(32)"`
	QuotaAmount  int64  `json:"quota_amount" gorm:"bigint"`
	Kind         string `json:"kind" gorm:"type:varchar(16)"`
	Method       string `json:"method" gorm:"type:varchar(32)"`
	ExternalRef  string `json:"external_ref" gorm:"type:varchar(128);index"`
	ProofURL     string `json:"proof_url" gorm:"type:varchar(512)"`
	Remark       string `json:"remark" gorm:"type:varchar(255)"`
	GrantId      int64  `json:"grant_id" gorm:"bigint"`
	CreatedBy    int    `json:"created_by"`
	ConfirmedBy  int    `json:"confirmed_by"`
	ConfirmedAt  int64  `json:"confirmed_at" gorm:"bigint"`
	VoidedBy     int    `json:"voided_by"`
	VoidedAt     int64  `json:"voided_at" gorm:"bigint"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index:idx_reseller_settlement_time,priority:1"`
}

// Settlements are evidence. Confirming or voiding updates status, but a row is
// never deleted and its recorded amounts never change afterwards.
func (*ResellerSettlement) BeforeDelete(*gorm.DB) error { return ErrBillingEvidenceImmutable }

// NormalizeResellerSettlementCurrency validates the settlement currency,
// defaulting to the wallet currency.
func NormalizeResellerSettlementCurrency(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case ResellerCurrencyCNY:
		return ResellerCurrencyCNY
	case ResellerCurrencyUSD:
		return ResellerCurrencyUSD
	}
	return ResellerCurrencyUSD
}

type ResellerSettlementRequest struct {
	EventKey     string
	ResellerId   int
	Direction    string
	Currency     string
	AmountMinor  int64
	ExchangeRate string
	QuotaAmount  int64
	Kind         string
	Method       string
	ExternalRef  string
	ProofURL     string
	Remark       string
	ActorId      int
}

// CreateResellerSettlement records an unconfirmed payment. It moves no money;
// confirmation is a separate, explicit step.
func CreateResellerSettlement(req ResellerSettlementRequest) (*ResellerSettlement, error) {
	if req.ResellerId <= 0 || req.AmountMinor <= 0 || req.EventKey == "" {
		return nil, ErrResellerSettlementInvalid
	}
	if req.Direction != ResellerSettlementIn && req.Direction != ResellerSettlementOut {
		return nil, ErrResellerSettlementInvalid
	}
	if req.AmountMinor > int64(common.MaxWalletQuota) || req.QuotaAmount < 0 || req.QuotaAmount > int64(common.MaxWalletQuota) {
		return nil, ErrResellerSettlementInvalid
	}
	profile, err := GetResellerProfileByUser(req.ResellerId)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrResellerNotEnabled
	}
	kind, err := NormalizeResellerGrantKind(req.Kind)
	if err != nil {
		return nil, err
	}
	row := &ResellerSettlement{
		EventKey:     req.EventKey,
		ResellerId:   req.ResellerId,
		Direction:    req.Direction,
		Status:       ResellerSettlementPending,
		Currency:     NormalizeResellerSettlementCurrency(req.Currency),
		AmountMinor:  req.AmountMinor,
		ExchangeRate: strings.TrimSpace(req.ExchangeRate),
		QuotaAmount:  req.QuotaAmount,
		Kind:         kind,
		Method:       strings.TrimSpace(req.Method),
		ExternalRef:  strings.TrimSpace(req.ExternalRef),
		ProofURL:     strings.TrimSpace(req.ProofURL),
		Remark:       truncateResellerText(req.Remark, 255),
		CreatedBy:    req.ActorId,
		CreatedAt:    common.GetTimestamp(),
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing ResellerSettlement
		err := tx.Where("event_key = ?", row.EventKey).First(&existing).Error
		if err == nil {
			if existing.ResellerId != row.ResellerId {
				return ErrResellerGrantReplay
			}
			*row = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(row).Error
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// ConfirmResellerSettlement credits the reseller wallet and links the grant it
// produced. A settlement that was already confirmed returns the existing grant
// instead of paying twice.
func ConfirmResellerSettlement(settlementId int64, actorId int) (*ResellerSettlement, error) {
	if settlementId <= 0 {
		return nil, ErrResellerSettlementInvalid
	}
	var result ResellerSettlement
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row ResellerSettlement
		if err := lockForUpdate(tx).First(&row, settlementId).Error; err != nil {
			return err
		}
		if row.Status == ResellerSettlementConfirmed {
			result = row
			return nil
		}
		if row.Status != ResellerSettlementPending {
			return ErrResellerSettlementConflict
		}
		if row.QuotaAmount <= 0 {
			return ErrResellerSettlementInvalid
		}
		grant, err := grantResellerWalletTx(tx, row.ResellerId, actorId, row.QuotaAmount, row.Kind,
			"settlement:"+row.EventKey, row.EventKey+":grant")
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(&ResellerSettlement{}).Where("id = ?", row.Id).Updates(map[string]any{
			"status":       ResellerSettlementConfirmed,
			"grant_id":     grant.Id,
			"confirmed_by": actorId,
			"confirmed_at": now,
		}).Error; err != nil {
			return err
		}
		row.Status, row.GrantId, row.ConfirmedBy, row.ConfirmedAt = ResellerSettlementConfirmed, grant.Id, actorId, now
		result = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = invalidateUserCache(result.ResellerId)
	return &result, nil
}

// VoidResellerSettlement cancels a pending settlement. A confirmed settlement
// cannot be voided because its money already moved; reverse it with a new
// outbound settlement and an explicit grant instead.
func VoidResellerSettlement(settlementId int64, actorId int) (*ResellerSettlement, error) {
	if settlementId <= 0 {
		return nil, ErrResellerSettlementInvalid
	}
	var result ResellerSettlement
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row ResellerSettlement
		if err := lockForUpdate(tx).First(&row, settlementId).Error; err != nil {
			return err
		}
		if row.Status == ResellerSettlementVoid {
			result = row
			return nil
		}
		if row.Status != ResellerSettlementPending {
			return ErrResellerSettlementConflict
		}
		now := common.GetTimestamp()
		if err := tx.Model(&ResellerSettlement{}).Where("id = ?", row.Id).Updates(map[string]any{
			"status":    ResellerSettlementVoid,
			"voided_by": actorId,
			"voided_at": now,
		}).Error; err != nil {
			return err
		}
		row.Status, row.VoidedBy, row.VoidedAt = ResellerSettlementVoid, actorId, now
		result = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListResellerSettlements pages the ledger, newest first. An empty status
// returns every status.
func ListResellerSettlements(resellerId int, status string, limit int) ([]ResellerSettlement, error) {
	if resellerId <= 0 {
		return nil, ErrResellerSettlementInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerSettlement, 0, limit)
	query := DB.Where("reseller_id = ?", resellerId)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("id desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ResellerSettlementTotals reconciles cash received against quota credited.
// The difference is what an operator must still explain.
type ResellerSettlementTotals struct {
	ReceivedMinor  int64 `json:"received_minor"`
	ConfirmedMinor int64 `json:"confirmed_minor"`
	PendingMinor   int64 `json:"pending_minor"`
	CreditedQuota  int64 `json:"credited_quota"`
	PendingCount   int64 `json:"pending_count"`
}

func GetResellerSettlementTotals(resellerId int) (*ResellerSettlementTotals, error) {
	if resellerId <= 0 {
		return nil, ErrResellerSettlementInvalid
	}
	totals := &ResellerSettlementTotals{}
	var row struct {
		Received  int64 `gorm:"column:received"`
		Confirmed int64 `gorm:"column:confirmed"`
		Pending   int64 `gorm:"column:pending"`
		Credited  int64 `gorm:"column:credited"`
		Count     int64 `gorm:"column:pending_count"`
	}
	err := DB.Model(&ResellerSettlement{}).
		Where("reseller_id = ? AND status <> ?", resellerId, ResellerSettlementVoid).
		Select(`COALESCE(SUM(amount_minor),0) AS received,
			COALESCE(SUM(CASE WHEN status = 'confirmed' THEN amount_minor ELSE 0 END),0) AS confirmed,
			COALESCE(SUM(CASE WHEN status = 'pending' THEN amount_minor ELSE 0 END),0) AS pending,
			COALESCE(SUM(CASE WHEN status = 'confirmed' THEN quota_amount ELSE 0 END),0) AS credited,
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END),0) AS pending_count`).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	totals.ReceivedMinor, totals.ConfirmedMinor = row.Received, row.Confirmed
	totals.PendingMinor, totals.CreditedQuota, totals.PendingCount = row.Pending, row.Credited, row.Count
	return totals, nil
}

// ListResellerSettlementsForExport returns every settlement of one reseller in
// ascending order for a statement export.
func ListResellerSettlementsForExport(resellerId int) ([]ResellerSettlement, error) {
	if resellerId <= 0 {
		return nil, ErrResellerSettlementInvalid
	}
	rows := make([]ResellerSettlement, 0, 256)
	err := DB.Where("reseller_id = ?", resellerId).Order("id asc").Find(&rows).Error
	return rows, err
}

func truncateResellerText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
