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
	}
}
