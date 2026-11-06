package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Reseller status values. A reseller profile is a capability layer bound to a
// normal platform user; it never changes the user's role or group.
const (
	ResellerStatusDisabled = 1
	ResellerStatusEnabled  = 2
)

// Reseller transfer directions. A fund moves retail quota into a customer
// wallet and charges the reseller wallet the wholesale amount; a reclaim moves
// unused retail quota back and refunds the reseller proportionally.
const (
	ResellerTransferFund    = "fund"
	ResellerTransferReclaim = "reclaim"
)

// Funding kinds. A recharge is money the reseller has already paid for; a
// precredit (授信) is quota handed to the customer on credit, which the reseller
// settles against a bank statement later. The kind only classifies a transfer,
// it never changes the 1:1 movement.
const (
	ResellerFundingRecharge  = "recharge"
	ResellerFundingPrecredit = "precredit"
)

// NormalizeResellerFundingKind validates a funding kind, defaulting to recharge
// because an unlabelled transfer is by far the common case.
func NormalizeResellerFundingKind(kind string) string {
	if strings.TrimSpace(kind) == ResellerFundingPrecredit {
		return ResellerFundingPrecredit
	}
	return ResellerFundingRecharge
}

var (
	ErrResellerNotEnabled       = errors.New("not an enabled reseller")
	ErrResellerCustomerNotFound = errors.New("customer is not owned by this reseller")
	ErrResellerGroupNotAllowed  = errors.New("group is not authorized for this reseller")
	ErrResellerPriceBelowCost   = errors.New("customer price cannot be lower than the reseller cost")
	ErrResellerWalletShort      = errors.New("reseller wallet is insufficient")
	ErrResellerCustomerShort    = errors.New("customer wallet is insufficient")
	ErrResellerTransferInvalid  = errors.New("invalid reseller transfer")
	// A precredit already matched to a received payment must be un-matched
	// before it can be voided, otherwise the statement keeps pointing at quota
	// that was taken back.
	ErrResellerTransferReconciled = errors.New("transfer is already reconciled")
)

// ResellerProfile binds a reseller capability to an existing platform user.
// UserId is the reseller's own account and it is the single owner key used by
// every binding, cost row, price row and accounting snapshot. The profile Id is
// a storage detail and must never be used as an owner reference.
type ResellerProfile struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex"`
	Name      string `json:"name" gorm:"type:varchar(64)"`
	Code      string `json:"code" gorm:"type:varchar(64);index"`
	Status    int    `json:"status" gorm:"type:int;index"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

// ResellerGroupCost is the wholesale ratio the platform charges a reseller for
// a billing group. Stored as a decimal string to avoid float drift. It also
// defines which groups a reseller may resell at all.
type ResellerGroupCost struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	ResellerId int    `json:"reseller_id" gorm:"uniqueIndex:idx_reseller_group_cost,priority:1"`
	GroupName  string `json:"group" gorm:"type:varchar(64);uniqueIndex:idx_reseller_group_cost,priority:2"`
	CostRatio  string `json:"cost_ratio" gorm:"type:varchar(64)"`
	Enabled    bool   `json:"enabled"`
	UpdatedBy  int    `json:"updated_by"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

// ResellerDefaultPrice is the reseller's standing retail ratio per group. It
// applies to every owned customer that has no customer-specific price.
type ResellerDefaultPrice struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	ResellerId int    `json:"reseller_id" gorm:"uniqueIndex:idx_reseller_default_price,priority:1"`
	GroupName  string `json:"group" gorm:"type:varchar(64);uniqueIndex:idx_reseller_default_price,priority:2"`
	Ratio      string `json:"ratio" gorm:"type:varchar(64)"`
	UpdatedBy  int    `json:"updated_by"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

// CustomerGroupPrice is a per-customer override for one billing group. It wins
// over the reseller default. A reseller may set it, but never below its own
// wholesale cost for that group.
type CustomerGroupPrice struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex:idx_customer_group_price,priority:1"`
	GroupName string `json:"group" gorm:"type:varchar(64);uniqueIndex:idx_customer_group_price,priority:2"`
	Ratio     string `json:"ratio" gorm:"type:varchar(64)"`
	Source    string `json:"source" gorm:"type:varchar(16)"` // admin | reseller
	UpdatedBy int    `json:"updated_by"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

// ResellerCustomerBinding records the current owner of a customer. ResellerId
// is the reseller's user id; 0 means platform-direct ownership. Ownership
// changes are append-only in ResellerCustomerEvent.
type ResellerCustomerBinding struct {
	CustomerUserId int   `json:"customer_user_id" gorm:"primaryKey;autoIncrement:false"`
	ResellerId     int   `json:"reseller_id" gorm:"index"`
	EffectiveFrom  int64 `json:"effective_from" gorm:"bigint"`
	UpdatedBy      int   `json:"updated_by"`
	UpdatedAt      int64 `json:"updated_at" gorm:"bigint"`
}

// ResellerCustomerEvent is the immutable ownership history.
type ResellerCustomerEvent struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	CustomerUserId int    `json:"customer_user_id" gorm:"index"`
	FromResellerId int    `json:"from_reseller_id"`
	ToResellerId   int    `json:"to_reseller_id"`
	EffectiveAt    int64  `json:"effective_at" gorm:"bigint"`
	ActorId        int    `json:"actor_id"`
	Reason         string `json:"reason" gorm:"type:varchar(500)"`
}

func (*ResellerCustomerEvent) BeforeUpdate(*gorm.DB) error { return ErrBillingEvidenceImmutable }
func (*ResellerCustomerEvent) BeforeDelete(*gorm.DB) error { return ErrBillingEvidenceImmutable }

// ResellerTransfer is the append-only wallet transfer ledger. RetailQuota is
// the quota credited to (fund) or debited from (reclaim) the customer wallet;
// CostQuota is the money that actually moved between the two wallets.
type ResellerTransfer struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	EventKey       string `json:"event_key" gorm:"type:varchar(128);uniqueIndex"`
	ResellerId     int    `json:"reseller_id" gorm:"index:idx_reseller_transfer_owner,priority:1"`
	CustomerUserId int    `json:"customer_user_id" gorm:"index:idx_reseller_transfer_customer,priority:1"`
	Direction      string `json:"direction" gorm:"type:varchar(16);index"`
	GroupName      string `json:"group" gorm:"type:varchar(64)"`
	RetailQuota    int64  `json:"retail_quota" gorm:"bigint"`
	CostQuota      int64  `json:"cost_quota" gorm:"bigint"`
	RetailRatio    string `json:"retail_ratio" gorm:"type:varchar(64)"`
	CostRatio      string `json:"cost_ratio" gorm:"type:varchar(64)"`
	OperatorId     int    `json:"operator_id"`
	// Kind classifies funding as a paid recharge or a precredit awaiting
	// settlement. Reclaims carry the kind of the funding they reverse.
	Kind string `json:"kind" gorm:"type:varchar(16);index"`
	// ReversesId points at the transfer this row undoes. A void is not an
	// in-place edit — transfers are append-only evidence — so undoing a wrong
	// amount appends a compensating reclaim carrying the original id here.
	ReversesId int64  `json:"reverses_id" gorm:"bigint;index"`
	Remark     string `json:"remark" gorm:"type:varchar(255)"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index:idx_reseller_transfer_time,priority:1"`
}

func (*ResellerTransfer) BeforeUpdate(*gorm.DB) error { return ErrBillingEvidenceImmutable }
func (*ResellerTransfer) BeforeDelete(*gorm.DB) error { return ErrBillingEvidenceImmutable }

// GetResellerProfileByUser returns the profile bound to a platform user, or nil
// when the user is not a reseller.
func GetResellerProfileByUser(userId int) (*ResellerProfile, error) {
	if userId <= 0 {
		return nil, nil
	}
	var profile ResellerProfile
	err := DB.Where("user_id = ?", userId).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetEnabledResellerProfile returns the caller's profile only when the reseller
// capability is enabled.
func GetEnabledResellerProfile(userId int) (*ResellerProfile, error) {
	profile, err := GetResellerProfileByUser(userId)
	if err != nil || profile == nil {
		return nil, err
	}
	if profile.Status != ResellerStatusEnabled {
		return nil, ErrResellerNotEnabled
	}
	return profile, nil
}

// GetResellerCustomerOwner returns the reseller user id owning the customer, or
// 0 when the customer is platform-direct.
func GetResellerCustomerOwner(customerUserId int) (int, error) {
	binding := ResellerCustomerBinding{CustomerUserId: customerUserId}
	err := DB.Where("customer_user_id = ?", customerUserId).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return binding.ResellerId, nil
}

// SetResellerCustomerOwner updates the current owner and appends the immutable
// history event in one transaction.
func SetResellerCustomerOwner(customerUserId, resellerId, actorId int, reason string) error {
	now := common.GetTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current ResellerCustomerBinding
		err := lockForUpdate(tx).Where("customer_user_id = ?", customerUserId).First(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		fromResellerId := current.ResellerId
		event := ResellerCustomerEvent{
			CustomerUserId: customerUserId,
			FromResellerId: fromResellerId,
			ToResellerId:   resellerId,
			EffectiveAt:    now,
			ActorId:        actorId,
			Reason:         reason,
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return tx.Save(&ResellerCustomerBinding{
			CustomerUserId: customerUserId,
			ResellerId:     resellerId,
			EffectiveFrom:  now,
			UpdatedBy:      actorId,
			UpdatedAt:      now,
		}).Error
	})
	if err != nil {
		return err
	}
	_ = invalidateUserCache(customerUserId)
	return InvalidateResellerPricingCache(customerUserId)
}

// ListResellerCustomerIDs returns every customer user id owned by one reseller.
func ListResellerCustomerIDs(resellerId int) ([]int, error) {
	ids := make([]int, 0, 64)
	if resellerId <= 0 {
		return ids, nil
	}
	err := DB.Model(&ResellerCustomerBinding{}).
		Where("reseller_id = ?", resellerId).
		Order("customer_user_id asc").
		Pluck("customer_user_id", &ids).Error
	return ids, err
}

// requireResellerCustomer resolves an owned customer for the caller. It is the
// single authorization gate for every owner-scoped console mutation.
func requireResellerCustomer(tx *gorm.DB, resellerUserId, customerUserId int) (*User, error) {
	var binding ResellerCustomerBinding
	err := tx.Where("customer_user_id = ?", customerUserId).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrResellerCustomerNotFound
	}
	if err != nil {
		return nil, err
	}
	if binding.ResellerId != resellerUserId {
		return nil, ErrResellerCustomerNotFound
	}
	var customer User
	if err := tx.First(&customer, customerUserId).Error; err != nil {
		return nil, err
	}
	return &customer, nil
}

// resellerRemainingTransfer sums the unconsumed funded position of a customer
// with one reseller. Transfers are 1:1, so the retail position is also the
// amount the reseller paid.
func resellerRemainingTransfer(tx *gorm.DB, resellerUserId, customerUserId int) (int64, error) {
	var row struct {
		Retail int64 `gorm:"column:retail"`
	}
	err := tx.Model(&ResellerTransfer{}).
		Where("reseller_id = ? AND customer_user_id = ?", resellerUserId, customerUserId).
		Select("COALESCE(SUM(CASE WHEN direction = 'fund' THEN retail_quota ELSE -retail_quota END),0) AS retail").
		Scan(&row).Error
	if err != nil {
		return 0, err
	}
	return row.Retail, nil
}

// ResellerTransferRequest is the full description of one wallet movement.
type ResellerTransferRequest struct {
	ResellerId     int
	CustomerUserId int
	OperatorId     int
	Direction      string
	RetailQuota    int64
	EventKey       string
	Kind           string
	Remark         string
	// ReversesId is set only on the compensating reclaim a void appends.
	ReversesId int64
}

// TransferResellerQuota moves quota between the reseller wallet and one owned
// customer wallet atomically. direction is "fund" or "reclaim". The event key is
// caller-owned so a retried request never moves money twice.
//
// The transfer itself is 1:1: funding X quota costs the reseller X and credits
// the customer X, and a reclaim reverses exactly that. A customer is an ordinary
// account that may call any group its key selects, so charging a wholesale rate
// here would require guessing which group the quota will be spent in. The
// reseller's profit is instead the spread between the retail price it set and
// its own wholesale cost for the group actually used, which the billing path
// records per request and the margin rollup credits back to the reseller wallet.
func TransferResellerQuota(resellerUserId, customerUserId, operatorId int, direction string, retailQuota int64, eventKey, kind, remark string) (*ResellerTransfer, error) {
	return transferResellerQuotaTx(DB, ResellerTransferRequest{
		ResellerId:     resellerUserId,
		CustomerUserId: customerUserId,
		OperatorId:     operatorId,
		Direction:      direction,
		RetailQuota:    retailQuota,
		EventKey:       eventKey,
		Kind:           kind,
		Remark:         remark,
	})
}

func transferResellerQuotaTx(database *gorm.DB, req ResellerTransferRequest) (*ResellerTransfer, error) {
	resellerUserId := req.ResellerId
	customerUserId := req.CustomerUserId
	operatorId := req.OperatorId
	direction := req.Direction
	retailQuota := req.RetailQuota
	eventKey := req.EventKey
	remark := req.Remark

	if resellerUserId <= 0 || customerUserId <= 0 || retailQuota <= 0 || eventKey == "" {
		return nil, ErrResellerTransferInvalid
	}
	if retailQuota > int64(common.MaxQuota) {
		return nil, ErrResellerTransferInvalid
	}
	fundingKind := NormalizeResellerFundingKind(req.Kind)
	transfer := &ResellerTransfer{}
	err := database.Transaction(func(tx *gorm.DB) error {
		var existing ResellerTransfer
		err := tx.Where("event_key = ?", eventKey).First(&existing).Error
		if err == nil {
			// The event key is globally unique, so a key reused by a different
			// reseller or customer would otherwise replay another tenant's
			// transfer row as a success. Only the same movement may replay.
			if existing.ResellerId != resellerUserId || existing.CustomerUserId != customerUserId || existing.Direction != direction {
				return ErrResellerTransferInvalid
			}
			*transfer = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		customer, err := requireResellerCustomer(tx, resellerUserId, customerUserId)
		if err != nil {
			return err
		}
		// Recorded for display only: the transfer is priced 1:1 regardless of
		// which group the customer happens to sit in or spends in.
		group := customer.Group

		// Lock both wallets in a stable id order so two concurrent transfers in
		// opposite directions cannot deadlock.
		firstId, secondId := resellerUserId, customerUserId
		if firstId > secondId {
			firstId, secondId = secondId, firstId
		}
		var first, second User
		if err := lockForUpdate(tx).First(&first, firstId).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&second, secondId).Error; err != nil {
			return err
		}
		reseller, owner := &first, &second
		if resellerUserId != firstId {
			reseller, owner = &second, &first
		}

		var resellerDelta, customerDelta, costQuota int64
		switch direction {
		case ResellerTransferFund:
			if int64(reseller.Quota) < retailQuota {
				return ErrResellerWalletShort
			}
			costQuota = retailQuota
			resellerDelta, customerDelta = -retailQuota, retailQuota
		case ResellerTransferReclaim:
			// The outstanding funded position is checked first: reclaiming more
			// than the reseller ever put in is a different error from the
			// customer having already spent the quota.
			remainingRetail, err := resellerRemainingTransfer(tx, resellerUserId, customerUserId)
			if err != nil {
				return err
			}
			if retailQuota > remainingRetail {
				return ErrResellerTransferInvalid
			}
			if int64(owner.Quota) < retailQuota {
				return ErrResellerCustomerShort
			}
			costQuota = retailQuota
			resellerDelta, customerDelta = retailQuota, -retailQuota
		default:
			return ErrResellerTransferInvalid
		}

		if err := changeBillingWallet(tx, reseller, resellerDelta); err != nil {
			return err
		}
		if err := changeBillingWallet(tx, owner, customerDelta); err != nil {
			return err
		}
		resellerEntry := BillingEntry{
			EventKey:    eventKey + ":r",
			Kind:        "funding",
			WalletDelta: resellerDelta,
			ActorID:     operatorId,
		}
		if err := appendBillingEntry(tx, reseller, &resellerEntry); err != nil {
			return err
		}
		customerEntry := BillingEntry{
			EventKey:    eventKey + ":c",
			Kind:        "funding",
			WalletDelta: customerDelta,
			ActorID:     operatorId,
		}
		if err := appendBillingEntry(tx, owner, &customerEntry); err != nil {
			return err
		}

		*transfer = ResellerTransfer{
			EventKey:       eventKey,
			ResellerId:     resellerUserId,
			CustomerUserId: customerUserId,
			Direction:      direction,
			GroupName:      group,
			RetailQuota:    retailQuota,
			CostQuota:      costQuota,
			RetailRatio:    "1",
			CostRatio:      "1",
			OperatorId:     operatorId,
			Kind:           fundingKind,
			ReversesId:     req.ReversesId,
			Remark:         remark,
			CreatedAt:      common.GetTimestamp(),
		}
		return tx.Create(transfer).Error
	})
	if err != nil {
		return nil, err
	}
	_ = invalidateUserCache(resellerUserId)
	_ = invalidateUserCache(customerUserId)
	_ = InvalidateResellerPricingCache(customerUserId)
	return transfer, nil
}

// ResellerReclaimableQuota is the retail quota a customer can hand back without
// dipping into quota funded outside the reseller relationship.
func ResellerReclaimableQuota(resellerUserId, customerUserId int) (int64, error) {
	retail, err := resellerRemainingTransfer(DB, resellerUserId, customerUserId)
	if err != nil {
		return 0, err
	}
	if retail < 0 {
		return 0, nil
	}
	return retail, nil
}

// VoidResellerTransfer undoes one funding by appending the matching reclaim.
//
// Transfers are append-only evidence, so a void never edits the original row:
// it writes a reclaim of the same amount whose ReversesId points back at it.
// That keeps both the mistake and its correction on the ledger, and makes the
// funded/remaining aggregates correct without a special case.
func VoidResellerTransfer(resellerUserId, transferId, operatorId int, reason string) (*ResellerTransfer, error) {
	if resellerUserId <= 0 || transferId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	var original ResellerTransfer
	if err := DB.Where("id = ? AND reseller_id = ?", transferId, resellerUserId).First(&original).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResellerTransferInvalid
		}
		return nil, err
	}
	if original.Direction != ResellerTransferFund {
		// A reclaim is already a correction; voiding one would mean re-funding,
		// which is a new funding the operator should record explicitly.
		return nil, ErrResellerTransferInvalid
	}

	// A precredit covered by any live receipt is already part of a reconciliation
	// batch; voiding it behind that batch's back would leave the receipt's total
	// pointing at quota that no longer exists. Void the receipt first.
	var covered int64
	if err := DB.Model(&ResellerFundingReceiptItem{}).
		Joins("JOIN reseller_funding_receipts ON reseller_funding_receipts.id = reseller_funding_receipt_items.receipt_id").
		Where("reseller_funding_receipt_items.transfer_id = ? AND reseller_funding_receipts.status <> ?",
			original.Id, ResellerReceiptVoid).
		Count(&covered).Error; err != nil {
		return nil, err
	}
	if covered > 0 {
		return nil, ErrResellerTransferReconciled
	}

	remark := strings.TrimSpace(reason)
	if remark == "" {
		remark = "void #" + strconv.FormatInt(original.Id, 10)
	}
	return transferResellerQuotaTx(DB, ResellerTransferRequest{
		ResellerId:     resellerUserId,
		CustomerUserId: original.CustomerUserId,
		OperatorId:     operatorId,
		Direction:      ResellerTransferReclaim,
		RetailQuota:    original.RetailQuota,
		// Derived from the original id, so a double click replays instead of
		// reversing the same funding twice.
		EventKey: "void:" + strconv.FormatInt(original.Id, 10),
		Kind:     original.Kind,
		Remark:   remark,
		// Links the correction to the mistake it undoes; without this the row
		// reads as an unexplained reclaim and the original stays voidable.
		ReversesId: original.Id,
	})
}
