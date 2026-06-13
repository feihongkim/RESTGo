package indicator

import "RESTGo/box"

func CalculateStochastic(candles []*box.Candle, k, d, smooth int) {
	n := len(candles)
	if n < k+smooth+d-2 {
		return
	}

	rawK := make([]float64, n)
	for i := k - 1; i < n; i++ {
		lo := candles[i].Low
		hi := candles[i].High
		for j := i - k + 1; j <= i; j++ {
			if candles[j].Low < lo {
				lo = candles[j].Low
			}
			if candles[j].High > hi {
				hi = candles[j].High
			}
		}
		rng := hi - lo
		if rng > 0 {
			rawK[i] = (candles[i].Close - lo) / rng * 100
		} else {
			rawK[i] = 50
		}
	}

	smoothK := make([]float64, n)
	for i := k - 1 + smooth - 1; i < n; i++ {
		var sum float64
		for j := i - smooth + 1; j <= i; j++ {
			sum += rawK[j]
		}
		smoothK[i] = sum / float64(smooth)
	}

	start := k - 1 + smooth - 1 + d - 1
	for i := start; i < n; i++ {
		var sum float64
		for j := i - d + 1; j <= i; j++ {
			sum += smoothK[j]
		}
		candles[i].StochK = smoothK[i]
		candles[i].StochD = sum / float64(d)
	}
}
