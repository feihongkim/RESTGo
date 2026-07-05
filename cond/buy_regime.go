package cond

// buy_regime.go — 시장 국면 조건 (2026-07-05).
//
// 골든크로스 임박(GC-pending): MA60 < MA120 역배열인데 간격이 지속 축소돼
// 곧 골든크로스가 날 것 같은 국면 = 하락 추세 관성이 소진되고 바닥을 다지는 중이라는 방증.
// (사용자 설계 — W패턴·strategy1의 국면 필터 후보)
//
// 판정 (기본값):
//   ① MA60 < MA120 (미교차)
//   ② 간격 gap = (MA120-MA60)/MA120 이 최근 GCPendingShrinkBars(20봉)에 걸쳐 지속 축소
//      — 노이즈 내성을 위해 5봉 간격 체크포인트 4개(0,5,10,15,20)가 전부 단조 축소
//   ③ "임박" = gap ≤ GCPendingMaxGapPct(3%)
// 연속량(gapPct·shrinkChk)도 반환해 게이트 없이 속성 기록·사후 스윕이 가능하다.

import "RESTGo/box"

// GCPendingShrinkBars 는 간격 축소 확인 구간 (달력 아닌 거래일 봉).
const GCPendingShrinkBars = 20

// GCPendingMaxGapPct 는 "임박" 간격 임계 (%).
const GCPendingMaxGapPct = 3.0

// GoldenCrossPendingInfo 는 pos 시점의 GC-pending 판정과 연속량을 반환한다.
//   pending    — ①②③ 모두 충족
//   inverted   — ① MA60 < MA120 (역배열 상태)
//   gapPct     — 간격 % (역배열이 아니면 음수가 될 수 있음 — 교차 후)
//   shrinkChk  — 연속 축소 체크포인트 수 (5봉 간격, 최대 8 = 40봉. ②는 4 이상)
func GoldenCrossPendingInfo(candles []*box.Candle, pos int) (pending, inverted bool, gapPct float64, shrinkChk int) {
	gap := func(i int) (float64, bool) {
		c := candles[i]
		if c.Ma60 <= 0 || c.Ma120 <= 0 {
			return 0, false
		}
		return (c.Ma120 - c.Ma60) / c.Ma120 * 100, true
	}
	g0, ok := gap(pos)
	if !ok {
		return false, false, 0, 0
	}
	gapPct = g0
	inverted = candles[pos].Ma60 < candles[pos].Ma120

	// 연속 축소 체크포인트: gap[pos-5k] < gap[pos-5(k+1)] 이 이어지는 개수 (최대 8)
	prev := g0
	for k := 1; k <= 8; k++ {
		i := pos - 5*k
		if i < 0 {
			break
		}
		g, ok2 := gap(i)
		if !ok2 || prev >= g {
			break
		}
		prev = g
		shrinkChk++
	}

	need := GCPendingShrinkBars / 5
	pending = inverted && shrinkChk >= need && gapPct <= GCPendingMaxGapPct && gapPct > 0
	return pending, inverted, gapPct, shrinkChk
}
