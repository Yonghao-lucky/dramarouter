package controller

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Settlement ledger (administrator)
// ---------------------------------------------------------------------------

type resellerSettlementRequest struct {
	ResellerId   int    `json:"reseller_id"`
	Direction    string `json:"direction"`
	Currency     string `json:"currency"`
	AmountMinor  int64  `json:"amount_minor"`
	ExchangeRate string `json:"exchange_rate"`
	Quota        int64  `json:"quota"`
	Kind         string `json:"kind"`
	Method       string `json:"method"`
	ExternalRef  string `json:"external_ref"`
	ProofURL     string `json:"proof_url"`
	Remark       string `json:"remark"`
	EventKey     string `json:"event_key"`
}

// AdminCreateResellerSettlement records a payment without moving money. The
// operator confirms it separately once the funds are actually received.
func AdminCreateResellerSettlement(c *gin.Context) {
	var req resellerSettlementRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.ResellerId <= 0 {
		common.ApiErrorMsg(c, "invalid settlement")
		return
	}
	if req.EventKey == "" {
		req.EventKey = common.NewRequestId()
	}
	row, err := model.CreateResellerSettlement(model.ResellerSettlementRequest{
		EventKey:     req.EventKey,
		ResellerId:   req.ResellerId,
		Direction:    req.Direction,
		Currency:     req.Currency,
		AmountMinor:  req.AmountMinor,
		ExchangeRate: req.ExchangeRate,
		QuotaAmount:  req.Quota,
		Kind:         req.Kind,
		Method:       req.Method,
		ExternalRef:  req.ExternalRef,
		ProofURL:     req.ProofURL,
		Remark:       req.Remark,
		ActorId:      c.GetInt("id"),
	})
	if err != nil {
		writeSettlementError(c, err)
		return
	}
	recordManageAudit(c, "reseller.settlement.create", map[string]interface{}{
		"reseller_id":  req.ResellerId,
		"amount_minor": req.AmountMinor,
		"currency":     req.Currency,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

// AdminConfirmResellerSettlement credits the wallet and links the grant it
// produced. Confirming twice returns the original grant instead of paying again.
func AdminConfirmResellerSettlement(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid settlement id")
		return
	}
	row, err := model.ConfirmResellerSettlement(id, c.GetInt("id"))
	if err != nil {
		writeSettlementError(c, err)
		return
	}
	recordManageAudit(c, "reseller.settlement.confirm", map[string]interface{}{
		"settlement_id": id,
		"reseller_id":   row.ResellerId,
		"quota":         row.QuotaAmount,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func AdminVoidResellerSettlement(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid settlement id")
		return
	}
	row, err := model.VoidResellerSettlement(id, c.GetInt("id"))
	if err != nil {
		writeSettlementError(c, err)
		return
	}
	recordManageAudit(c, "reseller.settlement.void", map[string]interface{}{
		"settlement_id": id,
		"reseller_id":   row.ResellerId,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func AdminResellerSettlements(c *gin.Context) {
	resellerId, err := strconv.Atoi(c.Query("reseller_id"))
	if err != nil || resellerId <= 0 {
		common.ApiErrorMsg(c, "invalid reseller id")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerSettlements(resellerId, strings.TrimSpace(c.Query("status")), limit)
	if err != nil {
		writeSettlementError(c, err)
		return
	}
	totals, err := model.GetResellerSettlementTotals(resellerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "totals": totals}})
}

func writeSettlementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrResellerSettlementInvalid):
		common.ApiErrorMsg(c, "invalid settlement")
	case errors.Is(err, model.ErrResellerSettlementConflict):
		common.ApiErrorMsg(c, "settlement is not pending")
	case errors.Is(err, model.ErrResellerGrantReplay):
		common.ApiErrorMsg(c, "this event key belongs to another reseller")
	default:
		common.ApiError(c, err)
	}
}

// ---------------------------------------------------------------------------
// Settlement ledger (reseller console, read-only)
// ---------------------------------------------------------------------------

// ResellerGrants lets a reseller see what the platform credited it, including
// pre-credit. The list is filtered by the caller's own user id, so it can never
// read another reseller's grants.
func ResellerGrants(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerWalletGrants(profile.UserId, limit)
	if err != nil {
		writeResellerError(c, err)
		return
	}
	totals, err := model.GetResellerSettlementTotals(profile.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Pre-credit rows are returned with kind = "precredit"; the console renders
	// that label so unpaid platform credit is never mistaken for settled money.
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"items":  rows,
		"totals": totals,
	}})
}

// ResellerSettlements is the reseller's own settlement ledger.
func ResellerSettlements(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerSettlements(profile.UserId, strings.TrimSpace(c.Query("status")), limit)
	if err != nil {
		writeSettlementError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ResellerSettlementExport streams the caller's ledger as CSV. It is scoped to
// the caller's own settlements, so the export cannot be used to dump another
// reseller's accounting.
func ResellerSettlementExport(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	rows, err := model.ListResellerSettlementsForExport(profile.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="reseller-settlements.csv"`)
	// A BOM keeps Excel from mangling UTF-8 group and remark text.
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF"))
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()
	_ = writer.Write([]string{
		"id", "created_at", "direction", "status", "currency", "amount_minor",
		"exchange_rate", "quota", "kind", "method", "external_ref", "remark",
	})
	for _, row := range rows {
		_ = writer.Write([]string{
			strconv.FormatInt(row.Id, 10),
			strconv.FormatInt(row.CreatedAt, 10),
			row.Direction,
			row.Status,
			row.Currency,
			strconv.FormatInt(row.AmountMinor, 10),
			row.ExchangeRate,
			strconv.FormatInt(row.QuotaAmount, 10),
			row.Kind,
			row.Method,
			row.ExternalRef,
			row.Remark,
		})
	}
}

// ---------------------------------------------------------------------------
// Customer dashboard (reseller console)
// ---------------------------------------------------------------------------

// ResellerDashboard aggregates the caller's owned customers only. The window is
// clamped server-side, so the handler cannot be used to scan an unbounded range.
func ResellerDashboard(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	dashboard, err := model.GetResellerCustomerDashboard(profile.UserId, days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dashboard})
}

// ResellerAuditTrail returns the caller's own reseller actions with their IP.
func ResellerAuditTrail(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := model.ListResellerOperationAudits(profile.UserId, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ---------------------------------------------------------------------------
// Price templates (reseller console)
// ---------------------------------------------------------------------------

func ResellerPriceTemplates(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	templates, items, err := model.ListResellerPriceTemplates(profile.UserId)
	if err != nil {
		writeResellerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"templates": templates, "items": items}})
}

type resellerTemplateRequest struct {
	Id     int               `json:"id"`
	Name   string            `json:"name"`
	Remark string            `json:"remark"`
	Items  map[string]string `json:"items"`
}

func ResellerSavePriceTemplate(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	var req resellerTemplateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid price template")
		return
	}
	saved, err := model.UpsertResellerPriceTemplate(profile.UserId, profile.UserId, req.Id, model.ResellerPriceTemplateInput{
		Name:   req.Name,
		Remark: req.Remark,
		Items:  req.Items,
	})
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	recordResellerAudit(c, profile.UserId, "reseller.template.save", map[string]interface{}{"template_id": saved.Id})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": saved})
}

func ResellerDeletePriceTemplate(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid template id")
		return
	}
	if err := model.DeleteResellerPriceTemplate(profile.UserId, id); err != nil {
		writeTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResellerApplyPriceTemplate writes a template onto the customers the reseller
// explicitly selected. Every customer is re-checked against the owner binding,
// so a forged id cannot move another reseller's customer onto this price book.
func ResellerApplyPriceTemplate(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	var req struct {
		TemplateId  int   `json:"template_id"`
		CustomerIds []int `json:"customer_ids"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid template application")
		return
	}
	if len(req.CustomerIds) > 200 {
		common.ApiErrorMsg(c, "too many customers in one request")
		return
	}
	applied, err := model.ApplyResellerPriceTemplate(profile.UserId, profile.UserId, req.TemplateId, req.CustomerIds)
	if err != nil {
		writeTemplateError(c, err)
		return
	}
	recordResellerAudit(c, profile.UserId, "reseller.template.apply", map[string]interface{}{
		"template_id": req.TemplateId,
		"customers":   applied,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"applied": applied}})
}

func writeTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrResellerTemplateInvalid):
		common.ApiErrorMsg(c, "invalid price template")
	case errors.Is(err, model.ErrResellerTemplateEmpty):
		common.ApiErrorMsg(c, "price template has no groups")
	case errors.Is(err, model.ErrResellerGroupNotAllowed):
		common.ApiErrorMsg(c, resellerErrorMessage(err))
	case errors.Is(err, model.ErrResellerPriceBelowCost):
		common.ApiErrorMsg(c, resellerErrorMessage(err))
	case errors.Is(err, model.ErrResellerCustomerNotFound):
		common.ApiErrorMsg(c, resellerErrorMessage(err))
	default:
		common.ApiError(c, err)
	}
}

// ---------------------------------------------------------------------------
// Quota alert rule (reseller console)
// ---------------------------------------------------------------------------

func ResellerQuotaAlert(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	rule, err := model.GetResellerQuotaAlert(profile.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

func ResellerSetQuotaAlert(c *gin.Context) {
	profile, ok := requireReseller(c)
	if !ok {
		return
	}
	var req struct {
		Enabled        bool   `json:"enabled"`
		ThresholdQuota int64  `json:"threshold_quota"`
		NotifyEmail    string `json:"notify_email"`
		CooldownHours  int    `json:"cooldown_hours"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid quota alert rule")
		return
	}
	rule, err := model.SetResellerQuotaAlert(profile.UserId, req.Enabled, req.ThresholdQuota, req.NotifyEmail, req.CooldownHours)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordResellerAudit(c, profile.UserId, "reseller.alert.save", map[string]interface{}{
		"enabled":   rule.Enabled,
		"threshold": rule.ThresholdQuota,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rule})
}

// recordResellerAudit attributes a reseller-console action to the acting
// reseller user, including the caller IP, so the audit trail is queryable from
// the console without exposing administrator-only log fields.
func recordResellerAudit(c *gin.Context, actorId int, action string, params map[string]interface{}) {
	model.RecordOperationAuditLog(actorId, auditContentEN(action, params), c.ClientIP(), action, params, nil, nil)
	markAuditLogged(c)
}
