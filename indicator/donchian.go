package indicator

import "RESTGo/box"

func CalculateDonchian(candles []*box.Candle, period int) {
	n := len(candles)
	if n < period {
		return
	}
	for i := period - 1; i < n; i++ {
		hi := candles[i-period+1].HighOrigin
		lo := candles[i-period+1].LowOrigin
		for j := i - period + 2; j <= i; j++ {
			if candles[j].HighOrigin > hi {
				hi = candles[j].HighOrigin
			}
			if candles[j].LowOrigin < lo {
				lo = candles[j].LowOrigin
			}
		}
		candles[i].DonchianUpper = hi
		candles[i].DonchianLower = lo
	}
}
