package model

// ResellerOverviewRow is one reseller as the administrator manages it: the
// wallet and everything the reseller console shows for itself, in one row so the
// admin table needs no per-row request.
type ResellerOverviewRow struct {
	ResellerId          int    `json:"reseller_id"`
	Username            string `json:"username"`
	DisplayName         string `json:"display_name"`
	Name                string `json:"name"`
	Code                string `json:"code"`
	Status              int    `json:"status"`
	WalletQuota         int64  `json:"wallet_quota"`
	CustomerCount       int64  `json:"customer_count"`
	CustomerBalance     int64  `json:"customer_balance_quota"`
	FundedQuota         int64  `json:"funded_quota"`
	RevenueQuota        int64  `json:"revenue_quota"`
	MarginQuota         int64  `json:"margin_quota"`
	CreditedMarginQuota int64  `json:"credited_margin_quota"`
	PendingMarginQuota  int64  `json:"pending_margin_quota"`
	GrantedRecharge     int64  `json:"granted_recharge_quota"`
	GrantedPrecredit    int64  `json:"granted_precredit_quota"`
	LastActivityAt      int64  `json:"last_activity_at"`
	CreatedAt           int64  `json:"created_at"`
}

// ListResellerOverviews aggregates every reseller in a fixed number of queries,
// not one per row. A reseller with no customers, no usage and no grants still
// appears, with zeroes, so an operator always sees the account they created.
func ListResellerOverviews() ([]ResellerOverviewRow, error) {
	rows := make([]ResellerOverviewRow, 0, 32)
	err := DB.Model(&ResellerProfile{}).
		Select("reseller_profiles.user_id AS reseller_id, reseller_profiles.name, reseller_profiles.code, reseller_profiles.status, reseller_profiles.created_at, users.username, users.display_name, users.quota AS wallet_quota").
		Joins("LEFT JOIN users ON users.id = reseller_profiles.user_id").
		Order("reseller_profiles.user_id asc").
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return rows, err
	}
	index := make(map[int]int, len(rows))
	for i, row := range rows {
		index[row.ResellerId] = i
	}

	var customerRows []struct {
		Owner int64 `gorm:"column:owner"`
		Count int64 `gorm:"column:count"`
		Quota int64 `gorm:"column:quota"`
	}
	if err := DB.Model(&User{}).
		Joins("JOIN reseller_customer_bindings b ON b.customer_user_id = users.id").
		Select("b.reseller_id AS owner, COUNT(*) AS count, COALESCE(SUM(users.quota),0) AS quota").
		Group("b.reseller_id").
		Scan(&customerRows).Error; err != nil {
		return nil, err
	}
	for _, agg := range customerRows {
		if i, ok := index[int(agg.Owner)]; ok {
			rows[i].CustomerCount, rows[i].CustomerBalance = agg.Count, agg.Quota
		}
	}

	var transferRows []struct {
		Owner int64 `gorm:"column:owner"`
		Fund  int64 `gorm:"column:fund"`
		Last  int64 `gorm:"column:last"`
	}
	if err := DB.Model(&ResellerTransfer{}).
		Select(`reseller_id AS owner,
			COALESCE(SUM(CASE WHEN direction = 'fund' THEN retail_quota ELSE -retail_quota END),0) AS fund,
			MAX(created_at) AS last`).
		Group("reseller_id").
		Scan(&transferRows).Error; err != nil {
		return nil, err
	}
	for _, agg := range transferRows {
		if i, ok := index[int(agg.Owner)]; ok {
			rows[i].FundedQuota, rows[i].LastActivityAt = agg.Fund, agg.Last
		}
	}

	var usageRows []struct {
		Owner   int64 `gorm:"column:owner"`
		Revenue int64 `gorm:"column:revenue"`
		Margin  int64 `gorm:"column:margin"`
		Last    int64 `gorm:"column:last"`
	}
	if err := DB.Model(&CostAccountingSnapshot{}).
		Where("owner_id > 0").
		Select("owner_id AS owner, COALESCE(SUM(revenue_quota),0) AS revenue, COALESCE(SUM(reseller_margin_quota),0) AS margin, MAX(occurred_at) AS last").
		Group("owner_id").
		Scan(&usageRows).Error; err != nil {
		return nil, err
	}
	for _, agg := range usageRows {
		if i, ok := index[int(agg.Owner)]; ok {
			rows[i].RevenueQuota, rows[i].MarginQuota = agg.Revenue, agg.Margin
			if agg.Last > rows[i].LastActivityAt {
				rows[i].LastActivityAt = agg.Last
			}
		}
	}

	// Margin is reported with the same paging a claim uses. A claim credits a page
	// only when the page total is positive, so a plain SUM would report less than
	// the wallet can actually receive. `margin_quota` is everything earned
	// (credited plus still claimable); `pending_margin_quota` is what a claim
	// right now would pay.
	for i := range rows {
		ownerId := rows[i].ResellerId
		pending, err := SumClaimableResellerMargin(ownerId, 0)
		if err != nil {
			return nil, err
		}
		if pending < 0 {
			// A net-negative history earns nothing; never show a negative profit.
			pending = 0
		}
		credited, err := CreditedResellerMargin(ownerId)
		if err != nil {
			return nil, err
		}
		rows[i].PendingMarginQuota = pending
		rows[i].CreditedMarginQuota = credited
		rows[i].MarginQuota = credited + pending
	}

	var grantRows []struct {
		Owner int64  `gorm:"column:owner"`
		Kind  string `gorm:"column:kind"`
		Total int64  `gorm:"column:total"`
	}
	if err := DB.Model(&ResellerWalletGrant{}).
		Select("reseller_id AS owner, kind, COALESCE(SUM(amount),0) AS total").
		Group("reseller_id, kind").
		Scan(&grantRows).Error; err != nil {
		return nil, err
	}
	for _, agg := range grantRows {
		i, ok := index[int(agg.Owner)]
		if !ok {
			continue
		}
		if agg.Kind == ResellerGrantPrecredit {
			rows[i].GrantedPrecredit = agg.Total
		} else {
			rows[i].GrantedRecharge = agg.Total
		}
	}
	return rows, nil
}

// GetResellerOverview returns the same row for one reseller, or nil when the
// user is not a reseller.
func GetResellerOverview(resellerId int) (*ResellerOverviewRow, error) {
	if resellerId <= 0 {
		return nil, ErrResellerGrantInvalid
	}
	rows, err := ListResellerOverviews()
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ResellerId == resellerId {
			return &rows[i], nil
		}
	}
	return nil, nil
}
