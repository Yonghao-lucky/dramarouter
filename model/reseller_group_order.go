package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// maxResellerGroupOrder bounds one saved ordering so a malformed client cannot
// write an unbounded number of rows for a single reseller.
const maxResellerGroupOrder = 500

// ResellerGroupOrder is one reseller's preferred price-book ordering. The list
// is flat: vendor sections are derived from the first appearance of a provider,
// so saving a flat order is enough to express both "which group comes first"
// and "which vendor card comes first".
type ResellerGroupOrder struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	ResellerId int    `json:"reseller_id" gorm:"uniqueIndex:idx_reseller_group_order,priority:1"`
	GroupName  string `json:"group" gorm:"type:varchar(64);uniqueIndex:idx_reseller_group_order,priority:2"`
	Position   int    `json:"position" gorm:"index"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

// LoadResellerGroupOrder returns group name -> position. An empty map means the
// reseller never reordered the price book and the default ordering applies.
func LoadResellerGroupOrder(resellerId int) (map[string]int, error) {
	positions := make(map[string]int, 32)
	if resellerId <= 0 {
		return positions, nil
	}
	rows := make([]ResellerGroupOrder, 0, 32)
	if err := DB.Where("reseller_id = ?", resellerId).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		positions[row.GroupName] = row.Position
	}
	return positions, nil
}

// SaveResellerGroupOrder replaces the whole ordering in one transaction. A
// later save always wins, so the client never has to diff the list.
func SaveResellerGroupOrder(resellerId int, groups []string) error {
	if resellerId <= 0 {
		return ErrResellerTransferInvalid
	}
	if len(groups) > maxResellerGroupOrder {
		return errors.New("group order is too long")
	}
	now := common.GetTimestamp()
	rows := make([]ResellerGroupOrder, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if group == "" {
			continue
		}
		if _, dup := seen[group]; dup {
			continue
		}
		seen[group] = struct{}{}
		rows = append(rows, ResellerGroupOrder{
			ResellerId: resellerId,
			GroupName:  group,
			Position:   len(rows),
			UpdatedAt:  now,
		})
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reseller_id = ?", resellerId).Delete(&ResellerGroupOrder{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	})
}
