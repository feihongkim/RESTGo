package indicator

import (
	"math"
	"testing"

	"RESTGo/box"
)

// ─────────────────────────────────────────────
// 픽스처: 결정적 의사난수 캔들 생성 (LCG — 시드 고정으로 재현 가능)
// ─────────────────────────────────────────────

func genCandles(n int) []*box.Candle {
	candles := make([]*box.Candle, n)
	seed := uint64(42)
	next := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11) / float64(1<<53) // [0,1)
	}

	price := 50000.0
	for i := range candles {
		change := (next() - 0.5) * 0.06 // ±3% 변동
		open := price
		close := price * (1 + change)
		high := math.Max(open, close) * (1 + next()*0.02)
		low := math.Min(open, close) * (1 - next()*0.02)
		price = close

		candles[i] = &box.Candle{
			OpenOrigin:  open,
			CloseOrigin: close,
			HighOrigin:  high,
			LowOrigin:   low,
			Volume:      1000 + next()*100000,
		}
	}
	return candles
}

const eps = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < eps
}

// ─────────────────────────────────────────────
// 동치 검증: 롤링 구현 vs 단순 재합산(naive) 구현
// 리팩토링(O(N×period) → O(N)) 전후 결과가 동일함을 보증
// ─────────────────────────────────────────────

func TestPrepareCandles_MA_EquivalentToNaive(t *testing.T) {
	candles := genCandles(300)
	PrepareCandles(candles)

	getMa := func(c *box.Candle, period int) float64 {
		switch period {
		case 5:
			return c.Ma5
		case 20:
			return c.Ma20
		case 60:
			return c.Ma60
		default:
			return c.Ma120
		}
	}

	for _, period := range []int{5, 20, 60, 120} {
		for i := range candles {
			// naive: window 전체 재합산
			want := 0.0
			if i >= period-1 {
				sum := 0.0
				for j := i + 1 - period; j <= i; j++ {
					sum += candles[j].Close
				}
				want = sum / float64(period)
			}
			if got := getMa(candles[i], period); !almostEqual(got, want) {
				t.Fatalf("MA%d[%d] = %.12f, naive = %.12f", period, i, got, want)
			}
		}
	}
}

func TestPrepareCandles_VolMa_EquivalentToNaive(t *testing.T) {
	candles := genCandles(300)
	PrepareCandles(candles)

	for i := range candles {
		for _, tc := range []struct {
			period int
			got    float64
		}{
			{5, candles[i].VolMa5},
			{20, candles[i].VolMa20},
		} {
			want := 0.0
			if i >= tc.period-1 {
				sum := 0.0
				for j := i + 1 - tc.period; j <= i; j++ {
					sum += candles[j].Volume
				}
				want = sum / float64(tc.period)
			}
			if !almostEqual(tc.got, want) {
				t.Fatalf("VolMa%d[%d] = %.12f, naive = %.12f", tc.period, i, tc.got, want)
			}
		}
	}
}

func TestPrepareCandles_ATR_EquivalentToNaive(t *testing.T) {
	const period = 14
	candles := genCandles(300)
	PrepareCandles(candles)

	for i := range candles {
		// naive: C# CalculateATRItem과 동일 — position >= period일 때 TR period개 재합산
		wantATR, wantPct := 0.0, 0.0
		if i >= period {
			trSum := 0.0
			for j := i - period + 1; j <= i; j++ {
				if j > 0 {
					hl := candles[j].HighOrigin - candles[j].LowOrigin
					hc := math.Abs(candles[j].HighOrigin - candles[j-1].CloseOrigin)
					lc := math.Abs(candles[j].LowOrigin - candles[j-1].CloseOrigin)
					trSum += math.Max(hl, math.Max(hc, lc))
				}
			}
			wantATR = trSum / float64(period)
			if candles[i].CloseOrigin > 0 {
				wantPct = wantATR / candles[i].CloseOrigin
			}
		}
		if !almostEqual(candles[i].ATR, wantATR) {
			t.Fatalf("ATR[%d] = %.12f, naive = %.12f", i, candles[i].ATR, wantATR)
		}
		if !almostEqual(candles[i].ATRPercentage, wantPct) {
			t.Fatalf("ATRPercentage[%d] = %.12f, naive = %.12f", i, candles[i].ATRPercentage, wantPct)
		}
	}
}

func TestCalculateBollinger_EquivalentToNaive(t *testing.T) {
	const period = 20
	const multiplier = 2.0
	candles := genCandles(300)
	PrepareCandles(candles)

	for i := period - 1; i < len(candles); i++ {
		// naive: 편차 제곱합 재계산 (C# CalculateBollingerBandsItem 방식)
		sum := 0.0
		for j := i + 1 - period; j <= i; j++ {
			sum += candles[j].Close
		}
		sma := sum / float64(period)

		variance := 0.0
		for j := i + 1 - period; j <= i; j++ {
			d := candles[j].Close - sma
			variance += d * d
		}
		stdDev := math.Sqrt(variance / float64(period))

		wantUpper := sma + multiplier*stdDev
		wantLower := sma - multiplier*stdDev
		wantWidth := (wantUpper - wantLower) / sma * 100

		if !almostEqual(candles[i].BollingerUpper, wantUpper) {
			t.Fatalf("BollingerUpper[%d] = %.12f, naive = %.12f", i, candles[i].BollingerUpper, wantUpper)
		}
		if !almostEqual(candles[i].BollingerLower, wantLower) {
			t.Fatalf("BollingerLower[%d] = %.12f, naive = %.12f", i, candles[i].BollingerLower, wantLower)
		}
		if !almostEqual(candles[i].BollingerWidth, wantWidth) {
			t.Fatalf("BollingerWidth[%d] = %.12f, naive = %.12f", i, candles[i].BollingerWidth, wantWidth)
		}
	}
}

// ─────────────────────────────────────────────
// 워밍업 경계: MA는 period-1 이전까지 0 (C# 동작과 동일)
// ─────────────────────────────────────────────

func TestPrepareCandles_WarmupZeros(t *testing.T) {
	candles := genCandles(150)
	PrepareCandles(candles)

	checks := []struct {
		name   string
		period int
		get    func(c *box.Candle) float64
	}{
		{"Ma5", 5, func(c *box.Candle) float64 { return c.Ma5 }},
		{"Ma20", 20, func(c *box.Candle) float64 { return c.Ma20 }},
		{"Ma60", 60, func(c *box.Candle) float64 { return c.Ma60 }},
		{"Ma120", 120, func(c *box.Candle) float64 { return c.Ma120 }},
	}
	for _, ck := range checks {
		if v := ck.get(candles[ck.period-2]); v != 0 {
			t.Errorf("%s[%d](워밍업 구간) = %f, want 0", ck.name, ck.period-2, v)
		}
		if v := ck.get(candles[ck.period-1]); v == 0 {
			t.Errorf("%s[%d](첫 유효 위치) = 0, want > 0", ck.name, ck.period-1)
		}
	}
}

// ─────────────────────────────────────────────
// Gradient: C# CalculateGradientMethod와 동일하게
// 양쪽 MA가 모두 유효한 i >= period부터만 계산 (첫 유효 MA 위치는 0 유지)
// ─────────────────────────────────────────────

func TestPrepareCandles_GradientWarmup(t *testing.T) {
	candles := genCandles(150)
	PrepareCandles(candles)

	checks := []struct {
		name   string
		period int
		get    func(c *box.Candle) float64
	}{
		{"Gradient(Ma5)", 5, func(c *box.Candle) float64 { return c.Gradient }},
		{"Gradient20", 20, func(c *box.Candle) float64 { return c.Gradient20 }},
		{"Gradient60", 60, func(c *box.Candle) float64 { return c.Gradient60 }},
		{"Gradient120", 120, func(c *box.Candle) float64 { return c.Gradient120 }},
	}
	for _, ck := range checks {
		// 첫 유효 MA 위치(period-1): Ma[i-1]=0이므로 기울기 미계산 (C#: 루프가 i=period부터)
		if v := ck.get(candles[ck.period-1]); v != 0 {
			t.Errorf("%s[%d] = %f, want 0 (Ma[i-1] 미유효 구간)", ck.name, ck.period-1, v)
		}
		// i=period부터: naive 공식과 일치
		i := ck.period
		var ma, maPrev float64
		switch ck.period {
		case 5:
			ma, maPrev = candles[i].Ma5, candles[i-1].Ma5
		case 20:
			ma, maPrev = candles[i].Ma20, candles[i-1].Ma20
		case 60:
			ma, maPrev = candles[i].Ma60, candles[i-1].Ma60
		default:
			ma, maPrev = candles[i].Ma120, candles[i-1].Ma120
		}
		want := (ma - maPrev) / ma * 100.0
		if got := ck.get(candles[i]); !almostEqual(got, want) {
			t.Errorf("%s[%d] = %.12f, want %.12f", ck.name, i, got, want)
		}
	}
}

// ─────────────────────────────────────────────
// RSI 기본 성질 (C#에는 없는 Go 신규 지표)
// ─────────────────────────────────────────────

func TestCalculateRSI_Basics(t *testing.T) {
	t.Run("연속 상승 → RSI 100", func(t *testing.T) {
		candles := make([]*box.Candle, 30)
		for i := range candles {
			candles[i] = &box.Candle{Close: 100 + float64(i)}
		}
		CalculateRSI(candles, 14)
		if got := candles[29].RSI; got != 100 {
			t.Errorf("연속 상승 RSI = %f, want 100", got)
		}
	})

	t.Run("값 범위 0~100", func(t *testing.T) {
		candles := genCandles(300)
		PrepareCandles(candles)
		for i := 14; i < len(candles); i++ {
			if candles[i].RSI < 0 || candles[i].RSI > 100 {
				t.Fatalf("RSI[%d] = %f, 범위 밖", i, candles[i].RSI)
			}
		}
	})
}
