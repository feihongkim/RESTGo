package indicator

import "RESTGo/box"

func CalculateOBV(candles []*box.Candle) {
	n := len(candles)
	if n == 0 {
		return
	}
	candles[0].OBV = candles[0].Volume
	for i := 1; i < n; i++ {
		if candles[i].Close > candles[i-1].Close {
			candles[i].OBV = candles[i-1].OBV + candles[i].Volume
		} else if candles[i].Close < candles[i-1].Close {
			candles[i].OBV = candles[i-1].OBV - candles[i].Volume
		} else {
			candles[i].OBV = candles[i-1].OBV
		}
	}
}
