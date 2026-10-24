package service

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func IsUserSelectableGroup(userGroup, groupName string) bool {
	if groupName == "" || groupName == "auto" {
		return false
	}
	return GroupInUserUsableGroups(userGroup, groupName) && ratio_setting.ContainsGroupRatio(groupName)
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	autoGroups := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range setting.GetAutoGroups() {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		autoGroups = append(autoGroups, group)
	}
	return autoGroups
}

// FilterUserTokenAutoGroups applies current permissions before the current
// per-token limit. It intentionally does not fall back to the global Auto list.
func FilterUserTokenAutoGroups(userGroup string, groups []string) []string {
	maxCount := setting.GetMaxTokenAutoGroups()
	filtered := make([]string, 0, min(len(groups), maxCount))
	seen := make(map[string]struct{})
	for _, group := range groups {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		filtered = append(filtered, group)
		if len(filtered) == maxCount {
			break
		}
	}
	return filtered
}

// GetRequestAutoGroups resolves the ordered Auto groups for the current token.
// The absence of the context value means that the token inherits the complete
// global Auto list; a present (even empty) value is an explicit token snapshot.
func GetRequestAutoGroups(c *gin.Context, userGroup string) []string {
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenAutoGroups)
	if !ok {
		return GetUserAutoGroup(userGroup)
	}
	groups, ok := value.([]string)
	if !ok {
		return []string{}
	}
	return FilterUserTokenAutoGroups(userGroup, groups)
}

// GetRequestVendorGroups resolves the ordered Auto groups for the requested
// model. A token may pin a different order per model vendor, because one group
// can serve models from several vendors. Vendors without a configured order
// fall back to the token's Auto list; when that list is empty too, the union of
// all configured vendor routes keeps the key usable.
func GetRequestVendorGroups(c *gin.Context, userGroup string, modelName string) []string {
	fallback := GetRequestAutoGroups(c, userGroup)
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenVendorAutoGroups)
	if !ok {
		return fallback
	}
	orders, ok := value.([]model.VendorAutoGroupOrder)
	if !ok || len(orders) == 0 {
		return fallback
	}
	resolveFallback := func() []string {
		if len(fallback) > 0 {
			return fallback
		}
		// 只配置了厂商顺序的 Key：用全部厂商线路的并集兜底，避免其他厂商无路可走。
		union := make([]string, 0, len(orders))
		seen := make(map[string]struct{}, len(orders))
		for _, order := range orders {
			for _, group := range order.Groups {
				if _, exists := seen[group]; exists {
					continue
				}
				seen[group] = struct{}{}
				union = append(union, group)
			}
		}
		return FilterUserTokenAutoGroups(userGroup, union)
	}

	vendorID := model.GetModelVendorID(modelName)
	if vendorID <= 0 {
		return resolveFallback()
	}
	for _, order := range orders {
		if order.VendorId != vendorID {
			continue
		}
		groups := FilterUserTokenAutoGroups(userGroup, order.Groups)
		if len(groups) > 0 {
			return groups
		}
		return resolveFallback()
	}
	return resolveFallback()
}

// GetRequestAutoGroupCandidates 返回当前令牌在聚合分组下可以走到的全部分组：
// Token/全局 Auto 列表，加上各厂商顺序中出现过的分组。
// 后者只用于模型列表展示，实际路由仍由 GetRequestVendorGroups 逐模型解析。
func GetRequestAutoGroupCandidates(c *gin.Context, userGroup string) []string {
	candidates := GetRequestAutoGroups(c, userGroup)
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenVendorAutoGroups)
	if !ok {
		return candidates
	}
	orders, ok := value.([]model.VendorAutoGroupOrder)
	if !ok || len(orders) == 0 {
		return candidates
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, group := range candidates {
		seen[group] = struct{}{}
	}
	for _, order := range orders {
		// 与路由侧一致：每个厂商顺序先过一遍用户权限与数量上限。
		for _, group := range FilterUserTokenAutoGroups(userGroup, order.Groups) {
			if _, exists := seen[group]; exists {
				continue
			}
			seen[group] = struct{}{}
			candidates = append(candidates, group)
		}
	}
	return candidates
}

// FilterModelsByVendorRoute 只保留当前令牌厂商顺序下确实可达的模型。
// 分组里启用了模型，不代表该模型在厂商顺序指向的分组里也有渠道；
// 这种模型列进 /v1/models 后必然调用失败，因此按逐模型解析结果过滤。
// 未配置厂商顺序的令牌原样返回，行为不变。
func FilterModelsByVendorRoute(c *gin.Context, userGroup string, modelNames []string) []string {
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenVendorAutoGroups)
	if !ok {
		return modelNames
	}
	orders, ok := value.([]model.VendorAutoGroupOrder)
	if !ok || len(orders) == 0 {
		return modelNames
	}
	filtered := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		routes := GetRequestVendorGroups(c, userGroup, modelName)
		if len(routes) == 0 {
			continue
		}
		enableGroups := model.GetModelEnableGroups(modelName)
		if len(enableGroups) == 0 {
			// 定价缓存尚未覆盖该模型时无法判断可达性，保留以免误删。
			filtered = append(filtered, modelName)
			continue
		}
		reachable := slices.ContainsFunc(routes, func(route string) bool {
			return slices.Contains(enableGroups, route)
		})
		if reachable {
			filtered = append(filtered, modelName)
		}
	}
	return filtered
}

// GetGroupsEnabledModels 按 groups 顺序获取各分组启用的模型并去重
func GetGroupsEnabledModels(groups []string) []string {
	modelsByGroup, err := model.GetGroupEnabledModelsMap(groups)
	if err != nil {
		common.SysError("failed to load enabled models by group: " + err.Error())
		return []string{}
	}
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, group := range groups {
		for _, modelName := range modelsByGroup[group] {
			if _, ok := seen[modelName]; !ok {
				seen[modelName] = struct{}{}
				models = append(models, modelName)
			}
		}
	}
	return models
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userID int, userGroup, group string) float64 {
	base := ratio_setting.GetGroupRatio(group)
	if override, ok := ratio_setting.GetGroupGroupRatio(userGroup, group); ok {
		base = override
	}
	// Reseller prices must be visible wherever the ratio is displayed, otherwise
	// the pricing page and the actual charge drift apart.
	pricing, err := model.ResolveResellerPricing(userID, group)
	if err != nil {
		common.SysError("failed to resolve reseller pricing: " + err.Error())
		return base
	}
	return pricing.EffectiveRatio(base)
}
