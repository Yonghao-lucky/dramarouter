package controller

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId, canViewFinancialAccounting(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if c.GetInt("role") < common.RoleRootUser {
		model.FormatAdminLogs(logs)
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	// A reseller watches their customers through the same self log view as any
	// ordinary account: the scope is the caller plus every customer they own,
	// not a separate console table with its own columns and filters. The
	// username filter narrows inside that scope, never outside it.
	username := c.Query("username")
	userIds, resellerId := resellerLogScope(userId)
	logs, total, err := model.GetUserLogsForUsers(userIds, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId, canViewFinancialAccounting(c), username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachResellerLogMargins(resellerId, logs); err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// resellerLogScope widens the caller's log scope to the customers they own.
// It returns the reseller id as the margin owner, or 0 when the caller is an
// ordinary account, in which case the scope stays the single caller id.
func resellerLogScope(userId int) ([]int, int) {
	profile, err := model.GetEnabledResellerProfile(userId)
	if err != nil || profile == nil {
		return []int{userId}, 0
	}
	customerIds, err := model.ListResellerCustomerIDs(userId)
	if err != nil || len(customerIds) == 0 {
		return []int{userId}, 0
	}
	return append([]int{userId}, customerIds...), userId
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	filter := costAccountingFilterFromQuery(c)
	stat, err := model.SumLogStatistics(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	data := gin.H{
		"quota":    stat.Quota,
		"records":  stat.Records,
		"requests": stat.Requests,
		"rpm":      stat.Rpm,
		"tpm":      stat.Tpm,
	}
	if canViewFinancialAccounting(c) {
		accounting, accountingErr := model.SumCostAccounting(filter)
		if accountingErr != nil {
			common.ApiError(c, accountingErr)
			return
		}
		accounting = model.ReconcileCostAccountingTotals(stat, accounting)
		data["revenue_quota"] = accounting.RevenueQuota
		data["cost_quota"] = accounting.CostQuota
		data["profit_quota"] = accounting.ProfitQuota
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	filter := costAccountingFilterFromQuery(c)
	filter.IncludeBillingCorrections = false
	// The statistics card must cover exactly what the table below it lists, so a
	// reseller's totals include their customers' consumption.
	userIds, _ := resellerLogScope(c.GetInt("id"))
	filter.SetUserScope(userIds)
	filter.Username = ""
	statistics, err := model.SumLogStatistics(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	data := gin.H{
		"quota":    statistics.Quota,
		"records":  statistics.Records,
		"requests": statistics.Requests,
		"rpm":      statistics.Rpm,
		"tpm":      statistics.Tpm,
	}
	if canViewFinancialAccounting(c) {
		accounting, accountingErr := model.SumCostAccounting(filter)
		if accountingErr != nil {
			common.ApiError(c, accountingErr)
			return
		}
		accounting = model.ReconcileCostAccountingTotals(statistics, accounting)
		data["revenue_quota"] = accounting.RevenueQuota
		data["cost_quota"] = accounting.CostQuota
		data["profit_quota"] = accounting.ProfitQuota
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

// GetUserLogsExport streams the caller's own usage logs as CSV. For a reseller
// the scope is the caller plus every owned customer, exactly like the on-screen
// list, so the file never contains rows the caller could not already read.
func GetUserLogsExport(c *gin.Context) {
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	username := c.Query("username")

	const maxExportRows = 5000
	userIds, resellerId := resellerLogScope(userId)
	logs, _, err := model.GetUserLogsForUsers(userIds, logType, startTimestamp, endTimestamp, modelName, tokenName, 0, maxExportRows, group, requestId, upstreamRequestId, canViewFinancialAccounting(c), username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachResellerLogMargins(resellerId, logs); err != nil {
		common.ApiError(c, err)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="usage-logs.csv"`)
	// A BOM keeps Excel from mangling UTF-8 model and user text.
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF"))
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()
	_ = writer.Write([]string{
		"id", "created_at", "type", "user_id", "username", "model_name", "group",
		"token_name", "quota", "prompt_tokens", "completion_tokens", "use_time",
		"is_stream", "request_id", "content",
	})
	for _, row := range logs {
		_ = writer.Write([]string{
			strconv.Itoa(row.Id),
			strconv.FormatInt(row.CreatedAt, 10),
			strconv.Itoa(row.Type),
			strconv.Itoa(row.UserId),
			row.Username,
			row.ModelName,
			row.Group,
			row.TokenName,
			strconv.Itoa(row.Quota),
			strconv.Itoa(row.PromptTokens),
			strconv.Itoa(row.CompletionTokens),
			strconv.Itoa(row.UseTime),
			strconv.FormatBool(row.IsStream),
			row.RequestId,
			row.Content,
		})
	}
}
