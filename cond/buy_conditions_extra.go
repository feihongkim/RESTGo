package cond

import (
	"RESTGo/box"
	"math"
)

// ── 캔들 패턴 ──────────────────────────────────────────────

func IsBullishCandle(ctx *box.TradingContext) bool {
	c := ctx.GetCurrentCandle()
	if c == nil {
		return false
	}
	return c.Close > c.Open
}

// HasPullbackOrCorrection 은 DefBox~현재 직전 구간에 눌림목(Gradient<0) 또는
// 조정(Low≤MA20 && 종가≤시가) 캔들이 1개 이상 있는지 확인 (C# CandlePatternEvaluator.HasPullbackOrCorrection)
func HasPullbackOrCorrection(ctx *box.TradingContext) bool {
	for i := ctx.DefboxPosition; i >= 0 && i < ctx.Position && i < len(ctx.CandleList); i++ {
		// 눌림목: DefBox 이후 상승세 꺾임
		if i > ctx.DefboxPosition && ctx.CandleList[i].Gradient < 0.0 {
			return true
		}
		// 조정: 저가가 20이평 이하이고 음봉(도지 포함)
		if ctx.CandleList[i].Low <= ctx.CandleList[i].Ma20 &&
			ctx.CandleList[i].Close <= ctx.CandleList[i].Open {
			return true
		}
	}
	return false
}

// ── Box 구조 조건 ──────────────────────────────────────────

func IsMainboxCloserThanCurrentPosition(ctx *box.TradingContext) bool {
	return (ctx.DefboxPosition - ctx.MainboxPosition) >= (ctx.Position - ctx.DefboxPosition)
}

func IsBoxCountBetweenDefAndMainLessThanOrEqual(ctx *box.TradingContext, maxCount int) bool {
	defBox := ctx.GetDefBox()
	if defBox == nil || len(defBox.MainDefLink) == 0 {
		return false
	}
	mainIdx := defBox.MainDefLink[0]
	return (ctx.DefboxIndex - mainIdx) <= maxCount
}

func GetMultiDefboxDamCount(ctx *box.TradingContext) int {
	defBox := ctx.GetDefBox()
	if defBox == nil || len(defBox.MainDefLink) == 0 {
		return 0
	}
	defCount := len(defBox.MainDefLink)
	mainIdx := defBox.MainDefLink[defCount-1]
	if mainIdx < 0 || mainIdx >= len(ctx.BoxList) {
		return 0
	}
	exposition := ctx.BoxList[mainIdx].BoxPosition
	damCount := 0
	for i := exposition; i < ctx.Position; i++ {
		if ctx.CandleList[i].Close > ctx.DefboxPrice {
			damCount++
		}
		if ctx.CandleList[i].Open > ctx.DefboxPrice {
			damCount++
		}
	}
	return damCount
}

// IsMultiDefRelaxedDamCondition: Strategy 6/11 — IsMa60StrongerThanMa120By2Percent 분기
// C# BuyDecisionProcessor.Options.cs:270 — multiDefRelaxedBuy_O2 의 damCount 조건:
//
//	(IsMa60StrongerThanMa120By2Percent && 2<damCount<5) ||
//	(!IsMa60StrongerThanMa120By2Percent && damCount<=2)
func IsMultiDefRelaxedDamCondition(ctx *box.TradingContext) bool {
	damCount := GetMultiDefboxDamCount(ctx)
	ma60Strong := IsMa60StrongerThanMa120By2Percent(ctx)
	if ma60Strong {
		return damCount > 2 && damCount < 5
	}
	return damCount <= 2
}

// ── 이동평균선 조건 ────────────────────────────────────────

// IsMa20NearMa60WithComplexValidation: MA20이 MA60 근처 + 정배열 (3단계 복합 검증)
func IsMa20NearMa60WithComplexValidation(ctx *box.TradingContext) bool {
	c := ctx.GetCurrentCandle()
	if c == nil {
		return false
	}
	if c.Ma60 == 0.0 {
		return true
	}
	if c.Ma20 < 0.99*c.Ma60 {
		return false
	}
	mvcond1 := c.Ma120 > c.Ma20 &&
		c.Ma120 > c.Ma60 &&
		(c.Ma120-c.Ma60) > (c.Ma120-c.Ma20) &&
		0.1*(c.Ma120-c.Ma60) <= (c.Ma120-c.Ma20)
	mvcond2 := 0.99*c.Ma120 < c.Ma60
	mvcond3 := (c.Ma20-c.Ma60) >= 2.0*(c.Ma60-c.Ma120) &&
		(c.Ma20-c.Ma60) >= 0.05*c.Close
	return (mvcond1 || mvcond2) && !mvcond3
}

// IsMa20NearMa60WithSimpleValidation: MA20이 MA60 근처 (단순 검증)
func IsMa20NearMa60WithSimpleValidation(ctx *box.TradingContext) bool {
	c := ctx.GetCurrentCandle()
	if c == nil {
		return false
	}
	if c.Ma60 == 0.0 {
		return true
	}
	if c.Ma20 < 0.99*c.Ma60 {
		return false
	}
	isMa60NearOrAboveMa120 := 0.99*c.Ma120 < c.Ma60
	hasSignificantGap := (c.Ma20-c.Ma60) >= 2.0*(c.Ma60-c.Ma120) &&
		(c.Ma20-c.Ma60) >= 0.05*c.Close
	return isMa60NearOrAboveMa120 && !hasSignificantGap
}

// IsMa60StrongerThanMa120By2Percent: MA60 > MA120 × 0.98
func IsMa60StrongerThanMa120By2Percent(ctx *box.TradingContext) bool {
	c := ctx.GetCurrentCandle()
	if c == nil || c.Ma60 == 0.0 {
		return false
	}
	return 0.98*c.Ma120 < c.Ma60
}

// IsMainboxPriceAboveMa60OrMa120: MainBox 형성 시점 캔들 기준
func IsMainboxPriceAboveMa60OrMa120(ctx *box.TradingContext) bool {
	defBox := ctx.GetDefBox()
	if defBox == nil || len(defBox.MainDefLink) == 0 {
		return false
	}
	mainBoxIdx := defBox.MainDefLink[0] + 1
	if mainBoxIdx >= len(ctx.BoxList) {
		return false
	}
	boxPrice := ctx.BoxList[mainBoxIdx].Price
	boxPos := ctx.BoxList[mainBoxIdx].BoxPosition
	if boxPos < 0 || boxPos >= len(ctx.CandleList) {
		return false
	}
	c := ctx.CandleList[boxPos]
	return boxPrice > c.Ma60 || boxPrice > c.Ma120
}

// HasLowTouchedMa20: MainboxPosition ~ Position 구간에서 저가가 MA20 이하
func HasLowTouchedMa20(ctx *box.TradingContext) bool {
	for i := ctx.MainboxPosition; i <= ctx.Position; i++ {
		if ctx.CandleList[i].Low <= ctx.CandleList[i].Ma20 {
			return true
		}
	}
	return false
}

// IsMainboxConditionValid: MainBox 위치 + MA 조건 복합 검증
func IsMainboxConditionValid(ctx *box.TradingContext) bool {
	if ctx.MainboxPosition <= 55 {
		return true
	}
	if (ctx.Position - ctx.MainboxPosition) > 15 {
		return true
	}
	tempHighChecker := 0
	start := ctx.MainboxPosition - 55
	for i := start; i < ctx.MainboxPosition; i++ {
		if i > 0 &&
			ctx.CandleList[i].Close > ctx.DefboxPrice &&
			ctx.CandleList[i-1].Close > ctx.DefboxPrice {
			tempHighChecker++
		}
	}
	c := ctx.CandleList[ctx.Position]
	return c.Ma60 > c.Ma120 &&
		c.Gradient60 >= 0.0 &&
		c.Gradient120 >= 0.0 &&
		tempHighChecker <= 2
}

// isMaSpreadExcessive: MA20-MA60 간격이 threshold 초과
func isMaSpreadExcessive(ctx *box.TradingContext, threshold float64) bool {
	c := ctx.GetCurrentCandle()
	if c == nil || c.Ma60 == 0.0 {
		return false
	}
	return (c.Ma20-c.Ma60) > threshold*c.Close &&
		(c.Ma20-c.Ma60) > (c.Ma60-c.Ma120)
}

// EvaluateMainBoxPositionBasedTiming: MainBox 위치 + MA 스프레드 기반 타이밍 검증
func EvaluateMainBoxPositionBasedTiming(ctx *box.TradingContext, maSpreadThreshold float64) bool {
	excessive := isMaSpreadExcessive(ctx, maSpreadThreshold)
	touched := HasLowTouchedMa20(ctx)

	if ctx.MainboxPosition <= 40 {
		if excessive && !touched {
			return false
		}
		return true
	}
	dist := ctx.Position - ctx.MainboxPosition
	if dist <= 15 {
		highBreakouts := countHighBreakouts(ctx)
		if highBreakouts > 0 || (excessive && !touched) {
			return false
		}
	} else if dist <= 20 {
		if excessive && !touched {
			return false
		}
	}
	return true
}

// EvaluateMainBoxPositionBasedTimingLess: Timing의 엄격 버전 — HasLowTouchedMa20 탈출 없음
func EvaluateMainBoxPositionBasedTimingLess(ctx *box.TradingContext, maSpreadThreshold float64) bool {
	excessive := isMaSpreadExcessive(ctx, maSpreadThreshold)

	if ctx.MainboxPosition <= 40 {
		return !excessive
	}
	dist := ctx.Position - ctx.MainboxPosition
	if dist <= 20 {
		return !excessive
	}
	return true
}

func countHighBreakouts(ctx *box.TradingContext) int {
	count := 0
	startPos := int(math.Max(0, float64(ctx.MainboxPosition-40)))
	for i := startPos; i < ctx.Position; i++ {
		if i > 0 &&
			ctx.CandleList[i].Close > ctx.DefboxPrice &&
			ctx.CandleList[i-1].Close > ctx.DefboxPrice {
			count++
		}
	}
	for i := ctx.MainboxPosition; i < ctx.Position; i++ {
		if ctx.CandleList[i].High > 1.02*ctx.DefboxPrice {
			count++
		}
	}
	return count
}

// ── 관통 조건 ─────────────────────────────────────────────

// firstGradientChecker: Gradient 상승→하락(Direction=false) 첫 전환 위치. 없으면 0.
func firstGradientChecker(candles []*box.Candle, start, end int) int {
	for i := start + 1; i <= end && i < len(candles); i++ {
		if candles[i-1].Gradient >= 0.0 && candles[i].Gradient < 0.0 {
			return i
		}
	}
	return 0
}

// IsPenetrationOptionValid: oscillator.go의 EvaluatePenetrationOption 위임
func IsPenetrationOptionValid(ctx *box.TradingContext) bool {
	return EvaluatePenetrationOption(ctx)
}
