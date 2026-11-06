package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ResellerCustomerDashboard is the reseller's view of the customers it owns.
//
// Permission boundary: every figure here is derived from cost_accounting_snapshots
// rows whose owner_id is the calling reseller. The query never accepts a customer
// id from the client, never joins a channel or upstream cost column, and never
// returns prompt content, so a reseller cannot use it to read another reseller's
// traffic or the platform's own margin.
type ResellerCustomerDashboard struct {
	CustomerCount     int64                          `json:"customer_count"`
	ActiveCount       int64                          `json:"active_count"`
	TotalBalance      int64                          `json:"total_balance_quota"`
	RevenueQuota      int64                          `json:"revenue_quota"`
	RequestCount      int64                          `json:"request_count"`
	UsedQuota         int64                          `json:"used_quota"`
	AveragePerRequest int64                          `json:"average_per_request"`
	WindowDays        int                            `json:"window_days"`
	TopCustomers      []ResellerDashboardTopCustomer `json:"top_customers"`
	ByModel           []ResellerDashboardModelRow    `json:"by_model"`
	ByDay             []ResellerMarginDay            `json:"by_day"`
	Customers         []ResellerDashboardCustomerRow `json:"customers"`
}

type ResellerDashboardTopCustomer struct {
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Revenue     int64  `json:"revenue_quota" gorm:"column:revenue"`
	Requests    int64  `json:"request_count" gorm:"column:requests"`
	Balance     int64  `json:"balance_quota"`
}

type ResellerDashboardModelRow struct {
	ModelName string `json:"model_name"`
	Revenue   int64  `json:"revenue_quota"`
	Requests  int64  `json:"request_count"`
	Customers int64  `json:"customer_count"`
}

// GetResellerCustomerDashboard aggregates the owned customers over a rolling
// window. windowDays is clamped so a client cannot ask for an unbounded scan.
func GetResellerCustomerDashboard(resellerUserId, windowDays int) (*ResellerCustomerDashboard, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	if windowDays > 90 {
		windowDays = 90
	}
	since := time.Now().AddDate(0, 0, -windowDays).Unix()
	dashboard := &ResellerCustomerDashboard{WindowDays: windowDays}

	// Customer count and balance come from the ownership binding, so a customer
	// with no traffic in the window is still counted.
	var wallet struct {
		Count   int64 `gorm:"column:count"`
		Balance int64 `gorm:"column:balance"`
	}
	if err := DB.Model(&User{}).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = users.id").
		Where("b.reseller_id = ?", resellerUserId).
		Select("COUNT(*) AS count, COALESCE(SUM(users.quota),0) AS balance").
		Scan(&wallet).Error; err != nil {
		return nil, err
	}
	dashboard.CustomerCount = wallet.Count
	dashboard.TotalBalance = wallet.Balance

	// Usage figures are scoped by owner_id, which is the reseller user id on
	// every snapshot written for its customers.
	var usage struct {
		Revenue  int64 `gorm:"column:revenue"`
		Requests int64 `gorm:"column:requests"`
		Used     int64 `gorm:"column:used"`
		Active   int64 `gorm:"column:active"`
	}
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ? AND occurred_at >= ?", resellerUserId, since).
		Select(`COALESCE(SUM(revenue_quota),0) AS revenue,
			COUNT(*) AS requests,
			COALESCE(SUM(revenue_quota),0) AS used,
			COUNT(DISTINCT user_id) AS active`).
		Scan(&usage).Error; err != nil {
		return nil, err
	}
	dashboard.RevenueQuota = usage.Revenue
	dashboard.RequestCount = usage.Requests
	dashboard.UsedQuota = usage.Used
	dashboard.ActiveCount = usage.Active
	if usage.Requests > 0 {
		dashboard.AveragePerRequest = int64(common.QuotaRound(float64(usage.Revenue) / float64(usage.Requests)))
	}

	top := make([]ResellerDashboardTopCustomer, 0, 10)
	if err := DB.Model(&CostAccountingSnapshot{}).
		Joins("JOIN users ON users.id = cost_accounting_snapshots.user_id").
		Where("cost_accounting_snapshots.owner_id = ? AND cost_accounting_snapshots.occurred_at >= ?", resellerUserId, since).
		Select(`cost_accounting_snapshots.user_id AS user_id,
			users.username AS username,
			users.display_name AS display_name,
			COALESCE(SUM(cost_accounting_snapshots.revenue_quota),0) AS revenue,
			COUNT(*) AS requests,
			users.quota AS balance_quota`).
		Group("cost_accounting_snapshots.user_id, users.username, users.display_name, users.quota").
		Order("revenue desc").
		Limit(10).
		Scan(&top).Error; err != nil {
		return nil, err
	}
	dashboard.TopCustomers = top

	byModel := make([]ResellerDashboardModelRow, 0, 20)
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ? AND occurred_at >= ?", resellerUserId, since).
		Select(`model_name,
			COALESCE(SUM(revenue_quota),0) AS revenue,
			COUNT(*) AS requests,
			COUNT(DISTINCT user_id) AS customers`).
		Group("model_name").
		Order("revenue desc").
		Limit(20).
		Scan(&byModel).Error; err != nil {
		return nil, err
	}
	dashboard.ByModel = byModel

	byDay := make([]ResellerMarginDay, 0, windowDays)
	// Day bucketing differs between PostgreSQL and SQLite, so the grouping key is
	// produced by dividing the timestamp by one day in the server's local zone.
	// This keeps the query portable instead of using to_char or strftime.
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ? AND occurred_at >= ?", resellerUserId, since).
		Select(`(occurred_at / 86400) AS day,
			COALESCE(SUM(revenue_quota),0) AS revenue_quota,
			COALESCE(SUM(reseller_margin_quota),0) AS margin_quota,
			COUNT(*) AS request_count`).
		Group("(occurred_at / 86400)").
		Order("day asc").
		Scan(&byDay).Error; err != nil {
		return nil, err
	}
	dashboard.ByDay = byDay

	customers, err := GetResellerCustomerRows(resellerUserId, windowDays)
	if err != nil {
		return nil, err
	}
	dashboard.Customers = customers

	return dashboard, nil
}

// ResellerOperationAudit is one auditable reseller action. The row is written by
// the shared operation audit log, and this model only reads it back so a
// reseller can review its own history including the actor IP.
type ResellerOperationAudit struct {
	Id         int64  `json:"id"`
	OperatorId int    `json:"operator_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	Detail     string `json:"detail"`
	Ip         string `json:"ip"`
	CreatedAt  int64  `json:"created_at"`
}

// ListResellerOperationAudits reads the management-type audit entries written by
// reseller actions. Entries are attributed to the acting reseller user id, so
// this never returns another reseller's rows.
func ListResellerOperationAudits(resellerUserId int, limit int) ([]ResellerOperationAudit, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// Other holds the structured op field (action + params) written by
	// RecordOperationAuditLog; content is the English fallback sentence. Both are
	// read so the console can show a stable action id next to readable detail.
	rows := make([]struct {
		Id         int64  `gorm:"column:id"`
		OperatorId int    `gorm:"column:operator_id"`
		Username   string `gorm:"column:username"`
		Content    string `gorm:"column:content"`
		Other      string `gorm:"column:other"`
		Ip         string `gorm:"column:ip"`
		CreatedAt  int64  `gorm:"column:created_at"`
	}, 0, limit)
	err := DB.Model(&Log{}).
		Where("user_id = ? AND type = ?", resellerUserId, LogTypeManage).
		Order("id desc").
		Limit(limit).
		Select("id, user_id AS operator_id, username, content, other, ip, created_at").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	audits := make([]ResellerOperationAudit, 0, len(rows))
	for _, row := range rows {
		audit := ResellerOperationAudit{
			Id:         row.Id,
			OperatorId: row.OperatorId,
			Username:   row.Username,
			Detail:     row.Content,
			Ip:         row.Ip,
			CreatedAt:  row.CreatedAt,
		}
		// The op field is JSON owned by the audit writer; an unreadable value
		// must not drop the whole row from the trail.
		var other map[string]any
		if common.UnmarshalJsonStr(row.Other, &other) == nil {
			if op, ok := other["op"].(map[string]any); ok {
				if action, ok := op["action"].(string); ok {
					audit.Action = action
				}
			}
		}
		audits = append(audits, audit)
	}
	return audits, nil
}

// ResellerDashboardCustomerRow is one owned customer's standing figures. It is
// the per-customer breakdown behind the aggregate tiles, so a reseller can see
// traffic, spend and wallet per customer without opening each record.
type ResellerDashboardCustomerRow struct {
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Group       string `json:"group"`
	Balance     int64  `json:"balance_quota" gorm:"column:balance"`
	UsedQuota   int64  `json:"used_quota" gorm:"column:used_quota"`
	Revenue     int64  `json:"revenue_quota" gorm:"column:revenue"`
	Requests    int64  `json:"request_count" gorm:"column:requests"`
	LastActive  int64  `json:"last_active_at" gorm:"column:last_active"`
}

// GetResellerCustomerRows returns every customer this reseller owns with its
// usage inside the window. The binding table is the source of the customer list,
// so a customer with no traffic still appears with zeroed figures. Everything is
// filtered by owner_id, which is the caller's own user id.
func GetResellerCustomerRows(resellerUserId, windowDays int) ([]ResellerDashboardCustomerRow, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	if windowDays <= 0 {
		windowDays = 7
	}
	if windowDays > 90 {
		windowDays = 90
	}
	since := time.Now().AddDate(0, 0, -windowDays).Unix()
	usage := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ? AND occurred_at >= ?", resellerUserId, since).
		Select(`user_id,
			COALESCE(SUM(revenue_quota),0) AS revenue,
			COUNT(*) AS requests,
			MAX(occurred_at) AS last_active`).
		Group("user_id")

	rows := make([]ResellerDashboardCustomerRow, 0, 64)
	err := DB.Table("reseller_customer_bindings b").
		Joins("JOIN users u ON u.id = b.customer_user_id").
		Joins("LEFT JOIN (?) s ON s.user_id = b.customer_user_id", usage).
		Where("b.reseller_id = ?", resellerUserId).
		Select(`u.id AS user_id,
			u.username AS username,
			u.display_name AS display_name,
			u.` + commonGroupCol + ` AS ` + commonGroupCol + `,
			u.quota AS balance,
			u.used_quota AS used_quota,
			COALESCE(s.revenue,0) AS revenue,
			COALESCE(s.requests,0) AS requests,
			COALESCE(s.last_active,0) AS last_active`).
		Order("revenue desc, u.username asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// GetResellerQuotaData aggregates the model-call analytics of every customer the
// caller owns, grouped per customer and model. The customer list comes from the
// ownership binding, so a reseller never sees traffic outside its book.
func GetResellerQuotaData(resellerUserId int, startTime, endTime int64) ([]*QuotaData, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	customerIDs, err := ListResellerCustomerIDs(resellerUserId)
	if err != nil {
		return nil, err
	}
	if len(customerIDs) == 0 {
		return []*QuotaData{}, nil
	}
	quotaDatas := make([]*QuotaData, 0, 128)
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("user_id IN ? and created_at >= ? and created_at <= ?", customerIDs, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	if err != nil {
		return nil, err
	}
	// Billing corrections are keyed by the customer that paid, so only the ones
	// belonging to this reseller's customers may be projected onto these rows.
	corrections, err := billingDashboardCorrections(0, "", startTime, endTime)
	if err != nil {
		return nil, err
	}
	owned := make(map[int]bool, len(customerIDs))
	for _, id := range customerIDs {
		owned[id] = true
	}
	scoped := make([]billingDashboardCorrection, 0, len(corrections))
	for _, correction := range corrections {
		if owned[correction.Source.UserID] {
			scoped = append(scoped, correction)
		}
	}
	if len(scoped) > 0 {
		if err := projectBillingQuotaData(quotaDatas, scoped, "user_model"); err != nil {
			return nil, err
		}
	}
	return quotaDatas, nil
}
