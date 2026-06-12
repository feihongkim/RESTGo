package stg

import "RESTGo/box"

// BuySignal 은 매수 신호 평가 결과
type BuySignal struct {
	Triggered    bool
	Reason       string
	Position     int
	Date         string  // 신호 발생 캔들 날짜
	DefboxPrice  float64 // DefBox 원본 가격 (스케일 전)
	MainboxPrice float64 // MainBox 원본 가격 (스케일 전)
	MainboxDate  string  // MainBox 캔들 날짜
	Helper       string  // 매수 헬퍼 리포트 (C# BuyHelperReport — 예: "SR-진동지지", "MD즉시매수")
}

// AnalysisResult 는 전체 분석 후 결과
type AnalysisResult struct {
	BoxList    []*box.Box
	BuySignals []BuySignal
	// Positions 는 분석 중 생성된 모든 거래 포지션 (활성/청산 포함)
	Positions []*box.TradePosition
}
