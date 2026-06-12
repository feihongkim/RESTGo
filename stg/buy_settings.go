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
	}
}
