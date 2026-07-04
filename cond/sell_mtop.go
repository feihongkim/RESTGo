package cond

// sell_mtop.go — 상방 M자(이중천장) 패턴 매도 신호 (2026-07-04, 사용자 설계).
//
// W바텀(FindBBWBottomBoxPattern)의 상하 대칭 미러 + 별도 붕괴 트리거:
//   ① M 완성(장전): 5이평 변곡 3박스 resist(P1) → support → resist(P2)
//      - P1 직전 10봉 중 고가≥BB상단 봉 5개 이상 (상단 이탈 = 과열 고점) + BBW 팽창
//      - 중간 support 박스 종가가 MA20 위 (여전히 강세 구간)
//      - P2는 BB 내부: 고가 < BB상단 AND 직전 10봉 상단 이탈 5개 미만
//      - DefBox와 무관
//   ② 붕괴(트리거): P2 인지 후 20봉 내, 20이평 부근에서 음-양-음 반등 실패
//      - 연속 동색 봉은 하나의 런(run)으로 묶음 (음양양음, 음음양음음 등 모두 음양음)
//      - 마지막 음봉 종가 < 양봉 런의 최저 저가 (반등분 전량 반납)
//      - 20이평 부근: 3개 런의 저가~고가 범위가 트리거 캔들 MA20을 걸치거나 종가가 MA20 ±3% 이내
//      - 도지(시=종)는 런을 끊는다 (패턴 불성립)

import "RESTGo/box"

// MTopCollapseNearMA20Pct 는 "20이평 부근" 판정의 종가 거리 허용치.
const MTopCollapseNearMA20Pct = 0.03

// FindBBMTopBoxPattern 은 pos(마지막 resist 박스 인지 캔들)에서 M자 3박스 패턴을 찾는다.
// FindBBWBottomBoxPattern의 상하 대칭. 반환: P1/P2 박스 캔들 인덱스.
func FindBBMTopBoxPattern(ctx *box.TradingContext, lookback int) (p1Pos, p2Pos int, found bool) {
	pos := ctx.Position
	candles := ctx.CandleList
	boxes := ctx.BoxList

	if !hasValidBollinger(candles[pos]) {
		return 0, 0, false
	}
	startPos := pos - lookback
	if startPos < 20 {
		startPos = 20
	}
	if startPos >= pos-6 {
		return 0, 0, false
	}

	type slot struct {
		bpos          int
		btype         int
		bbBreachUpper bool // 직전 10봉 중 고가>=BB상단 봉 5개 이상
	}

	var slots []slot
	for _, b := range boxes {
		if b.BoxPosition < startPos || b.BoxPosition >= pos {
			continue
		}
		wStart := b.BoxPosition - 10
		if wStart < 0 {
			wStart = 0
		}
		count := 0
		for i := wStart; i < b.BoxPosition; i++ {
			c := candles[i]
			if hasValidBollinger(c) && c.High >= c.BollingerUpper {
				count++
			}
		}
		slots = append(slots, slot{b.BoxPosition, b.BoxType, count >= 5})
	}

	const maxP2Gap = 15
	n := len(slots)
	if n < 3 {
		return 0, 0, false
	}
	// P2 (마지막 resist): BB 내부 — 고가 < 상단 AND 상단 이탈 통계 미충족
	s3 := slots[n-1]
	if s3.btype != box.BoxTypeResistance || s3.bbBreachUpper || pos-s3.bpos > maxP2Gap {
		return 0, 0, false
	}
	if c3 := candles[s3.bpos]; !hasValidBollinger(c3) || c3.High >= c3.BollingerUpper {
		return 0, 0, false
	}
	// 중간 support: 종가가 MA20 위 (여전히 강세 구간임을 확인 — W의 대칭)
	s2 := slots[n-2]
	if s2.btype != box.BoxTypeSupport || s2.bpos >= s3.bpos {
		return 0, 0, false
	}
	if c2 := candles[s2.bpos]; c2.Ma20 == 0 || c2.Close <= c2.Ma20 {
		return 0, 0, false
	}
	// P1 (첫 resist): BB 상단 이탈 + BBW 팽창
	s1 := slots[n-3]
	if s1.btype != box.BoxTypeResistance || s1.bpos >= s2.bpos || !s1.bbBreachUpper {
		return 0, 0, false
	}
	if !isBBWExpanding(candles, s1.bpos) {
		return 0, 0, false
	}
	return s1.bpos, s3.bpos, true
}

// candleColor 는 봉 색: +1 양봉, -1 음봉, 0 도지.
func candleColor(c *box.Candle) int {
	switch {
	case c.Close > c.Open:
		return 1
	case c.Close < c.Open:
		return -1
	default:
		return 0
	}
}

// FindMTopCollapseRuns 는 pos 캔들에서 끝나는 음-양-음 런 붕괴 패턴을 찾는다.
// 연속 동색 봉은 하나의 런으로 취급, 도지는 패턴을 끊는다. 런 시작은 minStart 이상이어야 한다.
// 조건: pos는 음봉 / 마지막 음봉 종가 < 양봉 런 최저 저가 / 20이평 부근(런 전체 범위가 MA20을
// 걸치거나 종가가 MA20 ±3% 이내). 반환: 첫 음봉 런의 시작 인덱스.
func FindMTopCollapseRuns(candles []*box.Candle, pos, minStart int) (runStart int, ok bool) {
	if pos < 2 || minStart < 0 || candleColor(candles[pos]) != -1 {
		return 0, false
	}
	// 런 3 (마지막 음봉 런): pos에서 뒤로
	i := pos
	for i-1 >= minStart && candleColor(candles[i-1]) == -1 {
		i--
	}
	run3Start := i
	// 런 2 (양봉 런)
	if run3Start-1 < minStart || candleColor(candles[run3Start-1]) != 1 {
		return 0, false
	}
	i = run3Start - 1
	for i-1 >= minStart && candleColor(candles[i-1]) == 1 {
		i--
	}
	run2Start := i
	// 런 1 (첫 음봉 런)
	if run2Start-1 < minStart || candleColor(candles[run2Start-1]) != -1 {
		return 0, false
	}
	i = run2Start - 1
	for i-1 >= minStart && candleColor(candles[i-1]) == -1 {
		i--
	}
	run1Start := i

	// 반등분 전량 반납: 마지막 음봉 종가 < 양봉 런 최저 저가
	yangLow := candles[run2Start].Low
	for j := run2Start; j < run3Start; j++ {
		if candles[j].Low < yangLow {
			yangLow = candles[j].Low
		}
	}
	if candles[pos].Close >= yangLow {
		return 0, false
	}

	// 20이평 부근: 런 전체 범위가 MA20을 걸치거나 종가가 ±3% 이내
	ma20 := candles[pos].Ma20
	if ma20 <= 0 {
		return 0, false
	}
	lo, hi := candles[run1Start].Low, candles[run1Start].High
	for j := run1Start; j <= pos; j++ {
		if candles[j].Low < lo {
			lo = candles[j].Low
		}
		if candles[j].High > hi {
			hi = candles[j].High
		}
	}
	straddle := lo <= ma20 && ma20 <= hi
	nearClose := candles[pos].Close >= ma20*(1-MTopCollapseNearMA20Pct) &&
		candles[pos].Close <= ma20*(1+MTopCollapseNearMA20Pct)
	if !straddle && !nearClose {
		return 0, false
	}
	return run1Start, true
}
