package model

import (
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
)

// ResellerMarginCreditBatchSize is the page size used both when crediting
// margin and when reporting it. The console must aggregate in exactly the same
// pages as a claim, otherwise it reports a number the wallet will never pay.
const ResellerMarginCreditBatchSize = 500

const resellerClaimableMarginCacheNamespace = "new-api:reseller_claimable_margin:v1"

var (
	resellerClaimableMarginCache     *cachex.HybridCache[int64]
	resellerClaimableMarginCacheOnce sync.Once
)

func resellerClaimableMarginTTL() time.Duration {
	seconds := common.GetEnvOrDefault("RESELLER_CLAIMABLE_MARGIN_CACHE_TTL", 30)
	if seconds <= 0 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func getResellerClaimableMarginCache() *cachex.HybridCache[int64] {
	resellerClaimableMarginCacheOnce.Do(func() {
		ttl := resellerClaimableMarginTTL()
		resellerClaimableMarginCache = cachex.NewHybridCache[int64](cachex.HybridCacheConfig[int64]{
			Namespace: cachex.Namespace(resellerClaimableMarginCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[int64]{},
			Memory: func() *hot.HotCache[string, int64] {
				return hot.NewHotCache[string, int64](hot.LRU, 4096).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return resellerClaimableMarginCache
}

// InvalidateResellerClaimableMarginCache drops the cached claimable figures for
// one owner. Crediting margin changes both the remaining pending amount and the
// lifetime total, so every caller that moves the cursor must drop it.
func InvalidateResellerClaimableMarginCache(ownerId int) error {
	prefix := strconv.Itoa(ownerId) + ":"
	_, err := getResellerClaimableMarginCache().DeleteByPrefix(prefix)
	return err
}

// SumClaimableResellerMargin is what a claim would actually move into the
// wallet for one owner.
//
// A claim reads fixed pages of ResellerMarginCreditBatchSize snapshots in id
// order and credits a page only when the page total is positive; a page that
// nets to zero or below is skipped without debiting the wallet. Reporting the
// plain SUM of every snapshot would therefore disagree with the money, because
// a loss recorded in one page is never netted against profit recorded in
// another. The same paging is applied here so the number on screen is the
// number the wallet receives.
func SumClaimableResellerMargin(ownerId int, afterId int64) (int64, error) {
	if ownerId <= 0 {
		return 0, nil
	}
	cache := getResellerClaimableMarginCache()
	key := strconv.Itoa(ownerId) + ":" + strconv.FormatInt(afterId, 10)
	if cached, found, err := cache.Get(key); err == nil && found {
		return cached, nil
	}
	if DB == nil {
		return 0, nil
	}
	total, err := sumClaimableResellerMargin(ownerId, afterId)
	if err != nil {
		return 0, err
	}
	if err := cache.SetWithTTL(key, total, resellerClaimableMarginTTL()); err != nil {
		common.SysError("failed to cache claimable reseller margin: " + err.Error())
	}
	return total, nil
}

// sumClaimableResellerMargin streams the owner's snapshots in id order and
// reproduces the claim's paging. It reads only the two columns the sum needs so
// the scan stays narrow even for a reseller with a long history.
func sumClaimableResellerMargin(ownerId int, afterId int64) (int64, error) {
	rows, err := DB.Model(&CostAccountingSnapshot{}).
		Select("id", "reseller_margin_quota").
		Where("owner_id = ? AND reseller_settlement = ? AND id > ?", ownerId, "self", afterId).
		Order("id asc").
		Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var total, page int64
	pageRows := 0
	for rows.Next() {
		var id, margin int64
		if err := rows.Scan(&id, &margin); err != nil {
			return 0, err
		}
		page += margin
		pageRows++
		if pageRows == ResellerMarginCreditBatchSize {
			if page > 0 {
				total += page
			}
			page, pageRows = 0, 0
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	// The final, partial page is still a page a claim would credit.
	if pageRows > 0 && page > 0 {
		total += page
	}
	return total, nil
}

// CreditedResellerMargin is the margin already moved into the wallet.
func CreditedResellerMargin(ownerId int) (int64, error) {
	if ownerId <= 0 {
		return 0, nil
	}
	var row struct {
		Total int64 `gorm:"column:total"`
	}
	err := DB.Model(&BillingEntry{}).
		Where("user_id = ? AND event_key LIKE ?", ownerId, "reseller:margin:%").
		Select("COALESCE(SUM(quota),0) AS total").
		Scan(&row).Error
	return row.Total, err
}
