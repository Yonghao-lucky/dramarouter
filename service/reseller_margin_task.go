package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const (
	resellerMarginCreditBatchSize   = 500
	resellerMarginCreditOwnerLimit  = 200
	resellerMarginCreditDefaultTick = time.Minute
)

// resellerMarginCreditHandler credits self-pay margin to reseller wallets.
// Pool-path margin is already realized on the reseller wallet by the wholesale
// charge, so only "self" snapshots are credited here.
type resellerMarginCreditHandler struct{}

func (resellerMarginCreditHandler) Type() string { return model.SystemTaskTypeResellerMarginCredit }

func (resellerMarginCreditHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("RESELLER_MARGIN_CREDIT_ENABLED", true)
}

func (resellerMarginCreditHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("RESELLER_MARGIN_CREDIT_INTERVAL_MINUTES", 1)
	if minutes < 1 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

func (resellerMarginCreditHandler) NewPayload() any { return nil }

func (handler resellerMarginCreditHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	credited, err := runResellerMarginCredit(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, map[string]any{"credited_quota": credited}, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(resellerMarginCreditHandler{})
}

// runResellerMarginCredit drains the pending self-pay margin of every owner
// with new snapshots. Each batch is a no-op when replayed, because the ledger
// event key is derived from the watermark range.
func runResellerMarginCredit(ctx context.Context) (int64, error) {
	var credited int64
	for {
		if err := ctx.Err(); err != nil {
			return credited, err
		}
		owners, err := model.ListResellerOwnersPendingMarginCredit(resellerMarginCreditOwnerLimit)
		if err != nil {
			return credited, err
		}
		if len(owners) == 0 {
			return credited, nil
		}
		for _, owner := range owners {
			if err := ctx.Err(); err != nil {
				return credited, err
			}
			batch, err := model.NextResellerMarginCreditBatch(owner, resellerMarginCreditBatchSize)
			if err != nil {
				return credited, err
			}
			if batch.LastSnapshotId == 0 {
				continue
			}
			if _, err := model.CreditResellerMargin(owner, batch); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("reseller margin credit failed: owner=%d cursor=%d err=%v", owner, batch.LastSnapshotId, err))
				continue
			}
			credited += batch.MarginQuota
		}
	}
}
