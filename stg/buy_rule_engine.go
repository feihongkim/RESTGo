package stg

import (
	"RESTGo/box"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

// ConditionFn 은 매수 조건 평가 함수 타입
type ConditionFn func(ctx *box.TradingContext, s Settings) bool

// RuleConfig 는 YAML에서 읽어들이는 전략 정의
type RuleConfig struct {
	Name        string   `yaml:"name"`
	DefCount    int      `yaml:"def_count"`     // 정확히 일치 (0 = 무관)
	DefCountMin int      `yaml:"def_count_min"` // 이상 조건 (0 = 무관)
	When        []string `yaml:"when"`          // AND 조건
	AnyOf       []string `yaml:"any_of"`        // OR 조건 (하나 이상)
	WhenNot     []string `yaml:"when_not"`      // 반드시 false여야 하는 조건
	Signal      string   `yaml:"signal"`
	Evaluation  string   `yaml:"evaluation"` // "on_breakout" (기본) or "per_candle"
}

// strategiesFile 은 YAML 파일 최상위 구조
type strategiesFile struct {
	Settings   map[string]interface{} `yaml:"settings"`
	Strategies []RuleConfig           `yaml:"strategies"`
}

// LoadRules 는 YAML 파일에서 전략 목록을 읽어 반환 (설정 오버라이드 없음)
func LoadRules(path string) ([]RuleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg strategiesFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.Strategies, nil
}

// LoadRulesWithSettings 는 YAML 파일에서 전략 목록과 Settings 오버라이드를 함께 로드한다.
// YAML의 settings 블록 값이 DefaultSettings()를 덮어쓴다. 알 수 없는 키는 무시된다.
func LoadRulesWithSettings(path string) ([]RuleConfig, Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Settings{}, err
	}
	var cfg strategiesFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, Settings{}, err
	}
	s := DefaultSettings()
	applySettingsOverrides(&s, cfg.Settings)
	return cfg.Strategies, s, nil
}

// ApplySettingsOverrides 는 map[string]interface{} 오버라이드를 Settings에 적용한다.
// 외부 패키지(그리드 러너 등)에서 사용할 수 있도록 익스포트.
func ApplySettingsOverrides(s *Settings, overrides map[string]interface{}) {
	applySettingsOverrides(s, overrides)
}

// applySettingsOverrides 는 map[string]interface{} 형태의 YAML 설정을 Settings 구조체에 적용한다.
// 알 수 없는 키는 조용히 무시한다 (전방 호환성).
func applySettingsOverrides(s *Settings, overrides map[string]interface{}) {
	if overrides == nil {
		return
	}
	for k, v := range overrides {
		switch k {
		case "VolumeLimit":
			if f, ok := toFloat64(v); ok {
				s.VolumeLimit = f
			}
		case "DefBoxNearPriceThreshold":
			if f, ok := toFloat64(v); ok {
				s.DefBoxNearPriceThreshold = f
			}
		case "MainBoxNearPriceThreshold":
			if f, ok := toFloat64(v); ok {
				s.MainBoxNearPriceThreshold = f
			}
		case "ATREntryFilterEnabled":
			if b, ok := v.(bool); ok {
				s.ATREntryFilterEnabled = b
			}
		case "ATREntryMaxThreshold":
			if f, ok := toFloat64(v); ok {
				s.ATREntryMaxThreshold = f
			}
		case "DefBoxUpperWickToBodyRatioThreshold":
			if f, ok := toFloat64(v); ok {
				s.DefBoxUpperWickToBodyRatioThreshold = f
			}
		case "MaSpreadThreshold":
			if f, ok := toFloat64(v); ok {
				s.MaSpreadThreshold = f
			}
		case "RSIOversoldThreshold":
			if f, ok := toFloat64(v); ok {
				s.RSIOversoldThreshold = f
			}
		case "RSIOverboughtThreshold":
			if f, ok := toFloat64(v); ok {
				s.RSIOverboughtThreshold = f
			}
		case "RSIRecoveryLookback":
			if i, ok := toInt(v); ok {
				s.RSIRecoveryLookback = i
			}
		case "RSIRisingPeriod":
			if i, ok := toInt(v); ok {
				s.RSIRisingPeriod = i
			}
		case "RSIBullZoneLow":
			if f, ok := toFloat64(v); ok {
				s.RSIBullZoneLow = f
			}
		case "RSIBullZoneHigh":
			if f, ok := toFloat64(v); ok {
				s.RSIBullZoneHigh = f
			}
		case "BBReboundLookback":
			if i, ok := toInt(v); ok {
				s.BBReboundLookback = i
			}
		case "BBReboundPercentB":
			if f, ok := toFloat64(v); ok {
				s.BBReboundPercentB = f
			}
		case "BBSqueezeLookback":
			if i, ok := toInt(v); ok {
				s.BBSqueezeLookback = i
			}
		case "BBSqueezeWidthThreshold":
			if f, ok := toFloat64(v); ok {
				s.BBSqueezeWidthThreshold = f
			}
		case "BBBreakoutPercentB":
			if f, ok := toFloat64(v); ok {
				s.BBBreakoutPercentB = f
			}
		case "BBMiddleHoldDuration":
			if i, ok := toInt(v); ok {
				s.BBMiddleHoldDuration = i
			}
		case "MaConvergenceThreshold":
			if f, ok := toFloat64(v); ok {
				s.MaConvergenceThreshold = f
			}
		case "FeeRate":
			if f, ok := toFloat64(v); ok {
				s.FeeRate = f
			}
		case "SlippageRate":
			if f, ok := toFloat64(v); ok {
				s.SlippageRate = f
			}
		case "PerCandleCooldownBars":
			if i, ok := toInt(v); ok {
				s.PerCandleCooldownBars = i
			}
		case "ADXTrendThreshold":
			if f, ok := toFloat64(v); ok {
				s.ADXTrendThreshold = f
			}
		case "VWAPDeviationK":
			if f, ok := toFloat64(v); ok {
				s.VWAPDeviationK = f
			}
		case "VolumeZScoreThreshold":
			if f, ok := toFloat64(v); ok {
				s.VolumeZScoreThreshold = f
			}
		case "VolumeZScoreWindow":
			if i, ok := toInt(v); ok {
				s.VolumeZScoreWindow = i
			}
		case "OBVRisingPeriod":
			if i, ok := toInt(v); ok {
				s.OBVRisingPeriod = i
			}
		case "SuperTrendPeriod":
			if i, ok := toInt(v); ok {
				s.SuperTrendPeriod = i
			}
		case "SuperTrendMultiplier":
			if f, ok := toFloat64(v); ok {
				s.SuperTrendMultiplier = f
			}
		case "DonchianPeriod":
			if i, ok := toInt(v); ok {
				s.DonchianPeriod = i
			}
		case "NarrowRangePeriod":
			if i, ok := toInt(v); ok {
				s.NarrowRangePeriod = i
			}
		case "MACDHistRisingBars":
			if i, ok := toInt(v); ok {
				s.MACDHistRisingBars = i
			}
		case "StochOversoldThreshold":
			if f, ok := toFloat64(v); ok {
				s.StochOversoldThreshold = f
			}
		case "StochOverboughtThreshold":
			if f, ok := toFloat64(v); ok {
				s.StochOverboughtThreshold = f
			}
		}
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func toInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	}
	return 0, false
}

// evaluateSingleRule 는 하나의 룰을 컨텍스트에 대해 평가한다.
// 매칭 시 (signal, ruleName)을 반환하고, 매칭 실패 시 ("", "")을 반환한다.
// DefCount 필터는 per_candle 룰에는 적용하지 않는다 (호출자가 제어).
func evaluateSingleRule(rule RuleConfig, ctx *box.TradingContext, s Settings) (string, string) {
	// when: 모두 true여야 함
	for _, name := range rule.When {
		fn, ok := conditionRegistry[name]
		if !ok {
			fmt.Printf("[rule] 미등록 조건: %s\n", name)
			return "", ""
		}
		if !fn(ctx, s) {
			return "", ""
		}
	}

	// when_not: 모두 false여야 함
	for _, name := range rule.WhenNot {
		fn, ok := conditionRegistry[name]
		if !ok {
			fmt.Printf("[rule] 미등록 조건: %s\n", name)
			return "", ""
		}
		if fn(ctx, s) {
			return "", ""
		}
	}

	// any_of: 하나 이상 true여야 함
	if len(rule.AnyOf) > 0 {
		anyPassed := false
		for _, name := range rule.AnyOf {
			fn, ok := conditionRegistry[name]
			if ok && fn(ctx, s) {
				anyPassed = true
				break
			}
		}
		if !anyPassed {
			return "", ""
		}
	}

	return rule.Signal, rule.Name
}

// EvaluateRules 는 등록된 전략 목록에 대해 순서대로 평가하고
// 첫 번째로 매칭된 전략의 신호 이름을 반환. 없으면 "".
// C# BuyDecisionProcessor.Options의 LastBuySignalPosition_StrategyN 포팅:
// 같은 DefBox 구간에서 이미 발화한 전략은 건너뛴다 (중복 신호 방지).
// 기록은 DefBox 변경 시 analyzer의 ResetBuySignalPositions()로 초기화된다.
// per_candle 룰은 이 경로를 건너뛴다 (evaluatePerCandleSignals에서 처리).
func EvaluateRules(rules []RuleConfig, ctx *box.TradingContext, s Settings) (string, string) {
	for _, rule := range rules {
		// per_candle 룰은 돌파 경로에서 제외
		if rule.Evaluation == "per_candle" {
			continue
		}

		// 전략별 중복 신호 방지: 이번 DefBox에서 이미 발화한 전략은 스킵
		if _, fired := ctx.LastBuySignalPosition[rule.Name]; fired {
			continue
		}

		// DefCount 정확 일치 필터
		if rule.DefCount > 0 && ctx.DefCount != rule.DefCount {
			continue
		}
		// DefCount 최솟값 필터
		if rule.DefCountMin > 0 && ctx.DefCount < rule.DefCountMin {
			continue
		}

		sig, name := evaluateSingleRule(rule, ctx, s)
		if sig != "" {
			ctx.LastBuySignalPosition[rule.Name] = ctx.Position
			return sig, name
		}
	}
	return "", ""
}

// conditionRegistry 는 조건명 → 평가 함수 매핑
var conditionRegistry = map[string]ConditionFn{}

// RegisterCondition 은 조건명과 함수를 등록
func RegisterCondition(name string, fn ConditionFn) {
	conditionRegistry[name] = fn
}

// ConditionRegistryGet 은 조건명으로 함수를 조회 (테스트/검증용)
func ConditionRegistryGet(name string) (ConditionFn, bool) {
	fn, ok := conditionRegistry[name]
	return fn, ok
}
