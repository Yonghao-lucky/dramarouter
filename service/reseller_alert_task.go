package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// resellerAlertTickInterval is deliberately slow: low-balance mail is a nudge,
// not an operational alarm, and each pass walks every enabled rule.
const resellerAlertTickInterval = 15 * time.Minute

var (
	resellerAlertOnce    sync.Once
	resellerAlertRunning atomic.Bool
)

// StartResellerQuotaAlertTask evaluates the reseller low-balance rules on a
// timer. Only the master node runs it so a multi-instance deployment does not
// mail the same reseller once per container.
func StartResellerQuotaAlertTask() {
	resellerAlertOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("reseller quota alert task started: tick=%s", resellerAlertTickInterval))
			ticker := time.NewTicker(resellerAlertTickInterval)
			defer ticker.Stop()
			for range ticker.C {
				runResellerQuotaAlertOnce()
			}
		})
	})
}

func runResellerQuotaAlertOnce() {
	if !resellerAlertRunning.CompareAndSwap(false, true) {
		return
	}
	defer resellerAlertRunning.Store(false)
	model.RunResellerQuotaAlerts()
}
