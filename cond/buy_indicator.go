package cond

import (
	"math"

	"RESTGo/box"
)

// 지표(RSI / Bollinger / MA) 기반 매수 조건 모음.
// Box 구조 조건과 달리 indicator 패키지가 채운 Candle 필드만 사용한다.
//
// 워밍업 주의:
//   - RSI는 period(14) 이전 캔들에서 0으로 남아 있으므로 rsiWarmup 이전 위치는 false 처리
//   - Bollinger는 period(20) 이전 캔들에서 Upper/Lower가 0이므로 hasValidBollinger로 가드

// rsiWarmup 은 indicator.CalculateRSI(period=14) 기준 RSI가 유효해지는 최소 위치
const rsiWarmup = 14

func hasValidRSI(ctx *box.TradingContext) bool {
	return ctx.Position >= rsiWarmup
}

func hasValidBollinger(c *box.Candle) bool {
	return c.BollingerUpper > c.BollingerLower
}

// ============================================================================
// RSI 조건
// ============================================================================

// IsRSIOversold 는 현재 RSI가 과매도 임계값(기본 30) 미만인지 확인
func IsRSIOversold(ctx *box.TradingContext, threshold float64) bool {
	if !hasValidRSI(ctx) {
		return false
	}
	return ctx.CandleList[ctx.Position].RSI < threshold
}

// IsRSIOverbought 는 현재 RSI가 과매수 임계값(기본 70) 초과인지 확인.
// 주로 when_not(과열 구간 진입 배제)으로 사용.
func IsRSIOverbought(ctx *box.TradingContext, threshold float64) bool {
	if !hasValidRSI(ctx) {
		return false
	}
	return ctx.CandleList[ctx.Position].RSI > threshold
}

// IsRSIRecoveringFromOversold 는 최근 lookback 구간에 과매도(RSI < oversold)가 존재했고,
// 현재 RSI가 과매도 구간을 벗어나(>= oversold) 직전 캔들보다 상승 중인지 확인 (과매도 탈출 반등)
func IsRSIRecoveringFromOversold(ctx *box.TradingContext, oversold float64, lookback int) bool {
	pos := ctx.Position
	if pos < rsiWarmup+1 {
		return false
	}
	cur := ctx.CandleList[pos]
	if cur.RSI < oversold || cur.RSI <= ctx.CandleList[pos-1].RSI {
		return false
	}
	start := pos - lookback
	if start < rsiWarmup {
		start = rsiWarmup
	}
	for i := start; i < pos; i++ {
		if ctx.CandleList[i].RSI < oversold {
			return true
		}
	}
	return false
}

// IsRSIRising 은 최근 period 캔들 동안 RSI가 단조 상승했는지 확인
func IsRSIRising(ctx *box.TradingContext, period int) bool {
	pos := ctx.Position
	if pos < rsiWarmup+period {
		return false
	}
	for i := pos - period + 1; i <= pos; i++ {
		if ctx.CandleList[i].RSI <= ctx.CandleList[i-1].RSI {
			return false
		}
	}
	return true
}

// IsRSIInBullZone 은 현재 RSI가 건전 모멘텀 구간 [low, high](기본 50~70)에 있는지 확인
func IsRSIInBullZone(ctx *box.TradingContext, low, high float64) bool {
	if !hasValidRSI(ctx) {
		return false
	}
	rsi := ctx.CandleList[ctx.Position].RSI
	return rsi >= low && rsi <= high
}

// ============================================================================
// Bollinger Band 조건 (매수 측)
// 주의: BollingerWidth 단위는 percent (4.0 = 4%) — sell_bb_volatility.go와 동일
// ============================================================================

// IsBBLowerTouch 는 당일 저가가 볼린저 하단 밴드 이하로 닿았는지 확인
func IsBBLowerTouch(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(c) {
		return false
	}
	return c.Low <= c.BollingerLower
}

// IsBBReboundFromLower 는 최근 lookback 구간에 하단 밴드 터치가 있었고,
// 현재 %B가 recoveryPercentB(기본 0.3) 이상으로 회복했는지 확인 (하단 반등)
func IsBBReboundFromLower(ctx *box.TradingContext, lookback int, recoveryPercentB float64) bool {
	pos := ctx.Position
	cur := ctx.CandleList[pos]
	if !hasValidBollinger(cur) || cur.BBPercent < recoveryPercentB {
		return false
	}
	start := pos - lookback
	if start < 0 {
		start = 0
	}
	for i := start; i < pos; i++ {
		c := ctx.CandleList[i]
		if hasValidBollinger(c) && c.Low <= c.BollingerLower {
			return true
		}
	}
	return false
}

// IsBBSqueezeBreakout 은 최근 lookback 구간에 스퀴즈(BBWidth < widthThreshold)가 존재했고,
// 현재 %B가 breakoutPercentB(기본 0.8) 초과로 상방 이탈 중인지 확인 (스퀴즈 후 상방 돌파)
func IsBBSqueezeBreakout(ctx *box.TradingContext, lookback int, widthThreshold, breakoutPercentB float64) bool {
	cur := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(cur) || cur.BBPercent <= breakoutPercentB {
		return false
	}
	return HasRecentSqueeze(ctx, lookback, widthThreshold)
}

// IsBBUpperBreakout 은 당일 종가가 볼린저 상단 밴드를 돌파(%B > 1)했는지 확인
func IsBBUpperBreakout(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(c) {
		return false
	}
	return c.Close > c.BollingerUpper
}

// IsAboveBBMiddle 은 최근 minDuration 캔들이 모두 중심선 위(%B >= 0.5)를 유지했는지 확인
func IsAboveBBMiddle(ctx *box.TradingContext, minDuration int) bool {
	pos := ctx.Position
	if pos < minDuration-1 {
		return false
	}
	for i := pos - minDuration + 1; i <= pos; i++ {
		c := ctx.CandleList[i]
		if !hasValidBollinger(c) || c.BBPercent < 0.5 {
			return false
		}
	}
	return true
}

// IsBBSqueezeHistorical 은 볼린저 "Method I" 정통 버전:
// 직전 squeezeLookback(20봉) 구간에 "역사적 스퀴즈"(BBWidth ≤ 과거 historicalLookback 최솟값 × maxRatio)가
// 존재했는지 확인. DefBox 돌파 자체가 방향을 확인하므로 %B 임계는 별도로 요구하지 않는다.
func IsBBSqueezeHistorical(ctx *box.TradingContext, historicalLookback int, maxRatio float64) bool {
	pos := ctx.Position
	cur := ctx.CandleList[pos]
	if !hasValidBollinger(cur) {
		return false
	}
	histStart := pos - historicalLookback
	if histStart < 0 {
		histStart = 0
	}
	minWidth := math.MaxFloat64
	for i := histStart; i < pos; i++ {
		c := ctx.CandleList[i]
		if hasValidBollinger(c) && c.BollingerWidth < minWidth {
			minWidth = c.BollingerWidth
		}
	}
	if minWidth == math.MaxFloat64 || minWidth == 0 {
		return false
	}
	sqStart := pos - 20
	if sqStart < 0 {
		sqStart = 0
	}
	for i := sqStart; i < pos; i++ {
		c := ctx.CandleList[i]
		if hasValidBollinger(c) && c.BollingerWidth <= minWidth*maxRatio {
			return true
		}
	}
	return false
}

// IsBBWalkingUp 은 볼린저 "Method II" (Band Walk):
// 최근 minDuration 봉 중 과반 이상이 %B >= minPercentB 를 유지 — 상단 밴드 근처 지속성 확인.
// DefBox 돌파와 결합 시 %B는 0.6 이상(중심선~상단)이면 충분 (0.8은 과엄격).
func IsBBWalkingUp(ctx *box.TradingContext, minDuration int, minPercentB float64) bool {
	pos := ctx.Position
	if pos < minDuration-1 {
		return false
	}
	cur := ctx.CandleList[pos]
	if !hasValidBollinger(cur) || cur.BBPercent < minPercentB {
		return false
	}
	count := 0
	for i := pos - minDuration + 1; i <= pos; i++ {
		c := ctx.CandleList[i]
		if hasValidBollinger(c) && c.BBPercent >= minPercentB {
			count++
		}
	}
	return count*2 >= minDuration // 과반 이상
}

// IsBBWBottomPattern 은 볼린저 "Method III" (W바텀 반등):
// P1(BB하단 이탈) → 중간 반등(%B≥0.4) → P2(BB하단 위에서 형성된 2차 저점) → 현재 반등 중.
// DefBox 돌파가 "중심선 돌파" 역할을 하므로 현재 %B 임계는 별도로 요구하지 않는다.
// P2가 BB하단 위에서 형성된다는 사실 자체가 매도 압력 약화(강도 다이버전스)를 의미한다.
func IsBBWBottomPattern(ctx *box.TradingContext, lookback int) bool {
	pos := ctx.Position
	cur := ctx.CandleList[pos]
	if !hasValidBollinger(cur) {
		return false
	}
	start := pos - lookback
	if start < 20 {
		start = 20
	}
	if start >= pos-6 {
		return false
	}

	// P1: BB하단 이탈 저점 (가장 먼저 발견된 것)
	p1Pos := -1
	p1PercentB := 1.0
	for i := start; i < pos-5; i++ {
		c := ctx.CandleList[i]
		if hasValidBollinger(c) && c.Low <= c.BollingerLower {
			if c.BBPercent < p1PercentB {
				p1Pos = i
				p1PercentB = c.BBPercent
			}
		}
	}
	if p1Pos < 0 {
		return false
	}

	// 중간 반등: P1 이후 %B가 0.4 이상으로 회복
	recovPos := -1
	for i := p1Pos + 1; i < pos-2; i++ {
		c := ctx.CandleList[i]
		if hasValidBollinger(c) && c.BBPercent >= 0.4 {
			recovPos = i
			break
		}
	}
	if recovPos < 0 {
		return false
	}

	// P2: 회복 이후 BB하단 위의 저점(%B < 0.5) — 밴드 내부에서 매도 압력 약화
	for i := recovPos + 1; i < pos; i++ {
		c := ctx.CandleList[i]
		if !hasValidBollinger(c) {
			continue
		}
		if c.BBPercent < 0.5 && c.Low > c.BollingerLower {
			return true
		}
	}
	return false
}

// IsBBWBottomBoxPattern 은 Box 시퀀스(5이평 기울기 전환점) 기반 W바텀 패턴을 탐지한다.
// 하단Box(slope-→+, BB하단 이탈) → 상단Box(slope+→-) → 하단Box(slope-→+, BB하단 위)
//
// 탐색 방향: 가장 최근 시퀀스부터 역방향 탐색 → 패턴 완성 직후 신호 발화 보장.
// P2(두 번째 하단Box)는 현재 위치에서 maxP2Gap 봉 이내여야 한다.
// BB 하단 이탈 체크는 박스 위치 ±5봉 창에서 수행한다.
// FindBBWBottomBoxPattern 은 IsBBWBottomBoxPattern 과 동일 조건으로 P1/P2 박스 위치도 반환한다.
// found=false 이면 p1Pos, p2Pos 는 무효.
func FindBBWBottomBoxPattern(ctx *box.TradingContext, lookback int) (p1Pos, p2Pos int, found bool) {
	p1Pos, p2Pos, _, found = findBBWBottomBoxPatternOpt(ctx, lookback, true)
	return p1Pos, p2Pos, found
}

// FindBBWBottomBoxPatternRelaxed 는 P1 급락 조건(BB 하단 이탈 + 밴드폭 팽창)을 게이트가 아닌
// 속성으로 반환하는 완화판 (2026-07-05, GC-pending 국면 연구용 — 운영 경로 불변).
// bbCrash = 엄격판(P1 하단 이탈 + BBW 팽창) 충족 여부.
func FindBBWBottomBoxPatternRelaxed(ctx *box.TradingContext, lookback int) (p1Pos, p2Pos int, bbCrash, found bool) {
	return findBBWBottomBoxPatternOpt(ctx, lookback, false)
}

func findBBWBottomBoxPatternOpt(ctx *box.TradingContext, lookback int, requireBBCrash bool) (p1Pos, p2Pos int, bbCrash, found bool) {
	pos := ctx.Position
	candles := ctx.CandleList
	boxes := ctx.BoxList

	if !hasValidBollinger(candles[pos]) {
		return 0, 0, false, false
	}
	startPos := pos - lookback
	if startPos < 20 {
		startPos = 20
	}
	if startPos >= pos-6 {
		return 0, 0, false, false
	}

	type slot struct {
		bpos     int
		btype    int
		bbBreach bool // P1 조건: bp 직전 10봉 중 저가<=BB하단 봉이 5개 이상
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
			if hasValidBollinger(c) && c.Low <= c.BollingerLower {
				count++
			}
		}
		slots = append(slots, slot{b.BoxPosition, b.BoxType, count >= 5})
	}

	const maxP2Gap = 15
	n := len(slots)
	if n < 3 {
		return 0, 0, false, false
	}
	s3 := slots[n-1]
	if s3.btype != box.BoxTypeSupport || s3.bbBreach || pos-s3.bpos > maxP2Gap {
		return 0, 0, false, false
	}
	s2 := slots[n-2]
	if s2.btype != box.BoxTypeResistance || s2.bpos >= s3.bpos {
		return 0, 0, false, false
	}
	// 상단Box 종가가 MA20 아래여야 함 (여전히 약세 구간임을 확인)
	// 상단Box 종가가 MA20 아래여야 함 (여전히 약세 구간임을 확인)
	if c2 := candles[s2.bpos]; c2.Ma20 == 0 || c2.Close >= c2.Ma20 {
		return 0, 0, false, false
	}
	s1 := slots[n-3]
	if s1.btype != box.BoxTypeSupport || s1.bpos >= s2.bpos {
		return 0, 0, false, false
	}
	// P1 급락 조건 (BB 하단 이탈 + 밴드폭 팽창): 엄격판은 게이트, 완화판은 속성(bbCrash)
	bbCrash = s1.bbBreach && isBBWExpanding(candles, s1.bpos)
	if requireBBCrash && !bbCrash {
		return 0, 0, false, false
	}
	return s1.bpos, s3.bpos, bbCrash, true
}

// isBBWExpanding 은 pos 시점에서 볼린저 밴드폭이 팽창 중인지 확인한다.
// 조건 ①: BBW[pos] > BBW[pos-5]  (5봉 기울기 양수)
// 조건 ②: BBW[pos] > min(BBW[pos-20:pos]) * 1.2  (최근 20봉 최솟값 대비 20% 이상 확대)
func isBBWExpanding(candles []*box.Candle, pos int) bool {
	bbw := func(i int) float64 {
		c := candles[i]
		if c.Ma20 == 0 {
			return 0
		}
		return (c.BollingerUpper - c.BollingerLower) / c.Ma20
	}

	cur := bbw(pos)
	if cur == 0 {
		return false
	}

	// ① 5봉 전보다 밴드폭이 넓어야
	slopeRef := pos - 5
	if slopeRef < 0 {
		slopeRef = 0
	}
	if cur <= bbw(slopeRef) {
		return false
	}

	// ② 최근 20봉 최솟값 대비 20% 이상 확대
	minStart := pos - 20
	if minStart < 0 {
		minStart = 0
	}
	minBBW := cur
	for i := minStart; i < pos; i++ {
		if v := bbw(i); v > 0 && v < minBBW {
			minBBW = v
		}
	}
	return cur >= minBBW*1.2
}

func IsBBWBottomBoxPattern(ctx *box.TradingContext, lookback int) bool {
	_, _, found := FindBBWBottomBoxPattern(ctx, lookback)
	return found
}

// ============================================================================
// 이동평균(MA) 조건 (매수 측)
// 정렬 헬퍼(IsProperArrangement 등 *Candle 단위)는 sell_helpers.go에 있으며 여기서 재사용
// ============================================================================

// IsMaGoldenCross5x20 은 당일 MA5가 MA20을 상향 돌파(골든크로스)했는지 확인
func IsMaGoldenCross5x20(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	prev, cur := ctx.CandleList[pos-1], ctx.CandleList[pos]
	if prev.Ma5 == 0 || prev.Ma20 == 0 || cur.Ma20 == 0 {
		return false
	}
	return prev.Ma5 <= prev.Ma20 && cur.Ma5 > cur.Ma20
}

// IsMaGoldenCross20x60 은 당일 MA20이 MA60을 상향 돌파했는지 확인
func IsMaGoldenCross20x60(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	prev, cur := ctx.CandleList[pos-1], ctx.CandleList[pos]
	if prev.Ma20 == 0 || prev.Ma60 == 0 || cur.Ma60 == 0 {
		return false
	}
	return prev.Ma20 <= prev.Ma60 && cur.Ma20 > cur.Ma60
}

// IsMaProperArrangementNow 는 현재 캔들이 MA 정배열(MA5 > MA20 > MA60)인지 확인
func IsMaProperArrangementNow(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.Ma60 == 0 {
		return false
	}
	return IsProperArrangement(c)
}

// IsAllMaRising 은 MA5/MA20/MA60 기울기가 모두 양수(동반 상승)인지 확인
func IsAllMaRising(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.Ma60 == 0 {
		return false
	}
	return c.Gradient > 0 && c.Gradient20 > 0 && c.Gradient60 > 0
}

// IsMaConvergence 는 MA5/MA20/MA60이 threshold(기본 0.03 = 3%) 이내로 수렴했는지 확인.
// 수렴 후 발산 직전 구간 포착용 — (max-min)/MA60 <= threshold
func IsMaConvergence(ctx *box.TradingContext, threshold float64) bool {
	c := ctx.CandleList[ctx.Position]
	if c.Ma60 == 0 {
		return false
	}
	maxMa := math.Max(c.Ma5, math.Max(c.Ma20, c.Ma60))
	minMa := math.Min(c.Ma5, math.Min(c.Ma20, c.Ma60))
	return (maxMa-minMa)/c.Ma60 <= threshold
}

// IsPriceAboveAllMa 는 현재 종가가 MA5/MA20/MA60 모두 위에 있는지 확인
func IsPriceAboveAllMa(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.Ma60 == 0 {
		return false
	}
	return c.Close > c.Ma5 && c.Close > c.Ma20 && c.Close > c.Ma60
}
