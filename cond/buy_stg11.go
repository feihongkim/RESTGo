package cond

// buy_stg11.go — r_stg 전략11 "에스코넥 유형" 크립토 핵심형 이식 (2026-07-05).
//
// 원본: r_stg/전략11 (국내 5분봉, 세션 시각 의존 조건 다수). 명세: zpicture/minute_stg_spec.md §6.
// 가설: 장기간 완전 정배열(20>60>120)을 유지하던 강한 추세 종목이 60이평을 처음으로 무너뜨리는
// 순간 — "반등 없이 지속 하락할 때만 매수"(원 주석) 후보의 시작점.
//
// 크립토 변환 (세션 개념 제거, 15분봉):
//   전일 정배열 → 96봉(24시간) 연속 정배열 / 전일 60 무접촉 → 같은 96봉 내 저가의 60 터치 없음
//   당일 60 붕괴 → close < MA60 첫 마감 (edge) / 오전·시가고가 조건 → 제거
//   양봉 60 관통 <3회 (최근 4봉) 유지 / 20이평 각도 ≥2틱 → |ΔMA20(4봉)| ≥ 0.2%

import "RESTGo/box"

const (
	Stg11AlignedBarsDefault = 96 // 정배열·60 무접촉 유지 구간 기본값 (15분봉 24시간)
	Stg11GridWindow   = 4     // 양봉 60 관통 검사 구간
	Stg11GridMaxFails = 3     // 양봉 관통 허용 상한 (이상이면 지저분한 붕괴로 제외)
	Stg11ArcWindow    = 4     // MA20 각도 측정 구간
	Stg11ArcMinPct    = 0.002 // MA20 각도 하한 (0.2%)
)

// IsStg11MA60BreakdownEvent 는 "장기 정배열 유지 중 60이평 첫 붕괴" 순간(edge)인지.
// alignedBars: 정배열·무접촉 유지 요구 봉수 (0 이하면 기본값 96).
func IsStg11MA60BreakdownEvent(ctx *box.TradingContext, alignedBars int) bool {
	if alignedBars <= 0 {
		alignedBars = Stg11AlignedBarsDefault
	}
	candles := ctx.CandleList
	pos := ctx.Position
	if pos < alignedBars+1 {
		return false
	}
	cur, prev := candles[pos], candles[pos-1]
	if cur.Ma60 <= 0 || prev.Ma60 <= 0 {
		return false
	}
	// 붕괴 edge: 직전 봉은 60 위 마감, 이번 봉은 60 아래 마감
	if !(prev.Close >= prev.Ma60 && cur.Close < cur.Ma60) {
		return false
	}
	// 직전 alignedBars봉: 완전 정배열 연속 + 저가의 60 무접촉 (처음 무너지는 것이어야 함)
	for j := pos - alignedBars; j < pos; j++ {
		c := candles[j]
		if c.Ma20 <= 0 || c.Ma60 <= 0 || c.Ma120 <= 0 || !(c.Ma20 > c.Ma60 && c.Ma60 > c.Ma120) {
			return false
		}
		if c.Low <= c.Ma60 {
			return false
		}
	}
	// 최근 4봉 내 양봉의 60 관통 3회 미만 (양음양 연속체 방지)
	grid := 0
	for j := pos - Stg11GridWindow + 1; j <= pos; j++ {
		c := candles[j]
		if c.Close > c.Open && c.Low <= c.Ma60 && c.Ma60 > 0 {
			grid++
		}
	}
	if grid >= Stg11GridMaxFails {
		return false
	}
	// MA20 각도: 최근 4봉 변화폭 ≥ 0.2% (추세가 살아있는 상태에서의 붕괴)
	ref := candles[pos-Stg11ArcWindow]
	if ref.Ma20 <= 0 {
		return false
	}
	arc := cur.Ma20 - ref.Ma20
	if arc < 0 {
		arc = -arc
	}
	return arc/ref.Ma20 >= Stg11ArcMinPct
}
