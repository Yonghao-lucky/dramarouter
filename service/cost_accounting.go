package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

func taskCostAccountingEventKey(taskID string) string {
	if taskID == "" {
		return ""
	}
	return fmt.Sprintf("task:%s:initial", taskID)
}

func buildCostAccountingInput(relayInfo *relaycommon.RelayInfo, saleQuota int) *model.CostAccountingInput {
	if relayInfo == nil || relayInfo.ChannelMeta == nil || saleQuota < 0 {
		return nil
	}
	input, clamp := calculateSaleCostAccountingInput(
		model.LogTypeConsume,
		saleQuota,
		relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		relayInfo.CostDiscount,
	)
	noteQuotaClamp(relayInfo, clamp)
	attachResellerEvidence(input, relayInfo.ResellerOwnerID, relayInfo.ResellerRetailRatio, relayInfo.ResellerCostRatio, relayInfo.ResellerPath, int64(saleQuota), relayInfo.PriceData.GroupRatioInfo.GroupRatio)
	return input
}

// BuildCostAccountingInputForSaleQuota is used by relay packages that record
// per-call usage logs outside the service package.
func BuildCostAccountingInputForSaleQuota(relayInfo *relaycommon.RelayInfo, saleQuota int) *model.CostAccountingInput {
	return buildCostAccountingInput(relayInfo, saleQuota)
}

func zeroCostAccountingInput(relayInfo *relaycommon.RelayInfo) *model.CostAccountingInput {
	return buildCostAccountingInput(relayInfo, 0)
}

func buildTaskCostAccountingInput(task *model.Task, logType int, saleQuota int) *model.CostAccountingInput {
	if task == nil || task.PrivateData.BillingContext == nil {
		return nil
	}
	billingContext := task.PrivateData.BillingContext
	input := buildSaleCostAccountingInput(logType, saleQuota, billingContext.GroupRatio, billingContext.CostDiscount)
	signedQuota := int64(saleQuota)
	if logType == model.LogTypeRefund {
		signedQuota = -signedQuota
	}
	attachResellerEvidence(input, billingContext.ResellerOwnerID, billingContext.ResellerRetailRatio, billingContext.ResellerCostRatio, billingContext.ResellerPath, signedQuota, billingContext.GroupRatio)
	return input
}

// attachResellerEvidence freezes who earns the margin on this charge and how the
// request was funded. The margin uses the same sign as the charge, so a refund
// records a negative margin and the daily rollup nets out naturally.
func attachResellerEvidence(input *model.CostAccountingInput, ownerID int, retailRatio, costRatio float64, path string, signedRetailQuota int64, effectiveRatio float64) {
	if input == nil || ownerID <= 0 || retailRatio <= 0 || costRatio <= 0 {
		return
	}
	settlement := path
	if settlement == "" {
		settlement = "self"
	}
	ratio := effectiveRatio
	if ratio <= 0 {
		ratio = retailRatio
	}
	input.OwnerID = ownerID
	input.RetailRatio = decimal.NewFromFloat(retailRatio).String()
	input.ResellerCostRatio = decimal.NewFromFloat(costRatio).String()
	input.ResellerMarginQuota = model.ResellerMarginQuota(signedRetailQuota, ratio, costRatio)
	input.ResellerSettlement = settlement
}

func buildSaleCostAccountingInput(logType int, saleQuota int, billingDiscount float64, costDiscount string) *model.CostAccountingInput {
	input, clamp := calculateSaleCostAccountingInput(logType, saleQuota, billingDiscount, costDiscount)
	if clamp != nil {
		common.SysError(clamp.Error())
	}
	return input
}

func signedSaleQuota(logType int, saleQuota int) (decimal.Decimal, error) {
	if saleQuota < 0 || saleQuota > common.MaxQuota ||
		(logType != model.LogTypeConsume && logType != model.LogTypeRefund) {
		return decimal.Zero, fmt.Errorf("invalid cost accounting sale quota")
	}
	signedQuota := int64(saleQuota)
	if logType == model.LogTypeRefund {
		signedQuota = -signedQuota
	}
	return decimal.NewFromInt(signedQuota), nil
}

func originalPriceCostBasis(logType int, saleQuota int, billingDiscount decimal.Decimal) (decimal.Decimal, error) {
	saleQuotaDecimal, err := signedSaleQuota(logType, saleQuota)
	if err != nil {
		return decimal.Zero, err
	}
	if !billingDiscount.IsPositive() {
		return decimal.Zero, fmt.Errorf("invalid billing discount at cost settlement")
	}
	return saleQuotaDecimal.Div(billingDiscount), nil
}

func calculateSaleCostAccountingInput(logType int, saleQuota int, billingDiscount float64, costDiscount string) (*model.CostAccountingInput, *common.QuotaClamp) {
	if strings.TrimSpace(costDiscount) == "" {
		costBasis, err := signedSaleQuota(logType, saleQuota)
		if err != nil {
			common.SysError(err.Error())
			return nil, nil
		}
		costQuota, clamp := common.QuotaFromDecimalChecked(costBasis)
		return &model.CostAccountingInput{
			CostBasisQuota: costBasis.StringFixed(6),
			CostQuota:      int64(costQuota),
			Source:         model.CostSnapshotSourceRealtime,
		}, clamp
	}
	if billingDiscount <= 0 || math.IsNaN(billingDiscount) || math.IsInf(billingDiscount, 0) {
		common.SysError("invalid billing discount at cost settlement")
		return nil, nil
	}
	discount, err := model.NormalizeCostDiscount(costDiscount)
	if err != nil {
		common.SysError("invalid cost discount at settlement: " + err.Error())
		return nil, nil
	}
	costBasis, err := originalPriceCostBasis(logType, saleQuota, decimal.NewFromFloat(billingDiscount))
	if err != nil {
		common.SysError(err.Error())
		return nil, nil
	}
	costQuota, clamp := common.QuotaFromDecimalChecked(costBasis.Mul(decimal.RequireFromString(discount)))
	return &model.CostAccountingInput{
		CostBasisQuota: costBasis.StringFixed(6),
		CostDiscount:   discount,
		CostQuota:      int64(costQuota),
		Source:         model.CostSnapshotSourceRealtime,
	}, clamp
}
