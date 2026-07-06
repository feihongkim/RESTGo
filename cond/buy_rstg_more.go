package cond

// buy_rstg_more.go — r_stg 전략9·7·14(일봉) + 전략2(크립토 15분봉) 이식 (2026-07-05).
// 명세: zpicture/r_stg_catalog.md §2-8/2-6/1-7, zpicture/minute_stg_spec.md §1.
// 원본 스크리너의 미래 참조(백테스트 라벨링용 조건)는 제거하고 신호 성분만 이식.

import "RESTGo/box"

// ── 전략9: 정배열 + 5이평 상승변곡 + 박스권 천장 근접 (일봉) ─────────────────
// 가설: 비율 정배열(20>60×1.008, 60>120×1.008) 유지 중 5이평이 상승 변곡했고,
// 가격이 직전 10일 고점(박스권 천장)의 93~109% 범위에 붙어 있으면 돌파 준비 상태.
func isStg9State(candles []*box.Candle, i int) bool {
	if i < 12 {
		return false
	}
	c := candles[i]
	if c.Ma20 <= 0 || c.Ma60 <= 0 || c.Ma120 <= 0 || c.Ma5 <= 0 {
		return false
	}
	if !(c.Ma20 > c.Ma60*1.008 && c.Ma60 > c.Ma120*1.008) {
		return false
	}
	// 5이평 상승 변곡 (하락/평탄 → 상승)
	if !(candles[i-1].Ma5 <= candles[i-2].Ma5 && c.Ma5 > candles[i-1].Ma5) {
		return false
	}
	// 직전 10일 고점 대비 저가 93~109% + 당일 고가는 천장 아래
	ph := 0.0
	for j := i - 10; j < i; j++ {
		if candles[j].High > ph {
			ph = candles[j].High
		}
	}
	if ph <= 0 || c.Low < ph*0.93 || c.Low > ph*1.09 || c.High >= ph {
		return false
	}
	// 거래대금: 5일 평균가 × 당일 거래량 ≥ 9억 (원본 avg5*volume)
	return c.Ma5Origin*c.Volume >= 900_000_000
}

// IsStg9ApexPerchEvent 는 전략9 상태 성립 순간(edge).
func IsStg9ApexPerchEvent(ctx *box.TradingContext) bool {
	i := ctx.Position
	return i >= 1 && isStg9State(ctx.CandleList, i) && !isStg9State(ctx.CandleList, i-1)
}

// ── 전략7: GC 후 20이 60을 빠르게 끌어올리는 반등 (일봉) ─────────────────────
// 가설: 20-60 골든크로스가 최근 5~30일 내 발생했고, 20이 8일 전 대비 1%+ 상승하며
// 60보다 빠르게 상승 중이면 W자형 반등의 우상향 국면.
func isStg7State(candles []*box.Candle, i int) bool {
	if i < 31 {
		return false
	}
	c := candles[i]
	if c.Ma20 <= 0 || c.Ma60 <= 0 || candles[i-8].Ma20 <= 0 || candles[i-8].Ma60 <= 0 {
		return false
	}
	if !(c.Ma20 > c.Ma60*1.02) {
		return false
	}
	// GC가 최근 30일 내 (30일 내 20<60 시점 존재) + 최소 5일 전에는 이미 20>60
	gcRecent := false
	for j := i - 30; j < i-5; j++ {
		if candles[j].Ma20 < candles[j].Ma60 {
			gcRecent = true
			break
		}
	}
	if !gcRecent {
		return false
	}
	for j := i - 5; j <= i; j++ {
		if candles[j].Ma20 <= candles[j].Ma60 {
			return false
		}
	}
	// 20 상승각 + 20이 60보다 빠르게 상승
	if !(c.Ma20 > candles[i-8].Ma20*1.01) {
		return false
	}
	return c.Ma20/candles[i-8].Ma20 > c.Ma60/candles[i-8].Ma60
}

// IsStg7GCAccelEvent 는 전략7 상태 성립 순간(edge).
func IsStg7GCAccelEvent(ctx *box.TradingContext) bool {
	i := ctx.Position
	return i >= 1 && isStg7State(ctx.CandleList, i) && !isStg7State(ctx.CandleList, i-1)
}

// ── 전략14: 정배열 유지 중 120이평까지의 급락 (일봉, oversold_candidate) ──────
// 가설: 정배열(20>60>120)이 10일 유지되고 20이평이 3일 연속 상승 중인데,
// 당일 시가가 60 위에서 출발해 종가가 120 아래까지 밀린 극단 과매도는 반등 후보.
func IsStg14OversoldEvent(ctx *box.TradingContext) bool {
	candles := ctx.CandleList
	i := ctx.Position
	if i < 10 {
		return false
	}
	c := candles[i]
	if c.Ma60 <= 0 || c.Ma120 <= 0 {
		return false
	}
	for j := i - 9; j <= i; j++ {
		cj := candles[j]
		if cj.Ma20 <= 0 || cj.Ma60 <= 0 || cj.Ma120 <= 0 || !(cj.Ma20 > cj.Ma60 && cj.Ma60 > cj.Ma120) {
			return false
		}
	}
	if !(c.Ma20 > candles[i-1].Ma20 && candles[i-1].Ma20 > candles[i-2].Ma20 && candles[i-2].Ma20 > candles[i-3].Ma20) {
		return false
	}
	// 당일: 시가 60 위 → 종가 120 아래 (하루 만의 극단 급락 — 자체로 edge)
	return c.Open > c.Ma60 && c.Close < c.Ma120
}

// ── 전략2: 완전 역배열 + 120 첫 돌파 후 되돌림 (크립토 15분봉, armed용 부품) ──
// 가설: 완전 역배열(20<60<120) 극단에서 120이평을 처음 찔러본 뒤 되돌아온 자리는 반등 후보.

const (
	Stg2InvertedBars = 48 // 완전 역배열 + 120 무접촉 유지 요구 봉수 (15분봉 12시간)
)

// IsStg2FirstPierce120 는 i가 "역배열 유지 후 120이평 첫 상향 관통" 순간인지 (장전 조건).
func IsStg2FirstPierce120(candles []*box.Candle, i int) bool {
	if i < Stg2InvertedBars+1 {
		return false
	}
	c := candles[i]
	if c.Ma120 <= 0 || c.High <= c.Ma120 {
		return false
	}
	for j := i - Stg2InvertedBars; j < i; j++ {
		cj := candles[j]
		if cj.Ma20 <= 0 || cj.Ma60 <= 0 || cj.Ma120 <= 0 {
			return false
		}
		if !(cj.Ma20 < cj.Ma60 && cj.Ma20 < cj.Ma120 && cj.Ma60 < cj.Ma120) {
			return false
		}
		if cj.High >= cj.Ma120 { // pure: 이전에 120 반응 없어야
			return false
		}
	}
	return true
}

// IsStg2RetreatEntry 는 관통 후 되돌림 진입 조건 (발화): 2연속 종가 < MA20 이면서
// 5이평이 20-60 중간선 이상으로 올라와 있음 (회복 진행 중).
func IsStg2RetreatEntry(candles []*box.Candle, i int) bool {
	if i < 1 {
		return false
	}
	c, p := candles[i], candles[i-1]
	if c.Ma5 <= 0 || c.Ma20 <= 0 || c.Ma60 <= 0 {
		return false
	}
	if !(c.Close < c.Ma20 && p.Close < p.Ma20) {
		return false
	}
	return c.Ma5 >= 0.5*(c.Ma20+c.Ma60)
}
