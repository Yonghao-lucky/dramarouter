package model

import "gorm.io/gorm"

// AttachResellerLogMargins fills the reseller share on a page of customer logs.
// The evidence is the cost accounting snapshot written by the billing path for
// that exact log, so the number shown on a log is the number that was actually
// settled, not a recomputation from today's prices. Only snapshots owned by the
// asking reseller are considered, so one reseller can never see another's
// margin on a shared customer.
func AttachResellerLogMargins(ownerId int, logs []*Log) error {
	if ownerId <= 0 || len(logs) == 0 {
		return nil
	}
	logIDs := make([]int64, 0, len(logs))
	for _, log := range logs {
		if log.Id > 0 && log.UserId != ownerId {
			logIDs = append(logIDs, int64(log.Id))
		}
	}
	// A reseller's own funding rows carry no reseller margin, and querying them
	// would only widen the IN list.
	if len(logIDs) == 0 {
		return nil
	}
	rows := make([]CostAccountingSnapshot, 0, len(logIDs))
	if err := DB.Where("owner_id = ? AND source_log_id IN ?", ownerId, logIDs).
		Select("source_log_id, reseller_margin_quota, retail_ratio, reseller_cost_ratio").
		Find(&rows).Error; err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	byLogID := make(map[int64]CostAccountingSnapshot, len(rows))
	for _, row := range rows {
		byLogID[row.SourceLogID] = row
	}
	for _, log := range logs {
		snapshot, ok := byLogID[int64(log.Id)]
		if !ok {
			continue
		}
		margin := snapshot.ResellerMarginQuota
		retail := snapshot.RetailRatio
		cost := snapshot.ResellerCostRatio
		log.ResellerMarginQuota = &margin
		log.ResellerRetailRatio = &retail
		log.ResellerCostRatio = &cost
	}
	return nil
}
