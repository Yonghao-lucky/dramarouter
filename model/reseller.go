package model

import (
	"errors"
	"strconv"

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

var (
	ErrResellerNotEnabled       = errors.New("not an enabled reseller")
	ErrResellerCustomerNotFound = errors.New("customer is not owned by this reseller")
	ErrResellerGroupNotAllowed  = errors.New("group is not authorized for this reseller")
	ErrResellerPriceBelowCost   = errors.New("customer price cannot be lower than the reseller cost")
	ErrResellerWalletShort      = errors.New("reseller wallet is insufficient")
	ErrResellerCustomerShort    = errors.New("customer wallet is insufficient")
	ErrResellerTransferInvalid  = errors.New("invalid reseller transfer")
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
	Remark         string `json:"remark" gorm:"type:varchar(255)"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index:idx_reseller_transfer_time,priority:1"`
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

// resellerGroupRatios returns the wholesale and retail ratios a reseller sells
// one group at. The retail ratio falls back to the wholesale ratio itself when
// the reseller never set a standing price, so a transfer is always priced and
// the reseller can never sell below cost by omission.
func resellerGroupRatios(tx *gorm.DB, resellerUserId, customerUserId int, group string) (costRatio, retailRatio float64, err error) {
	var cost ResellerGroupCost
	err = tx.Where("reseller_id = ? AND group_name = ? AND enabled = ?", resellerUserId, group, true).First(&cost).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, ErrResellerGroupNotAllowed
	}
	if err != nil {
		return 0, 0, err
	}
	costRatio, ok := parsePositiveRatio(cost.CostRatio)
	if !ok {
		return 0, 0, ErrResellerGroupNotAllowed
	}

	var customerPrice CustomerGroupPrice
	err = tx.Where("user_id = ? AND group_name = ?", customerUserId, group).First(&customerPrice).Error
	if err == nil {
		if ratio, valid := parsePositiveRatio(customerPrice.Ratio); valid {
			return costRatio, ratio, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, err
	}

	var defaultPrice ResellerDefaultPrice
	err = tx.Where("reseller_id = ? AND group_name = ?", resellerUserId, group).First(&defaultPrice).Error
	if err == nil {
		if ratio, valid := parsePositiveRatio(defaultPrice.Ratio); valid {
			return costRatio, ratio, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, err
	}
	return costRatio, costRatio, nil
}

// resellerRemainingTransfer sums the unconsumed transfer position of a customer
// with one reseller. The average paid cost per retail quota is the only safe
// reclaim basis, because ratios may change between transfers.
func resellerRemainingTransfer(tx *gorm.DB, resellerUserId, customerUserId int) (retail int64, cost int64, err error) {
	var row struct {
		Retail int64 `gorm:"column:retail"`
		Cost   int64 `gorm:"column:cost"`
	}
	err = tx.Model(&ResellerTransfer{}).
		Where("reseller_id = ? AND customer_user_id = ?", resellerUserId, customerUserId).
		Select("COALESCE(SUM(CASE WHEN direction = 'fund' THEN retail_quota ELSE -retail_quota END),0) AS retail, COALESCE(SUM(CASE WHEN direction = 'fund' THEN cost_quota ELSE -cost_quota END),0) AS cost").
		Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}
	return row.Retail, row.Cost, nil
}

// TransferResellerQuota moves quota between the reseller wallet and one owned
// customer wallet atomically. direction is "fund" or "reclaim". The event key is
// caller-owned so a retried request never moves money twice.
//
// A fund of X retail quota charges the reseller X·cost/retail: the platform is
// paid its wholesale price at transfer time, and the reseller's margin becomes
// real immediately. A reclaim reverses the exchange at the average cost the
// outstanding position was bought at, so ratio changes cannot be arbitraged.
//
// Both ratios come from the customer's own group. A customer consumes at the
// ratio of the group they belong to, so pricing a transfer at any other group
// would let the reseller pay one cost while the customer spends at another —
// a silent loss whenever the two differ.
func TransferResellerQuota(resellerUserId, customerUserId, operatorId int, direction string, retailQuota int64, eventKey, remark string) (*ResellerTransfer, error) {
	if resellerUserId <= 0 || customerUserId <= 0 || retailQuota <= 0 || eventKey == "" {
		return nil, ErrResellerTransferInvalid
	}
	if retailQuota > int64(common.MaxQuota) {
		return nil, ErrResellerTransferInvalid
	}
	transfer := &ResellerTransfer{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing ResellerTransfer
		err := tx.Where("event_key = ?", eventKey).First(&existing).Error
		if err == nil {
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
		group := customer.Group
		costRatio, retailRatio, err := resellerGroupRatios(tx, resellerUserId, customerUserId, group)
		if err != nil {
			return err
		}

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
			costQuota = int64(common.QuotaRound(float64(retailQuota) * costRatio / retailRatio))
			if costQuota < 0 {
				return ErrResellerTransferInvalid
			}
			if int64(reseller.Quota) < costQuota {
				return ErrResellerWalletShort
			}
			resellerDelta, customerDelta = -costQuota, retailQuota
		case ResellerTransferReclaim:
			// The outstanding funded position is checked first: reclaiming more
			// than the reseller ever put in is a different error from the
			// customer having already spent the quota.
			remainingRetail, remainingCost, err := resellerRemainingTransfer(tx, resellerUserId, customerUserId)
			if err != nil {
				return err
			}
			if retailQuota > remainingRetail {
				return ErrResellerTransferInvalid
			}
			if int64(owner.Quota) < retailQuota {
				return ErrResellerCustomerShort
			}
			if remainingRetail > 0 && remainingCost > 0 {
				costQuota = int64(float64(remainingCost) * float64(retailQuota) / float64(remainingRetail))
			}
			resellerDelta, customerDelta = costQuota, -retailQuota
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
			RetailRatio:    strconv.FormatFloat(retailRatio, 'f', -1, 64),
			CostRatio:      strconv.FormatFloat(costRatio, 'f', -1, 64),
			OperatorId:     operatorId,
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
	retail, _, err := resellerRemainingTransfer(DB, resellerUserId, customerUserId)
	if err != nil {
		return 0, err
	}
	if retail < 0 {
		return 0, nil
	}
	return retail, nil
}
