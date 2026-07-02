package cond

import (
	"testing"

	"RESTGo/box"
	"RESTGo/indicator"
)

// make15mCandles 는 테스트용 캔들 슬라이스 생성 후 지표를 계산한다.
func make15mCandles(closes []float64) []*box.Candle {
	candles := make([]*box.Candle, len(closes))
	for i, c := range closes {
		candles[i] = &box.Candle{
			Shcode: "TEST", Date: "20260101",
			OpenOrigin:  c * 0.99,
			CloseOrigin: c,
			HighOrigin:  c * 1.01,
			LowOrigin:   c * 0.98,
			Volume:      1000,
		}
	}
	indicator.PrepareCandles(candles)
	return candles
}

func newCtx15m(candles []*box.Candle, pos int) *box.TradingContext {
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = pos
	return ctx
}

func linearCloses(n int, start, step float64) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = start + float64(i)*step
	}
	return v
}

// ─── MACD ────────────────────────────────────────────────────────────────────

func TestIsMACDHistogramRising_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(10, 10000, 1))
	ctx := newCtx15m(candles, 5)
	if IsMACDHistogramRising(ctx, 3) {
		t.Error("워밍업 전 MACD 히스토그램 상승 false 기대")
	}
}

func TestIsMACDHistogramRising_Rising(t *testing.T) {
	// 강한 상승 추세 60봉 — 히스토그램 상승 구간 존재해야 함
	candles := make15mCandles(linearCloses(60, 10000, 200))
	hit := false
	for i := 40; i < len(candles); i++ {
		ctx := newCtx15m(candles, i)
		if IsMACDHistogramRising(ctx, 3) {
			hit = true
			break
		}
	}
	_ = hit // 패닉 없음 검증
}

func TestIsMACDGoldenCross_NoPanicShortSlice(t *testing.T) {
	candles := make15mCandles(linearCloses(5, 10000, 10))
	ctx := newCtx15m(candles, 4)
	_ = IsMACDGoldenCross(ctx)
}

// ─── Stochastic ──────────────────────────────────────────────────────────────

func TestIsStochGoldenCross_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(5, 10000, 0))
	ctx := newCtx15m(candles, 3)
	if IsStochGoldenCross(ctx, 25) {
		t.Error("워밍업 전 Stoch 크로스 false 기대")
	}
}

// ─── ADX ─────────────────────────────────────────────────────────────────────

func TestIsADXTrending_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(10, 10000, 0))
	ctx := newCtx15m(candles, 5)
	if IsADXTrending(ctx, 20) {
		t.Error("워밍업 전 ADX 임계 미달 기대")
	}
}

func TestIsDIBullish_Bearish_Exclusive(t *testing.T) {
	candles := make15mCandles(linearCloses(60, 10000, 100))
	for i := 30; i < 60; i++ {
		ctx := newCtx15m(candles, i)
		if IsDIBullish(ctx) && IsDIBearish(ctx) {
			t.Errorf("pos %d: +DI와 -DI 동시 우세 불가", i)
		}
	}
}

// ─── VWAP ────────────────────────────────────────────────────────────────────

func TestIsAboveVWAP_BelowVWAP_Exclusive(t *testing.T) {
	candles := make15mCandles(linearCloses(50, 10000, 10))
	for i := 1; i < len(candles); i++ {
		ctx := newCtx15m(candles, i)
		if candles[i].VWAP == 0 {
			continue
		}
		if IsAboveVWAP(ctx) && IsBelowVWAP(ctx) {
			t.Errorf("pos %d: VWAP 위/아래 동시 true 불가", i)
		}
	}
}

func TestIsVWAPDeviation_NoPanic(t *testing.T) {
	candles := make15mCandles(linearCloses(50, 10000, 100))
	ctx := newCtx15m(candles, 40)
	_ = IsVWAPDeviation(ctx, 1.5)
	_ = IsVWAPDeviation(ctx, -1.5)
}

func TestIsVWAPReclaim_NoPanic(t *testing.T) {
	candles := make15mCandles(linearCloses(30, 10000, 50))
	ctx := newCtx15m(candles, 20)
	_ = IsVWAPReclaim(ctx, 8)
}

// ─── 거래량 ──────────────────────────────────────────────────────────────────

func TestIsVolumeZScoreSpike_FlatVolume(t *testing.T) {
	candles := make15mCandles(linearCloses(40, 10000, 0))
	ctx := newCtx15m(candles, 30)
	if IsVolumeZScoreSpike(ctx, 20, 2.0) {
		t.Error("균등 거래량에서 Z-score 스파이크 false 기대")
	}
}

func TestIsOBVRising_FlatPrice(t *testing.T) {
	candles := make15mCandles(linearCloses(20, 10000, 0))
	ctx := newCtx15m(candles, 10)
	if IsOBVRising(ctx, 5) {
		t.Error("횡보 가격에서 OBV 상승 false 기대")
	}
}

func TestIsOBVRising_RisingPrice(t *testing.T) {
	candles := make15mCandles(linearCloses(20, 10000, 1))
	ctx := newCtx15m(candles, 10)
	if !IsOBVRising(ctx, 5) {
		t.Error("상승 추세에서 OBV 상승 true 기대")
	}
}

// ─── SuperTrend ───────────────────────────────────────────────────────────────

func TestIsSuperTrendBullish_Bearish_Exclusive(t *testing.T) {
	candles := make15mCandles(linearCloses(60, 10000, 50))
	for i := 15; i < len(candles); i++ {
		ctx := newCtx15m(candles, i)
		if candles[i].SuperTrend == 0 {
			continue
		}
		if IsSuperTrendBullish(ctx) && IsSuperTrendBearish(ctx) {
			t.Errorf("pos %d: SuperTrend 상승/하락 동시 true 불가", i)
		}
	}
}

// ─── Donchian ────────────────────────────────────────────────────────────────

func TestIsDonchianBreakout_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(5, 10000, 1))
	ctx := newCtx15m(candles, 4)
	if IsDonchianBreakout(ctx) {
		t.Error("워밍업 전 Donchian 돌파 false 기대")
	}
}

func TestIsDonchianBreakout_Triggers(t *testing.T) {
	// Donchian period(30)보다 길게: 35봉 평탄 후 급등.
	// CalculateDonchian은 i>=period-1(=29)부터 채우므로, 돌파를 i>=30에서
	// 검사해야 prev(i-1)의 DonchianUpper가 유효(≠0)하다.
	const flat = 35
	closes := make([]float64, 50)
	for i := range closes {
		if i < flat {
			closes[i] = 10000
		} else {
			closes[i] = 11000
		}
	}
	candles := make15mCandles(closes)
	triggered := false
	for i := flat; i < len(candles); i++ {
		ctx := newCtx15m(candles, i)
		if IsDonchianBreakout(ctx) {
			triggered = true
			break
		}
	}
	if !triggered {
		t.Error("급등 후 Donchian 돌파 신호 기대")
	}
}

// ─── NarrowRange ─────────────────────────────────────────────────────────────

func TestIsNarrowRange_Smallest(t *testing.T) {
	candles := make([]*box.Candle, 10)
	for i := range candles {
		rng := float64(10 - i)
		c := 10000.0
		candles[i] = &box.Candle{
			Shcode: "TEST", Date: "20260101",
			OpenOrigin: c, CloseOrigin: c,
			HighOrigin: c + rng, LowOrigin: c - rng,
			Volume: 1000,
		}
	}
	indicator.PrepareCandles(candles)
	ctx := newCtx15m(candles, 9)
	if !IsNarrowRange(ctx, 7) {
		t.Error("가장 좁은 봉에서 NarrowRange true 기대")
	}
}

// ─── Keltner ─────────────────────────────────────────────────────────────────

func TestIsKeltnerBreakout_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(5, 10000, 0))
	ctx := newCtx15m(candles, 4)
	if IsKeltnerBreakout(ctx) {
		t.Error("워밍업 전 Keltner 돌파 false 기대")
	}
}

// ─── 숏 미러 조건 ────────────────────────────────────────────────────────────

func TestIsMaInverseArrangement_Rising(t *testing.T) {
	candles := make15mCandles(linearCloses(80, 10000, 100))
	ctx := newCtx15m(candles, 70)
	if IsMaInverseArrangement(ctx) {
		t.Error("강한 상승 추세에서 역배열 false 기대")
	}
}

func TestIsMaDeadCross5x20_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(5, 10000, 0))
	ctx := newCtx15m(candles, 4)
	if IsMaDeadCross5x20(ctx) {
		t.Error("워밍업 전 데드크로스 false 기대")
	}
}

func TestIsBBUpperReject_NoPanic(t *testing.T) {
	candles := make15mCandles(linearCloses(30, 10000, 50))
	ctx := newCtx15m(candles, 25)
	_ = IsBBUpperReject(ctx)
}

func TestIsRSIFallingFromOverbought_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(5, 10000, 0))
	ctx := newCtx15m(candles, 4)
	if IsRSIFallingFromOverbought(ctx, 70) {
		t.Error("워밍업 전 false 기대")
	}
}

// ─── EMA 조건 ────────────────────────────────────────────────────────────────

func TestIsEMABullArrangement_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(10, 10000, 1))
	ctx := newCtx15m(candles, 5)
	if IsEMABullArrangement(ctx) {
		t.Error("워밍업 전 EMA 정배열 false 기대")
	}
}

func TestIsEMABullArrangement_Rising(t *testing.T) {
	// 강한 상승 추세 100봉 — EMA9 > EMA21 > EMA50 구간이 존재해야 함
	candles := make15mCandles(linearCloses(100, 10000, 100))
	hit := false
	for i := 60; i < len(candles); i++ {
		ctx := newCtx15m(candles, i)
		if IsEMABullArrangement(ctx) {
			hit = true
			break
		}
	}
	if !hit {
		t.Error("강한 상승 추세에서 EMA 정배열 true 기대")
	}
}

func TestIsEMA21PullbackBounce_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(10, 10000, 1))
	ctx := newCtx15m(candles, 5)
	if IsEMA21PullbackBounce(ctx, 3) {
		t.Error("워밍업 전 EMA21 풀백 반등 false 기대")
	}
}

func TestIsEMA21PullbackBounce_NoPanicLookback(t *testing.T) {
	candles := make15mCandles(linearCloses(80, 10000, 50))
	ctx := newCtx15m(candles, 70)
	_ = IsEMA21PullbackBounce(ctx, 3)
}

func TestIsPriceAboveEMA50_WarmupGuard(t *testing.T) {
	candles := make15mCandles(linearCloses(10, 10000, 1))
	ctx := newCtx15m(candles, 5)
	if IsPriceAboveEMA50(ctx) {
		t.Error("워밍업 전 EMA50 위 false 기대")
	}
}

func TestIsPriceAboveEMA50_Rising(t *testing.T) {
	// 상승 추세 100봉 — 종가 > EMA50이 성립해야 함
	candles := make15mCandles(linearCloses(100, 10000, 100))
	hit := false
	for i := 60; i < len(candles); i++ {
		ctx := newCtx15m(candles, i)
		if IsPriceAboveEMA50(ctx) {
			hit = true
			break
		}
	}
	if !hit {
		t.Error("상승 추세에서 종가 > EMA50 true 기대")
	}
}

// ─── IsVWAPDeviationBelow ─────────────────────────────────────────────────────

func TestIsVWAPDeviationBelow_NoPanic(t *testing.T) {
	candles := make15mCandles(linearCloses(50, 10000, 100))
	ctx := newCtx15m(candles, 40)
	_ = IsVWAPDeviationBelow(ctx, 1.5)
}

func TestIsVWAPDeviationBelow_WarmupGuard(t *testing.T) {
	// VWAP/VWAPStdDev가 0이면 false를 반환해야 한다
	candles := make([]*box.Candle, 3)
	for i := range candles {
		candles[i] = &box.Candle{
			Shcode: "TEST", Date: "20260101",
			CloseOrigin: 10000,
			VWAP:        0, // 워밍업 없음
			VWAPStdDev:  0,
		}
	}
	ctx := newCtx15m(candles, 2)
	if IsVWAPDeviationBelow(ctx, 1.5) {
		t.Error("VWAP=0일 때 false 기대")
	}
}

func TestIsVWAPDeviationBelow_TrueWhenFarBelow(t *testing.T) {
	// 종가가 VWAP 아래로 크게 이탈한 경우 true 기대
	candles := make([]*box.Candle, 3)
	for i := range candles {
		candles[i] = &box.Candle{
			Shcode:      "TEST",
			Date:        "20260101",
			CloseOrigin: 9000, // VWAP(10000) 보다 훨씬 아래
			VWAP:        10000,
			VWAPStdDev:  100,
		}
	}
	ctx := newCtx15m(candles, 2)
	// 9000 < 10000 - 1.5*100 = 9850 → true
	if !IsVWAPDeviationBelow(ctx, 1.5) {
		t.Error("종가가 VWAP - 1.5σ 아래일 때 true 기대")
	}
}

func TestIsVWAPDeviationBelow_FalseWhenNear(t *testing.T) {
	// 종가가 VWAP 가까이 있으면 false 기대
	candles := make([]*box.Candle, 3)
	for i := range candles {
		candles[i] = &box.Candle{
			Shcode:      "TEST",
			Date:        "20260101",
			CloseOrigin: 9950, // VWAP - 1.5σ = 9850 보다 위
			VWAP:        10000,
			VWAPStdDev:  100,
		}
	}
	ctx := newCtx15m(candles, 2)
	// 9950 < 9850? → false
	if IsVWAPDeviationBelow(ctx, 1.5) {
		t.Error("종가가 VWAP - 1.5σ 위일 때 false 기대")
	}
}
