package cond

// buy_pullback.go — 20이평 눌림 돌파 매수 (2026-07-04, 사용자 설계).
//
// 추세 지속형: 20이평이 상승 각도를 유지하는 중의 얕은 눌림에서
//   ① 이평 아래 resist box — box 전후 ±3봉에서 고가로는 20이평을 넘었으나(≥1봉)
//      종가로는 넘은 적 없음(0봉) = 첫 반등 시도가 이평에서 거부됨
//   ② 이후 support box (눌림 바닥, 종가가 이평 아래)
//   ③ 트리거: support 인지 후 20봉 내, 양봉이 종가 기준으로 20이평을 상향 돌파
//      (전일 종가 ≤ 전일 MA20 → 당일 종가 > 당일 MA20 edge)
//
// "상승 각도 유지" 정의 (사용자 확정): MA20[이전] < MA20[지금]을 +로 정의, 3연속 + (+++).
// resist box 시점과 트리거 캔들 양쪽에서 충족해야 한다.
//
// 2026-07-04 갱신: +++ 요구가 신호를 연 ~17건으로 희소화(중앙값 음수 복권형)해 사용자 결정으로
// 폐지 — streak 파라미터화 (0 = 미적용). streak=3 결과(296건, 기각)는 pullback_scan_report.md 참조.

import "RESTGo/box"

// PullbackTouchWindow 는 resist box 전후 검사 범위 (±3봉).
const PullbackTouchWindow = 3

// PullbackMaxRSGap 은 resist→support box 최대 간격.
const PullbackMaxRSGap = 20

// PullbackPattern 은 눌림 구조 탐지 결과 + 사후 분석 속성.
type PullbackPattern struct {
	RPos, SPos       int
	RPrice, SPrice   float64 // 구간 극값 (R=고점, S=저점)
	TouchCount       int     // R±3봉 중 고가>MA20 봉 수
	DepthPct         float64 // (MA20[S] - S저점)/MA20[S] × 100 — 눌림 깊이
	MA20SlopePct     float64 // R 시점 (MA20[R]-MA20[R-5])/MA20[R-5] × 100 — 추세 강도
	RToSBars         int
}

// IsMA20RisingStreak 는 pos 시점에 MA20가 streak봉 연속 상승 중인지 (이전<지금 × streak회).
func IsMA20RisingStreak(candles []*box.Candle, pos, streak int) bool {
	if pos-streak < 0 {
		return false
	}
	for k := 0; k < streak; k++ {
		cur, prev := candles[pos-k], candles[pos-k-1]
		if cur.Ma20 <= 0 || prev.Ma20 <= 0 || prev.Ma20 >= cur.Ma20 {
			return false
		}
	}
	return true
}

// FindMA20PullbackPattern 은 pos(새 support box 인지 캔들)에서 눌림 R→S 구조를 찾는다.
// streak: R box 시점 MA20 연속 상승 요구 봉수 (0 = 미적용).
func FindMA20PullbackPattern(ctx *box.TradingContext, streak int) (*PullbackPattern, bool) {
	pos := ctx.Position
	candles := ctx.CandleList

	// 마지막 두 박스 = [resist, support] (지지/저항만).
	// DefBox는 제외 — 분석 루프가 CheckAndCreateDefBox를 호출해도 패턴 구조 탐지는
	// 추세 전환 박스(KindBox·MainBox 승격 포함)만 본다 (DefBox 도입 전 동작 보존).
	var slots []*box.Box
	for _, b := range ctx.BoxList {
		if b.KindOfBox == box.KindDefBox {
			continue
		}
		if b.BoxType == box.BoxTypeSupport || b.BoxType == box.BoxTypeResistance {
			slots = append(slots, b)
		}
	}
	n := len(slots)
	if n < 2 {
		return nil, false
	}
	s, r := slots[n-1], slots[n-2]
	const maxSGap = 15
	if s.BoxType != box.BoxTypeSupport || pos-s.BoxPosition > maxSGap {
		return nil, false
	}
	if r.BoxType != box.BoxTypeResistance || r.BoxPosition >= s.BoxPosition ||
		s.BoxPosition-r.BoxPosition > PullbackMaxRSGap {
		return nil, false
	}

	// support box 캔들: 종가가 이평 아래 (눌림 중)
	sc := candles[s.BoxPosition]
	if sc.Ma20 <= 0 || sc.Close >= sc.Ma20 {
		return nil, false
	}

	// resist box 시점: MA20 연속 상승 (streak=0이면 미적용)
	if streak > 0 && !IsMA20RisingStreak(candles, r.BoxPosition, streak) {
		return nil, false
	}

	// resist box ±3봉: 고가>MA20 ≥1봉 AND 종가>MA20 0봉 (각 봉의 자기 MA20 기준)
	lo, hi := r.BoxPosition-PullbackTouchWindow, r.BoxPosition+PullbackTouchWindow
	if lo < 0 {
		lo = 0
	}
	if hi >= pos {
		hi = pos - 1
	}
	touch := 0
	for i := lo; i <= hi; i++ {
		c := candles[i]
		if c.Ma20 <= 0 {
			return nil, false
		}
		if c.Close > c.Ma20 {
			return nil, false // 종가로 넘은 적 있음 → 불성립
		}
		if c.High > c.Ma20 {
			touch++
		}
	}
	if touch == 0 {
		return nil, false // 고가로도 못 건드림 → "거부" 증거 없음
	}

	slope := 0.0
	if rp := r.BoxPosition; rp-5 >= 0 && candles[rp-5].Ma20 > 0 {
		slope = (candles[rp].Ma20 - candles[rp-5].Ma20) / candles[rp-5].Ma20 * 100
	}
	return &PullbackPattern{
		RPos: r.BoxPosition, SPos: s.BoxPosition,
		RPrice: r.Price, SPrice: s.Price,
		TouchCount:   touch,
		DepthPct:     (sc.Ma20 - s.Price) / sc.Ma20 * 100,
		MA20SlopePct: slope,
		RToSBars:     s.BoxPosition - r.BoxPosition,
	}, true
}

// IsBBExpanding 은 pos 시점 볼린저 밴드폭이 팽창 중인지 (스퀴즈 아님).
// 정의는 W바텀 P1 팽창 조건과 동일: BBW[pos] > BBW[pos-5] AND BBW[pos] > min(BBW[pos-20:pos])×1.2.
// (2026-07-04 눌림돌파 시장국면b — 트리거 시점 밴드 확장 여부)
func IsBBExpanding(candles []*box.Candle, pos int) bool {
	return isBBWExpanding(candles, pos)
}

// LastDefBoxAboveMA20 은 pos 이전에 형성된 가장 최근 DefBox가 존재하고 그 가격이
// pos 캔들의 MA20보다 위인지 반환한다 (2026-07-04 눌림돌파 시장국면a — W중력 유사 상방 중력).
// 반환: (위에 있는가, MA20 대비 거리 %, DefBox 존재 여부)
func LastDefBoxAboveMA20(ctx *box.TradingContext, pos int) (above bool, distPct float64, exists bool) {
	candles := ctx.CandleList
	ma := candles[pos].Ma20
	if ma <= 0 {
		return false, 0, false
	}
	for i := len(ctx.BoxList) - 1; i >= 0; i-- {
		b := ctx.BoxList[i]
		if b.KindOfBox != box.KindDefBox || b.BoxPosition >= pos {
			continue
		}
		return b.Price > ma, (b.Price - ma) / ma * 100, true
	}
	return false, 0, false
}

// IsMA20BullishBreakout 은 i 캔들이 "양봉 + 종가 기준 20이평 상향 돌파(edge) [+ MA20 연속 상승]"인지.
// streak: 트리거 시점 MA20 연속 상승 요구 봉수 (0 = 미적용).
func IsMA20BullishBreakout(candles []*box.Candle, i, streak int) bool {
	if i < 1 {
		return false
	}
	cur, prev := candles[i], candles[i-1]
	if cur.Ma20 <= 0 || prev.Ma20 <= 0 {
		return false
	}
	if cur.Close <= cur.Open { // 양봉
		return false
	}
	if prev.Close > prev.Ma20 { // 전일은 이평 아래(이하)
		return false
	}
	if cur.Close <= cur.Ma20 { // 당일 종가 돌파
		return false
	}
	return streak <= 0 || IsMA20RisingStreak(candles, i, streak)
}
