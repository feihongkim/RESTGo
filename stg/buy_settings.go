package stg

// Settings 는 Box 분석에 필요한 설정값 구조체
type Settings struct {
	// DefBox 생성 관련
	DamOption int // 박스 손상 허용 횟수 (기본 0: 손상 없어야 함)

	// 매수 진입 관련
	VolumeLimit               float64 // 거래량 배수 기준 (기본 5)
	DefBoxNearPriceThreshold  float64 // DefBox 가격 근접 임계값 (기본 0.07 = 7%)
	MainBoxNearPriceThreshold float64 // MainBox 가격 근접 임계값 (기본 0.15 = 15%)

	// ATR 필터
	ATREntryFilterEnabled bool
	ATREntryMaxThreshold  float64 // ATR/Price 최대값 (기본 0.05 = 5%)

	// 윗꼬리 필터
	DefBoxUpperWickToBodyRatioThreshold float64 // 윗꼬리/몸통 비율 (기본 4.0)

	// MA 스프레드 필터
	MaSpreadThreshold float64 // MA20-MA60 간격 임계값 (기본 0.05 = 5%)

	// RSI 조건
	RSIOversoldThreshold   float64 // 과매도 기준 (기본 30)
	RSIOverboughtThreshold float64 // 과매수 기준 (기본 70)
	RSIRecoveryLookback    int     // 과매도 탈출 반등 탐색 구간 (기본 5)
	RSIRisingPeriod        int     // RSI 단조 상승 확인 캔들 수 (기본 3)
	RSIBullZoneLow         float64 // 건전 모멘텀 구간 하한 (기본 50)
	RSIBullZoneHigh        float64 // 건전 모멘텀 구간 상한 (기본 70)

	// Bollinger Band 조건 (Width 단위: percent — 4.0 = 4%)
	BBReboundLookback       int     // 하단 반등 탐색 구간 (기본 5)
	BBReboundPercentB       float64 // 하단 반등 인정 %B (기본 0.3)
	BBSqueezeLookback       int     // 스퀴즈 돌파 탐색 구간 (기본 20)
	BBSqueezeWidthThreshold float64 // 스퀴즈 판정 BBWidth (기본 4.0%)
	BBBreakoutPercentB      float64 // 스퀴즈 돌파 인정 %B (기본 0.8)
	BBMiddleHoldDuration    int     // 중심선 위 유지 캔들 수 (기본 3)

	// MA 수렴 조건
	MaConvergenceThreshold float64 // MA5/20/60 수렴 임계값 (기본 0.03 = 3%)

	// 백테스트 비용 모델
	FeeRate      float64 // 수수료율 (Upbit 기본 0.0005 = 0.05%)
	SlippageRate float64 // 슬리피지율 (기본 0.0005)

	// 매 캔들 평가 (per_candle 룰)
	PerCandleCooldownBars int // 발화 후 재발화 금지 봉 수 (기본 4)

	// EMA 풀백 조건
	EMA21PullbackLookback int // EMA21 풀백 반등 탐색 구간 (기본 3)

	// per_candle 포지션 청산 (E1~E4)
	ATRStopMultiplier   float64 // E1 ATR 스탑 배수 (기본 1.5)
	ATRTargetMultiplier float64 // E2 타겟 ATR 배수 (기본 2.0)
	TargetSellWeight    float64 // E2 부분청산 비율 (기본 0.5)
	TrailingEMAPeriod   int     // E3 트레일링 EMA 기간 (기본 21)
	TimeExitBars        int     // E4 시간 청산 봉 수 (기본 16)

	// ADX/MACD/Stoch 임계값
	ADXTrendThreshold        float64 // ADX 추세 판단 임계 (기본 20)
	MACDHistRisingBars       int     // MACD 히스토그램 상승 확인 봉 수 (기본 3)
	StochOversoldThreshold   float64 // Stochastic 과매도 (기본 25)
	StochOverboughtThreshold float64 // Stochastic 과매수 (기본 75)

	// VWAP/거래량 임계값
	VWAPDeviationK        float64 // VWAP σ 배수 (기본 1.5)
	VolumeZScoreThreshold float64 // 거래량 Z-score 임계 (기본 2.0)
	VolumeZScoreWindow    int     // Z-score 계산 창 (기본 20)
	OBVRisingPeriod       int     // OBV 상승 확인 봉 수 (기본 5)

	// 변동성 임계값
	SuperTrendPeriod     int     // SuperTrend ATR 기간 (기본 10)
	SuperTrendMultiplier float64 // SuperTrend 승수 (기본 3.0)
	DonchianPeriod       int     // Donchian 기간 (기본 20)
	NarrowRangePeriod    int     // NR 판단 봉 수 (기본 7)
}

// DefaultSettings 는 기본값으로 초기화된 Settings 반환
func DefaultSettings() Settings {
	return Settings{
		DamOption:                           0,
		VolumeLimit:                         5.0,
		DefBoxNearPriceThreshold:            0.07,
		MainBoxNearPriceThreshold:           0.15,
		ATREntryFilterEnabled:               true,
		ATREntryMaxThreshold:                0.05,
		DefBoxUpperWickToBodyRatioThreshold: 4.0,
		MaSpreadThreshold:                   0.05,

		RSIOversoldThreshold:   30,
		RSIOverboughtThreshold: 70,
		RSIRecoveryLookback:    5,
		RSIRisingPeriod:        3,
		RSIBullZoneLow:         50,
		RSIBullZoneHigh:        70,

		BBReboundLookback:       5,
		BBReboundPercentB:       0.3,
		BBSqueezeLookback:       20,
		BBSqueezeWidthThreshold: 4.0,
		BBBreakoutPercentB:      0.8,
		BBMiddleHoldDuration:    3,

		MaConvergenceThreshold: 0.03,

		FeeRate:      0.0005,
		SlippageRate: 0.0005,

		PerCandleCooldownBars: 4,

		EMA21PullbackLookback: 3,

		ATRStopMultiplier:   1.5,
		ATRTargetMultiplier: 2.0,
		TargetSellWeight:    0.5,
		TrailingEMAPeriod:   21,
		TimeExitBars:        16,

		ADXTrendThreshold:        20,
		MACDHistRisingBars:       3,
		StochOversoldThreshold:   25,
		StochOverboughtThreshold: 75,

		VWAPDeviationK:        1.5,
		VolumeZScoreThreshold: 2.0,
		VolumeZScoreWindow:    20,
		OBVRisingPeriod:       5,

		SuperTrendPeriod:     10,
		SuperTrendMultiplier: 3.0,
		DonchianPeriod:       20,
		NarrowRangePeriod:    7,
	}
}
