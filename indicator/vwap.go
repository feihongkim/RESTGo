package indicator

import (
	"RESTGo/box"
	"math"
)

func CalculateVWAP(candles []*box.Candle) {
	n := len(candles)
	if n == 0 {
		return
	}

	var cumTPV, cumVol float64
	var sumSqDev float64
	var sessionCount int

	for i := 0; i < n; i++ {
		c := candles[i]
		if i > 0 && c.Date != candles[i-1].Date {
			cumTPV = 0
			cumVol = 0
			sumSqDev = 0
			sessionCount = 0
		}

		tp := (c.HighOrigin + c.LowOrigin + c.CloseOrigin) / 3.0
		vol := c.Volume
		if vol <= 0 {
			vol = 1
		}
		cumTPV += tp * vol
		cumVol += vol
		sessionCount++

		vwap := cumTPV / cumVol
		c.VWAP = vwap

		dev := c.CloseOrigin - vwap
		sumSqDev += dev * dev
		if sessionCount > 1 {
			c.VWAPStdDev = math.Sqrt(sumSqDev / float64(sessionCount))
		}
	}
}
