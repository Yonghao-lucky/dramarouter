package ratio_setting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// 峰谷定价：在基础模型定价之上，按请求发生时刻对输入 / 输出 / 缓存分别乘一个系数。
//
// 设计要点：
//   - 系数在请求开始时求值一次，预扣与结算都从同一份 PriceData 读取，避免跨时段错账。
//   - 系数支持纯数字，也支持表达式；表达式可读取配置时区下的 hour / minute / weekday 等变量。
//   - 未命中任何峰时规则时使用谷时系数（默认 1），规则之间不叠加。

const (
	PeakPricingDefaultTimezone = "Asia/Shanghai"
)

// PeakPricingRule 描述一个峰时段。Start/End 为 "HH:MM"，End <= Start 表示跨夜。
type PeakPricingRule struct {
	Name             string `json:"name"`
	Days             []int  `json:"days"` // 0=周日 … 6=周六；为空表示每天
	Start            string `json:"start"`
	End              string `json:"end"`
	InputFactor      string `json:"input_factor"`
	OutputFactor     string `json:"output_factor"`
	CacheFactor      string `json:"cache_factor"`
	CacheWriteFactor string `json:"cache_write_factor"`
	// 5 分钟 / 1 小时缓存写入可单独定价；留空时沿用 CacheWriteFactor。
	CacheWrite5mFactor string `json:"cache_write_5m_factor"`
	CacheWrite1hFactor string `json:"cache_write_1h_factor"`
}

// PeakPricingModelOverride 按模型覆盖峰谷规则。
// 命中的模型只使用这里的规则，不再叠加全局规则，便于不同模型各自作息。
type PeakPricingModelOverride struct {
	Model   string            `json:"model"` // 精确名称，或以 * 结尾的通配
	Rules   []PeakPricingRule `json:"rules"`
	OffPeak PeakPricingRule   `json:"off_peak"`
}

type PeakPricingSetting struct {
	Enabled  bool              `json:"enabled"`
	Timezone string            `json:"timezone"`
	Models   []string          `json:"models"` // 为空表示全部模型；支持结尾 * 通配
	Rules    []PeakPricingRule `json:"rules"`
	// OffPeak 为未命中峰时规则时的系数；为空表示全部为 1。
	OffPeak PeakPricingRule `json:"off_peak"`
	// ModelOverrides 优先于全局 Models/Rules，匹配顺序为配置顺序。
	ModelOverrides []PeakPricingModelOverride `json:"model_overrides"`
}

// PeakFactors 是某次请求最终采用的系数。
type PeakFactors struct {
	Input      float64
	Output     float64
	Cache      float64
	CacheWrite float64
	// 5 分钟 / 1 小时缓存写入；未单独配置时等于 CacheWrite。
	CacheWrite5m float64
	CacheWrite1h float64
	RuleName     string // 命中的峰时规则名；空表示谷时
}

// IsNeutral 表示所有系数都是 1，计价链路可以完全跳过折算。
func (f PeakFactors) IsNeutral() bool {
	return f.Input == 1 && f.Output == 1 && f.Cache == 1 &&
		f.CacheWrite == 1 && f.CacheWrite5m == 1 && f.CacheWrite1h == 1
}

var (
	peakPricingSetting   = PeakPricingSetting{Timezone: PeakPricingDefaultTimezone}
	peakPricingLock      = sync.RWMutex{}
	peakFactorPrograms   = make(map[string]*vm.Program)
	peakFactorProgLock   = sync.Mutex{}
	peakFactorEvalFailed sync.Map
)

func GetPeakPricingSetting() PeakPricingSetting {
	peakPricingLock.RLock()
	defer peakPricingLock.RUnlock()
	return peakPricingSetting
}

func PeakPricing2JSONString() string {
	data, err := common.Marshal(GetPeakPricingSetting())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func UpdatePeakPricingByJSONString(jsonString string) error {
	var setting PeakPricingSetting
	if strings.TrimSpace(jsonString) == "" {
		jsonString = "{}"
	}
	if err := common.Unmarshal([]byte(jsonString), &setting); err != nil {
		return err
	}
	if err := ValidatePeakPricingSetting(setting); err != nil {
		return err
	}
	setting.Timezone = normalizePeakTimezone(setting.Timezone)
	peakPricingLock.Lock()
	peakPricingSetting = setting
	peakPricingLock.Unlock()
	return nil
}

func normalizePeakTimezone(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return PeakPricingDefaultTimezone
	}
	return tz
}

func ValidatePeakPricingSetting(setting PeakPricingSetting) error {
	if tz := strings.TrimSpace(setting.Timezone); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return fmt.Errorf("时区无效: %s", tz)
		}
	}
	for _, name := range setting.Models {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("模型名称不能为空")
		}
	}
	rules := make([]PeakPricingRule, 0, len(setting.Rules)+1)
	rules = append(rules, setting.Rules...)
	rules = append(rules, setting.OffPeak)
	validateRules := func(rules []PeakPricingRule) error {
		for _, rule := range rules {
			if _, _, err := parsePeakClock(rule.Start); err != nil {
				return fmt.Errorf("规则 %s 开始时间无效: %v", rule.Name, err)
			}
			if _, _, err := parsePeakClock(rule.End); err != nil {
				return fmt.Errorf("规则 %s 结束时间无效: %v", rule.Name, err)
			}
			for _, day := range rule.Days {
				if day < 0 || day > 6 {
					return fmt.Errorf("规则 %s 的星期取值必须在 0-6 之间", rule.Name)
				}
			}
			for field, value := range map[string]string{
				"input_factor":          rule.InputFactor,
				"output_factor":         rule.OutputFactor,
				"cache_factor":          rule.CacheFactor,
				"cache_write_factor":    rule.CacheWriteFactor,
				"cache_write_5m_factor": rule.CacheWrite5mFactor,
				"cache_write_1h_factor": rule.CacheWrite1hFactor,
			} {
				if err := validatePeakFactor(field, value); err != nil {
					return fmt.Errorf("规则 %s: %w", rule.Name, err)
				}
			}
		}
		return nil
	}
	if err := validateRules(rules); err != nil {
		return err
	}

	seenOverrides := make(map[string]struct{}, len(setting.ModelOverrides))
	for _, override := range setting.ModelOverrides {
		pattern := strings.TrimSpace(override.Model)
		if pattern == "" {
			return fmt.Errorf("按模型覆盖的模型名称不能为空")
		}
		if _, ok := seenOverrides[pattern]; ok {
			return fmt.Errorf("模型 %s 的峰谷覆盖重复", pattern)
		}
		seenOverrides[pattern] = struct{}{}
		overrideRules := make([]PeakPricingRule, 0, len(override.Rules)+1)
		overrideRules = append(overrideRules, override.Rules...)
		overrideRules = append(overrideRules, override.OffPeak)
		if err := validateRules(overrideRules); err != nil {
			return fmt.Errorf("模型 %s: %w", pattern, err)
		}
	}
	return nil
}

func validatePeakFactor(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		if math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
			return fmt.Errorf("%s 必须大于 0", field)
		}
		return nil
	}
	// 表达式在请求期求值；这里只做语法校验，避免写入后才在线上暴露错误。
	if _, err := compilePeakFactor(value); err != nil {
		return fmt.Errorf("%s 表达式无效: %v", field, err)
	}
	return nil
}

func compilePeakFactor(source string) (*vm.Program, error) {
	peakFactorProgLock.Lock()
	defer peakFactorProgLock.Unlock()
	if program, ok := peakFactorPrograms[source]; ok {
		return program, nil
	}
	program, err := expr.Compile(source, expr.Env(peakFactorEnv(time.Now())), expr.AsFloat64())
	if err != nil {
		return nil, err
	}
	peakFactorPrograms[source] = program
	return program, nil
}

// peakFactorEnv 暴露配置时区与 UTC 两套时间变量，供系数表达式使用。
func peakFactorEnv(now time.Time) map[string]any {
	location, err := time.LoadLocation(GetPeakPricingSetting().Timezone)
	if err != nil {
		location = time.UTC
	}
	local := now.In(location)
	utc := now.UTC()
	return map[string]any{
		"hour":        local.Hour(),
		"minute":      local.Minute(),
		"weekday":     int(local.Weekday()),
		"month":       int(local.Month()),
		"day":         local.Day(),
		"utc_hour":    utc.Hour(),
		"utc_minute":  utc.Minute(),
		"utc_weekday": int(utc.Weekday()),
		"max":         math.Max,
		"min":         math.Min,
		"abs":         math.Abs,
		"ceil":        math.Ceil,
		"floor":       math.Floor,
	}
}

func evalPeakFactor(source string, now time.Time, fallback float64) float64 {
	source = strings.TrimSpace(source)
	if source == "" {
		return fallback
	}
	if number, err := strconv.ParseFloat(source, 64); err == nil {
		if math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
			return fallback
		}
		return number
	}
	program, err := compilePeakFactor(source)
	if err != nil {
		logPeakFactorFailure(source, err)
		return fallback
	}
	result, err := expr.Run(program, peakFactorEnv(now))
	if err != nil {
		logPeakFactorFailure(source, err)
		return fallback
	}
	value, ok := result.(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		logPeakFactorFailure(source, fmt.Errorf("结果无效: %v", result))
		return fallback
	}
	return value
}

// logPeakFactorFailure 对同一表达式只记录一次，避免高频请求刷日志。
func logPeakFactorFailure(source string, err error) {
	if _, loaded := peakFactorEvalFailed.LoadOrStore(source, struct{}{}); loaded {
		return
	}
	common.SysError(fmt.Sprintf("峰谷定价系数表达式求值失败，已按 1 处理: %s: %v", source, err))
}

// parsePeakClock 解析 "HH:MM"，返回当天分钟数。
func parsePeakClock(value string) (int, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("需要 HH:MM 格式")
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("小时无效")
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("分钟无效")
	}
	return hour, minute, nil
}

func peakRuleMatches(rule PeakPricingRule, now time.Time, location *time.Location) bool {
	local := now.In(location)
	if len(rule.Days) > 0 {
		matched := false
		for _, day := range rule.Days {
			if day == int(local.Weekday()) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	startHour, startMinute, startErr := parsePeakClock(rule.Start)
	endHour, endMinute, endErr := parsePeakClock(rule.End)
	if startErr != nil || endErr != nil {
		return false
	}
	start := startHour*60 + startMinute
	end := endHour*60 + endMinute
	current := local.Hour()*60 + local.Minute()
	if start == end {
		// 起止相同视为全天生效，便于只按星期区分峰谷。
		return true
	}
	if start < end {
		return current >= start && current < end
	}
	// 跨夜窗口，例如 22:00 → 06:00。
	return current >= start || current < end
}

func peakModelMatches(patterns []string, modelName string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == "*" {
			return true
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(modelName, strings.TrimSuffix(pattern, "*")) {
				return true
			}
			continue
		}
		if pattern == modelName {
			return true
		}
	}
	return false
}

// GetPeakFactors 返回给定模型在指定时刻生效的峰谷系数。
// 优先匹配按模型覆盖；其次按全局模型范围过滤；都未命中时返回中性系数。
func GetPeakFactors(modelName string, at time.Time) PeakFactors {
	neutral := PeakFactors{Input: 1, Output: 1, Cache: 1, CacheWrite: 1, CacheWrite5m: 1, CacheWrite1h: 1}
	setting := GetPeakPricingSetting()
	if !setting.Enabled {
		return neutral
	}
	location, err := time.LoadLocation(normalizePeakTimezone(setting.Timezone))
	if err != nil {
		location = time.UTC
	}

	rules := setting.Rules
	offPeak := setting.OffPeak
	if override, ok := findPeakModelOverride(setting.ModelOverrides, modelName); ok {
		rules = override.Rules
		offPeak = override.OffPeak
	} else if !peakModelMatches(setting.Models, modelName) {
		return neutral
	}

	for _, rule := range rules {
		if !peakRuleMatches(rule, at, location) {
			continue
		}
		return peakFactorsFromRule(rule, at)
	}
	return peakFactorsFromRule(offPeak, at)
}

// peakFactorsFromRule 求值单条规则的系数；未单独配置的 5m/1h 缓存写入沿用 CacheWriteFactor。
func peakFactorsFromRule(rule PeakPricingRule, at time.Time) PeakFactors {
	cacheWrite := evalPeakFactor(rule.CacheWriteFactor, at, 1)
	cacheWrite5m := evalPeakFactor(rule.CacheWrite5mFactor, at, cacheWrite)
	cacheWrite1h := evalPeakFactor(rule.CacheWrite1hFactor, at, cacheWrite)
	return PeakFactors{
		Input:        evalPeakFactor(rule.InputFactor, at, 1),
		Output:       evalPeakFactor(rule.OutputFactor, at, 1),
		Cache:        evalPeakFactor(rule.CacheFactor, at, 1),
		CacheWrite:   cacheWrite,
		CacheWrite5m: cacheWrite5m,
		CacheWrite1h: cacheWrite1h,
		RuleName:     rule.Name,
	}
}

func findPeakModelOverride(overrides []PeakPricingModelOverride, modelName string) (PeakPricingModelOverride, bool) {
	for _, override := range overrides {
		name := strings.TrimSpace(override.Model)
		if name == "" {
			continue
		}
		if strings.HasSuffix(name, "*") {
			if strings.HasPrefix(modelName, strings.TrimSuffix(name, "*")) {
				return override, true
			}
			continue
		}
		if name == modelName {
			return override, true
		}
	}
	return PeakPricingModelOverride{}, false
}

// PeakWindowView 是给前端展示用的峰时段描述。
type PeakWindowView struct {
	Name               string  `json:"name"`
	Days               []int   `json:"days"`
	Start              string  `json:"start"`
	End                string  `json:"end"`
	InputFactor        float64 `json:"input_factor"`
	OutputFactor       float64 `json:"output_factor"`
	CacheFactor        float64 `json:"cache_factor"`
	CacheWriteFactor   float64 `json:"cache_write_factor"`
	CacheWrite5mFactor float64 `json:"cache_write_5m_factor"`
	CacheWrite1hFactor float64 `json:"cache_write_1h_factor"`
}

// PeakPricingView 描述某模型当前生效的峰谷策略，用于模型广场展示。
type PeakPricingView struct {
	Timezone string           `json:"timezone"`
	Source   string           `json:"source"` // model | global
	Windows  []PeakWindowView `json:"windows"`
	OffPeak  PeakWindowView   `json:"off_peak"`
}

// GetModelPeakPricingView 返回模型当前生效的峰谷策略；未启用或不适用时返回 nil。
// 系数按当前时刻求值，因此广场展示的是「此刻实际生效」的价格倍数。
func GetModelPeakPricingView(modelName string, at time.Time) *PeakPricingView {
	setting := GetPeakPricingSetting()
	if !setting.Enabled {
		return nil
	}
	override, hasOverride := findPeakModelOverride(setting.ModelOverrides, modelName)
	if !hasOverride && !peakModelMatches(setting.Models, modelName) {
		return nil
	}
	rules := setting.Rules
	offPeak := setting.OffPeak
	source := "global"
	if hasOverride {
		rules = override.Rules
		offPeak = override.OffPeak
		source = "model"
	}
	view := &PeakPricingView{
		Timezone: normalizePeakTimezone(setting.Timezone),
		Source:   source,
		Windows:  make([]PeakWindowView, 0, len(rules)),
	}
	for _, rule := range rules {
		view.Windows = append(view.Windows, peakWindowView(rule, at))
	}
	offPeakView := peakWindowView(offPeak, at)
	offPeakView.Name = "off_peak"
	view.OffPeak = offPeakView
	return view
}

// peakWindowView 把规则求值为可直接展示的系数；5m/1h 未单独配置时沿用 CacheWrite。
func peakWindowView(rule PeakPricingRule, at time.Time) PeakWindowView {
	cacheWrite := evalPeakFactor(rule.CacheWriteFactor, at, 1)
	return PeakWindowView{
		Name:               rule.Name,
		Days:               rule.Days,
		Start:              rule.Start,
		End:                rule.End,
		InputFactor:        evalPeakFactor(rule.InputFactor, at, 1),
		OutputFactor:       evalPeakFactor(rule.OutputFactor, at, 1),
		CacheFactor:        evalPeakFactor(rule.CacheFactor, at, 1),
		CacheWriteFactor:   cacheWrite,
		CacheWrite5mFactor: evalPeakFactor(rule.CacheWrite5mFactor, at, cacheWrite),
		CacheWrite1hFactor: evalPeakFactor(rule.CacheWrite1hFactor, at, cacheWrite),
	}
}
