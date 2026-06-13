package cond

import (
	"math"

	"RESTGo/box"
)

// 15분봉 단타용 P0 매수 조건 모음.
// W5 indicator 패키지가 채운 신규 필드(MACD/Stoch/ADX/VWAP/SuperTrend/Donchian/Keltner/OBV/EMA)를 사용한다.
//
// 워밍업 가드 기준:
//   - MACD: MACDSignal != 0 (26+9-1 봉 필요)
//   - Stochastic: StochK != 0 (14+3+3-2 봉 필요)
//   - ADX: ADX != 0 (period*2+1 봉 필요)
//   - VWAP: VWAP != 0 (첫 봉부터 유효)
//   - SuperTrend: SuperTrend != 0 (period+1 봉 필요)
//   - Donchian: DonchianUpper != 0 (period 봉 필요)
//   - Keltner: KeltnerUpper != 0
//   - OBV: 항상 유효 (첫 봉부터)
//   - EMA9/21/50: EMA50 != 0 (50봉 필요)

// ============================================================================
// 모멘텀·추세 강도 조건
// ============================================================================

// IsMACDGoldenCross 는 MACD선이 시그널선을 상향 돌파했는지 확인한다.
func IsMACDGoldenCross(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	if cur.MACDSignal == 0 || prev.MACDSignal == 0 {
		return false
	}
	return prev.MACD <= prev.MACDSignal && cur.MACD > cur.MACDSignal
}

// IsMACDHistogramRising 는 MACD 히스토그램이 N봉 연속 증가 중인지 확인한다.
func IsMACDHistogramRising(ctx *box.TradingContext, n int) bool {
	pos := ctx.Position
	if pos < n {
		return false
	}
	if ctx.CandleList[pos].MACDSignal == 0 {
		return false
	}
	for i := pos - n + 1; i <= pos; i++ {
		if i == 0 {
			continue
		}
		if ctx.CandleList[i].MACDHist <= ctx.CandleList[i-1].MACDHist {
			return false
		}
	}
	return true
}

// IsStochGoldenCross 는 %K가 %D를 상향 돌파 + 과매도권 아래에서 발생했는지 확인한다.
func IsStochGoldenCross(ctx *box.TradingContext, oversoldThreshold float64) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	if cur.StochK == 0 || prev.StochK == 0 {
		return false
	}
	return prev.StochK <= prev.StochD && cur.StochK > cur.StochD && prev.StochK < oversoldThreshold
}

// IsADXTrending 는 ADX가 임계값 이상인지 확인한다 (추세장 게이트).
func IsADXTrending(ctx *box.TradingContext, threshold float64) bool {
	c := ctx.CandleList[ctx.Position]
	if c.ADX == 0 {
		return false
	}
	return c.ADX >= threshold
}

// IsDIBullish 는 +DI > -DI (상승 방향 우세) 인지 확인한다.
func IsDIBullish(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.ADX == 0 {
		return false
	}
	return c.PlusDI > c.MinusDI
}

// IsDIBearish 는 -DI > +DI (하락 방향 우세) 인지 확인한다.
func IsDIBearish(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.ADX == 0 {
		return false
	}
	return c.MinusDI > c.PlusDI
}

// ============================================================================
// VWAP·거래량 조건
// ============================================================================

// IsAboveVWAP 는 종가가 세션 VWAP 위에 있는지 확인한다.
func IsAboveVWAP(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.VWAP == 0 {
		return false
	}
	return c.CloseOrigin > c.VWAP
}

// IsBelowVWAP 는 종가가 세션 VWAP 아래에 있는지 확인한다.
func IsBelowVWAP(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.VWAP == 0 {
		return false
	}
	return c.CloseOrigin < c.VWAP
}

// IsVWAPDeviation 는 종가가 VWAP ± k × StdDev 이상 이탈했는지 확인한다.
// k > 0: VWAP 위 k σ 이상 | k < 0: VWAP 아래 |k| σ 이상
func IsVWAPDeviation(ctx *box.TradingContext, k float64) bool {
	c := ctx.CandleList[ctx.Position]
	if c.VWAP == 0 || c.VWAPStdDev == 0 {
		return false
	}
	if k >= 0 {
		return c.CloseOrigin >= c.VWAP+k*c.VWAPStdDev
	}
	return c.CloseOrigin <= c.VWAP+k*c.VWAPStdDev // k is negative
}

// IsVWAPReclaim 는 최근 lookback 봉 내 VWAP 하향 이탈 후 현재 재돌파했는지 확인한다.
func IsVWAPReclaim(ctx *box.TradingContext, lookback int) bool {
	pos := ctx.Position
	if pos < lookback || ctx.CandleList[pos].VWAP == 0 {
		return false
	}
	cur := ctx.CandleList[pos]
	// 현재 봉: VWAP 위
	if cur.CloseOrigin <= cur.VWAP {
		return false
	}
	// lookback 내 VWAP 하회 봉 존재
	for i := pos - lookback; i < pos; i++ {
		if ctx.CandleList[i].VWAP > 0 && ctx.CandleList[i].CloseOrigin < ctx.CandleList[i].VWAP {
			return true
		}
	}
	return false
}

// IsVolumeZScoreSpike 는 현재 거래량이 최근 window 봉 평균 대비 Z-score 임계 이상인지 확인한다.
func IsVolumeZScoreSpike(ctx *box.TradingContext, window int, zThreshold float64) bool {
	pos := ctx.Position
	if pos < window {
		return false
	}
	var sum, sumSq float64
	for i := pos - window; i < pos; i++ {
		v := ctx.CandleList[i].Volume
		sum += v
		sumSq += v * v
	}
	mean := sum / float64(window)
	variance := sumSq/float64(window) - mean*mean
	if variance <= 0 {
		return false
	}
	stdDev := math.Sqrt(variance)
	if stdDev == 0 {
		return false
	}
	z := (ctx.CandleList[pos].Volume - mean) / stdDev
	return z >= zThreshold
}

// IsOBVRising 는 OBV가 최근 n봉 연속 증가 중인지 확인한다.
func IsOBVRising(ctx *box.TradingContext, n int) bool {
	pos := ctx.Position
	if pos < n {
		return false
	}
	for i := pos - n + 1; i <= pos; i++ {
		if ctx.CandleList[i].OBV <= ctx.CandleList[i-1].OBV {
			return false
		}
	}
	return true
}

// ============================================================================
// 변동성 구조 조건
// ============================================================================

// IsSuperTrendBullish 는 SuperTrend 방향이 상승(1)인지 확인한다.
func IsSuperTrendBullish(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	return c.SuperTrend != 0 && c.SuperTrendDir == 1
}

// IsSuperTrendBearish 는 SuperTrend 방향이 하락(-1)인지 확인한다.
func IsSuperTrendBearish(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	return c.SuperTrend != 0 && c.SuperTrendDir == -1
}

// IsDonchianBreakout 는 종가가 Donchian 상단을 돌파했는지 확인한다.
func IsDonchianBreakout(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	if cur.DonchianUpper == 0 || prev.DonchianUpper == 0 {
		return false
	}
	return cur.CloseOrigin > prev.DonchianUpper
}

// IsDonchianBreakdown 는 종가가 Donchian 하단을 하향 돌파했는지 확인한다.
func IsDonchianBreakdown(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	if cur.DonchianLower == 0 || prev.DonchianLower == 0 {
		return false
	}
	return cur.CloseOrigin < prev.DonchianLower
}

// IsKeltnerBreakout 는 종가가 Keltner 채널 상단을 돌파했는지 확인한다.
func IsKeltnerBreakout(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.KeltnerUpper == 0 {
		return false
	}
	return c.CloseOrigin > c.KeltnerUpper
}

// IsNarrowRange 는 NR-n 패턴 (최근 n봉 중 현재 봉 범위가 가장 좁음) 인지 확인한다.
func IsNarrowRange(ctx *box.TradingContext, n int) bool {
	pos := ctx.Position
	if pos < n-1 {
		return false
	}
	curRange := ctx.CandleList[pos].HighOrigin - ctx.CandleList[pos].LowOrigin
	for i := pos - n + 1; i < pos; i++ {
		r := ctx.CandleList[i].HighOrigin - ctx.CandleList[i].LowOrigin
		if curRange >= r {
			return false
		}
	}
	return true
}

// ============================================================================
// 숏 미러 조건 (거부 필터 when_not 용)
// ============================================================================

// IsRSIFallingFromOverbought 는 RSI가 과매수권에서 하락 중인지 확인한다.
func IsRSIFallingFromOverbought(ctx *box.TradingContext, overboughtThreshold float64) bool {
	pos := ctx.Position
	if pos < 1 || !hasValidRSI(ctx) {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	return prev.RSI >= overboughtThreshold && cur.RSI < prev.RSI
}

// IsBBUpperReject 는 스케일 고가가 상단 밴드에 닿은 뒤 음봉인지 확인한다.
func IsBBUpperReject(ctx *box.TradingContext) bool {
	cur := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(cur) {
		return false
	}
	return cur.High >= cur.BollingerUpper && cur.Close < cur.Open
}

// IsMaDeadCross5x20 는 MA5가 MA20을 하향 돌파했는지 확인한다 (데드크로스).
func IsMaDeadCross5x20(ctx *box.TradingContext) bool {
	pos := ctx.Position
	if pos < 1 {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	if cur.Ma60 == 0 || prev.Ma60 == 0 {
		return false
	}
	return prev.Ma5 >= prev.Ma20 && cur.Ma5 < cur.Ma20
}

// IsMaInverseArrangement 는 역배열 (MA5 < MA20 < MA60) 인지 확인한다.
func IsMaInverseArrangement(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.Ma60 == 0 {
		return false
	}
	return c.Ma5 < c.Ma20 && c.Ma20 < c.Ma60
}

// IsPriceBelowAllMa 는 스케일 종가가 MA5·MA20·MA60 모두 아래인지 확인한다.
func IsPriceBelowAllMa(ctx *box.TradingContext) bool {
	c := ctx.CandleList[ctx.Position]
	if c.Ma60 == 0 {
		return false
	}
	return c.Close < c.Ma5 && c.Close < c.Ma20 && c.Close < c.Ma60
}
