package cond

import (
	"RESTGo/box"
	"math"
)

// ── OscillatorEvaluator 포팅 ──────────────────────────────────────────────────
// C# 참조: Stock1/biz/Evaluators/OscillatorEvaluator.cs
// C# 참조: Stock1/biz/Evaluators/PenetrationEvaluator.cs

type oscilResult struct {
	lastPos  int
	firstPos int
	count    int
}

// lastOscilloPositionForMV: 구간 [start, end] 내 MA(mv) 상향/하향 돌파 위치 수집.
func lastOscilloPositionForMV(candles []*box.Candle, mv int, start, end int) oscilResult {
	var arr []int
	tempkey := 0

	getMV := func(c *box.Candle) float64 {
		if mv == 20 {
			return c.Ma20
		}
		return c.Ma5
	}

	for i := start; i <= end && i < len(candles); i++ {
		c := candles[i]
		mv20 := getMV(c)

		if tempkey == -1 && (c.Close > mv20 || c.Open > mv20) {
			arr = append(arr, i)
		}
		if tempkey == 1 && (c.Close < mv20 || c.Open < mv20) {
			arr = append(arr, i)
		}

		if c.Close < mv20 {
			tempkey = -1
		} else if c.Close > mv20 {
			tempkey = 1
		}
	}

	if len(arr) == 0 {
		return oscilResult{}
	}
	return oscilResult{
		lastPos:  arr[len(arr)-1],
		firstPos: arr[0],
		count:    len(arr),
	}
}

// crossMVCheck: DefBox~현재 구간의 MA5/MA20 교차 및 Gradient 패턴 분석.
func crossMVCheck(candles []*box.Candle, exposition, position int) string {
	if exposition < 0 || position >= len(candles) || exposition > position {
		return ""
	}

	defClose := candles[exposition].Close
	defMa20 := candles[exposition].Ma20

	if defClose > defMa20 {
		tempGradChk := 0
		tempCrossChk := 0
		lowGrad := 0.0

		for i := exposition; i <= position; i++ {
			if candles[i].Gradient <= 0.0 {
				tempGradChk++
			}
			if candles[i].Ma5 <= candles[i].Ma20 {
				tempCrossChk++
			}
			if lowGrad > candles[i].Gradient {
				lowGrad = candles[i].Gradient
			}
		}

		curGrad := candles[position].Gradient
		if tempGradChk == 0 {
			return "비변곡상승"
		}
		if tempCrossChk == 0 && math.Abs(lowGrad) <= curGrad {
			return "상승눌림"
		}
		if tempCrossChk != 0 && math.Abs(lowGrad) <= curGrad {
			return "유효눌림각"
		}
		if tempCrossChk != 0 && math.Abs(lowGrad) > curGrad {
			return "이평교차"
		}
		return ""
	}

	if defClose < defMa20 {
		tempGradChk := 0
		tempCrossChk := 0
		highGrad := 0.0

		for i := exposition; i <= position; i++ {
			if candles[i].Gradient >= 0.0 {
				tempGradChk++
			}
			if candles[i].Ma5 >= candles[i].Ma20 {
				tempCrossChk++
			}
			if highGrad < candles[i].Gradient {
				highGrad = candles[i].Gradient
			}
		}

		curGrad := candles[position].Gradient
		if tempGradChk == 0 {
			return "비변곡하락"
		}
		if tempCrossChk == 0 && -1.0*math.Abs(highGrad) >= curGrad {
			return "하락눌림"
		}
		if tempCrossChk != 0 {
			return "이평교차"
		}
		return ""
	}

	return ""
}

// piercingLimit: DefBox~현재 구간에서 "관통한계돌파" / "관통한계붕괴" 판정.
func piercingLimit(candles []*box.Candle, boxPrice float64, exposition, position int) string {
	if exposition < 0 || position >= len(candles) {
		return ""
	}

	defCandle := candles[exposition]
	curCandle := candles[position]

	if defCandle.Close >= defCandle.Ma20 {
		if curCandle.Close > curCandle.Ma20 {
			tempLowOpenPos := -1
			for i := exposition; i <= position; i++ {
				if candles[i].Open < candles[i].Ma20 {
					tempLowOpenPos = i
				}
			}
			if tempLowOpenPos >= 0 &&
				firstGradientChecker(candles, tempLowOpenPos, position) == 0 &&
				curCandle.Close >= boxPrice {
				return "관통한계돌파"
			}
			return ""
		}

		if curCandle.Close < curCandle.Ma20 {
			res := lastOscilloPositionForMV(candles, 20, exposition, position)
			if res.count == 1 {
				if firstGradientCheckerUp(candles, exposition, position) == 0 {
					return "관통한계붕괴"
				}
				return ""
			}
			if res.count > 2 {
				lowPrice := lowPriceInRange(candles, exposition, res.lastPos-1)
				if firstGradientCheckerUp(candles, exposition, position) == 0 &&
					curCandle.Close < lowPrice {
					return "관통한계붕괴"
				}
			}
		}
	}
	return ""
}

func firstGradientCheckerUp(candles []*box.Candle, start, end int) int {
	for i := start + 1; i <= end && i < len(candles); i++ {
		if candles[i-1].Gradient <= 0.0 && candles[i].Gradient > 0.0 {
			return i
		}
	}
	return 0
}

func lowPriceInRange(candles []*box.Candle, start, end int) float64 {
	low := math.MaxFloat64
	for i := start; i <= end && i < len(candles); i++ {
		if candles[i].Low < low {
			low = candles[i].Low
		}
	}
	if low == math.MaxFloat64 {
		return 0
	}
	return low
}

// countPricesBelowMa60: DefBox~현재 구간에서 종가 또는 시가가 MA60 이하인 캔들 수
func countPricesBelowMa60(candles []*box.Candle, defboxPos, position int) int {
	count := 0
	for i := defboxPos; i <= position && i < len(candles); i++ {
		c := candles[i]
		if c.Close < c.Ma60 || c.Open < c.Ma60 {
			count++
		}
	}
	return count
}

// evaluateUpwardMomentum: 유효한 상승 모멘텀 여부
func evaluateUpwardMomentum(candles []*box.Candle, firstOscilloPos, lastMa20CrossPos int, crossMVResult string) bool {
	if firstOscilloPos <= 0 {
		return false
	}
	firstCandle := candles[firstOscilloPos]
	lastCandle := candles[lastMa20CrossPos]

	firstIsBearish := (firstCandle.Close - firstCandle.Open) < 0.0
	lastIsBullish := (lastCandle.Close - lastCandle.Open) > 0.0

	posCondition := ((lastMa20CrossPos-1) == firstOscilloPos && firstIsBearish) ||
		(lastMa20CrossPos == firstOscilloPos)

	return posCondition && lastIsBullish && crossMVResult == "상승눌림"
}

// evaluateGradientReversal: Gradient 반전 없음 = true
func evaluateGradientReversal(candles []*box.Candle, firstOscilloPos, position int) bool {
	if firstOscilloPos <= 0 {
		return false
	}
	startPos := firstOscilloPos
	if candles[firstOscilloPos].Gradient > 0.0 {
		startPos = firstOscilloPos + 1
	}
	return firstGradientChecker(candles, startPos, position) == 0
}

// evaluateConsistentUpTrend: 일관된 상승 추세 여부
func evaluateConsistentUpTrend(candles []*box.Candle, firstOscilloPos int, oscRes oscilResult, defboxPos, position int, crossMVResult string) bool {
	if firstOscilloPos <= 0 {
		return true
	}
	firstCandle := candles[firstOscilloPos]
	firstIsBearish := (firstCandle.Close - firstCandle.Open) < 0.0
	if !firstIsBearish {
		return true
	}

	lastOscilloPos := oscRes.lastPos

	firstBullishPos := 0
	for i := firstOscilloPos; i <= lastOscilloPos && i < len(candles); i++ {
		c := candles[i]
		if (c.Close - c.Open) >= 0.0 {
			firstBullishPos = i
			break
		}
	}
	if firstBullishPos == 0 {
		return true
	}

	firstBearishAfter := 0
	for i := firstBullishPos; i <= lastOscilloPos && i < len(candles); i++ {
		c := candles[i]
		if (c.Close - c.Open) <= 0.0 {
			firstBearishAfter = i
			break
		}
	}

	if firstBearishAfter == 0 {
		if crossMVResult == "상승눌림" || crossMVResult == "유효눌림각" || crossMVResult == "비변곡상승" {
			return false
		}
	}
	return true
}

// EvaluatePenetrationOption: C# PenetrationEvaluator.EvaluatePenetrationOption 포팅
// (구 IsPenetrationOptionValid 는 C#에서 DEPRECATED 처리되어 이 함수로 대체됨)
func EvaluatePenetrationOption(ctx *box.TradingContext) bool {
	candles := ctx.CandleList
	defboxPos := ctx.DefboxPosition
	position := ctx.Position

	conclusion := true

	// Step 2
	// C# 원본은 LastOscilloPositionForMV 를 두 가지 시작위치로 호출하며 stateful
	// FirstOscilloPosition 을 갱신한다:
	//   - Step1 / UpwardMomentum  : 시작 defboxPos+1 (Penetration.cs:157)
	//   - GradientReversal / ConsistentUpTrend : 시작 defboxPos (Penetration.cs:270, 299)
	oscRes := lastOscilloPositionForMV(candles, 20, defboxPos+1, position)
	oscResFromDefbox := lastOscilloPositionForMV(candles, 20, defboxPos, position)
	lastMa20CrossPos := oscRes.lastPos

	if lastMa20CrossPos > 0 {
		crossMVResult := crossMVCheck(candles, defboxPos, position)
		piercingResult := piercingLimit(candles, ctx.DefboxPrice, defboxPos, position)
		isPiercingLimitBreakthrough := piercingResult == "관통한계돌파"

		countBelowMa60 := countPricesBelowMa60(candles, defboxPos, position)
		isValidUpwardMomentum := evaluateUpwardMomentum(candles, oscRes.firstPos, lastMa20CrossPos, crossMVResult)
		isNoGradientReversal := evaluateGradientReversal(candles, oscResFromDefbox.firstPos, position)
		isConsistentUpTrend := evaluateConsistentUpTrend(candles, oscResFromDefbox.firstPos, oscResFromDefbox, defboxPos, position, crossMVResult)

		if isPiercingLimitBreakthrough &&
			!isValidUpwardMomentum &&
			countBelowMa60 == 0 &&
			isNoGradientReversal &&
			isConsistentUpTrend {
			conclusion = false
		}
	}

	// Step 3
	if conclusion {
		lastBoxIdx := len(ctx.BoxList) - 1
		if lastBoxIdx > ctx.DefboxIndex {
			lastBox := ctx.BoxList[lastBoxIdx]
			if lastBox.BoxType == box.BoxTypeSupport &&
				lastBox.BoxPosition >= 0 &&
				lastBox.BoxPosition < len(candles) {
				c := candles[lastBox.BoxPosition]
				if c.Close < c.Ma60 {
					gradCheck := firstGradientChecker(candles, lastBox.CurvePosition-1, position)
					if gradCheck == 0 {
						conclusion = false
					}
				}
			}
		}
	}

	return conclusion
}

// ============================================================================
// 공용 오실로 헬퍼 (buy/sell 양쪽에서 사용 — C# OscillatorEvaluator 포팅)
// ============================================================================

// HighPriceInRange 는 [start, end] 구간의 최고가(High)를 반환한다.
// C# OscillatorEvaluator.HighPrice 포팅.
func HighPriceInRange(candles []*box.Candle, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end >= len(candles) {
		end = len(candles) - 1
	}
	high := 0.0
	for i := start; i <= end; i++ {
		if candles[i].High > high {
			high = candles[i].High
		}
	}
	return high
}

// LastOscilloPositionByPrice 는 boxprice 기준 [start, end] 구간의 마지막 상/하향 돌파 위치를 반환한다.
// C# OscillatorEvaluator.LastOscilloPosition(boxprice 버전) 포팅.
// 돌파가 없으면 0 반환.
func LastOscilloPositionByPrice(candles []*box.Candle, boxprice float64, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end >= len(candles) {
		end = len(candles) - 1
	}
	tempkey := 0
	lastBreak := 0
	for i := start; i <= end; i++ {
		c := candles[i]
		if tempkey == -1 && (c.Close > boxprice || c.Open > boxprice) {
			lastBreak = i
		}
		if tempkey == 1 && (c.Close < boxprice || c.Open < boxprice) {
			lastBreak = i
		}
		if c.Close < boxprice {
			tempkey = -1
		}
		if c.Close > boxprice {
			tempkey = 1
		}
	}
	return lastBreak
}
