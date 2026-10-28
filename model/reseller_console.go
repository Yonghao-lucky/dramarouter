package model

import (
	"errors"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
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
	// PendingMarginQuota is earned margin that the reseller has not yet moved
	// into the wallet. Claiming it is an explicit action in the console.
	PendingMarginQuota int64 `json:"pending_margin_quota"`
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
	FundedCost     int64  `json:"funded_cost_quota" gorm:"column:funded_cost_quota"`
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
	pending, err := PendingResellerMarginQuota(resellerUserId)
	if err != nil {
		return nil, err
	}
	summary.PendingMarginQuota = pending
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
	// The drawer shows the same wallet state as the list row, so the transfer
	// aggregate is filled here too instead of leaving the funded columns at zero.
	var transfersAgg struct {
		Funded         int64 `gorm:"column:funded_quota"`
		FundedCost     int64 `gorm:"column:funded_cost_quota"`
		LastActivityAt int64 `gorm:"column:last_activity_at"`
	}
	if err := DB.Model(&ResellerTransfer{}).
		Where("reseller_id = ? AND customer_user_id = ?", resellerUserId, customerUserId).
		Select(`COALESCE(SUM(CASE WHEN direction = 'fund' THEN retail_quota ELSE -retail_quota END),0) AS funded_quota,
			COALESCE(SUM(CASE WHEN direction = 'fund' THEN cost_quota ELSE -cost_quota END),0) AS funded_cost_quota,
			COALESCE(MAX(created_at),0) AS last_activity_at`).
		Scan(&transfersAgg).Error; err != nil {
		return nil, err
	}
	detail.FundedQuota = transfersAgg.Funded
	detail.FundedCost = transfersAgg.FundedCost
	detail.LastActivityAt = transfersAgg.LastActivityAt
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

// ResellerPlatformGroupRow is one platform group as a reseller sees it: what
// the group costs the reseller (when it has a wholesale row) and the price a
// specific customer would be charged there.
type ResellerPlatformGroupRow struct {
	Group     string `json:"group"`
	CostRatio string `json:"cost_ratio"`
	Ratio     string `json:"ratio"`
	Source    string `json:"source"`
	// Sellable is false for a group without an enabled wholesale cost. Such a
	// group is still usable by the customer, but the reseller earns nothing on
	// it and cannot set a price floor there.
	Sellable bool `json:"sellable"`
}

// platformGroupNames returns every group the platform serves, sorted. The
// reseller's price book may cover a subset; customer placement is independent
// of it.
func platformGroupNames(extra ...string) []string {
	names := make(map[string]struct{}, 32)
	for name := range setting.GetUserUsableGroupsCopy() {
		names[name] = struct{}{}
	}
	for _, name := range extra {
		if name != "" {
			names[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// ResellerPlatformGroups lists every platform group with the caller's cost and
// standing price. customerUserId is 0 when only the price book is needed.
func ResellerPlatformGroups(resellerUserId, customerUserId int) ([]ResellerPlatformGroupRow, error) {
	if resellerUserId <= 0 {
		return nil, ErrResellerTransferInvalid
	}
	costs := make([]ResellerGroupCost, 0)
	if err := DB.Where("reseller_id = ? AND enabled = ?", resellerUserId, true).Find(&costs).Error; err != nil {
		return nil, err
	}
	costByGroup := make(map[string]string, len(costs))
	extra := make([]string, 0, len(costs))
	for _, cost := range costs {
		costByGroup[cost.GroupName] = cost.CostRatio
		extra = append(extra, cost.GroupName)
	}
	defaults := make([]ResellerDefaultPrice, 0)
	if err := DB.Where("reseller_id = ?", resellerUserId).Find(&defaults).Error; err != nil {
		return nil, err
	}
	defaultByGroup := make(map[string]string, len(defaults))
	for _, price := range defaults {
		defaultByGroup[price.GroupName] = price.Ratio
	}
	overrideByGroup := make(map[string]string)
	if customerUserId > 0 {
		prices, err := ListCustomerGroupPrices(customerUserId)
		if err != nil {
			return nil, err
		}
		for _, price := range prices {
			overrideByGroup[price.GroupName] = price.Ratio
		}
	}

	groupNames := platformGroupNames(extra...)
	rows := make([]ResellerPlatformGroupRow, 0, len(groupNames))
	for _, name := range groupNames {
		row := ResellerPlatformGroupRow{Group: name, CostRatio: costByGroup[name]}
		// A group without an explicit wholesale row still costs the reseller the
		// platform's own group ratio, which is exactly what the billing path
		// charges against when it computes the margin.
		if row.CostRatio == "" {
			if ratio, err := resellerCostRatio(DB, resellerUserId, name); err != nil {
				return nil, err
			} else if ratio > 0 {
				row.CostRatio = strconv.FormatFloat(ratio, 'f', -1, 64)
			}
		}
		row.Sellable = row.CostRatio != ""
		switch {
		case overrideByGroup[name] != "":
			row.Ratio, row.Source = overrideByGroup[name], "customer"
		case defaultByGroup[name] != "":
			row.Ratio, row.Source = defaultByGroup[name], "reseller_default"
		case row.CostRatio != "":
			row.Ratio, row.Source = row.CostRatio, "cost"
		default:
			row.Source = "platform"
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// ResellerCustomerGroups returns every platform group with the customer's
// effective price. Groups outside the reseller's price book are included with
// an empty cost so the console can show that the customer may use them but the
// reseller earns nothing there.
func ResellerCustomerGroups(resellerUserId, customerUserId int) ([]ResellerPlatformGroupRow, error) {
	return ResellerPlatformGroups(resellerUserId, customerUserId)
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
