package indicator

import (
	"RESTGo/box"
	"math"
)

// CalculateBollinger 는 Bollinger Bands(period=20, multiplier=2σ)를 계산하여
// Candle.BollingerUpper / BollingerLower / BollingerWidth에 저장합니다.
// 스케일된 종가(Close) 및 Ma20을 기준으로 합니다.
func CalculateBollinger(candles []*box.Candle, period int, multiplier float64) {
	// rolling sum of squares — O(N)
	// variance = E[x²] - E[x]²  (E[x] = Ma20, 이미 계산됨)
	var rollingSumSq float64
	for i := range candles {
		c := candles[i].Close
		rollingSumSq += c * c
		if i >= period {
			old := candles[i-period].Close
			rollingSumSq -= old * old
		}
		if i < period-1 {
			continue
		}
		mid := candles[i].Ma20
		if mid == 0 {
			continue
		}
		variance := rollingSumSq/float64(period) - mid*mid
		if variance < 0 {
			variance = 0
		}
		stdDev := math.Sqrt(variance)
		upper := mid + multiplier*stdDev
		lower := mid - multiplier*stdDev
		candles[i].BollingerUpper = upper
		candles[i].BollingerLower = lower
		candles[i].BollingerWidth = (upper - lower) / mid * 100
		// %B = (Close - Lower) / (Upper - Lower)
		bandRange := upper - lower
		if bandRange > 0 {
			candles[i].BBPercent = (candles[i].Close - lower) / bandRange
		}
	}
}
