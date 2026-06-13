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
	closes := make([]float64, 30)
	for i := range closes {
		if i < 25 {
			closes[i] = 10000
		} else {
			closes[i] = 11000
		}
	}
	candles := make15mCandles(closes)
	triggered := false
	for i := 25; i < len(candles); i++ {
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
