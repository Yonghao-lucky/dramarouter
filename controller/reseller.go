package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// requireReseller resolves the calling user's enabled reseller profile.
func requireReseller(c *gin.Context) (*model.ResellerProfile, bool) {
	profile, err := model.GetResellerProfileByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if profile == nil || profile.Status != model.ResellerStatusEnabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "not an enabled reseller"})
		return nil, false
	}
	return profile, true
}

// requireOwnedCustomer rejects every customer id outside the caller's book.
func requireOwnedCustomer(c *gin.Context, resellerId int) (int, bool) {
	customerId, err := strconv.Atoi(c.Param("id"))
	if err != nil || customerId <= 0 {
		common.ApiErrorMsg(c, "invalid customer id")
		return 0, false
	}
	owner, err := model.GetResellerCustomerOwner(customerId)
	if err != nil {
		common.ApiError(c, err)
		return 0, false
	}
	if owner != resellerId {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "customer is not owned by this reseller"})
		return 0, false
	}
	return customerId, true
}

// ---------------------------------------------------------------------------
// Administrator endpoints
// ---------------------------------------------------------------------------

type resellerProfileRequest struct {
	UserId int    `json:"user_id"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Status int    `json:"status"`
}

func AdminUpsertReseller(c *gin.Context) {
	var req resellerProfileRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.UserId <= 0 {
		common.ApiErrorMsg(c, "invalid reseller profile")
		return
	}
	profile, err := model.UpsertResellerProfile(req.UserId, strings.TrimSpace(req.Name), strings.TrimSpace(req.Code), req.Status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateResellerDefaultPricingCache()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

func AdminListResellers(c *gin.Context) {
	profiles, err := model.ListResellerProfiles()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profiles})
}

// AdminResellerPriceBook returns one reseller's wholesale cost book and standing
// retail prices for the administration view.
func AdminResellerPriceBook(c *gin.Context) {
	resellerId, err := strconv.Atoi(c.Query("reseller_id"))
	if err != nil || resellerId <= 0 {
		common.ApiErrorMsg(c, "invalid reseller id")
		return
	}
	costs, defaults, err := model.ListResellerPricingBook(resellerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"costs": costs, "defaults": defaults}})
}

// AdminResellerCustomers lists the customers owned by one reseller.
func AdminResellerCustomers(c *gin.Context) {
	resellerId, err := strconv.Atoi(c.Query("reseller_id"))
	if err != nil || resellerId <= 0 {
		common.ApiErrorMsg(c, "invalid reseller id")
		return
	}
	after, _ := strconv.Atoi(c.DefaultQuery("after_id", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := model.ListResellerCustomers(resellerId, after, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

type resellerRatioRequest struct {
	ResellerId int    `json:"reseller_id"`
	Group      string `json:"group"`
	Ratio      string `json:"ratio"`
	Enabled    *bool  `json:"enabled"`
}

func AdminSetResellerCost(c *gin.Context) {
	var req resellerRatioRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ResellerId <= 0 || req.Group == "" {
		common.ApiErrorMsg(c, "invalid reseller cost")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := model.SetResellerGroupCost(req.ResellerId, req.Group, strings.TrimSpace(req.Ratio), enabled, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateResellerDefaultPricingCache()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminSetResellerDefaultPrice(c *gin.Context) {
	var req resellerRatioRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ResellerId <= 0 || req.Group == "" {
		common.ApiErrorMsg(c, "invalid reseller price")
		return
	}
	if err := model.SetResellerDefaultPrice(req.ResellerId, req.Group, strings.TrimSpace(req.Ratio), c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	// A standing price applies to every owned customer without a specific price.
	_ = model.InvalidateResellerDefaultPricingCache()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type customerPriceRequest struct {
	Group string `json:"group"`
	Ratio string `json:"ratio"`
}

// AdminGetCustomerGroupPricing returns a user's current price overrides.
func AdminGetCustomerGroupPricing(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	prices, err := model.ListCustomerGroupPrices(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": prices})
}

// AdminSetCustomerGroupPrice is the administrator's per-user, per-group price.
func AdminSetCustomerGroupPrice(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	var req customerPriceRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Group == "" {
		common.ApiErrorMsg(c, "invalid group price")
		return
	}
	if err := model.SetCustomerGroupPrice(userId, req.Group, strings.TrimSpace(req.Ratio), "admin", c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateResellerPricingCache(userId)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type ownerRequest struct {
	ResellerId int    `json:"reseller_id"`
	Reason     string `json:"reason"`
}

// AdminSetCustomerOwner changes ownership. Only administrators can do this.
func AdminSetCustomerOwner(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	var req ownerRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid ownership request")
		return
	}
	if err := model.SetResellerCustomerOwner(userId, req.ResellerId, c.GetInt("id"), req.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateResellerPricingCache(userId)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type allocationRequest struct {
	CustomerUserId int   `json:"customer_user_id"`
	AllocatedQuota int64 `json:"allocated_quota"`
}

func AdminSetResellerAllocation(c *gin.Context) {
	var req allocationRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.CustomerUserId <= 0 {
		common.ApiErrorMsg(c, "invalid allocation request")
		return
	}
	owner, err := model.GetResellerCustomerOwner(req.CustomerUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AdjustResellerAllocation(req.CustomerUserId, owner, req.AllocatedQuota, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// Reseller console endpoints (read-mostly, owner-scoped)
// ---------------------------------------------------------------------------

func ResellerSummary(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	summary, err := model.GetResellerSummary(profile.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func ResellerCustomers(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	after, _ := strconv.Atoi(c.DefaultQuery("after_id", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerCustomers(profile.Id, after, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ResellerCreateCustomer creates a normal platform user and binds it to the
// caller. The new user keeps the default group; reseller pricing comes from the
// pricing layer, never from a special group.
func ResellerCreateCustomer(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		common.ApiErrorMsg(c, "username and password are required")
		return
	}
	user := &model.User{
		Username:    strings.TrimSpace(req.Username),
		Password:    req.Password,
		DisplayName: strings.TrimSpace(req.Username),
		Role:        common.RoleCommonUser,
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := user.InsertWithTx(tx, 0); err != nil {
			return err
		}
		return tx.Save(&model.ResellerCustomerBinding{
			CustomerUserId: user.Id,
			ResellerId:     profile.Id,
			EffectiveFrom:  common.GetTimestamp(),
			UpdatedBy:      profile.UserId,
			UpdatedAt:      common.GetTimestamp(),
		}).Error
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	user.FinishInsert(0)
	_ = model.InvalidateResellerPricingCache(user.Id)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": user.Id, "username": user.Username}})
}

func ResellerSetCustomerAllocation(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.Id)
	if !ok {
		return
	}
	var req struct {
		AllocatedQuota int64 `json:"allocated_quota"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid allocation request")
		return
	}
	if err := model.AdjustResellerAllocation(customerId, profile.Id, req.AllocatedQuota, profile.UserId); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func ResellerSetCustomerPrice(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.Id)
	if !ok {
		return
	}
	var req customerPriceRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Group == "" {
		common.ApiErrorMsg(c, "invalid group price")
		return
	}
	// A reseller may only price its own groups' retail; the platform cost price
	// is administrator-only.
	if err := model.SetCustomerGroupPrice(customerId, req.Group, strings.TrimSpace(req.Ratio), "reseller", profile.UserId); err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateResellerPricingCache(customerId)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func ResellerUsage(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, _ := strconv.Atoi(c.DefaultQuery("customer_id", "0"))
	before, _ := strconv.ParseInt(c.DefaultQuery("before_id", "0"), 10, 64)
	start, _ := strconv.ParseInt(c.DefaultQuery("start", "0"), 10, 64)
	end, _ := strconv.ParseInt(c.DefaultQuery("end", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerUsage(profile.Id, customerId, before, start, end, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func ResellerMargins(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	start, _ := strconv.ParseInt(c.DefaultQuery("start", "0"), 10, 64)
	end, _ := strconv.ParseInt(c.DefaultQuery("end", "0"), 10, 64)
	now := common.GetTimestamp()
	if end <= 0 {
		end = now
	}
	// Keep the aggregate bounded to 31 days so the console cannot trigger a
	// long range scan.
	if start <= 0 || end-start > 31*86400 {
		start = end - 31*86400
	}
	rows, err := model.ListResellerMarginDays(profile.Id, start, end)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ResellerPriceBook returns the reseller's own cost book and default prices so
// the console can show the margin it makes per group.
func ResellerPriceBook(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	costs, defaults, err := model.ListResellerPricingBook(profile.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"costs": costs, "defaults": defaults}})
}
