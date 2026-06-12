package cond

import (
	"RESTGo/box"
	"math"
)

// IsDefBoxBreakout 는 DefBox 돌파 여부 확인
func IsDefBoxBreakout(ctx *box.TradingContext) bool {
	if ctx.Position <= 0 {
		return false
	}
	cur := ctx.GetCurrentCandle()
	prev := ctx.GetPreviousCandle(1)
	if cur == nil || prev == nil {
		return false
	}
	return prev.Close < ctx.DefboxPrice &&
		(cur.Close >= ctx.DefboxPrice || cur.Open > ctx.DefboxPrice)
}

// IsCloseNearDefboxPrice 는 현재가가 DefBox/MainBox 가격 근처인지 확인
func IsCloseNearDefboxPrice(ctx *box.TradingContext, threshold, mainThreshold float64) bool {
	cur := ctx.GetCurrentCandle()
	if cur == nil {
		return false
	}
	isNearDefBox := (cur.Close - ctx.DefboxPrice) < (threshold * ctx.DefboxPrice)

	mainBox := ctx.GetMainBox()
	mainBoxPrice := 0.0
	if mainBox != nil {
		mainBoxPrice = mainBox.Price
	}
	isNearMainBox := mainBoxPrice == 0 || (cur.Close-mainBoxPrice) <= (mainThreshold*mainBoxPrice)

	return isNearDefBox && isNearMainBox
}

// IsMainboxDistanceTwiceOrMore 는 MainBox 거리가 현재 위치의 2배 이상인지 확인
func IsMainboxDistanceTwiceOrMore(ctx *box.TradingContext) bool {
	return 2*(ctx.DefboxPosition-ctx.MainboxPosition) >= (ctx.Position - ctx.DefboxPosition)
}

// IsBoxDensityValidByCount 는 DefBox와 MainBox 사이의 Box 개수가 4개 이하인지 확인
func IsBoxDensityValidByCount(ctx *box.TradingContext) bool {
	defBox := ctx.GetDefBox()
	if defBox == nil || len(defBox.MainDefLink) == 0 {
		return false
	}
	mainBoxIdx := defBox.MainDefLink[0]
	return (ctx.DefboxIndex - mainBoxIdx) <= 4
}

// IsBoxDensityValidByDistribution 는 고점 분포를 분석하여 과밀 저항 여부 확인
func IsBoxDensityValidByDistribution(ctx *box.TradingContext, density int) bool {
	densityPer := float64(100-density) / 2 * 0.01
	interMainDef := ctx.DefboxPosition - ctx.MainboxPosition
	interDefPos := ctx.Position - ctx.DefboxPosition

	tempDensityChecker := 0
	tempDensityCount := 0

	for i := ctx.MainboxPosition + 1; i < ctx.Position; i++ {
		if ctx.CandleList[i].High >= ctx.DefboxPrice {
			tempDensityCount++
			mainLow := float64(ctx.MainboxPosition) + densityPer*float64(interMainDef)
			mainHigh := float64(ctx.MainboxPosition) + (1.0-densityPer)*float64(interMainDef)
			defLow := float64(ctx.DefboxPosition) + densityPer*float64(interDefPos)
			defHigh := float64(ctx.DefboxPosition) + (1.0-densityPer)*float64(interDefPos)

			fi := float64(i)
			if (fi >= mainLow && fi <= mainHigh) || (fi >= defLow && fi <= defHigh) {
				tempDensityChecker++
			}
		}
	}

	return !((tempDensityCount >= 3) && (tempDensityChecker > 0))
}

// IsSingleBreakout 는 MainBox 이전에 DefBox 가격 돌파 이력이 1회 이하인지 확인
func IsSingleBreakout(ctx *box.TradingContext) bool {
	interMainDef := ctx.DefboxPosition - ctx.MainboxPosition
	exposition := ctx.MainboxPosition - (3 * interMainDef) - 1

	tempChecker := 0
	start := 1
	if exposition >= 0 {
		start = exposition + 1
	}
	for i := start; i < ctx.MainboxPosition; i++ {
		if ctx.CandleList[i].Close > ctx.DefboxPrice {
			tempChecker++
		}
	}
	return tempChecker <= 1
}

// HasExcessiveUpperWick 는 DefBox 또는 MainBox 위치의 캔들 윗꼬리가 과도한지 확인
// C# 원본(BoxConditionEvaluator.HasExcessiveUpperWickAtDefOrMainBox)은 DefBox 쪽에
// BoxList 인덱스(DefboxIndex)로 CandleList를 조회하는 버그가 있어, 여기서는 의도대로
// 캔들 위치(DefboxPosition)를 사용한다 — C#과 결과가 다를 수 있음
func HasExcessiveUpperWick(ctx *box.TradingContext, wickToBodyRatio float64) bool {
	return checkUpperWick(ctx, ctx.DefboxPosition, wickToBodyRatio) ||
		checkUpperWick(ctx, ctx.MainboxPosition, wickToBodyRatio)
}

func checkUpperWick(ctx *box.TradingContext, position int, wickToBodyRatio float64) bool {
	if position < 0 || position >= len(ctx.CandleList) {
		return false
	}
	c := ctx.CandleList[position]
	bodySize := math.Abs(c.Close - c.Open)
	if bodySize < 0.0001 {
		return true
	}
	upperWick := c.High - math.Max(c.Open, c.Close)
	return upperWick > wickToBodyRatio*bodySize
}

// IsBoxConditionValid2 는 MainBox 이전에 "양봉 후 음봉" 패턴이 없는지 확인
func IsBoxConditionValid2(ctx *box.TradingContext) bool {
	if ctx.MainboxPosition <= 0 {
		return true
	}
	count := ctx.DefboxPosition - ctx.MainboxPosition
	exposition := ctx.MainboxPosition - (3 * count) - 1

	tempChecker := 0
	start := 1
	if exposition >= 0 {
		start = exposition + 1
	}
	for i := start; i < ctx.MainboxPosition; i++ {
		c := ctx.CandleList[i]
		prev := ctx.CandleList[i-1]
		if c.Close > ctx.DefboxPrice && (c.Close-c.Open) > 0.0 &&
			prev.Close > ctx.DefboxPrice && (prev.Close-prev.Open) < 0.0 {
			tempChecker++
		}
	}
	return tempChecker < 1
}

// IsAdditionalBoxConditionValid 는 BoxConditionEvaluator.IsAdditionalBoxConditionValid 포팅
func IsAdditionalBoxConditionValid(ctx *box.TradingContext) bool {
	defBox := ctx.GetDefBox()
	if defBox == nil {
		return true
	}
	interMainPos := ctx.Position - ctx.MainboxPosition
	if len(defBox.MainDefLink) != 1 || (ctx.MainboxPosition-interMainPos) <= 0 {
		return true
	}
	matchedBoxCount := 0
	lastBoxPositionInRange := 0
	for i := 0; i < defBox.MainDefLink[0]; i++ {
		b := ctx.BoxList[i]
		if b.BoxPosition > (ctx.MainboxPosition-interMainPos) &&
			b.Price > ctx.DefboxPrice &&
			b.BoxType == box.BoxTypeResistance &&
			(b.BoxPosition-lastBoxPositionInRange) > 2 {
			lastBoxPositionInRange = b.BoxPosition
			matchedBoxCount++
		}
	}
	return matchedBoxCount < 2
}
