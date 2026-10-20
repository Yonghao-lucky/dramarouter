package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Reseller status values. A reseller profile is a capability layer bound to a
// normal platform user; it never changes the user's role or group.
const (
	ResellerStatusDisabled = 1
	ResellerStatusEnabled  = 2
)

// ResellerProfile binds a reseller capability to an existing platform user.
// The user's own users.quota remains their single wallet.
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
// a billing group. Stored as a decimal string to avoid float drift.
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
// over every other pricing layer.
type CustomerGroupPrice struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex:idx_customer_group_price,priority:1"`
	GroupName string `json:"group" gorm:"type:varchar(64);uniqueIndex:idx_customer_group_price,priority:2"`
	Ratio     string `json:"ratio" gorm:"type:varchar(64)"`
	Source    string `json:"source" gorm:"type:varchar(16)"` // admin | reseller
	UpdatedBy int    `json:"updated_by"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

// ResellerCustomerBinding records the current owner of a customer. ResellerID 0
// means platform-direct ownership. Ownership changes are append-only in
// ResellerCustomerEvent.
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

// ResellerCustomerAllocation caps how much retail quota a customer may consume
// from the reseller pool. It is a limit, not money: allocations and reclaims
// move no funds. ReservedQuota covers in-flight reservations so the cap cannot
// be exceeded by concurrent requests.
type ResellerCustomerAllocation struct {
	CustomerUserId int   `json:"customer_user_id" gorm:"primaryKey;autoIncrement:false"`
	AllocatedQuota int64 `json:"allocated_quota" gorm:"bigint"`
	ReservedQuota  int64 `json:"reserved_quota" gorm:"bigint"`
	UsedQuota      int64 `json:"used_quota" gorm:"bigint"`
	UpdatedAt      int64 `json:"updated_at" gorm:"bigint"`
}

// AvailableQuota is what the customer may still consume from the pool.
func (a *ResellerCustomerAllocation) AvailableQuota() int64 {
	available := a.AllocatedQuota - a.ReservedQuota - a.UsedQuota
	if available < 0 {
		return 0
	}
	return available
}

// ResellerMarginsDaily is the incremental rollup that backs the reseller
// console. It is derived evidence; CostAccountingSnapshot stays authoritative.
type ResellerMarginsDaily struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	OwnerId        int    `json:"owner_id" gorm:"uniqueIndex:idx_reseller_margin_day,priority:1"`
	Day            int64  `json:"day" gorm:"bigint;uniqueIndex:idx_reseller_margin_day,priority:2"`
	CustomerUserId int    `json:"customer_user_id" gorm:"uniqueIndex:idx_reseller_margin_day,priority:3"`
	ModelName      string `json:"model_name" gorm:"type:varchar(191);uniqueIndex:idx_reseller_margin_day,priority:4"`
	RevenueQuota   int64  `json:"revenue_quota" gorm:"bigint"`
	MarginQuota    int64  `json:"margin_quota" gorm:"bigint"`
	RequestCount   int64  `json:"request_count" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

// TableName keeps the rollup table name stable; GORM would otherwise pluralize
// the last word into reseller_margins_dailies.
func (*ResellerMarginsDaily) TableName() string { return "reseller_margins_daily" }

// GetResellerCustomerOwner returns the reseller id owning the customer, or 0
// when the customer is platform-direct.
func GetResellerCustomerOwner(customerUserId int) (int, error) {
	binding := ResellerCustomerBinding{CustomerUserId: customerUserId}
	err := DB.Where("customer_user_id = ?", customerUserId).First(&binding).Error
	if err == gorm.ErrRecordNotFound {
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
	return DB.Transaction(func(tx *gorm.DB) error {
		var current ResellerCustomerBinding
		err := lockForUpdate(tx).Where("customer_user_id = ?", customerUserId).First(&current).Error
		if err != nil && err != gorm.ErrRecordNotFound {
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
}
