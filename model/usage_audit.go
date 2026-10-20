package model

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const MaxUsageAuditRecords = 10000

var ErrUsageAuditTooLarge = errors.New("usage audit exceeds 10000 records; narrow the period or filters")

type UsageAuditFilter struct {
	UserID    int    `json:"user_id"`
	TokenID   int    `json:"token_id"`
	ChannelID int    `json:"channel_id"`
	StartAt   int64  `json:"start_at"`
	EndAt     int64  `json:"end_at"`
	Model     string `json:"model"`
	Group     string `json:"group"`
	Protocol  string `json:"protocol"`
}

// Query immutable logs and then apply the same effective-amount projection as
// the consumption UI. Main and log databases may be on different servers.
func GetUsageAuditLogs(ctx context.Context, filter UsageAuditFilter) ([]*Log, error) {
	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("type IN ? AND created_at >= ? AND created_at < ?", []int{LogTypeConsume, LogTypeRefund}, filter.StartAt, filter.EndAt)
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.TokenID > 0 {
		query = query.Where("token_id = ?", filter.TokenID)
	}
	if filter.ChannelID > 0 {
		query = query.Where("channel_id = ?", filter.ChannelID)
	}
	if filter.Model != "" {
		query = query.Where("model_name = ?", filter.Model)
	}
	var adjustments []billingEffectiveLog
	var err error
	if filter.Group != "" {
		adjustments, err = billingEffectiveLogs(ctx, CostAccountingFilter{UserID: filter.UserID, StartTimestamp: filter.StartAt, EndTimestamp: filter.EndAt - 1, ModelName: filter.Model})
		if err != nil {
			return nil, err
		}
		query = applyBillingEffectiveGroup(query, filter.Group, adjustments)
	}
	order := "created_at asc, id asc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = "created_at asc, request_id asc"
	}
	logs := make([]*Log, 0)
	err = query.Select("id", "user_id", "username", "created_at", "type", "token_id", "token_name", "channel_id", "model_name", "group", "request_id", "upstream_request_id", "prompt_tokens", "completion_tokens", "quota", "is_stream", "other").
		Order(order).Limit(MaxUsageAuditRecords + 1).Find(&logs).Error
	if err != nil {
		return nil, err
	}
	if len(logs) > MaxUsageAuditRecords {
		return nil, ErrUsageAuditTooLarge
	}
	if filter.Group == "" {
		adjustments, err = billingEffectiveLogsForPage(ctx, logs)
		if err != nil {
			return nil, err
		}
	}
	if err := projectBillingEffectiveLogs(logs, adjustments); err != nil {
		return nil, err
	}
	return logs, nil
}

type UsageAuditKey struct {
	ID     int    `json:"id"`
	UserID int    `json:"user_id"`
	Name   string `json:"name"`
}

// Include deleted keys for historical searches, but never load their secrets.
func SearchUsageAuditKeys(ctx context.Context, userID int, keyword string) ([]UsageAuditKey, error) {
	query := DB.WithContext(ctx).Unscoped().Model(&Token{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
			query = query.Where("id = ?", id)
		} else {
			query = query.Where("name LIKE ?", "%"+strings.NewReplacer("%", "", "_", "").Replace(keyword)+"%")
		}
	}
	keys := make([]UsageAuditKey, 0)
	err := query.Select("id", "user_id", "name").Order("id desc").Limit(50).Find(&keys).Error
	return keys, err
}
