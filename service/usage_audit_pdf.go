package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/signintech/gopdf"
)

func (doc *billingPDFDocument) usageAuditMoney(x, y, width, size float64, value string) {
	if value == "" {
		doc.right(x, y, width, size, "未核价")
		return
	}
	amount, err := decimal.NewFromString(value)
	if err != nil {
		doc.err = err
		return
	}
	places := int32(6)
	for places < 9 && !amount.Equal(amount.Round(places)) {
		places++
	}
	text, err := billingDisplayAmount(value, places, false)
	if err != nil {
		doc.err = err
		return
	}
	doc.right(x, y, width, size, text)
}

// Keep model pages focused on token use and the extra ordinary-input overlap.
// Normal cache usage is retained; the overlap is shown once in the cache row.
func (doc *billingPDFDocument) usageAuditTokenTable(summary UsageAuditSummary, y float64) float64 {
	if y+150 > 767 {
		doc.err = errors.New("usage audit token table exceeds page boundary")
		return y
	}
	doc.pdf.SetFillColor(25, 51, 94)
	doc.pdf.RectFromUpperLeftWithStyle(40, y, 515, 24, "F")
	doc.pdf.SetTextColor(255, 255, 255)
	doc.text(50, y+7, 170, 8.3, "Token 分类")
	doc.right(220, y+7, 145, 8.3, "正常用量 Token")
	doc.right(375, y+7, 170, 8.3, "额外计入输入 Token")
	y += 24
	rows := []struct {
		label         string
		tokens, extra int64
	}{
		{"输入（不含缓存）", summary.Tokens.Input, 0},
		{"缓存", summary.Tokens.CacheRead + summary.Tokens.CacheWrite, summary.InputRecalculation.Tokens},
		{"输出", summary.Tokens.Output, 0},
		{"合计", summary.Tokens.TotalInput + summary.Tokens.Output, summary.InputRecalculation.Tokens},
	}
	for i, row := range rows {
		if i == len(rows)-1 {
			doc.pdf.SetFillColor(232, 239, 249)
		} else if i%2 == 0 {
			doc.pdf.SetFillColor(245, 247, 251)
		} else {
			doc.pdf.SetFillColor(255, 255, 255)
		}
		doc.pdf.RectFromUpperLeftWithStyle(40, y, 515, 22, "F")
		doc.pdf.SetTextColor(35, 49, 69)
		doc.text(50, y+6, 170, 8.5, row.label)
		doc.right(220, y+6, 145, 8.5, fmt.Sprint(row.tokens))
		doc.right(375, y+6, 170, 8.5, fmt.Sprint(row.extra))
		y += 22
	}
	doc.pdf.SetTextColor(89, 105, 127)
	y = doc.text(40, y+7, 515, 8, fmt.Sprintf("缓存明细：缓存读取 %d Token / 缓存写入 %d Token。", summary.Tokens.CacheRead, summary.Tokens.CacheWrite))
	return doc.text(40, y+4, 515, 8, fmt.Sprintf("需重算 %d 笔 / 有缓存写入，已排除 %d 笔。正常缓存读写费用保留。", summary.InputRecalculation.RequestCount, summary.InputRecalculation.ExcludedWriteRequestCount))
}

// Use the existing statement letterhead, operating logo, CJK font, palette and
// footer. A usage comparison has no customer-confirmation or payment state.
func RenderUsageAuditPDF(report *UsageAuditReport, branding *BillingSnapshot) ([]byte, error) {
	if report == nil || branding == nil || len(report.SHA256) != 64 || len(report.Models) > 200 {
		return nil, errors.New("invalid usage audit PDF; select up to 200 models")
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.SetInfo(gopdf.PdfInfo{Title: "Token usage and duplicate-input analysis", Author: branding.Issuer, Creator: "new-api usage audit", CreationDate: time.Unix(report.GeneratedAt, 0).UTC()})
	missingGlyph := false
	if err := pdf.AddTTFFontDataWithOption("billing", billingPDFFont, gopdf.TtfOption{OnGlyphNotFound: func(r rune) { missingGlyph = true }, OnGlyphNotFoundSubstitute: gopdf.DefaultOnGlyphNotFoundSubstitute}); err != nil {
		return nil, err
	}
	type modelSection struct {
		group UsageAuditGroup
	}
	pages := make([][]modelSection, 0)
	page := make([]modelSection, 0)
	used := 0.0
	for _, group := range report.Models {
		if err := pdf.SetFont("billing", "", 11); err != nil {
			return nil, err
		}
		nameLines, err := pdf.SplitText(group.Label, 515)
		if err != nil {
			return nil, err
		}
		height := float64(len(nameLines))*11*1.45 + 4 + 8*1.45 + 8 + 150 + 23
		if height > 589 {
			return nil, errors.New("usage audit model heading exceeds a page")
		}
		if used+height > 589 && len(page) > 0 {
			pages = append(pages, page)
			page = make([]modelSection, 0)
			used = 0
		}
		page = append(page, modelSection{group: group})
		used += height
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}
	doc := billingPDFDocument{pdf: pdf}
	doc.headerV3(branding, 1, 1+len(pages))
	pdf.SetTextColor(89, 105, 127)
	doc.text(40, 120, 515, 8.5, "TOKEN USAGE & DUPLICATE-INPUT ANALYSIS")
	pdf.SetTextColor(25, 51, 94)
	doc.text(40, 141, 515, 23, "Token 用量与重复输入核算单")
	pdf.SetTextColor(89, 105, 127)
	doc.text(40, 181, 515, 8.5, "报告编号  UA-"+report.SHA256[:12])
	doc.text(40, 198, 515, 8.5, "生成时间  "+time.Unix(report.GeneratedAt, 0).In(billingLocation).Format("2006-01-02 15:04:05")+"  /  Asia/Shanghai")
	left := doc.text(40, 229, 247, 9, "核对客户  "+branding.CompanyTitle)
	if report.Filter.UserID > 0 {
		left = doc.text(40, left+5, 247, 8.5, fmt.Sprintf("客户账户  %s (#%d)", branding.Username, report.Filter.UserID))
	}
	keyLabel := "全部"
	if report.Filter.TokenID > 0 {
		keyLabel = fmt.Sprintf("#%d", report.Filter.TokenID)
	}
	left = doc.text(40, left+5, 247, 8.5, "API Key  "+keyLabel)
	right := doc.text(308, 229, 247, 8.5, "开始  "+time.Unix(report.Filter.StartAt, 0).In(billingLocation).Format("2006-01-02 15:04:05"))
	right = doc.text(308, right+5, 247, 8.5, "结束  "+time.Unix(report.Filter.EndAt, 0).In(billingLocation).Format("2006-01-02 15:04:05")+" (不含)")
	right = doc.text(308, right+5, 247, 8.5, "计价币种  "+report.Currency.Code+"  /  换算率 "+report.Currency.Rate)
	filters := make([]string, 0, 4)
	if report.Filter.Model != "" {
		filters = append(filters, "模型 "+report.Filter.Model)
	}
	if report.Filter.Group != "" {
		filters = append(filters, "分组 "+report.Filter.Group)
	}
	if report.Filter.ChannelID > 0 {
		filters = append(filters, fmt.Sprintf("渠道 #%d", report.Filter.ChannelID))
	}
	if report.Filter.Protocol != "" {
		filters = append(filters, "协议 "+report.Filter.Protocol)
	}
	filterText := "全部模型 / 全部分组 / 全部渠道 / 全部协议"
	if len(filters) > 0 {
		filterText = strings.Join(filters, " / ")
	}
	end := doc.text(40, max(left, right)+10, 515, 8, "筛选范围  "+filterText)
	y := max(310.0, end+17)
	for i, metric := range []struct {
		label string
		count int64
	}{
		{"输入（不含缓存）", report.Summary.Tokens.Input},
		{"缓存（读取 + 写入）", report.Summary.Tokens.CacheRead + report.Summary.Tokens.CacheWrite},
		{"输出", report.Summary.Tokens.Output},
	} {
		x := 40 + float64(i)*176
		pdf.SetFillColor(232, 239, 249)
		pdf.RectFromUpperLeftWithStyle(x, y, 163, 50, "F")
		pdf.SetTextColor(83, 102, 129)
		doc.text(x+10, y+8, 143, 8, metric.label)
		pdf.SetTextColor(25, 51, 94)
		doc.right(x+10, y+24, 143, 15, fmt.Sprint(metric.count))
	}
	pdf.SetTextColor(89, 105, 127)
	y = doc.text(40, y+60, 515, 8.5, fmt.Sprintf("缓存拆分：读取 %d Token / 写入 %d Token。", report.Summary.Tokens.CacheRead, report.Summary.Tokens.CacheWrite))
	value := report.Summary.InputRecalculation
	y += 15
	pdf.SetFillColor(232, 239, 249)
	pdf.RectFromUpperLeftWithStyle(40, y, 515, 76, "F")
	pdf.SetTextColor(83, 102, 129)
	doc.text(52, y+9, 491, 9, "需重算的缓存读取 Token")
	pdf.SetTextColor(25, 51, 94)
	doc.text(52, y+27, 491, 24, fmt.Sprint(value.Tokens))
	doc.text(52, y+56, 491, 9, fmt.Sprintf("需重算 %d 笔 / 有缓存写入，已排除 %d 笔", value.RequestCount, value.ExcludedWriteRequestCount))
	y += 88
	pdf.SetTextColor(35, 49, 69)
	y = doc.text(40, y, 515, 9, "额外输入 Token = SUM(旧 input_tokens - 正确普通输入) = 缓存读取 Token 合计")
	pdf.SetTextColor(89, 105, 127)
	y = doc.text(40, y+8, 515, 8.5, "仅汇总无缓存写入请求中的缓存读取量。正常缓存读取费用保留，只重算额外按普通输入价收取的那一份。")
	y += 16
	for i, metric := range []struct{ label, amount string }{
		{"额外费用参考 / 平台输入价", value.PlatformAmount},
		{"额外费用参考 / 历史基准价", value.ListAmount},
	} {
		x := 40 + float64(i)*264
		pdf.SetFillColor(245, 247, 251)
		pdf.RectFromUpperLeftWithStyle(x, y, 251, 46, "F")
		pdf.SetTextColor(83, 102, 129)
		doc.text(x+12, y+8, 227, 8, metric.label)
		pdf.SetTextColor(25, 51, 94)
		amount := metric.amount
		if value.UnpricedTokens > 0 {
			amount = ""
		}
		doc.usageAuditMoney(x+12, y+24, 227, 13, amount)
	}
	y += 58
	pdf.SetTextColor(89, 105, 127)
	if report.RecalculationBasis == "legacy_openai_cache_write_tokens" {
		y = doc.text(40, y, 515, 8, "核算前提：历史上游使用 cache_write_tokens，有缓存写入的请求排除；只汇总旧转换未扣除的纯缓存读取量。") + 8
	}
	y = doc.text(40, y, 515, 8, fmt.Sprintf("本期 %d 条记录 / 缺少可信 Token 用量 %d 条，未纳入 Token 核算。", report.Summary.Count, report.Summary.UnknownUsageCount))
	y = doc.text(40, y+8, 515, 8, fmt.Sprintf("金额币种 %s，仅作额外输入费用参考：逐笔采用历史普通输入价，平台价含分组倍率，基准价不含倍率；缺价 %d Token 不猜价。", report.Currency.Code, value.UnpricedTokens))
	y = doc.text(40, y+8, 515, 8, "按模型的 Token 与重复部分见后页；逐笔请求和单价依据见 CSV / JSON 明细。本报告不执行退款或修改账目。")
	y = doc.text(40, y+10, 515, 6.5, "数据指纹  "+report.SHA256)
	if y > 767 {
		return nil, errors.New("usage audit cover exceeds page boundary")
	}

	for index, sections := range pages {
		doc.headerV3(branding, index+2, len(pages)+1)
		pdf.SetTextColor(25, 51, 94)
		doc.text(40, 120, 515, 18, "按模型统计")
		pdf.SetTextColor(89, 105, 127)
		doc.text(40, 151, 515, 8.5, "输入、缓存、输出分别统计；单列额外计入普通输入的缓存读取量。")
		y = 178
		for _, section := range sections {
			pdf.SetTextColor(25, 51, 94)
			y = doc.text(40, y, 515, 11, section.group.Label)
			pdf.SetTextColor(89, 105, 127)
			y = doc.text(40, y+4, 515, 8, fmt.Sprintf("记录数 %d / 缺少可信用量 %d", section.group.Count, section.group.UnknownUsageCount))
			y = doc.usageAuditTokenTable(section.group.UsageAuditSummary, y+8) + 23
		}
	}
	if doc.err != nil {
		return nil, doc.err
	}
	if missingGlyph {
		return nil, errors.New("usage audit PDF contains unsupported font glyphs")
	}
	return pdf.GetBytesPdfReturnErr()
}

func CreateUsageAuditPDF(ctx context.Context, input UsageAuditExportInput) ([]byte, error) {
	report, err := GetUsageAuditExportReport(ctx, input)
	if err != nil {
		return nil, err
	}
	branding, err := captureBillingDocumentBranding(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := &BillingSnapshot{Issuer: branding.Issuer, PDFLogoPNG: branding.LogoPNG, OperatingName: branding.OperatingName, PDFOperatingLogoPNG: branding.OperatingLogoPNG, PDFFooter: branding.Footer, CompanyTitle: "全部客户"}
	if input.Filter.UserID > 0 {
		customer, err := model.GetUserById(input.Filter.UserID, false)
		if err != nil {
			return nil, err
		}
		account, err := model.GetBillingAccount(input.Filter.UserID)
		if err != nil {
			return nil, err
		}
		snapshot.Username, snapshot.CompanyTitle = customer.Username, account.CompanyTitle
		if snapshot.CompanyTitle == "" {
			snapshot.CompanyTitle = customer.DisplayName
		}
		if snapshot.CompanyTitle == "" {
			snapshot.CompanyTitle = customer.Username
		}
	}
	return RenderUsageAuditPDF(report, snapshot)
}
