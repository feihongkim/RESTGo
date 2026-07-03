package stg

import (
	"fmt"
	"os"
	"sort"

	"RESTGo/box"

	"gopkg.in/yaml.v3"
)

// SellRuleConfig 는 매도 룰 YAML 한 항목.
type SellRuleConfig struct {
	Name              string       `yaml:"name"`
	Path              string       `yaml:"path"`     // critical/composite/extension/expiry/individual
	Priority          int          `yaml:"priority"` // individual Path 내 우선순위 (작을수록 우선)
	When              []string     `yaml:"when"`     // AND
	AnyOf             []string     `yaml:"any_of"`   // OR
	WhenNot           []string     `yaml:"when_not"` // NOT
	Tracking          SellTracking `yaml:"tracking"`
	Weight            float64      `yaml:"weight"`             // 부분 매도 비율
	CompositeEligible bool         `yaml:"composite_eligible"` // Composite Path 합산 대상
	CompositeWeight   float64      `yaml:"composite_weight"`   // Composite 합산용 가중치
	CanExtend         bool         `yaml:"can_extend"`         // Expiry Path에서 연장 가능 여부 평가
	Category          string       `yaml:"category"`           // "Critical"/"Profit"/"Loss"/"Technical"/"Extension"/"Expiry"
}

// SellStrategyFile 은 sell_default.yaml 최상위 구조.
type SellStrategyFile struct {
	Settings  SellSettingsYAML `yaml:"settings"`
	Composite CompositeYAML    `yaml:"composite"`
	SellRules []SellRuleConfig `yaml:"sell_rules"`
}

// SellSettingsYAML 은 YAML에서 받는 전역 설정 (DefaultSellSettings에 덮어쓴다).
type SellSettingsYAML struct {
	MaxHoldingPeriod        *int                  `yaml:"max_holding_period"`
	MinHoldingPeriod        *int                  `yaml:"min_holding_period"`
	AutoLiquidateOnExpiry   *bool                 `yaml:"auto_liquidate_on_expiry"`
	DefaultSellWeight       *float64              `yaml:"default_sell_weight"`
	SmallRemainingThreshold *float64              `yaml:"small_remaining_threshold"`
	MinimumExecutionSize    *float64              `yaml:"minimum_execution_size"`
	CriticalFailure         *CriticalFailureYAML  `yaml:"critical_failure"`
}

// CriticalFailureYAML 은 IsCriticalFailure 임계값 YAML 오버라이드.
// 모든 필드는 포인터로 선언하여 누락 시 DefaultSellSettings 값을 유지한다.
type CriticalFailureYAML struct {
	DailyDropThreshold      *float64 `yaml:"daily_drop_threshold"`
	PanicVolumeMultiplier   *float64 `yaml:"panic_volume_multiplier"`
	PanicMinDropRate        *float64 `yaml:"panic_min_drop_rate"`
	CumulativeDropThreshold *float64 `yaml:"cumulative_drop_threshold"`
	CumulativeDropDays      *int     `yaml:"cumulative_drop_days"`
	MAReversalDays          *int     `yaml:"ma_reversal_days"`
}

// CompositeYAML 은 Composite Path 임계값/가중치.
type CompositeYAML struct {
	ThresholdHighRecovery   *float64 `yaml:"threshold_high_recovery"`
	ThresholdMediumRecovery *float64 `yaml:"threshold_medium_recovery"`
	ThresholdLowRecovery    *float64 `yaml:"threshold_low_recovery"`
	WeightStrong            *float64 `yaml:"weight_strong"`
	WeightMedium            *float64 `yaml:"weight_medium"`
	WeightWeak              *float64 `yaml:"weight_weak"`
}

// activeSellRules 는 LoadSellStrategy에서 채워지는 활성 매도 룰 목록.
var activeSellRules []SellRuleConfig

// LoadSellStrategy 는 YAML에서 매도 룰과 설정을 로드한다.
// 반환된 SellSettings는 DefaultSellSettings + YAML 오버라이드 결과.
func LoadSellStrategy(path string) (SellSettings, error) {
	settings := DefaultSellSettings()
	data, err := os.ReadFile(path)
	if err != nil {
		return settings, fmt.Errorf("매도 전략 로드 실패 (%s): %w", path, err)
	}
	var sf SellStrategyFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return settings, fmt.Errorf("매도 전략 YAML 파싱 실패: %w", err)
	}

	// 글로벌 설정 오버라이드
	if v := sf.Settings.MaxHoldingPeriod; v != nil {
		settings.MaxHoldingPeriod = *v
	}
	if v := sf.Settings.MinHoldingPeriod; v != nil {
		settings.MinHoldingPeriod = *v
	}
	if v := sf.Settings.AutoLiquidateOnExpiry; v != nil {
		settings.AutoLiquidateOnExpiry = *v
	}
	if v := sf.Settings.DefaultSellWeight; v != nil {
		settings.DefaultSellWeight = *v
	}
	if v := sf.Settings.SmallRemainingThreshold; v != nil {
		settings.SmallRemainingThreshold = *v
	}
	if v := sf.Settings.MinimumExecutionSize; v != nil {
		settings.MinimumExecutionSize = *v
	}

	// Composite 설정 오버라이드
	if v := sf.Composite.ThresholdHighRecovery; v != nil {
		settings.CompositeThresholdHighRecovery = *v
	}
	if v := sf.Composite.ThresholdMediumRecovery; v != nil {
		settings.CompositeThresholdMediumRecovery = *v
	}
	if v := sf.Composite.ThresholdLowRecovery; v != nil {
		settings.CompositeThresholdLowRecovery = *v
	}
	if v := sf.Composite.WeightStrong; v != nil {
		settings.CompositeWeightStrong = *v
	}
	if v := sf.Composite.WeightMedium; v != nil {
		settings.CompositeWeightMedium = *v
	}
	if v := sf.Composite.WeightWeak; v != nil {
		settings.CompositeWeightWeak = *v
	}

	// CriticalFailure 임계값 오버라이드
	if cf := sf.Settings.CriticalFailure; cf != nil {
		if v := cf.DailyDropThreshold; v != nil {
			settings.Critical.DailyDropThreshold = *v
		}
		if v := cf.PanicVolumeMultiplier; v != nil {
			settings.Critical.PanicVolumeMultiplier = *v
		}
		if v := cf.PanicMinDropRate; v != nil {
			settings.Critical.PanicMinDropRate = *v
		}
		if v := cf.CumulativeDropThreshold; v != nil {
			settings.Critical.CumulativeDropThreshold = *v
		}
		if v := cf.CumulativeDropDays; v != nil {
			settings.Critical.CumulativeDropDays = *v
		}
		if v := cf.MAReversalDays; v != nil {
			settings.Critical.MAReversalDays = *v
		}
	}

	activeSellRules = sf.SellRules
	fmt.Printf("[stg] 매도 전략 %d개 로드: %s\n", len(activeSellRules), path)
	return settings, nil
}

// EvaluateSellSignals 는 한 포지션에 대해 모든 매도 룰을 평가하고
// 5-Path 결정 엔진을 거쳐 SellDecision을 반환한다 (C# SLogic + DecisionEngine 통합).
func EvaluateSellSignals(ctx *box.TradingContext, pos *box.TradePosition, s SellSettings) box.SellDecision {
	if len(activeSellRules) == 0 {
		return box.NoSellDecision("매도 룰 미로드")
	}

	// ===== Phase 1: 모든 룰 평가 + 신호 수집 =====
	signals := make([]box.SellSignal, 0, len(activeSellRules))
	inGracePeriod := s.MinHoldingPeriod > 0 && ctx.Position-pos.BuyPosition < s.MinHoldingPeriod
	for _, rule := range activeSellRules {
		// 손절 유예 기간: Critical/Loss 룰은 트리거·트래킹 모두 건너뛴다 (Profit/Technical/Expiry는 정상 평가)
		if inGracePeriod && isLossCutCategory(rule.Category) {
			signals = append(signals, box.SellSignal{
				ConditionName:     rule.Name,
				Path:              box.SellPath(rule.Path),
				Priority:          rule.Priority,
				SignalWeight:      rule.Weight,
				Category:          rule.Category,
				CompositeEligible: rule.CompositeEligible,
				CompositeWeight:   rule.CompositeWeight,
			})
			continue
		}
		triggered := evaluateRuleConditions(rule, ctx, pos, s)
		thresholdMet := false
		if triggered {
			thresholdMet = TrackAndCheck(ctx, pos, rule.Name, true, rule.Tracking, s)
		}
		sig := box.SellSignal{
			ConditionName:     rule.Name,
			Path:              box.SellPath(rule.Path),
			Priority:          rule.Priority,
			SignalWeight:      rule.Weight,
			IsTriggered:       triggered,
			ThresholdMet:      thresholdMet,
			Category:          rule.Category,
			CompositeEligible: rule.CompositeEligible,
			CompositeWeight:   rule.CompositeWeight,
			OccurrenceCount:   pos.GetSellConditionOccurrenceCount(rule.Name),
		}
		if h := ctx.Position - pos.BuyPosition; h > 0 {
			sig.OccurrenceRatio = float64(sig.OccurrenceCount) / float64(h)
		}
		signals = append(signals, sig)
	}

	// ===== Phase 2: 5-Path 결정 =====
	recovery := evaluateRecovery(ctx, pos, s)
	return makeSellDecision(signals, recovery, ctx, pos, s)
}

// isLossCutCategory 는 손절 유예(min_holding_period) 대상 카테고리인지 판별.
func isLossCutCategory(category string) bool {
	return category == "Critical" || category == "Loss"
}

// evaluateRuleConditions 는 한 룰의 when/any_of/when_not 평가.
func evaluateRuleConditions(rule SellRuleConfig, ctx *box.TradingContext, pos *box.TradePosition, s SellSettings) bool {
	// when: 모두 true
	for _, name := range rule.When {
		fn, ok := sellConditionRegistry[name]
		if !ok {
			fmt.Printf("[sell_rule] 미등록 조건: %s\n", name)
			return false
		}
		if !fn(ctx, pos, s) {
			return false
		}
	}
	// when_not: 모두 false
	for _, name := range rule.WhenNot {
		fn, ok := sellConditionRegistry[name]
		if !ok {
			fmt.Printf("[sell_rule] 미등록 조건: %s\n", name)
			return false
		}
		if fn(ctx, pos, s) {
			return false
		}
	}
	// any_of: 하나 이상 true
	if len(rule.AnyOf) > 0 {
		anyPassed := false
		for _, name := range rule.AnyOf {
			fn, ok := sellConditionRegistry[name]
			if ok && fn(ctx, pos, s) {
				anyPassed = true
				break
			}
		}
		if !anyPassed {
			return false
		}
	}
	return true
}

// evaluateRecovery 는 Composite Path 결정에 사용되는 RecoveryPotential 분류.
// 룰 엔진 외부에서 평가하여 Composite weight 계산에 사용.
func evaluateRecovery(ctx *box.TradingContext, pos *box.TradePosition, s SellSettings) box.RecoveryPotential {
	// cond.EvaluateRecoveryPotential을 호출하려면 import 순환이 발생할 수 있으므로
	// 평가 책임을 conditions_registry에 위임하기 위해 인라인으로 호출.
	return recoveryEvaluatorHook(ctx, pos, s)
}

// recoveryEvaluatorHook 은 init에서 conditions_registry가 채우는 hook.
// (sell_conditions_registry.go에서 cond.EvaluateRecoveryPotential 호출)
var recoveryEvaluatorHook = func(ctx *box.TradingContext, pos *box.TradePosition, s SellSettings) box.RecoveryPotential {
	return box.RecoveryMedium
}

// makeSellDecision 은 수집된 신호와 Recovery 등급으로 5-Path 결정.
// C# SellDecisionEngine.MakeDecision 포팅.
func makeSellDecision(signals []box.SellSignal, recovery box.RecoveryPotential, ctx *box.TradingContext, pos *box.TradePosition, s SellSettings) box.SellDecision {
	// ===== Path 1: Critical =====
	for _, sig := range signals {
		if sig.Path == box.PathCritical && sig.CanTriggerSell() {
			return box.SellDecision{
				ShouldSell:          true,
				SellWeight:          sig.SignalWeight,
				PrimaryReason:       sig.ConditionName,
				DecisionPath:        "Phase2-Critical",
				ContributingSignals: []string{sig.ConditionName},
				DecisionRationale:   "치명적 실패 감지 → 즉시 청산",
			}
		}
	}

	// ===== Path 2: Composite =====
	compStrength := 0.0
	contributors := []string{}
	for _, sig := range signals {
		if sig.CompositeEligible && sig.CanTriggerSell() {
			compStrength += sig.CompositeWeight
			contributors = append(contributors, sig.ConditionName)
		}
	}
	if compStrength > 0 {
		threshold := compositeThreshold(recovery, s)
		if compStrength >= threshold {
			weight := compositeWeightForStrength(compStrength, s)
			return box.SellDecision{
				ShouldSell:          true,
				SellWeight:          weight,
				PrimaryReason:       fmt.Sprintf("Composite_%s", joinNames(contributors)),
				DecisionPath:        "Phase2-Composite",
				ContributingSignals: contributors,
				DecisionRationale:   fmt.Sprintf("복합 신호 손절 Strength=%.2f Threshold=%.2f Recovery=%v", compStrength, threshold, recovery),
			}
		}
	}

	// ===== Path 3: Extension (홀딩 연장 대기 중) =====
	if pos.IsWaitingForSellSignalAfterExpiry {
		for _, sig := range signals {
			if sig.Path == box.PathExtension && sig.CanTriggerSell() {
				return box.SellDecision{
					ShouldSell:          true,
					SellWeight:          sig.SignalWeight,
					PrimaryReason:       sig.ConditionName,
					DecisionPath:        "Phase1-Extension",
					ContributingSignals: []string{sig.ConditionName},
					DecisionRationale:   fmt.Sprintf("홀딩 연장 중 매도 신호 (%s)", sig.ConditionName),
				}
			}
		}
		// 어떤 individual 신호든 발생하면 PeriodExpiryPlusSellSignal로 100% 청산
		for _, sig := range signals {
			if sig.Path == box.PathIndividual && sig.CanTriggerSell() {
				return box.SellDecision{
					ShouldSell:          true,
					SellWeight:          1.0,
					PrimaryReason:       "PeriodExpiryPlusSellSignal",
					DecisionPath:        "Phase1-Extension",
					ContributingSignals: []string{sig.ConditionName},
					DecisionRationale:   "홀딩 연장 중 개별 신호 발생 → 100% 청산",
				}
			}
		}
		return box.NoSellDecision("홀딩 연장 대기 중 — 매도 신호 없음")
	}

	// ===== Path 4: Period Expiry =====
	for _, sig := range signals {
		if sig.Path == box.PathExpiry && sig.CanTriggerSell() {
			return box.SellDecision{
				ShouldSell:                     false,
				SellWeight:                     sig.SignalWeight,
				PrimaryReason:                  sig.ConditionName,
				DecisionPath:                   "Phase1-Expiry",
				ContributingSignals:            []string{sig.ConditionName},
				DecisionRationale:              "기간 만료 — SFunction에서 홀딩 연장 평가 필요",
				RequiresHoldingExtensionUpdate: true,
			}
		}
	}

	// ===== Path 5: Individual (Priority 최소값 승) =====
	var triggered []box.SellSignal
	for _, sig := range signals {
		if sig.Path == box.PathIndividual && sig.CanTriggerSell() {
			triggered = append(triggered, sig)
		}
	}
	if len(triggered) == 0 {
		return box.NoSellDecision("매도 조건 없음")
	}
	sort.SliceStable(triggered, func(i, j int) bool {
		return triggered[i].Priority < triggered[j].Priority
	})
	top := triggered[0]
	return box.SellDecision{
		ShouldSell:          true,
		SellWeight:          top.SignalWeight,
		PrimaryReason:       top.ConditionName,
		DecisionPath:        "Phase1-Individual",
		ContributingSignals: []string{top.ConditionName},
		DecisionRationale:   fmt.Sprintf("개별 조건 트리거 — %s (Priority %d, Weight %.0f%%)", top.ConditionName, top.Priority, top.SignalWeight*100),
	}
}

func compositeThreshold(recovery box.RecoveryPotential, s SellSettings) float64 {
	switch recovery {
	case box.RecoveryHigh:
		return s.CompositeThresholdHighRecovery
	case box.RecoveryLow:
		return s.CompositeThresholdLowRecovery
	default:
		return s.CompositeThresholdMediumRecovery
	}
}

func compositeWeightForStrength(strength float64, s SellSettings) float64 {
	switch {
	case strength >= 1.0:
		return s.CompositeWeightStrong
	case strength >= 0.6:
		return s.CompositeWeightMedium
	default:
		return s.CompositeWeightWeak
	}
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += "+"
		}
		out += n
	}
	return out
}
