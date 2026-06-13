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
