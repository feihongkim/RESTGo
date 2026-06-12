package indicator

import (
	"RESTGo/box"
	"math"
)

// FindMaxPrice 는 캔들 리스트에서 OHLC 원본 기준 최대값 반환 (스케일링 기준)
func FindMaxPrice(candles []*box.Candle) float64 {
	max := 0.0
	for _, c := range candles {
		for _, v := range []float64{c.OpenOrigin, c.CloseOrigin, c.HighOrigin, c.LowOrigin} {
			if v > max {
				max = v
			}
		}
	}
	return max
}

// PrepareCandles 는 DB에서 가져온 원본 캔들에 스케일링/MA/기울기/ATR을 적용합니다.
// box.Analyze() 호출 전에 반드시 실행해야 합니다.
func PrepareCandles(candles []*box.Candle) {
	if len(candles) == 0 {
		return
	}

	maxP := FindMaxPrice(candles)
	if maxP == 0 {
		return
	}

	// 1. 가격 스케일링
	for _, c := range candles {
		c.Open = c.OpenOrigin / maxP
		c.Close = c.CloseOrigin / maxP
		c.High = c.HighOrigin / maxP
		c.Low = c.LowOrigin / maxP
	}

	// 2. 이동평균 (스케일된 종가 기준) — rolling sum O(N)
	for _, period := range []int{5, 20, 60, 120} {
		var rollingSum float64
		for i := range candles {
			rollingSum += candles[i].Close
			if i >= period {
				rollingSum -= candles[i-period].Close
			}
			if i < period-1 {
				continue
			}
			ma := rollingSum / float64(period)
			// 원본가 MA = 스케일 MA × maxP (스케일링이 상수배이므로 동일)
			// IsVolumeBreakout(거래대금 게이트) 등 원본가 기반 계산에 필요
			maOrigin := ma * maxP
			switch period {
			case 5:
				candles[i].Ma5 = ma
				candles[i].Ma5Origin = maOrigin
			case 20:
				candles[i].Ma20 = ma
				candles[i].Ma20Origin = maOrigin
			case 60:
				candles[i].Ma60 = ma
				candles[i].Ma60Origin = maOrigin
			case 120:
				candles[i].Ma120 = ma
				candles[i].Ma120Origin = maOrigin
			}
		}
	}

	// 3. MA 기울기: ((MA[i] - MA[i-1]) / MA[i]) * 100
	// C# CalculateGradientMethod는 i >= indexOfMa부터 계산하므로 양쪽 MA가 모두
	// 유효(≠0)해야 한다. Ma[i-1] 가드가 없으면 첫 유효 MA 위치(i=period-1)에서
	// Gradient가 100으로 튀어 C#과 결과가 달라진다.
	for i := 1; i < len(candles); i++ {
		if candles[i].Ma5 != 0 && candles[i-1].Ma5 != 0 {
			candles[i].Gradient = ((candles[i].Ma5 - candles[i-1].Ma5) / candles[i].Ma5) * 100.0
		}
		if candles[i].Ma20 != 0 && candles[i-1].Ma20 != 0 {
			candles[i].Gradient20 = ((candles[i].Ma20 - candles[i-1].Ma20) / candles[i].Ma20) * 100.0
		}
		if candles[i].Ma60 != 0 && candles[i-1].Ma60 != 0 {
			candles[i].Gradient60 = ((candles[i].Ma60 - candles[i-1].Ma60) / candles[i].Ma60) * 100.0
		}
		if candles[i].Ma120 != 0 && candles[i-1].Ma120 != 0 {
			candles[i].Gradient120 = ((candles[i].Ma120 - candles[i-1].Ma120) / candles[i].Ma120) * 100.0
		}
	}

	// 4. 거래량 이동평균 — rolling sum O(N)
	var volSum5, volSum20 float64
	for i := range candles {
		volSum5 += candles[i].Volume
		if i >= 5 {
			volSum5 -= candles[i-5].Volume
		}
		if i >= 4 {
			candles[i].VolMa5 = volSum5 / 5.0
		}
		volSum20 += candles[i].Volume
		if i >= 20 {
			volSum20 -= candles[i-20].Volume
		}
		if i >= 19 {
			candles[i].VolMa20 = volSum20 / 20.0
		}
	}

	// 5. RSI (스케일 종가 기준, Wilder period=14)
	CalculateRSI(candles, 14)

	// 6. Bollinger Bands (스케일 종가·Ma20 기준, period=20, 2σ)
	CalculateBollinger(candles, 20, 2.0)

	// 7. ATR (원본 가격 기준) — rolling sum O(N), SMA 방식 동일하게 유지
	atrPeriod := 14
	if len(candles) > atrPeriod {
		tr := make([]float64, len(candles))
		for j := 1; j < len(candles); j++ {
			hl := candles[j].HighOrigin - candles[j].LowOrigin
			hc := math.Abs(candles[j].HighOrigin - candles[j-1].CloseOrigin)
			lc := math.Abs(candles[j].LowOrigin - candles[j-1].CloseOrigin)
			tr[j] = math.Max(hl, math.Max(hc, lc))
		}
		var trSum float64
		for j := 1; j <= atrPeriod; j++ {
			trSum += tr[j]
		}
		candles[atrPeriod].ATR = trSum / float64(atrPeriod)
		if candles[atrPeriod].CloseOrigin > 0 {
			candles[atrPeriod].ATRPercentage = candles[atrPeriod].ATR / candles[atrPeriod].CloseOrigin
		}
		for i := atrPeriod + 1; i < len(candles); i++ {
			trSum += tr[i] - tr[i-atrPeriod]
			atr := trSum / float64(atrPeriod)
			candles[i].ATR = atr
			if candles[i].CloseOrigin > 0 {
				candles[i].ATRPercentage = atr / candles[i].CloseOrigin
			}
		}
	}
}
