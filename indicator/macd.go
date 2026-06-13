package indicator

import "RESTGo/box"

func CalculateMACD(candles []*box.Candle, fast, slow, signal int) {
	if len(candles) < slow+signal {
		return
	}

	fastEMA := computeEMASlice(candles, fast, func(c *box.Candle) float64 { return c.Close })
	slowEMA := computeEMASlice(candles, slow, func(c *box.Candle) float64 { return c.Close })

	macdLine := make([]float64, len(candles))
	for i := slow - 1; i < len(candles); i++ {
		macdLine[i] = fastEMA[i] - slowEMA[i]
	}

	k := 2.0 / float64(signal+1)
	sigStart := slow - 1 + signal - 1
	if sigStart >= len(candles) {
		return
	}

	var seedSum float64
	for i := slow - 1; i < slow-1+signal; i++ {
		if i >= len(candles) {
			return
		}
		seedSum += macdLine[i]
	}
	sigEMA := seedSum / float64(signal)
	candles[sigStart].MACDSignal = sigEMA
	candles[sigStart].MACD = macdLine[sigStart]
	candles[sigStart].MACDHist = macdLine[sigStart] - sigEMA

	for i := sigStart + 1; i < len(candles); i++ {
		sigEMA = macdLine[i]*k + sigEMA*(1-k)
		candles[i].MACD = macdLine[i]
		candles[i].MACDSignal = sigEMA
		candles[i].MACDHist = macdLine[i] - sigEMA
	}
}

func computeEMASlice(candles []*box.Candle, period int, val func(*box.Candle) float64) []float64 {
	result := make([]float64, len(candles))
	if len(candles) < period {
		return result
	}
	k := 2.0 / float64(period+1)
	var sum float64
	for i := 0; i < period; i++ {
		sum += val(candles[i])
	}
	ema := sum / float64(period)
	result[period-1] = ema
	for i := period; i < len(candles); i++ {
		ema = val(candles[i])*k + ema*(1-k)
		result[i] = ema
	}
	return result
}
