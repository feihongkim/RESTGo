package indicator

import "RESTGo/box"

func CalculateSuperTrend(candles []*box.Candle, period int, multiplier float64) {
	n := len(candles)
	if n < period+1 {
		return
	}

	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)

	for i := period; i < n; i++ {
		if candles[i].ATR == 0 {
			continue
		}
		mid := (candles[i].HighOrigin + candles[i].LowOrigin) / 2.0
		atr := candles[i].ATR

		basicUpper := mid + multiplier*atr
		basicLower := mid - multiplier*atr

		if i == period {
			upperBand[i] = basicUpper
			lowerBand[i] = basicLower
			candles[i].SuperTrend = upperBand[i]
			candles[i].SuperTrendDir = -1
			continue
		}

		if basicUpper < upperBand[i-1] || candles[i-1].CloseOrigin > upperBand[i-1] {
			upperBand[i] = basicUpper
		} else {
			upperBand[i] = upperBand[i-1]
		}
		if basicLower > lowerBand[i-1] || candles[i-1].CloseOrigin < lowerBand[i-1] {
			lowerBand[i] = basicLower
		} else {
			lowerBand[i] = lowerBand[i-1]
		}

		prevDir := candles[i-1].SuperTrendDir
		if prevDir == -1 && candles[i].CloseOrigin > upperBand[i-1] {
			candles[i].SuperTrendDir = 1
			candles[i].SuperTrend = lowerBand[i]
		} else if prevDir == 1 && candles[i].CloseOrigin < lowerBand[i-1] {
			candles[i].SuperTrendDir = -1
			candles[i].SuperTrend = upperBand[i]
		} else {
			candles[i].SuperTrendDir = prevDir
			if prevDir == 1 {
				candles[i].SuperTrend = lowerBand[i]
			} else {
				candles[i].SuperTrend = upperBand[i]
			}
		}
	}
}
