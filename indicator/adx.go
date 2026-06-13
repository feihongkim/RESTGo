package indicator

import (
	"RESTGo/box"
	"math"
)

func CalculateADX(candles []*box.Candle, period int) {
	n := len(candles)
	if n < period*2+1 {
		return
	}

	tr := make([]float64, n)
	plusDM := make([]float64, n)
	minusDM := make([]float64, n)
	for i := 1; i < n; i++ {
		hl := candles[i].High - candles[i].Low
		hc := math.Abs(candles[i].High - candles[i-1].Close)
		lc := math.Abs(candles[i].Low - candles[i-1].Close)
		tr[i] = math.Max(hl, math.Max(hc, lc))

		upMove := candles[i].High - candles[i-1].High
		downMove := candles[i-1].Low - candles[i].Low
		if upMove > downMove && upMove > 0 {
			plusDM[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM[i] = downMove
		}
	}

	var trSmooth, plusDMSmooth, minusDMSmooth float64
	for i := 1; i <= period; i++ {
		trSmooth += tr[i]
		plusDMSmooth += plusDM[i]
		minusDMSmooth += minusDM[i]
	}

	var prevADX float64
	for i := period; i < n; i++ {
		if i > period {
			trSmooth = trSmooth - trSmooth/float64(period) + tr[i]
			plusDMSmooth = plusDMSmooth - plusDMSmooth/float64(period) + plusDM[i]
			minusDMSmooth = minusDMSmooth - minusDMSmooth/float64(period) + minusDM[i]
		}
		if trSmooth == 0 {
			continue
		}
		pdi := 100 * plusDMSmooth / trSmooth
		mdi := 100 * minusDMSmooth / trSmooth
		candles[i].PlusDI = pdi
		candles[i].MinusDI = mdi

		dxDenom := pdi + mdi
		if dxDenom == 0 {
			continue
		}
		dx := 100 * math.Abs(pdi-mdi) / dxDenom

		if i == period {
			prevADX = dx
		} else {
			prevADX = (prevADX*float64(period-1) + dx) / float64(period)
		}
		candles[i].ADX = prevADX
	}
}
