package cond

import "RESTGo/box"

// IsEarlyDrop 은 매수 후 daysAfterBuy 이내 손실률이 임계값(thresholdPct, 음수 %) 이하로 떨어졌는지 확인한다.
// C# EarlyWarningEvaluator.IsEarlyDrop 포팅.
func IsEarlyDrop(ctx *box.TradingContext, buyPosition int, buyPrice float64, daysAfterBuy int, thresholdPct float64) bool {
	delta := ctx.Position - buyPosition
	if delta > daysAfterBuy {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	return CalculateReturnPercentage(cur.Close, buyPrice) <= thresholdPct
}

// IsEarlyMainBoxBreak 은 매수 후 daysAfterBuy 이내 종가가 MainBox 아래 + MA5<MA20 역배열 발생 시 true.
// C# EarlyWarningEvaluator.IsEarlyMainBoxBreak 포팅.
func IsEarlyMainBoxBreak(ctx *box.TradingContext, buyPosition int, mainBoxPrice float64, daysAfterBuy int) bool {
	delta := ctx.Position - buyPosition
	if delta > daysAfterBuy {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	return cur.Close < mainBoxPrice && IsPartialReversal(cur)
}

// IsBBSqueezeExpansionWarning 은 BB Squeeze → Expansion 하단 브레이크아웃 조기 경고를 평가한다.
// C# EarlyWarningEvaluator.IsBBSqueezeExpansionWarning 포팅.
//
// 조건:
//  1. 최근 lookback 구간 내 BB Squeeze 존재 (squeezeWidthThreshold 이하)
//  2. 현재 BBWidth >= squeezeWidth × widthIncreaseRatio (Expansion 확인)
//  3. 현재 %B < bbPercentThreshold (하단 브레이크아웃)
//  4. 최근 requiredCandles 캔들 모두 %B < bbPercentThreshold (지속 확인)
func IsBBSqueezeExpansionWarning(
	ctx *box.TradingContext,
	lookback int,
	squeezeWidthThreshold float64,
	widthIncreaseRatio float64,
	bbPercentThreshold float64,
	requiredCandles int,
) bool {
	pos := ctx.Position
	candles := ctx.CandleList
	if pos < requiredCandles {
		return false
	}

	// 1+2. 최근 lookback 구간에서 Squeeze 발생 후 현재 Expansion인지
	squeezeWidth, hasExpansion := detectBBExpansion(candles, pos, lookback, squeezeWidthThreshold, widthIncreaseRatio)
	if !hasExpansion {
		return false
	}
	_ = squeezeWidth

	// 3. 하단 브레이크아웃 (%B 임계 이하)
	if candles[pos].BBPercent >= bbPercentThreshold {
		return false
	}

	// 4. requiredCandles 연속 하단 브레이크아웃
	confirmed := 0
	for i := pos - requiredCandles + 1; i <= pos; i++ {
		if candles[i].BBPercent < bbPercentThreshold {
			confirmed++
		}
	}
	return confirmed >= requiredCandles
}

// detectBBExpansion 은 [pos-lookback, pos] 구간에서 가장 작은 BBWidth(squeezeWidth)를 찾고
// 현재 BBWidth가 그것의 widthIncreaseRatio 배 이상이면 expansion으로 판정한다.
// C# BBVolatilityEvaluator.IsBBExpansion 포팅 (간략화).
func detectBBExpansion(candles []*box.Candle, pos, lookback int, squeezeWidthThreshold, widthIncreaseRatio float64) (float64, bool) {
	start := pos - lookback
	if start < 0 {
		start = 0
	}
	minWidth := -1.0
	for i := start; i < pos; i++ {
		w := candles[i].BollingerWidth
		if w == 0 {
			continue
		}
		if w < squeezeWidthThreshold && (minWidth < 0 || w < minWidth) {
			minWidth = w
		}
	}
	if minWidth < 0 {
		return 0, false
	}
	curWidth := candles[pos].BollingerWidth
	if curWidth >= minWidth*widthIncreaseRatio {
		return minWidth, true
	}
	return minWidth, false
}
