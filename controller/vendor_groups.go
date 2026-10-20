package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type vendorGroupOption struct {
	Name       string  `json:"name"`
	Ratio      float64 `json:"ratio"`
	ModelCount int     `json:"model_count"`
}

type vendorGroupCatalog struct {
	Id     int                 `json:"id"`
	Name   string              `json:"name"`
	Icon   string              `json:"icon,omitempty"`
	Groups []vendorGroupOption `json:"groups"`
}

// GetUserVendorGroups 返回「模型厂商 → 该用户可用分组」的目录。
// Key 创建页据此让用户按厂商排列分组；数据来自定价缓存，频率与新模型上架同步。
func GetUserVendorGroups(c *gin.Context) {
	userId := c.GetInt("id")
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" {
		group, err := model.GetUserGroup(userId, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		userGroup = group
	}
	usableGroups := service.GetUserUsableGroups(userGroup)

	pricing := model.GetPricing()
	vendors := model.GetVendors()
	vendorByID := make(map[int]*model.PricingVendor, len(vendors))
	order := make([]int, 0, len(vendors))
	for i := range vendors {
		vendorByID[vendors[i].ID] = &vendors[i]
		order = append(order, vendors[i].ID)
	}

	counts := make(map[int]map[string]int)
	for _, item := range pricing {
		if item.VendorID <= 0 {
			continue
		}
		for _, group := range item.EnableGroup {
			if group == "" || group == "auto" {
				continue
			}
			if _, ok := usableGroups[group]; !ok {
				continue
			}
			if counts[item.VendorID] == nil {
				counts[item.VendorID] = make(map[string]int)
			}
			counts[item.VendorID][group]++
		}
	}

	result := make([]vendorGroupCatalog, 0, len(counts))
	for _, vendorID := range order {
		groupCounts, ok := counts[vendorID]
		if !ok || len(groupCounts) == 0 {
			continue
		}
		vendor := vendorByID[vendorID]
		if vendor == nil {
			continue
		}
		groups := make([]vendorGroupOption, 0, len(groupCounts))
		for group, count := range groupCounts {
			groups = append(groups, vendorGroupOption{
				Name:       group,
				Ratio:      service.GetUserGroupRatio(userId, userGroup, group),
				ModelCount: count,
			})
		}
		sortVendorGroupOptions(groups)
		result = append(result, vendorGroupCatalog{
			Id:     vendor.ID,
			Name:   vendor.Name,
			Icon:   vendor.Icon,
			Groups: groups,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "",
		"data":      result,
		"max_count": setting.GetMaxTokenAutoGroups(),
		"groups":    ratio_setting.GetGroupRatioCopy(),
	})
}

func sortVendorGroupOptions(options []vendorGroupOption) {
	for i := 1; i < len(options); i++ {
		for j := i; j > 0; j-- {
			prev, cur := options[j-1], options[j]
			if prev.Ratio < cur.Ratio || (prev.Ratio == cur.Ratio && prev.Name <= cur.Name) {
				break
			}
			options[j-1], options[j] = cur, prev
		}
	}
}
