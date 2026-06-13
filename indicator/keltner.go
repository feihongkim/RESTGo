package indicator

import "RESTGo/box"

func CalculateKeltner(candles []*box.Candle, period int, atrMult float64) {
	n := len(candles)
	if n < period {
		return
	}
	k := 2.0 / float64(period+1)

	var sum float64
	for i := 0; i < period; i++ {
		sum += candles[i].CloseOrigin
	}
	ema := sum / float64(period)

	for i := period - 1; i < n; i++ {
		if i > period-1 {
			ema = candles[i].CloseOrigin*k + ema*(1-k)
		}
		if candles[i].ATR > 0 {
			candles[i].KeltnerUpper = ema + atrMult*candles[i].ATR
			candles[i].KeltnerLower = ema - atrMult*candles[i].ATR
		}
	}
}
