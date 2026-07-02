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

	// 2+3. 이동평균(5/20/60/120) + MA 기울기 — 단일 패스 (rolling sum O(N))
	// 4개 period의 rolling sum을 동시에 굴려 기존 4패스를 1패스로 통합한다.
	// 원본가 MA = 스케일 MA × maxP (스케일링이 상수배이므로 동일).
	//   IsVolumeBreakout(거래대금 게이트) 등 원본가 기반 계산에 필요.
	// Gradient: ((MA[i] - MA[i-1]) / MA[i]) * 100 을 같은 패스에서 계산한다.
	//   MA[i-1]은 직전 iteration에서 채워진 값을 사용하므로 별도 패스가 불필요하다.
	//   C# CalculateGradientMethod 정합: 양쪽 MA가 모두 유효(≠0)일 때만 계산
	//   (Ma[i-1] 가드가 없으면 첫 유효 MA 위치에서 Gradient가 100으로 튄다).
	var sum5, sum20, sum60, sum120, sum200 float64
	for i := range candles {
		c := candles[i]
		sum5 += c.Close
		if i >= 5 {
			sum5 -= candles[i-5].Close
		}
		sum20 += c.Close
		if i >= 20 {
			sum20 -= candles[i-20].Close
		}
		sum60 += c.Close
		if i >= 60 {
			sum60 -= candles[i-60].Close
		}
		sum120 += c.Close
		if i >= 120 {
			sum120 -= candles[i-120].Close
		}
		sum200 += c.Close
		if i >= 200 {
			sum200 -= candles[i-200].Close
		}

		if i >= 4 {
			c.Ma5 = sum5 / 5.0
			c.Ma5Origin = c.Ma5 * maxP
		}
		if i >= 19 {
			c.Ma20 = sum20 / 20.0
			c.Ma20Origin = c.Ma20 * maxP
		}
		if i >= 59 {
			c.Ma60 = sum60 / 60.0
			c.Ma60Origin = c.Ma60 * maxP
		}
		if i >= 119 {
			c.Ma120 = sum120 / 120.0
			c.Ma120Origin = c.Ma120 * maxP
		}
		if i >= 199 {
			c.Ma200 = sum200 / 200.0
			c.Ma200Origin = c.Ma200 * maxP
		}

		if i >= 1 {
			p := candles[i-1]
			if c.Ma5 != 0 && p.Ma5 != 0 {
				c.Gradient = ((c.Ma5 - p.Ma5) / c.Ma5) * 100.0
			}
			if c.Ma20 != 0 && p.Ma20 != 0 {
				c.Gradient20 = ((c.Ma20 - p.Ma20) / c.Ma20) * 100.0
			}
			if c.Ma60 != 0 && p.Ma60 != 0 {
				c.Gradient60 = ((c.Ma60 - p.Ma60) / c.Ma60) * 100.0
			}
			if c.Ma120 != 0 && p.Ma120 != 0 {
				c.Gradient120 = ((c.Ma120 - p.Ma120) / c.Ma120) * 100.0
			}
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

	// 8. EMA 9/21/50 (원본가 기준)
	CalculateEMA(candles)

	// 9. MACD (12, 26, 9 — 스케일 종가)
	CalculateMACD(candles, 12, 26, 9)

	// 10. Stochastic (14, 3, 3 — 스케일 OHLC)
	CalculateStochastic(candles, 14, 3, 3)

	// 11. ADX/DMI (14 — 스케일 OHLC)
	CalculateADX(candles, 14)

	// 12. VWAP (원본가, 날짜 리셋)
	CalculateVWAP(candles)

	// 13. SuperTrend (10, 3.0 — 원본가 + ATR)
	CalculateSuperTrend(candles, 10, 3.0)

	// 14. Donchian Channel (30 — 원본가) — 2026-06-16 사용자 요청 20→40→30 절충
	CalculateDonchian(candles, 30)

	// 15. Keltner Channel (20, 1.5 — 원본가 + ATR)
	CalculateKeltner(candles, 20, 1.5)

	// 16. OBV (스케일 종가 방향)
	CalculateOBV(candles)
}
