package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// requireReseller resolves the calling user's enabled reseller profile. The
// profile carries the reseller's own user id; that id is the owner key for every
// binding, price, cost row and wallet movement.
func requireReseller(c *gin.Context) (*model.ResellerProfile, bool) {
	profile, err := model.GetEnabledResellerProfile(c.GetInt("id"))
	if err != nil {
		writeResellerError(c, err)
		return nil, false
	}
	if profile == nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "not an enabled reseller"})
		return nil, false
	}
	return profile, true
}

// resellerErrorMessage maps domain errors to a stable client-facing message.
func resellerErrorMessage(err error) string {
	switch {
	case errors.Is(err, model.ErrResellerCustomerNotFound):
		return "customer is not owned by this reseller"
	case errors.Is(err, model.ErrResellerGroupNotAllowed):
		return "group is not authorized for this reseller"
	case errors.Is(err, model.ErrResellerPriceBelowCost):
		return "price cannot be lower than the reseller cost"
	case errors.Is(err, model.ErrResellerWalletShort):
		return "reseller wallet is insufficient"
	case errors.Is(err, model.ErrResellerCustomerShort):
		return "customer wallet is insufficient"
	case errors.Is(err, model.ErrResellerTransferInvalid):
		return "invalid transfer amount"
	case errors.Is(err, model.ErrResellerNotEnabled):
		return "not an enabled reseller"
	}
	return err.Error()
}

func writeResellerError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": resellerErrorMessage(err)})
}

// requireOwnedCustomer rejects every customer id outside the caller's book.
func requireOwnedCustomer(c *gin.Context, resellerUserId int) (int, bool) {
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
	if owner != resellerUserId {
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
	rows, err := model.ListResellerCustomers(resellerId, strings.TrimSpace(c.Query("keyword")), after, limit)
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
		writeResellerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func AdminSetResellerDefaultPrice(c *gin.Context) {
	var req resellerRatioRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ResellerId <= 0 || req.Group == "" {
		common.ApiErrorMsg(c, "invalid reseller price")
		return
	}
	if err := model.SetResellerDefaultPrice(req.ResellerId, req.Group, strings.TrimSpace(req.Ratio), c.GetInt("id")); err != nil {
		writeResellerError(c, err)
		return
	}
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
// The administrator is not bound by the reseller cost floor.
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
	if err := model.SetCustomerGroupPrice(userId, req.Group, strings.TrimSpace(req.Ratio), "admin", c.GetInt("id"), false); err != nil {
		writeResellerError(c, err)
		return
	}
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
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AdminTransferResellerQuota lets an administrator move quota between a
// reseller wallet and one of its customers, e.g. to undo a mistake.
func AdminTransferResellerQuota(c *gin.Context) {
	var req transferRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ResellerId <= 0 || req.CustomerUserId <= 0 {
		common.ApiErrorMsg(c, "invalid transfer request")
		return
	}
	transfer, err := model.TransferResellerQuota(req.ResellerId, req.CustomerUserId, c.GetInt("id"), req.Direction, req.Group, req.RetailQuota, req.EventKey, req.Remark)
	if err != nil {
		writeResellerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": transfer})
}

// ---------------------------------------------------------------------------
// Reseller console endpoints (owner-scoped)
// ---------------------------------------------------------------------------

type transferRequest struct {
	ResellerId     int    `json:"reseller_id"`
	CustomerUserId int    `json:"customer_user_id"`
	Direction      string `json:"direction"`
	Group          string `json:"group"`
	RetailQuota    int64  `json:"retail_quota"`
	EventKey       string `json:"event_key"`
	Remark         string `json:"remark"`
}

func ResellerSummary(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	summary, err := model.GetResellerSummary(profile.UserId)
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
	rows, err := model.ListResellerCustomers(profile.UserId, strings.TrimSpace(c.Query("keyword")), after, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func ResellerCustomerDetail(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	detail, err := model.GetResellerCustomerDetail(profile.UserId, customerId)
	if err != nil {
		writeResellerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

// ResellerCreateCustomer creates a normal platform user in the reseller's
// default group and binds it to the caller in one transaction. The new user
// keeps a normal role; reseller pricing comes from the pricing layer.
func ResellerCreateCustomer(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Group       string `json:"group"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid customer payload")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 || len(req.Password) > 20 {
		common.ApiErrorMsg(c, "username is required and password must be 8-20 characters")
		return
	}
	group := strings.TrimSpace(req.Group)
	if group != "" {
		// A reseller may only place a customer in a group it is authorized to
		// resell; otherwise the customer would consume at an unpriced group.
		costs, _, err := model.ListResellerPricingBook(profile.UserId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		allowed := false
		for _, cost := range costs {
			if cost.Enabled && cost.GroupName == group {
				allowed = true
				break
			}
		}
		if !allowed {
			writeResellerError(c, model.ErrResellerGroupNotAllowed)
			return
		}
	} else {
		group = "default"
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}

	user := &model.User{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Email:       strings.TrimSpace(req.Email),
		Role:        common.RoleCommonUser,
		Group:       group,
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := user.InsertWithTx(tx, 0); err != nil {
			return err
		}
		now := common.GetTimestamp()
		return tx.Save(&model.ResellerCustomerBinding{
			CustomerUserId: user.Id,
			ResellerId:     profile.UserId,
			EffectiveFrom:  now,
			UpdatedBy:      profile.UserId,
			UpdatedAt:      now,
		}).Error
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	user.FinishInsert(0)
	_ = model.InvalidateResellerPricingCache(user.Id)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"user_id": user.Id, "username": user.Username}})
}

// ResellerSetCustomerStatus enables or disables one owned customer.
func ResellerSetCustomerStatus(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	var req struct {
		Status int `json:"status"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid status")
		return
	}
	if req.Status != common.UserStatusEnabled && req.Status != common.UserStatusDisabled {
		common.ApiErrorMsg(c, "invalid status")
		return
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", customerId).Update("status", req.Status).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(profile.UserId, model.LogTypeSystem, "reseller set customer status")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResellerResetCustomerPassword rotates one owned customer's password.
func ResellerResetCustomerPassword(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || len(req.Password) < 8 || len(req.Password) > 20 {
		common.ApiErrorMsg(c, "password must be 8-20 characters")
		return
	}
	hashed, err := common.Password2Hash(req.Password)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", customerId).Update("password", hashed).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResellerSetCustomerGroup moves one owned customer into another group the
// reseller is authorized to resell.
func ResellerSetCustomerGroup(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	var req struct {
		Group string `json:"group"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || strings.TrimSpace(req.Group) == "" {
		common.ApiErrorMsg(c, "invalid group")
		return
	}
	group := strings.TrimSpace(req.Group)
	costs, _, err := model.ListResellerPricingBook(profile.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	allowed := false
	for _, cost := range costs {
		if cost.Enabled && cost.GroupName == group {
			allowed = true
			break
		}
	}
	if !allowed {
		writeResellerError(c, model.ErrResellerGroupNotAllowed)
		return
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", customerId).Update("group", group).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	_ = model.InvalidateResellerPricingCache(customerId)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResellerTransfer moves quota between the caller's wallet and one owned
// customer wallet. The customer is credited retail quota; the reseller pays the
// wholesale price of that quota immediately.
func ResellerTransfer(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	var req struct {
		Direction   string `json:"direction"`
		Group       string `json:"group"`
		RetailQuota int64  `json:"retail_quota"`
		EventKey    string `json:"event_key"`
		Remark      string `json:"remark"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid transfer request")
		return
	}
	if req.EventKey == "" {
		req.EventKey = common.NewRequestId()
	}
	transfer, err := model.TransferResellerQuota(profile.UserId, customerId, profile.UserId, req.Direction, strings.TrimSpace(req.Group), req.RetailQuota, req.EventKey, strings.TrimSpace(req.Remark))
	if err != nil {
		writeResellerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": transfer})
}

func ResellerTransfers(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, _ := strconv.Atoi(c.DefaultQuery("customer_id", "0"))
	before, _ := strconv.ParseInt(c.DefaultQuery("before_id", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerTransfers(profile.UserId, customerId, before, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ResellerCustomerGroups returns the groups this reseller may sell together with
// each owned customer's effective price.
func ResellerCustomerGroups(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	rows, err := model.ResellerCustomerGroups(profile.UserId, customerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func ResellerSetCustomerPrice(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	customerId, ok := requireOwnedCustomer(c, profile.UserId)
	if !ok {
		return
	}
	var req customerPriceRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.Group == "" {
		common.ApiErrorMsg(c, "invalid group price")
		return
	}
	// Reseller-scoped: never below the caller's own wholesale cost.
	if err := model.SetCustomerGroupPrice(customerId, req.Group, strings.TrimSpace(req.Ratio), "reseller", profile.UserId, true); err != nil {
		writeResellerError(c, err)
		return
	}
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
	rows, err := model.ListResellerUsage(profile.UserId, customerId, before, start, end, limit)
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
	rows, err := model.ListResellerMarginDays(profile.UserId, start, end)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ResellerPriceBook returns the reseller's own cost book and standing prices so
// the console can show the margin it makes per group.
func ResellerPriceBook(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	costs, defaults, err := model.ListResellerPricingBook(profile.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"costs": costs, "defaults": defaults}})
}

// ResellerCustomerLogs pages the usage logs of owned customers with the same
// masking a normal user sees: no channel identity, no admin-only fields.
func ResellerCustomerLogs(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	customerId, _ := strconv.Atoi(c.DefaultQuery("customer_id", "0"))
	logs, total, err := model.ResellerCustomerLogs(profile.UserId, customerId, logType, startTimestamp, endTimestamp, modelName, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		writeResellerError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

// ResellerCustomerTasks pages the task log of owned customers with the channel
// stripped, mirroring the normal user view.
func ResellerCustomerTasks(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	params := model.SyncTaskQueryParams{
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
	}
	if customerId := c.Query("customer_id"); customerId != "" {
		params.UserID = customerId
	}
	tasks, total, err := model.ResellerCustomerTasks(profile.UserId, params, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		writeResellerError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	dtos, err := tasksToDto(tasks, true, common.RoleCommonUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(dtos)
	common.ApiSuccess(c, pageInfo)
}
