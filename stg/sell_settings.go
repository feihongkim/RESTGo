package stg

import "RESTGo/cond"

// SellSettings 는 매도 평가에 필요한 모든 임계값을 묶는다.
// C# Settings.cs의 매도 관련 키들을 1:1 포팅.
// YAML에서 로드되며, 누락된 값은 DefaultSellSettings()로 채워진다.
type SellSettings struct {
	// === 글로벌 ===
	MaxHoldingPeriod         int     // 보유 기간 만료 (기본 20)
	MinHoldingPeriod         int     // 손절 유예 기간 — 보유 N봉 미만이면 Critical/Loss 룰 미발화 (기본 0=비활성)
	AutoLiquidateOnExpiry    bool    // 만료 시 자동 청산 여부 (기본 true)
	DefaultSellWeight        float64 // 기본 매도 비율 (기본 0.5)
	SmallRemainingThreshold  float64 // 소량 잔여 자동 청산 (기본 0.125)
	MinimumExecutionSize     float64 // 최소 실행 크기 (기본 0.01)
	MinimumPositionThreshold float64 // 최소 포지션 임계 (기본 0.05)

	// === StopLoss 일반 ===
	StopLossThreshold float64 // 기본 손절 % (기본 -20.0 — C# Settings.cs:35. 평가자 doc 주석의 -10.0은 낡은 값)

	// === AdaptiveStopLoss ===
	Adaptive cond.AdaptiveStopLossParams

	// === TimeDelayedStopLoss ===
	TimeDelayedStopLossEnabled      bool
	TimeDelayedStopLossRequiredDays int // (기본 3)

	// === Recovery / Critical ===
	Recovery cond.RecoveryParams
	Critical cond.CriticalFailureParams

	// === EarlyWarning ===
	EarlyDropDaysAfterBuy         int     // (기본 3)
	EarlyDropThresholdPercent     float64 // (기본 -5.0)
	EarlyMainBoxBreakDaysAfterBuy int     // (기본 2)

	// === Profit ===
	BBUpperBreakoutMinBBPercent   float64 // (기본 0.95)
	BBUpperBreakoutMinProfitRatio float64 // (기본 0.08)

	// === MainBox Recovery / Persistent ===
	MainBoxRecoveryCheckPeriod int // (기본 5)

	// === MainBox BB Breakdown ===
	MainBoxBBBreakdownMinDays            int
	MainBoxBBBreakdownMaxDays            int
	MainBoxBBBreakdownBBPercentThreshold float64

	// === Technical Sell ===
	ConsecutiveNegativeLookback int // (기본 3 — C# ConsecutiveNegativeLookback)
	ConsecutiveNegativeMinCount int // (기본 2 — C# ConsecutiveNegativeMinCount)
	MAReversalBoxLookbackPeriod int // (기본 5)

	// === BB Volatility ===
	BBSqueezeDurationMin                 int     // (기본 3)
	BBSqueezeWidthThreshold              float64 // BollingerWidth 단위(percent, 예: 4.0)
	BBSqueezeExpansionWidthIncreaseRatio float64
	BBSqueezeExpansionBBPercentThreshold float64
	BBSqueezeExpansionRequiredCandles    int
	BBSqueezeExpansionLookback           int
	BBVolatilityHighThreshold            float64
	BBVolatilityLowThreshold             float64

	// === Composite Path ===
	CompositeThresholdHighRecovery   float64
	CompositeThresholdMediumRecovery float64
	CompositeThresholdLowRecovery    float64
	CompositeWeightStrong            float64
	CompositeWeightMedium            float64
	CompositeWeightWeak              float64

	// === Tracking 기본값 (룰에 명시되지 않을 때 fallback) ===
	DefaultSellConditionCountThreshold int     // (기본 3)
	DefaultSellConditionRatioThreshold float64 // (기본 0.05)
}

// DefaultSellSettings 는 C# Settings.cs 기본값과 동일한 매도 설정을 반환한다.
func DefaultSellSettings() SellSettings {
	return SellSettings{
		MaxHoldingPeriod:         20,
		AutoLiquidateOnExpiry:    true,
		DefaultSellWeight:        0.5,
		SmallRemainingThreshold:  0.125,
		MinimumExecutionSize:     0.01,
		MinimumPositionThreshold: 0.05,

		StopLossThreshold: -20.0,

		Adaptive: cond.AdaptiveStopLossParams{
			BaseThreshold:             -20.0,
			HighVolatilityMultiplier:  1.5,
			LowVolatilityMultiplier:   1.0,
			MinThreshold:              -30.0,
			MaxThreshold:              -10.0,
			VolatilityLookbackDays:    20,
			HighVolatilityThreshold:   0.10,
			MediumVolatilityThreshold: 0.05,
			StrongStructureThreshold:  0.15,
			MediumStructureThreshold:  0.10,
		},

		TimeDelayedStopLossEnabled:      true,
		TimeDelayedStopLossRequiredDays: 3,

		Recovery: cond.RecoveryParams{
			HighMaxDaysBelow:          2,
			HighMaxDropRate:           0.05,
			HighMinRecoveryRate:       0.02,
			MediumMaxDaysBelow:        5,
			VolumeSupportBearishRatio: 0.8,
			MA5Tolerance:              0.02,
			MA20Tolerance:             0.03,
		},

		Critical: cond.CriticalFailureParams{
			DailyDropThreshold:      -0.10,
			PanicVolumeMultiplier:   2.0,
			PanicMinDropRate:        -0.05,
			CumulativeDropThreshold: -0.15,
			CumulativeDropDays:      5,
			MAReversalDays:          3,
		},

		EarlyDropDaysAfterBuy:         3,
		EarlyDropThresholdPercent:     -5.0,
		EarlyMainBoxBreakDaysAfterBuy: 2,

		BBUpperBreakoutMinBBPercent:   0.95,
		BBUpperBreakoutMinProfitRatio: 0.08,

		MainBoxRecoveryCheckPeriod: 5,

		MainBoxBBBreakdownMinDays:            1,
		MainBoxBBBreakdownMaxDays:            2,
		MainBoxBBBreakdownBBPercentThreshold: 0.30,

		ConsecutiveNegativeLookback: 3,
		ConsecutiveNegativeMinCount: 2,
		MAReversalBoxLookbackPeriod: 5,

		BBSqueezeDurationMin:                 3,
		BBSqueezeWidthThreshold:              4.0, // BollingerWidth percent
		BBSqueezeExpansionWidthIncreaseRatio: 1.5,
		BBSqueezeExpansionBBPercentThreshold: 0.25,
		BBSqueezeExpansionRequiredCandles:    2,
		BBSqueezeExpansionLookback:           20,
		BBVolatilityHighThreshold:            15.0, // BollingerWidth percent
		BBVolatilityLowThreshold:             5.0,

		CompositeThresholdHighRecovery:   1.0,
		CompositeThresholdMediumRecovery: 0.6,
		CompositeThresholdLowRecovery:    0.3,
		CompositeWeightStrong:            1.0,
		CompositeWeightMedium:            0.5,
		CompositeWeightWeak:              0.25,

		DefaultSellConditionCountThreshold: 3,
		DefaultSellConditionRatioThreshold: 0.05,
	}
}
