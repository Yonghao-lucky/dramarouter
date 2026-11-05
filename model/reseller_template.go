package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResellerPriceTemplate is a reusable set of per-group retail ratios. A reseller
// builds one template and applies it to whichever customers it chooses, instead
// of typing every ratio for every customer. Applying a template only writes
// CustomerGroupPrice rows for the groups the template covers and the reseller is
// authorized to sell; nothing is applied automatically.
type ResellerPriceTemplate struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	ResellerId int    `json:"reseller_id" gorm:"index:idx_reseller_price_template_owner,priority:1"`
	Name       string `json:"name" gorm:"type:varchar(64)"`
	Remark     string `json:"remark" gorm:"type:varchar(255)"`
	CreatedBy  int    `json:"created_by"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

// ResellerPriceTemplateItem is one group ratio inside a template.
type ResellerPriceTemplateItem struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	TemplateId int    `json:"template_id" gorm:"uniqueIndex:idx_reseller_template_group,priority:1"`
	GroupName  string `json:"group" gorm:"type:varchar(64);uniqueIndex:idx_reseller_template_group,priority:2"`
	Ratio      string `json:"ratio" gorm:"type:varchar(64)"`
}

var (
	ErrResellerTemplateInvalid = errors.New("invalid price template")
	ErrResellerTemplateEmpty   = errors.New("price template has no groups")
)

// ListResellerPriceTemplates returns a reseller's templates with their items.
func ListResellerPriceTemplates(resellerId int) ([]ResellerPriceTemplate, map[int][]ResellerPriceTemplateItem, error) {
	templates := make([]ResellerPriceTemplate, 0)
	if resellerId <= 0 {
		return nil, nil, ErrResellerTemplateInvalid
	}
	if err := DB.Where("reseller_id = ?", resellerId).Order("id asc").Find(&templates).Error; err != nil {
		return nil, nil, err
	}
	items := make([]ResellerPriceTemplateItem, 0)
	if len(templates) > 0 {
		ids := make([]int, 0, len(templates))
		for _, tpl := range templates {
			ids = append(ids, tpl.Id)
		}
		if err := DB.Where("template_id IN ?", ids).Order("group_name asc").Find(&items).Error; err != nil {
			return nil, nil, err
		}
	}
	byTemplate := make(map[int][]ResellerPriceTemplateItem, len(templates))
	for _, item := range items {
		byTemplate[item.TemplateId] = append(byTemplate[item.TemplateId], item)
	}
	return templates, byTemplate, nil
}

type ResellerPriceTemplateInput struct {
	Name   string
	Remark string
	Items  map[string]string // group -> ratio
}

// UpsertResellerPriceTemplate creates or replaces one template. Ratios are
// validated against the reseller's own wholesale cost, so a template can never
// encode a loss-making price.
func UpsertResellerPriceTemplate(resellerId, actorId int, templateId int, input ResellerPriceTemplateInput) (*ResellerPriceTemplate, error) {
	if resellerId <= 0 {
		return nil, ErrResellerTemplateInvalid
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrResellerTemplateInvalid
	}
	clean := make(map[string]string, len(input.Items))
	for group, raw := range input.Items {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		normalized, err := normalizeRatio(raw)
		if err != nil || normalized == "" {
			return nil, ErrResellerTemplateInvalid
		}
		cost, err := resellerCostRatio(DB, resellerId, group)
		if err != nil {
			return nil, err
		}
		if cost <= 0 {
			return nil, ErrResellerGroupNotAllowed
		}
		price, ok := parsePositiveRatio(normalized)
		if !ok || price < cost {
			return nil, ErrResellerPriceBelowCost
		}
		clean[group] = normalized
	}
	if len(clean) == 0 {
		return nil, ErrResellerTemplateEmpty
	}

	now := common.GetTimestamp()
	var saved ResellerPriceTemplate
	err := DB.Transaction(func(tx *gorm.DB) error {
		if templateId > 0 {
			var existing ResellerPriceTemplate
			if err := tx.Where("id = ? AND reseller_id = ?", templateId, resellerId).First(&existing).Error; err != nil {
				return ErrResellerTemplateInvalid
			}
			existing.Name, existing.Remark, existing.UpdatedAt = truncateResellerText(name, 64), truncateResellerText(input.Remark, 255), now
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if err := tx.Where("template_id = ?", existing.Id).Delete(&ResellerPriceTemplateItem{}).Error; err != nil {
				return err
			}
			saved = existing
		} else {
			saved = ResellerPriceTemplate{
				ResellerId: resellerId,
				Name:       truncateResellerText(name, 64),
				Remark:     truncateResellerText(input.Remark, 255),
				CreatedBy:  actorId,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := tx.Create(&saved).Error; err != nil {
				return err
			}
		}
		items := make([]ResellerPriceTemplateItem, 0, len(clean))
		for group, ratio := range clean {
			items = append(items, ResellerPriceTemplateItem{TemplateId: saved.Id, GroupName: group, Ratio: ratio})
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func DeleteResellerPriceTemplate(resellerId, templateId int) error {
	if resellerId <= 0 || templateId <= 0 {
		return ErrResellerTemplateInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND reseller_id = ?", templateId, resellerId).Delete(&ResellerPriceTemplate{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrResellerTemplateInvalid
		}
		return tx.Where("template_id = ?", templateId).Delete(&ResellerPriceTemplateItem{}).Error
	})
}

// ApplyResellerPriceTemplate writes the template's ratios onto the given
// customers. Customers outside the reseller's book are rejected, and the write
// is all-or-nothing so a partial application never leaves mixed prices.
func ApplyResellerPriceTemplate(resellerId, actorId, templateId int, customerIds []int) (int, error) {
	if resellerId <= 0 || templateId <= 0 || len(customerIds) == 0 {
		return 0, ErrResellerTemplateInvalid
	}
	var tpl ResellerPriceTemplate
	if err := DB.Where("id = ? AND reseller_id = ?", templateId, resellerId).First(&tpl).Error; err != nil {
		return 0, ErrResellerTemplateInvalid
	}
	items := make([]ResellerPriceTemplateItem, 0)
	if err := DB.Where("template_id = ?", tpl.Id).Find(&items).Error; err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, ErrResellerTemplateEmpty
	}

	applied := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		for _, customerId := range customerIds {
			if _, err := requireResellerCustomer(tx, resellerId, customerId); err != nil {
				return err
			}
			for _, item := range items {
				err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "user_id"}, {Name: "group_name"}},
					DoUpdates: clause.AssignmentColumns([]string{"ratio", "source", "updated_by", "updated_at"}),
				}).Create(&CustomerGroupPrice{
					UserId: customerId, GroupName: item.GroupName, Ratio: item.Ratio,
					Source: "reseller", UpdatedBy: actorId, UpdatedAt: now,
				}).Error
				if err != nil {
					return err
				}
			}
			applied++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, customerId := range customerIds {
		_ = InvalidateResellerPricingCache(customerId)
	}
	return applied, nil
}
