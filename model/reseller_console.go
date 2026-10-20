package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ResellerSummary is the console home payload. Every number comes from one
// indexed aggregate or the cached wallet balance, so the console stays fast
// regardless of usage volume.
type ResellerSummary struct {
	ResellerId      int   `json:"reseller_id"`
	WalletQuota     int64 `json:"wallet_quota"`
	CustomerCount   int64 `json:"customer_count"`
	AllocatedQuota  int64 `json:"allocated_quota"`
	UsedQuota       int64 `json:"used_quota"`
	ReservedQuota   int64 `json:"reserved_quota"`
	RevenueQuota    int64 `json:"revenue_quota"`
	MarginQuota     int64 `json:"margin_quota"`
	SelfPayMargin   int64 `json:"self_pay_margin_quota"`
	PoolMarginQuota int64 `json:"pool_margin_quota"`
	LastActivityAt  int64 `json:"last_activity_at"`
}

// ResellerCustomerRow is one customer line in the console list.
type ResellerCustomerRow struct {
	UserId         int    `json:"user_id"`
	Username       string `json:"username"`
	Group          string `json:"group" gorm:"column:group"`
	Status         int    `json:"status"`
	WalletQuota    int64  `json:"wallet_quota"`
	AllocatedQuota int64  `json:"allocated_quota"`
	ReservedQuota  int64  `json:"reserved_quota"`
	UsedQuota      int64  `json:"used_quota"`
	RevenueQuota   int64  `json:"revenue_quota"`
	MarginQuota    int64  `json:"margin_quota"`
}

// ResellerUsageRow is one masked usage line. Prompt content and request bodies
// are never selected.
type ResellerUsageRow struct {
	ID           int64  `json:"id"`
	CustomerId   int    `json:"customer_id"`
	ModelName    string `json:"model_name"`
	GroupName    string `json:"group" gorm:"column:group_name"`
	LogType      int    `json:"log_type"`
	OccurredAt   int64  `json:"occurred_at"`
	RevenueQuota int64  `json:"revenue_quota"`
	MarginQuota  int64  `json:"margin_quota"`
	Settlement   string `json:"settlement"`
	RequestID    string `json:"request_id"`
}

// GetResellerSummary aggregates one reseller's console home. Each query is an
// indexed aggregate; none of them scan the whole snapshot table.
func GetResellerSummary(resellerId int) (*ResellerSummary, error) {
	if resellerId <= 0 {
		return nil, errors.New("invalid reseller")
	}
	summary := &ResellerSummary{ResellerId: resellerId}
	var wallet struct {
		Quota int64 `gorm:"column:quota"`
	}
	if err := DB.Model(&User{}).Select("quota").Where("id = ?", resellerId).Scan(&wallet).Error; err != nil {
		return nil, err
	}
	summary.WalletQuota = wallet.Quota
	if err := DB.Model(&ResellerCustomerBinding{}).Where("reseller_id = ?", resellerId).Count(&summary.CustomerCount).Error; err != nil {
		return nil, err
	}
	var allocation struct {
		Allocated int64 `gorm:"column:allocated_quota"`
		Used      int64 `gorm:"column:used_quota"`
		Reserved  int64 `gorm:"column:reserved_quota"`
	}
	if err := DB.Model(&ResellerCustomerAllocation{}).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = reseller_customer_allocations.customer_user_id").
		Where("b.reseller_id = ?", resellerId).
		Select("COALESCE(SUM(reseller_customer_allocations.allocated_quota),0) AS allocated_quota, COALESCE(SUM(reseller_customer_allocations.used_quota),0) AS used_quota, COALESCE(SUM(reseller_customer_allocations.reserved_quota),0) AS reserved_quota").
		Scan(&allocation).Error; err != nil {
		return nil, err
	}
	summary.AllocatedQuota, summary.UsedQuota, summary.ReservedQuota = allocation.Allocated, allocation.Used, allocation.Reserved
	var usage struct {
		Revenue int64 `gorm:"column:revenue_quota"`
		Margin  int64 `gorm:"column:margin_quota"`
		Last    int64 `gorm:"column:last_activity_at"`
	}
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ?", resellerId).
		Select("COALESCE(SUM(revenue_quota),0) AS revenue_quota, COALESCE(SUM(reseller_margin_quota),0) AS margin_quota, COALESCE(MAX(occurred_at),0) AS last_activity_at").
		Scan(&usage).Error; err != nil {
		return nil, err
	}
	summary.RevenueQuota, summary.MarginQuota, summary.LastActivityAt = usage.Revenue, usage.Margin, usage.Last
	var selfPay struct {
		Margin int64 `gorm:"column:self_pay_margin_quota"`
	}
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ? AND reseller_settlement = ?", resellerId, "self").
		Select("COALESCE(SUM(reseller_margin_quota),0) AS self_pay_margin_quota").
		Scan(&selfPay).Error; err != nil {
		return nil, err
	}
	summary.SelfPayMargin = selfPay.Margin
	summary.PoolMarginQuota = summary.MarginQuota - summary.SelfPayMargin
	return summary, nil
}

// ListResellerCustomers pages the customer list without a COUNT, ordered by
// user id so the cursor stays stable while usage is written concurrently.
func ListResellerCustomers(resellerId int, afterUserId, limit int) ([]ResellerCustomerRow, error) {
	if resellerId <= 0 {
		return nil, errors.New("invalid reseller")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerCustomerRow, 0, limit)
	// `group` is a reserved word in SQLite and MySQL, so the shared dialect-aware
	// column reference is mandatory in raw selects.
	columns := "users.id AS user_id, users.username, users." + commonGroupCol + " AS " + commonGroupCol + ", users.status, users.quota AS wallet_quota, COALESCE(a.allocated_quota,0) AS allocated_quota, COALESCE(a.reserved_quota,0) AS reserved_quota, COALESCE(a.used_quota,0) AS used_quota"
	err := DB.Model(&User{}).
		Select(columns).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = users.id").
		Joins("LEFT JOIN reseller_customer_allocations a ON a.customer_user_id = users.id").
		Where("b.reseller_id = ? AND users.id > ?", resellerId, afterUserId).
		Order("users.id asc").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

// ListResellerUsage pages usage with a descending id cursor. There is
// intentionally no COUNT: the console never blocks on a full-table count.
func ListResellerUsage(resellerId, customerId int, beforeId int64, start, end int64, limit int) ([]ResellerUsageRow, error) {
	if resellerId <= 0 {
		return nil, errors.New("invalid reseller")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerUsageRow, 0, limit)
	query := DB.Model(&CostAccountingSnapshot{}).
		Select("id, user_id AS customer_id, model_name, group_name, log_type, occurred_at, revenue_quota, reseller_margin_quota AS margin_quota, reseller_settlement AS settlement, request_id").
		Where("owner_id = ?", resellerId)
	if customerId > 0 {
		query = query.Where("user_id = ?", customerId)
	}
	if start > 0 {
		query = query.Where("occurred_at >= ?", start)
	}
	if end > 0 {
		query = query.Where("occurred_at < ?", end)
	}
	if beforeId > 0 {
		query = query.Where("id < ?", beforeId)
	}
	err := query.Order("id desc").Limit(limit).Scan(&rows).Error
	return rows, err
}

// ResellerMarginDay is one day of margin for the reseller report.
type ResellerMarginDay struct {
	Day          int64 `json:"day" gorm:"column:day"`
	RevenueQuota int64 `json:"revenue_quota"`
	MarginQuota  int64 `json:"margin_quota"`
	RequestCount int64 `json:"request_count"`
}

// ListResellerMarginDays aggregates a bounded day range. The caller caps the
// range so the aggregate stays index-bounded.
func ListResellerMarginDays(resellerId int, start, end int64) ([]ResellerMarginDay, error) {
	if resellerId <= 0 {
		return nil, errors.New("invalid reseller")
	}
	rows := make([]ResellerMarginDay, 0, 31)
	// occurred_at / 86400 is integer division on SQLite, MySQL and PostgreSQL.
	err := DB.Model(&CostAccountingSnapshot{}).
		Select("occurred_at / 86400 AS day, COALESCE(SUM(revenue_quota),0) AS revenue_quota, COALESCE(SUM(reseller_margin_quota),0) AS margin_quota, COUNT(*) AS request_count").
		Where("owner_id = ? AND occurred_at >= ? AND occurred_at < ?", resellerId, start, end).
		Group("occurred_at / 86400").
		Order("day asc").
		Scan(&rows).Error
	return rows, err
}

// AdjustResellerAllocation raises or lowers a customer's allocation cap. The cap
// can never sit below what is already consumed in flight, so the pool cannot be
// overdrawn by an operator edit either.
func AdjustResellerAllocation(customerUserId, resellerId int, allocatedQuota int64, actorId int) error {
	if customerUserId <= 0 || allocatedQuota < 0 {
		return errors.New("invalid allocation")
	}
	if allocatedQuota > common.MaxWalletQuota {
		return ErrWalletQuotaLimitExceeded
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var binding ResellerCustomerBinding
		if err := tx.Where("customer_user_id = ?", customerUserId).First(&binding).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrResellerOwnershipChanged
			}
			return err
		}
		if resellerId > 0 && binding.ResellerId != resellerId {
			return ErrResellerOwnershipChanged
		}
		allocation, err := lockResellerAllocation(tx, customerUserId)
		if err != nil {
			return err
		}
		if allocatedQuota < allocation.UsedQuota || allocatedQuota < allocation.ReservedQuota {
			return errors.New("allocation cannot be lower than quota already used or reserved")
		}
		now := common.GetTimestamp()
		allocation.AllocatedQuota = allocatedQuota
		allocation.UpdatedAt = now
		return tx.Model(allocation).Updates(map[string]any{
			"allocated_quota": allocatedQuota,
			"updated_at":      now,
		}).Error
	})
}
