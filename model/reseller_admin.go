package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertResellerProfile grants or updates the reseller capability. status 0
// keeps the existing value; a new profile defaults to enabled.
func UpsertResellerProfile(userId int, name, code string, status int) (*ResellerProfile, error) {
	if userId <= 0 {
		return nil, errors.New("invalid reseller user")
	}
	if IsAdmin(userId) {
		return nil, errors.New("administrator accounts cannot be resellers")
	}
	now := common.GetTimestamp()
	profile := ResellerProfile{UserId: userId}
	err := DB.Where("user_id = ?", userId).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if status == 0 {
			status = ResellerStatusEnabled
		}
		profile = ResellerProfile{UserId: userId, Name: name, Code: code, Status: status, CreatedAt: now, UpdatedAt: now}
		return &profile, DB.Create(&profile).Error
	}
	if err != nil {
		return nil, err
	}
	if name != "" {
		profile.Name = name
	}
	if code != "" {
		profile.Code = code
	}
	if status != 0 {
		profile.Status = status
	}
	profile.UpdatedAt = now
	return &profile, DB.Model(&profile).Updates(map[string]any{
		"name":       profile.Name,
		"code":       profile.Code,
		"status":     profile.Status,
		"updated_at": now,
	}).Error
}

func ListResellerProfiles() ([]ResellerProfile, error) {
	profiles := make([]ResellerProfile, 0)
	err := DB.Order("id asc").Find(&profiles).Error
	return profiles, err
}

// SetResellerGroupCost is the platform wholesale price. It never touches a
// customer price; it only defines what the reseller pays and which groups the
// reseller may resell.
func SetResellerGroupCost(resellerId int, group, ratio string, enabled bool, actorId int) error {
	if resellerId <= 0 || strings.TrimSpace(group) == "" {
		return errors.New("invalid reseller cost")
	}
	normalized, err := normalizeRatio(ratio)
	if err != nil {
		return err
	}
	if normalized == "" {
		return errors.New("cost ratio is required")
	}
	now := common.GetTimestamp()
	err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "reseller_id"}, {Name: "group_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"cost_ratio", "enabled", "updated_by", "updated_at"}),
	}).Create(&ResellerGroupCost{
		ResellerId: resellerId, GroupName: group, CostRatio: normalized, Enabled: enabled, UpdatedBy: actorId, UpdatedAt: now,
	}).Error
	if err != nil {
		return err
	}
	return InvalidateResellerDefaultPricingCache()
}

// resellerCostRatio returns the wholesale ratio for one group. An enabled
// ResellerGroupCost row wins; otherwise the group falls back to the platform
// group ratio, which is the platform's own cost for that traffic. A group the
// platform does not serve at all returns 0 and is not sellable.
func resellerCostRatio(tx *gorm.DB, resellerId int, group string) (float64, error) {
	var cost ResellerGroupCost
	err := tx.Where("reseller_id = ? AND group_name = ? AND enabled = ?", resellerId, group, true).First(&cost).Error
	if err == nil {
		if ratio, ok := parsePositiveRatio(cost.CostRatio); ok {
			return ratio, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if ratio_setting.ContainsGroupRatio(group) {
		if ratio := ratio_setting.GetGroupRatio(group); ratio > 0 {
			return ratio, nil
		}
	}
	return 0, nil
}

// SetResellerDefaultPrice is the reseller's standing retail price. It may never
// sit below the reseller's own wholesale cost for the same group.
func SetResellerDefaultPrice(resellerId int, group, ratio string, actorId int) error {
	if resellerId <= 0 || strings.TrimSpace(group) == "" {
		return errors.New("invalid reseller price")
	}
	normalized, err := normalizeRatio(ratio)
	if err != nil {
		return err
	}
	if normalized == "" {
		err = DB.Where("reseller_id = ? AND group_name = ?", resellerId, group).Delete(&ResellerDefaultPrice{}).Error
		if err != nil {
			return err
		}
		return InvalidateResellerDefaultPricingCache()
	}
	costRatio, err := resellerCostRatio(DB, resellerId, group)
	if err != nil {
		return err
	}
	if costRatio <= 0 {
		return ErrResellerGroupNotAllowed
	}
	price, _ := parsePositiveRatio(normalized)
	if price < costRatio {
		return ErrResellerPriceBelowCost
	}
	now := common.GetTimestamp()
	err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "reseller_id"}, {Name: "group_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"ratio", "updated_by", "updated_at"}),
	}).Create(&ResellerDefaultPrice{
		ResellerId: resellerId, GroupName: group, Ratio: normalized, UpdatedBy: actorId, UpdatedAt: now,
	}).Error
	if err != nil {
		return err
	}
	return InvalidateResellerDefaultPricingCache()
}

// SetCustomerGroupPrice sets one customer's price for one group. An empty ratio
// clears the override so the reseller default applies again. A reseller actor
// (resellerScoped true) is held to the floor of its own wholesale cost and may
// only price groups it is authorized to resell.
func SetCustomerGroupPrice(userId int, group, ratio, source string, actorId int, resellerScoped bool) error {
	if userId <= 0 || strings.TrimSpace(group) == "" {
		return errors.New("invalid customer price")
	}
	normalized, err := normalizeRatio(ratio)
	if err != nil {
		return err
	}
	if normalized == "" {
		err = DB.Where("user_id = ? AND group_name = ?", userId, group).Delete(&CustomerGroupPrice{}).Error
		if err != nil {
			return err
		}
		return InvalidateResellerPricingCache(userId)
	}
	if resellerScoped {
		owner, err := GetResellerCustomerOwner(userId)
		if err != nil {
			return err
		}
		if owner <= 0 || owner != actorId {
			return ErrResellerCustomerNotFound
		}
		costRatio, err := resellerCostRatio(DB, owner, group)
		if err != nil {
			return err
		}
		if costRatio <= 0 {
			return ErrResellerGroupNotAllowed
		}
		price, _ := parsePositiveRatio(normalized)
		if price < costRatio {
			return ErrResellerPriceBelowCost
		}
	}
	now := common.GetTimestamp()
	err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "group_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"ratio", "source", "updated_by", "updated_at"}),
	}).Create(&CustomerGroupPrice{
		UserId: userId, GroupName: group, Ratio: normalized, Source: source, UpdatedBy: actorId, UpdatedAt: now,
	}).Error
	if err != nil {
		return err
	}
	return InvalidateResellerPricingCache(userId)
}

// ListCustomerGroupPrices returns every per-group price override for one user.
func ListCustomerGroupPrices(userId int) ([]CustomerGroupPrice, error) {
	prices := make([]CustomerGroupPrice, 0)
	if userId <= 0 {
		return prices, errors.New("invalid user id")
	}
	err := DB.Where("user_id = ?", userId).Order("group_name asc").Find(&prices).Error
	return prices, err
}

// ListResellerPricingBook returns the reseller's cost ratios and standing prices
// for the console.
func ListResellerPricingBook(resellerId int) ([]ResellerGroupCost, []ResellerDefaultPrice, error) {
	costs := make([]ResellerGroupCost, 0)
	if err := DB.Where("reseller_id = ?", resellerId).Order("group_name asc").Find(&costs).Error; err != nil {
		return nil, nil, err
	}
	defaults := make([]ResellerDefaultPrice, 0)
	if err := DB.Where("reseller_id = ?", resellerId).Order("group_name asc").Find(&defaults).Error; err != nil {
		return nil, nil, err
	}
	return costs, defaults, nil
}

// normalizeRatio validates an operator-supplied ratio. An empty value is a valid
// "clear" signal and is returned as an empty string.
func normalizeRatio(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	ratio, ok := parsePositiveRatio(raw)
	if !ok {
		return "", errors.New("ratio must be a positive decimal")
	}
	return strconv.FormatFloat(ratio, 'f', -1, 64), nil
}
