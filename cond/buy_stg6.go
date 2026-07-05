package cond

// buy_stg6.go — r_stg 전략6 (정배열 5<20 조정 후 전고점 터치) 이식 (2026-07-05).
//
// 원본: r_stg/chck.entry.r + 전략6/stg6_FINAL.r (국내 30분봉). 명세: zpicture/minute_stg_spec.md §4.
// 가설: 정배열(20>60>120)이 유지되는 조정 국면에서 5이평이 20 아래로 내려갔고,
// 가격이 전고점을 터치했으나 돌파하지는 못한 상태 = 반등 직전 후보.
//
// 15분봉 변환 (30분 → 15분, 봉 수 2배):
//   정배열 연속 22봉 → 44봉 / prior_high "전 3일" 48봉 → 96봉 / 터치 검사 12봉 → 24봉
// 크립토 적용 시 제거·변환: CC존(14:00 고정시각) → 매 캔들 edge 판정 / 거래대금 8억 필터 →
// 메이저 4종(BTC/ETH/XRP/SOL) 대상이라 생략 / 0.15% 틱 기반 임계 → 비율 그대로 사용.

import "RESTGo/box"

const (
	Stg6AlignedBars     = 44     // 정배열(20>60>120) 연속 유지 봉 수
	Stg6PriorHighBars   = 96     // 전고점 산출 구간 (터치 구간 이전)
	Stg6TouchWindowBars = 24     // 전고점 터치/돌파 검사 최근 구간
	Stg6TouchTolerance  = 0.0015 // 터치 판정 허용 오차 (0.15%)
	Stg6MA5Below20Since = 24     // "얼마 전까진 5>20이었다" 확인 구간
)

// isStg6State 는 pos 시점에 전략6 상태(level)가 성립하는지.
func isStg6State(candles []*box.Candle, pos int) bool {
	if pos < Stg6PriorHighBars+Stg6TouchWindowBars {
		return false
	}
	// ① 정배열 44봉 연속 (20>60>120, 전부 유효)
	for j := pos - Stg6AlignedBars + 1; j <= pos; j++ {
		c := candles[j]
		if c.Ma20 <= 0 || c.Ma60 <= 0 || c.Ma120 <= 0 || !(c.Ma20 > c.Ma60 && c.Ma60 > c.Ma120) {
			return false
		}
	}
	// ② 조정: 현재 5<20 이고, 최근 24봉 내에 5>20인 시점이 있었음 (막 내려온 조정)
	cur := candles[pos]
	if cur.Ma5 <= 0 || cur.Ma5 >= cur.Ma20 {
		return false
	}
	was := false
	for j := pos - Stg6MA5Below20Since; j < pos; j++ {
		if candles[j].Ma5 > candles[j].Ma20 {
			was = true
			break
		}
	}
	if !was {
		return false
	}
	// ③ 전고점: [pos-96-24, pos-24) 구간 최고가
	lo := pos - Stg6PriorHighBars - Stg6TouchWindowBars
	priorHigh := 0.0
	for j := lo; j < pos-Stg6TouchWindowBars; j++ {
		if candles[j].High > priorHigh {
			priorHigh = candles[j].High
		}
	}
	if priorHigh <= 0 {
		return false
	}
	// ④ 최근 24봉: 초과 돌파(+0.15%) 없음 AND 터치(-0.15% 이내 도달) 있음
	touched := false
	for j := pos - Stg6TouchWindowBars + 1; j <= pos; j++ {
		h := candles[j].High
		if h > priorHigh*(1+Stg6TouchTolerance) {
			return false // 이미 돌파 — 전략 전제 붕괴
		}
		if h >= priorHigh*(1-Stg6TouchTolerance) {
			touched = true
		}
	}
	return touched
}

// IsStg6PullbackTouchEvent 는 전략6 상태가 "성립하는 순간"(edge)인지 — 직전 캔들엔 불성립.
func IsStg6PullbackTouchEvent(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	return isStg6State(ctx.CandleList, pos) && !isStg6State(ctx.CandleList, pos-1)
}
