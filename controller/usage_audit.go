package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func QueryUsageAudit(c *gin.Context) {
	if !canManageBillingAccounts(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var input model.UsageAuditFilter
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 8192), &input); err != nil {
		billingError(c, errors.New("invalid usage audit request"))
		return
	}
	input.Model, input.Group = strings.TrimSpace(input.Model), strings.TrimSpace(input.Group)
	report, err := service.GetUsageAuditReport(c.Request.Context(), input)
	if err != nil {
		billingError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

func SearchUsageAuditKeys(c *gin.Context) {
	if !canManageBillingAccounts(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	userID := 0
	if raw := c.Query("user_id"); raw != "" {
		var err error
		userID, err = strconv.Atoi(raw)
		if err != nil || userID < 0 {
			billingError(c, errors.New("invalid usage audit account"))
			return
		}
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if len(keyword) > 100 {
		billingError(c, errors.New("usage audit key search is too long"))
		return
	}
	keys, err := model.SearchUsageAuditKeys(c.Request.Context(), userID, keyword)
	if err != nil {
		billingError(c, err)
		return
	}
	common.ApiSuccess(c, keys)
}

func DownloadUsageAuditPDF(c *gin.Context) {
	if !canManageBillingAccounts(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var input service.UsageAuditExportInput
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 8192), &input); err != nil {
		billingError(c, errors.New("invalid usage audit PDF request"))
		return
	}
	body, err := service.CreateUsageAuditPDF(c.Request.Context(), input)
	if errors.Is(err, service.ErrUsageAuditChanged) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Usage audit data changed. Run the audit again."})
		return
	}
	if err != nil {
		billingError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="token-usage-`+input.SHA256[:12]+`.pdf"`)
	c.Data(http.StatusOK, "application/pdf", body)
}
