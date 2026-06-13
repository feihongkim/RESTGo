package indicator

import "RESTGo/box"

func CalculateSuperTrend(candles []*box.Candle, period int, multiplier float64) {
	n := len(candles)
	if n < period+1 {
		return
	}

	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)

	// ATR은 외부에서 별도 period(=14)로 계산되므로 period(=10) 시점에 ATR이 0일 수 있다.
	// 첫 번째 ATR 유효 인덱스를 초기화 기준으로 삼는다.
	start := period
	for start < n && candles[start].ATR == 0 {
		start++
	}
	if start >= n {
		return
	}

	mid := (candles[start].HighOrigin + candles[start].LowOrigin) / 2.0
	upperBand[start] = mid + multiplier*candles[start].ATR
	lowerBand[start] = mid - multiplier*candles[start].ATR
	candles[start].SuperTrend = upperBand[start]
	candles[start].SuperTrendDir = -1

	for i := start + 1; i < n; i++ {
		if candles[i].ATR == 0 {
			candles[i].SuperTrendDir = candles[i-1].SuperTrendDir
			candles[i].SuperTrend = candles[i-1].SuperTrend
			upperBand[i] = upperBand[i-1]
			lowerBand[i] = lowerBand[i-1]
			continue
		}
		mid := (candles[i].HighOrigin + candles[i].LowOrigin) / 2.0
		atr := candles[i].ATR

		basicUpper := mid + multiplier*atr
		basicLower := mid - multiplier*atr

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
