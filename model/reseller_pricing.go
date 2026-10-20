package model

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
	"gorm.io/gorm"
)

const (
	resellerPricingCacheNamespace = "new-api:reseller_pricing:v1"
	resellerPricingCacheCapacity  = 20000
)

var (
	resellerPricingCacheOnce sync.Once
	resellerPricingCache     *cachex.HybridCache[ResellerPricing]
)

// ResellerPricing is the pricing layer owned by the reseller subsystem. It is
// additive: when Override is false the caller keeps the existing group-ratio
// result untouched.
type ResellerPricing struct {
	OwnerID   int
	Retail    float64
	CostRatio float64
	Source    string // customer | reseller_default
	Override  bool
}

func resellerPricingCacheTTL() time.Duration {
	ttlSeconds := common.GetEnvOrDefault("RESELLER_PRICING_CACHE_TTL", 30)
	if ttlSeconds <= 0 {
		ttlSeconds = 30
	}
	return time.Duration(ttlSeconds) * time.Second
}

func getResellerPricingCache() *cachex.HybridCache[ResellerPricing] {
	resellerPricingCacheOnce.Do(func() {
		ttl := resellerPricingCacheTTL()
		resellerPricingCache = cachex.NewHybridCache[ResellerPricing](cachex.HybridCacheConfig[ResellerPricing]{
			Namespace: cachex.Namespace(resellerPricingCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[ResellerPricing]{},
			Memory: func() *hot.HotCache[string, ResellerPricing] {
				return hot.NewHotCache[string, ResellerPricing](hot.LRU, resellerPricingCacheCapacity).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return resellerPricingCache
}

func resellerPricingCacheKey(userID int, usingGroup string) string {
	return strconv.Itoa(userID) + ":" + usingGroup
}

// InvalidateResellerPricingCache drops every cached entry for one customer.
// Callers that change a customer price, an ownership binding, or a reseller
// default price must invoke it.
func InvalidateResellerPricingCache(userID int) error {
	_, err := getResellerPricingCache().DeleteByPrefix(strconv.Itoa(userID) + ":")
	return err
}

// InvalidateResellerDefaultPricingCache is used when a reseller changes a
// standing price, which affects every owned customer without a override.
func InvalidateResellerDefaultPricingCache() error {
	return getResellerPricingCache().Purge()
}

func parsePositiveRatio(raw string) (float64, bool) {
	ratio, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || ratio <= 0 {
		return 0, false
	}
	return ratio, true
}

// ResolveResellerPricing resolves the reseller pricing layer for one request.
// Priority: customer-specific price > reseller default price. When neither
// applies, Override is false and the existing group-ratio chain stays in
// charge.
func ResolveResellerPricing(userID int, usingGroup string) (ResellerPricing, error) {
	if userID <= 0 || usingGroup == "" {
		return ResellerPricing{}, nil
	}
	cache := getResellerPricingCache()
	key := resellerPricingCacheKey(userID, usingGroup)
	if cached, found, err := cache.Get(key); err == nil && found {
		return cached, nil
	}

	// Unit tests and early boot may run without a database. There is no reseller
	// pricing to resolve then, and the existing group ratio chain stays in charge.
	if DB == nil {
		return ResellerPricing{}, nil
	}
	result, err := loadResellerPricing(userID, usingGroup)
	if err != nil {
		return ResellerPricing{}, err
	}
	if err := cache.SetWithTTL(key, result, resellerPricingCacheTTL()); err != nil {
		common.SysError("failed to cache reseller pricing: " + err.Error())
	}
	return result, nil
}

func loadResellerPricing(userID int, usingGroup string) (ResellerPricing, error) {
	ownerID, err := GetResellerCustomerOwner(userID)
	if err != nil {
		return ResellerPricing{}, err
	}
	result := ResellerPricing{OwnerID: ownerID}

	var customerPrice CustomerGroupPrice
	err = DB.Where("user_id = ? AND group_name = ?", userID, usingGroup).First(&customerPrice).Error
	if err == nil {
		if ratio, ok := parsePositiveRatio(customerPrice.Ratio); ok {
			result.Retail, result.Override, result.Source = ratio, true, "customer"
		}
	} else if err != gorm.ErrRecordNotFound {
		return ResellerPricing{}, err
	}

	if ownerID > 0 {
		var cost ResellerGroupCost
		if err := DB.Where("reseller_id = ? AND group_name = ? AND enabled = ?", ownerID, usingGroup, true).First(&cost).Error; err == nil {
			if ratio, ok := parsePositiveRatio(cost.CostRatio); ok {
				result.CostRatio = ratio
			}
		} else if err != gorm.ErrRecordNotFound {
			return ResellerPricing{}, err
		}

		if !result.Override {
			var defaultPrice ResellerDefaultPrice
			err := DB.Where("reseller_id = ? AND group_name = ?", ownerID, usingGroup).First(&defaultPrice).Error
			if err == nil {
				if ratio, ok := parsePositiveRatio(defaultPrice.Ratio); ok {
					result.Retail, result.Override, result.Source = ratio, true, "reseller_default"
				}
			} else if err != gorm.ErrRecordNotFound {
				return ResellerPricing{}, err
			}
		}
	}

	// A customer override also carries the reseller cost so the frozen margin is
	// computable in one pass.
	return result, nil
}

// ResellerMarginQuota derives the reseller share for one charge. Retail is what
// the customer paid, costRatio the reseller's wholesale ratio, retailRatio the
// effective retail ratio. Rounding goes through the shared quota helper.
func ResellerWholesaleQuota(retailQuota int64, retailRatio, costRatio float64) int64 {
	if retailQuota == 0 || retailRatio <= 0 || costRatio <= 0 {
		return 0
	}
	return int64(common.QuotaRound(float64(retailQuota) * costRatio / retailRatio))
}

// ResellerMarginQuota is exactly retail minus wholesale, so the frozen margin
// plus the wholesale charge always re-add to the customer's retail charge.
func ResellerMarginQuota(retailQuota int64, retailRatio, costRatio float64) int64 {
	if retailQuota == 0 || retailRatio <= 0 || costRatio <= 0 {
		return 0
	}
	return retailQuota - ResellerWholesaleQuota(retailQuota, retailRatio, costRatio)
}

// EffectiveRatio returns the ratio this customer must be charged for the group.
// Unoverridden customers keep the caller's existing group ratio.
func (p ResellerPricing) EffectiveRatio(fallback float64) float64 {
	if p.Override && p.Retail > 0 {
		return p.Retail
	}
	return fallback
}

// PoolEnabled reports whether the reseller-funded path can pay for this
// customer. A reseller that never configured a wholesale cost cannot fund a
// pool: the customer falls back to their own wallet and no margin is derived.
func (p ResellerPricing) PoolEnabled() bool {
	return p.OwnerID > 0 && p.Override && p.Retail > 0 && p.CostRatio > 0
}
