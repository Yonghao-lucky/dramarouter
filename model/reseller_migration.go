package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// migrateResellerOwnerKeys rewrites the legacy reseller owner key from the
// profile primary key to the reseller's own user id.
//
// Bindings, cost rows, default prices and accounting snapshots used to store
// reseller_profiles.id. Every wallet operation also reads users.quota, so that
// key could resolve to an unrelated account — the defect this rewrite removes.
// The rewrite is idempotent: a value is only rewritten when it matches a profile
// primary key and no profile already uses it as its user id.
func migrateResellerOwnerKeys() error {
	if DB == nil {
		return nil
	}
	profiles := make([]ResellerProfile, 0)
	if err := DB.Select("id", "user_id").Find(&profiles).Error; err != nil {
		return err
	}
	if len(profiles) == 0 {
		return nil
	}
	userIDs := make(map[int]struct{}, len(profiles))
	for _, profile := range profiles {
		userIDs[profile.UserId] = struct{}{}
	}
	remap := make(map[int]int, len(profiles))
	for _, profile := range profiles {
		if profile.UserId <= 0 || profile.UserId == profile.Id {
			continue
		}
		// A profile whose user id already equals some other profile's primary key
		// would make the rewrite ambiguous; leave that row for an operator.
		if _, isResellerUser := userIDs[profile.Id]; isResellerUser {
			continue
		}
		remap[profile.Id] = profile.UserId
	}
	if len(remap) == 0 {
		return nil
	}

	for _, table := range []string{"reseller_group_costs", "reseller_default_prices", "reseller_customer_bindings", "cost_accounting_snapshots"} {
		column := "reseller_id"
		if table == "cost_accounting_snapshots" {
			column = "owner_id"
		}
		var exists int64
		if err := DB.Table(table).Where(column+" IN ?", keysOf(remap)).Count(&exists).Error; err != nil {
			// A missing table on a fresh install is not a failure.
			continue
		}
		if exists == 0 {
			continue
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			for from, to := range remap {
				if err := tx.Table(table).Where(column+" = ?", from).Update(column, to).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return errors.Join(errors.New("failed to migrate reseller owner key on "+table), err)
		}
		common.SysLog("migrated reseller owner key on " + table)
	}
	return nil
}

func keysOf(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
