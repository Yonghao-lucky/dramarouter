package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ResellerSummary is the console home payload. Every number is one indexed
// aggregate or the wallet balance, so the home page never scans usage volume.
type ResellerSummary struct {
	ResellerId         int   `json:"reseller_id"`
	WalletQuota        int64 `json:"wallet_quota"`
	CustomerCount      int64 `json:"customer_count"`
	FundedQuota        int64 `json:"funded_quota"`
	ReclaimedQuota     int64 `json:"reclaimed_quota"`
	FundedCostQuota    int64 `json:"funded_cost_quota"`
	ReclaimedCostQuota int64 `json:"reclaimed_cost_quota"`
	CustomerBalance    int64 `json:"customer_balance_quota"`
	RevenueQuota       int64 `json:"revenue_quota"`
	MarginQuota        int64 `json:"margin_quota"`
	LastActivityAt     int64 `json:"last_activity_at"`
}

// ResellerCustomerRow is one customer line in the console list.
type ResellerCustomerRow struct {
	UserId         int    `json:"user_id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Group          string `json:"group" gorm:"column:group_name"`
	Status         int    `json:"status"`
	WalletQuota    int64  `json:"wallet_quota"`
	FundedQuota    int64  `json:"funded_quota"`
	FundedCost     int64  `json:"funded_cost_quota"`
	UsedQuota      int64  `json:"used_quota"`
	RequestCount   int    `json:"request_count"`
	LastActivityAt int64  `json:"last_activity_at"`
}

// ResellerCustomerDetail is the per-customer drawer payload.
type ResellerCustomerDetail struct {
	ResellerCustomerRow
	Email            string               `json:"email"`
	ReclaimableQuota int64                `json:"reclaimable_quota"`
	Prices           []CustomerGroupPrice `json:"prices"`
	Transfers        []ResellerTransfer   `json:"recent_transfers"`
}

// ResellerUsageRow is one usage line. Prompt content and request bodies are
// never selected, and the channel is never exposed to a reseller.
type ResellerUsageRow struct {
	ID           int64  `json:"id"`
	CustomerId   int    `json:"customer_id"`
	Username     string `json:"username"`
	ModelName    string `json:"model_name"`
	GroupName    string `json:"group" gorm:"column:group_name"`
	LogType      int    `json:"log_type"`
	OccurredAt   int64  `json:"occurred_at"`
	RevenueQuota int64  `json:"revenue_quota"`
	MarginQuota  int64  `json:"margin_quota"`
	RequestID    string `json:"request_id"`
}

// ResellerMarginDay is one day of margin for the reseller report.
type ResellerMarginDay struct {
	Day          int64 `json:"day" gorm:"column:day"`
	RevenueQuota int64 `json:"revenue_quota"`
	MarginQuota  int64 `json:"margin_quota"`
	RequestCount int64 `json:"request_count"`
}

// GetResellerSummary aggregates one reseller's console home.
func GetResellerSummary(resellerUserId int) (*ResellerSummary, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	summary := &ResellerSummary{ResellerId: resellerUserId}
	var wallet struct {
		Quota int64 `gorm:"column:quota"`
	}
	if err := DB.Model(&User{}).Select("quota").Where("id = ?", resellerUserId).Scan(&wallet).Error; err != nil {
		return nil, err
	}
	summary.WalletQuota = wallet.Quota
	if err := DB.Model(&ResellerCustomerBinding{}).Where("reseller_id = ?", resellerUserId).Count(&summary.CustomerCount).Error; err != nil {
		return nil, err
	}
	var transfer struct {
		Funded         int64 `gorm:"column:funded_quota"`
		Reclaimed      int64 `gorm:"column:reclaimed_quota"`
		FundedCost     int64 `gorm:"column:funded_cost_quota"`
		ReclaimedCost  int64 `gorm:"column:reclaimed_cost_quota"`
		LastActivityAt int64 `gorm:"column:last_activity_at"`
	}
	if err := DB.Model(&ResellerTransfer{}).
		Where("reseller_id = ?", resellerUserId).
		Select(`COALESCE(SUM(CASE WHEN direction = 'fund' THEN retail_quota ELSE 0 END),0) AS funded_quota,
			COALESCE(SUM(CASE WHEN direction = 'reclaim' THEN retail_quota ELSE 0 END),0) AS reclaimed_quota,
			COALESCE(SUM(CASE WHEN direction = 'fund' THEN cost_quota ELSE 0 END),0) AS funded_cost_quota,
			COALESCE(SUM(CASE WHEN direction = 'reclaim' THEN cost_quota ELSE 0 END),0) AS reclaimed_cost_quota,
			COALESCE(MAX(created_at),0) AS last_activity_at`).
		Scan(&transfer).Error; err != nil {
		return nil, err
	}
	summary.FundedQuota = transfer.Funded
	summary.ReclaimedQuota = transfer.Reclaimed
	summary.FundedCostQuota = transfer.FundedCost
	summary.ReclaimedCostQuota = transfer.ReclaimedCost
	summary.LastActivityAt = transfer.LastActivityAt
	var balance struct {
		Total int64 `gorm:"column:total"`
	}
	if err := DB.Model(&User{}).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = users.id").
		Where("b.reseller_id = ?", resellerUserId).
		Select("COALESCE(SUM(users.quota),0) AS total").
		Scan(&balance).Error; err != nil {
		return nil, err
	}
	summary.CustomerBalance = balance.Total
	var usage struct {
		Revenue int64 `gorm:"column:revenue_quota"`
		Margin  int64 `gorm:"column:margin_quota"`
	}
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id = ?", resellerUserId).
		Select("COALESCE(SUM(revenue_quota),0) AS revenue_quota, COALESCE(SUM(reseller_margin_quota),0) AS margin_quota").
		Scan(&usage).Error; err != nil {
		return nil, err
	}
	summary.RevenueQuota, summary.MarginQuota = usage.Revenue, usage.Margin
	return summary, nil
}

// ListResellerCustomers pages the customer list with their wallet state.
func ListResellerCustomers(resellerUserId int, keyword string, afterUserId, limit int) ([]ResellerCustomerRow, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerCustomerRow, 0, limit)
	// `group` is a reserved word in SQLite and MySQL, so the shared dialect-aware
	// column reference is mandatory in raw selects.
	columns := "users.id AS user_id, users.username, users.display_name, users." + commonGroupCol + " AS group_name, users.status, users.quota AS wallet_quota, users.used_quota, users.request_count, COALESCE(t.funded_quota,0) AS funded_quota, COALESCE(t.funded_cost_quota,0) AS funded_cost_quota, COALESCE(t.last_activity_at,0) AS last_activity_at"
	query := DB.Model(&User{}).
		Select(columns).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = users.id").
		Joins(`LEFT JOIN (SELECT customer_user_id,
				SUM(CASE WHEN direction = 'fund' THEN retail_quota ELSE -retail_quota END) AS funded_quota,
				SUM(CASE WHEN direction = 'fund' THEN cost_quota ELSE -cost_quota END) AS funded_cost_quota,
				MAX(created_at) AS last_activity_at
			FROM reseller_transfers GROUP BY customer_user_id) t ON t.customer_user_id = users.id`).
		Where("b.reseller_id = ? AND users.id > ?", resellerUserId, afterUserId)
	if keyword != "" {
		query = query.Where("users.username LIKE ?", "%"+keyword+"%")
	}
	err := query.Order("users.id asc").Limit(limit).Scan(&rows).Error
	return rows, err
}

// GetResellerCustomerDetail returns one owned customer with its prices and the
// most recent wallet transfers.
func GetResellerCustomerDetail(resellerUserId, customerUserId int) (*ResellerCustomerDetail, error) {
	var customer User
	if _, err := requireResellerCustomer(DB, resellerUserId, customerUserId); err != nil {
		return nil, err
	}
	if err := DB.First(&customer, customerUserId).Error; err != nil {
		return nil, err
	}
	detail := &ResellerCustomerDetail{
		ResellerCustomerRow: ResellerCustomerRow{
			UserId:       customer.Id,
			Username:     customer.Username,
			DisplayName:  customer.DisplayName,
			Group:        customer.Group,
			Status:       customer.Status,
			WalletQuota:  int64(customer.Quota),
			UsedQuota:    int64(customer.UsedQuota),
			RequestCount: customer.RequestCount,
		},
		Email: customer.Email,
	}
	prices, err := ListCustomerGroupPrices(customerUserId)
	if err != nil {
		return nil, err
	}
	detail.Prices = prices
	transfers := make([]ResellerTransfer, 0, 20)
	if err := DB.Where("reseller_id = ? AND customer_user_id = ?", resellerUserId, customerUserId).
		Order("id desc").Limit(20).Find(&transfers).Error; err != nil {
		return nil, err
	}
	detail.Transfers = transfers
	retail, err := resellerRemainingTransfer(DB, resellerUserId, customerUserId)
	if err != nil {
		return nil, err
	}
	if retail > 0 {
		detail.ReclaimableQuota = retail
	}
	if detail.ReclaimableQuota > detail.WalletQuota {
		detail.ReclaimableQuota = detail.WalletQuota
	}
	return detail, nil
}

// ListResellerTransfers pages the wallet transfer ledger.
func ListResellerTransfers(resellerUserId, customerUserId int, beforeId int64, limit int) ([]ResellerTransfer, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows := make([]ResellerTransfer, 0, limit)
	query := DB.Where("reseller_id = ?", resellerUserId)
	if customerUserId > 0 {
		query = query.Where("customer_user_id = ?", customerUserId)
	}
	if beforeId > 0 {
		query = query.Where("id < ?", beforeId)
	}
	err := query.Order("id desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ListResellerUsage pages usage over every owned customer. There is
// intentionally no COUNT: the console never blocks on a full-table count.
func ListResellerUsage(resellerUserId, customerId int, beforeId int64, start, end int64, limit int) ([]ResellerUsageRow, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	customerIDs, err := ListResellerCustomerIDs(resellerUserId)
	if err != nil {
		return nil, err
	}
	if len(customerIDs) == 0 {
		return []ResellerUsageRow{}, nil
	}
	rows := make([]ResellerUsageRow, 0, limit)
	query := DB.Model(&CostAccountingSnapshot{}).
		Select("id, user_id AS customer_id, model_name, group_name, log_type, occurred_at, revenue_quota, reseller_margin_quota AS margin_quota, request_id").
		Where("owner_id = ? AND user_id IN ?", resellerUserId, customerIDs)
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
	if err := query.Order("id desc").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	usernames, err := GetUsernamesByIDs(customerIDs)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Username = usernames[rows[i].CustomerId]
	}
	return rows, nil
}

// ListResellerMarginDays aggregates a bounded day range. The caller caps the
// range so the aggregate stays index-bounded.
func ListResellerMarginDays(resellerUserId int, start, end int64) ([]ResellerMarginDay, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	rows := make([]ResellerMarginDay, 0, 31)
	// occurred_at / 86400 is integer division on SQLite, MySQL and PostgreSQL.
	err := DB.Model(&CostAccountingSnapshot{}).
		Select("occurred_at / 86400 AS day, COALESCE(SUM(revenue_quota),0) AS revenue_quota, COALESCE(SUM(reseller_margin_quota),0) AS margin_quota, COUNT(*) AS request_count").
		Where("owner_id = ? AND occurred_at >= ? AND occurred_at < ?", resellerUserId, start, end).
		Group("occurred_at / 86400").
		Order("day asc").
		Scan(&rows).Error
	return rows, err
}

// ResellerCustomerGroups returns the groups the reseller may sell, joined with
// the customer's current effective price.
func ResellerCustomerGroups(resellerUserId, customerUserId int) ([]map[string]any, error) {
	costs := make([]ResellerGroupCost, 0)
	if err := DB.Where("reseller_id = ? AND enabled = ?", resellerUserId, true).Order("group_name asc").Find(&costs).Error; err != nil {
		return nil, err
	}
	prices, err := ListCustomerGroupPrices(customerUserId)
	if err != nil {
		return nil, err
	}
	overrideByGroup := make(map[string]string, len(prices))
	for _, price := range prices {
		overrideByGroup[price.GroupName] = price.Ratio
	}
	result := make([]map[string]any, 0, len(costs))
	for _, cost := range costs {
		row := map[string]any{"group": cost.GroupName, "cost_ratio": cost.CostRatio}
		if ratio, ok := overrideByGroup[cost.GroupName]; ok {
			row["ratio"] = ratio
			row["source"] = "customer"
		} else {
			var defaultPrice ResellerDefaultPrice
			err := DB.Where("reseller_id = ? AND group_name = ?", resellerUserId, cost.GroupName).First(&defaultPrice).Error
			if err == nil {
				row["ratio"] = defaultPrice.Ratio
				row["source"] = "reseller_default"
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			} else {
				row["ratio"] = cost.CostRatio
				row["source"] = "cost"
			}
		}
		result = append(result, row)
	}
	return result, nil
}

// ResellerCustomerLogs pages the usage logs of every owned customer with the
// same masking a normal user sees. The channel column is blanked and the model
// never selects request bodies, so a reseller cannot discover channel identity.
func ResellerCustomerLogs(resellerUserId, customerUserId int, logType int, startTimestamp, endTimestamp int64, modelName string, startIdx, num int) ([]*Log, int64, error) {
	if resellerUserId <= 0 {
		return nil, 0, ErrResellerTransferInvalid
	}
	customerIDs, err := ListResellerCustomerIDs(resellerUserId)
	if err != nil {
		return nil, 0, err
	}
	if customerUserId > 0 {
		found := false
		for _, id := range customerIDs {
			if id == customerUserId {
				found = true
				break
			}
		}
		if !found {
			return nil, 0, ErrResellerCustomerNotFound
		}
		customerIDs = []int{customerUserId}
	}
	if len(customerIDs) == 0 {
		return []*Log{}, 0, nil
	}
	logs := make([]*Log, 0, num)
	tx := LOG_DB.Where("logs.user_id IN ?", customerIDs)
	if logType != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", logType)
	}
	tx = tx.Where("logs.type <> ?", LogTypeBillingCorrection)
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	limitedCountQuery := tx.Session(&gorm.Session{}).Model(&Log{}).Select("logs.id").Limit(logSearchCountLimit)
	var total int64
	if err := LOG_DB.Table("(?) AS limited_logs", limitedCountQuery).Count(&total).Error; err != nil {
		common.SysError("failed to count reseller customer logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	order := "logs.created_at desc, logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	if err := tx.Order(order).Limit(num).Offset(startIdx).Find(&logs).Error; err != nil {
		common.SysError("failed to search reseller customer logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	if err := attachHistoricalTaskLogPricing(logs); err != nil {
		return nil, 0, err
	}
	// A reseller sees exactly what the customer sees: no channel identity, no
	// admin-only fields. formatUserLogs also publishes display ids.
	formatUserLogs(logs, startIdx)
	for i := range logs {
		logs[i].ChannelId = 0
	}
	return logs, total, nil
}

// ResellerCustomerTasks pages the task log of every owned customer with the
// channel stripped, mirroring the normal user view.
func ResellerCustomerTasks(resellerUserId int, params SyncTaskQueryParams, startIdx, num int) ([]*Task, int64, error) {
	if resellerUserId <= 0 {
		return nil, 0, ErrResellerTransferInvalid
	}
	customerIDs, err := ListResellerCustomerIDs(resellerUserId)
	if err != nil {
		return nil, 0, err
	}
	if params.UserID != "" {
		customerId, convErr := strconv.Atoi(params.UserID)
		if convErr != nil || customerId <= 0 {
			return nil, 0, ErrResellerCustomerNotFound
		}
		found := false
		for _, id := range customerIDs {
			if id == customerId {
				found = true
				break
			}
		}
		if !found {
			return nil, 0, ErrResellerCustomerNotFound
		}
		customerIDs = []int{customerId}
	}
	if len(customerIDs) == 0 {
		return []*Task{}, 0, nil
	}
	// A reseller never filters by channel; the parameter is ignored entirely.
	params.ChannelID = ""
	params.UserIDs = customerIDs
	params.UserID = ""
	params.Username = ""
	tasks, err := TaskGetAllTasks(startIdx, num, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := TaskCountAllTasks(params)
	if err != nil {
		return nil, 0, err
	}
	for _, task := range tasks {
		task.ChannelId = 0
	}
	return tasks, total, nil
}
