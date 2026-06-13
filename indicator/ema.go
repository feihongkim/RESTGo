package indicator

import "RESTGo/box"

func CalculateEMA(candles []*box.Candle) {
	for _, period := range []int{9, 21, 50} {
		calculateEMAForPeriod(candles, period)
	}
}

func calculateEMAForPeriod(candles []*box.Candle, period int) {
	if len(candles) < period {
		return
	}
	k := 2.0 / float64(period+1)

	var sum float64
	for i := 0; i < period; i++ {
		sum += candles[i].CloseOrigin
	}
	ema := sum / float64(period)
	setEMA(candles[period-1], period, ema)

	for i := period; i < len(candles); i++ {
		ema = candles[i].CloseOrigin*k + ema*(1-k)
		setEMA(candles[i], period, ema)
	}
}

func setEMA(c *box.Candle, period int, val float64) {
	switch period {
	case 9:
		c.EMA9 = val
	case 21:
		c.EMA21 = val
	case 50:
		c.EMA50 = val
	}
}
