package cond

import (
	"testing"

	"RESTGo/box"
)

// ─────────────────────────────────────────────
// 테스트 픽스처
// ─────────────────────────────────────────────

// indicatorCandles 는 n개의 캔들을 생성하고 setup으로 지표 필드를 채운다
func indicatorCandles(n int, setup func(i int, c *box.Candle)) *box.TradingContext {
	candles := make([]*box.Candle, n)
	for i := range candles {
		candles[i] = &box.Candle{Open: 99, Close: 100, High: 101, Low: 98}
		if setup != nil {
			setup(i, candles[i])
		}
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = n - 1
	return ctx
}

func setBB(c *box.Candle, lower, upper float64) {
	c.BollingerLower = lower
	c.BollingerUpper = upper
	mid := (lower + upper) / 2
	c.BollingerWidth = (upper - lower) / mid * 100
	if upper > lower {
		c.BBPercent = (c.Close - lower) / (upper - lower)
	}
}

// ─────────────────────────────────────────────
// RSI 조건
// ─────────────────────────────────────────────

func TestIsRSIOversold(t *testing.T) {
	ctx := indicatorCandles(20, func(i int, c *box.Candle) { c.RSI = 25 })
	if !IsRSIOversold(ctx, 30) {
		t.Error("RSI 25 < 30 → 과매도여야 함")
	}
	ctx.CandleList[ctx.Position].RSI = 35
	if IsRSIOversold(ctx, 30) {
		t.Error("RSI 35 → 과매도 아님")
	}
	// 워밍업 가드: position < 14
	ctx.Position = 10
	ctx.CandleList[10].RSI = 0
	if IsRSIOversold(ctx, 30) {
		t.Error("워밍업 구간(RSI 미계산)에서는 false여야 함")
	}
}

func TestIsRSIOverbought(t *testing.T) {
	ctx := indicatorCandles(20, func(i int, c *box.Candle) { c.RSI = 75 })
	if !IsRSIOverbought(ctx, 70) {
		t.Error("RSI 75 > 70 → 과매수여야 함")
	}
	ctx.CandleList[ctx.Position].RSI = 65
	if IsRSIOverbought(ctx, 70) {
		t.Error("RSI 65 → 과매수 아님")
	}
}

func TestIsRSIRecoveringFromOversold(t *testing.T) {
	// 과매도(25) → 회복(32, 직전 28보다 상승)
	ctx := indicatorCandles(25, func(i int, c *box.Candle) {
		switch {
		case i == 21:
			c.RSI = 25 // 과매도 발생
		case i == 23:
			c.RSI = 28
		case i == 24:
			c.RSI = 32 // 탈출 + 상승
		default:
			c.RSI = 40
		}
	})
	if !IsRSIRecoveringFromOversold(ctx, 30, 5) {
		t.Error("과매도 탈출 반등이어야 함")
	}

	// 현재도 과매도면 false
	ctx.CandleList[24].RSI = 29
	if IsRSIRecoveringFromOversold(ctx, 30, 5) {
		t.Error("현재 RSI가 과매도 구간이면 false")
	}

	// lookback 내 과매도 없으면 false
	ctx.CandleList[21].RSI = 45
	ctx.CandleList[23].RSI = 40
	ctx.CandleList[24].RSI = 50
	if IsRSIRecoveringFromOversold(ctx, 30, 5) {
		t.Error("과매도 이력 없으면 false")
	}
}

func TestIsRSIRising(t *testing.T) {
	ctx := indicatorCandles(25, func(i int, c *box.Candle) { c.RSI = float64(30 + i) })
	if !IsRSIRising(ctx, 3) {
		t.Error("RSI 단조 상승이어야 함")
	}
	ctx.CandleList[23].RSI = 60 // 24의 RSI(54)보다 큼 → 하락
	if IsRSIRising(ctx, 3) {
		t.Error("중간 하락이 있으면 false")
	}
}

func TestIsRSIInBullZone(t *testing.T) {
	ctx := indicatorCandles(20, func(i int, c *box.Candle) { c.RSI = 60 })
	if !IsRSIInBullZone(ctx, 50, 70) {
		t.Error("RSI 60은 [50,70] 구간")
	}
	ctx.CandleList[ctx.Position].RSI = 75
	if IsRSIInBullZone(ctx, 50, 70) {
		t.Error("RSI 75는 구간 밖")
	}
}

// ─────────────────────────────────────────────
// Bollinger Band 조건
// ─────────────────────────────────────────────

func TestIsBBLowerTouch(t *testing.T) {
	ctx := indicatorCandles(5, func(i int, c *box.Candle) { setBB(c, 99, 110) })
	// Low=98 <= Lower=99 → 터치
	if !IsBBLowerTouch(ctx) {
		t.Error("저가가 하단 밴드 이하 → 터치여야 함")
	}
	setBB(ctx.CandleList[ctx.Position], 95, 110) // Low=98 > 95
	if IsBBLowerTouch(ctx) {
		t.Error("저가가 하단 밴드 위 → 터치 아님")
	}
	// 워밍업 가드 (밴드 미계산 = 0)
	c := ctx.CandleList[ctx.Position]
	c.BollingerUpper, c.BollingerLower = 0, 0
	if IsBBLowerTouch(ctx) {
		t.Error("밴드 미계산 캔들에서는 false")
	}
}

func TestIsBBReboundFromLower(t *testing.T) {
	ctx := indicatorCandles(10, func(i int, c *box.Candle) {
		if i == 7 {
			setBB(c, 99, 110) // Low=98 <= 99 → 하단 터치
		} else {
			setBB(c, 90, 110) // Close=100 → %B=0.5
		}
	})
	if !IsBBReboundFromLower(ctx, 5, 0.3) {
		t.Error("하단 터치 후 %B 0.5 회복 → 반등이어야 함")
	}
	// 현재 %B가 회복 기준 미만이면 false
	cur := ctx.CandleList[ctx.Position]
	cur.Close = 91 // %B = 0.05
	setBB(cur, 90, 110)
	if IsBBReboundFromLower(ctx, 5, 0.3) {
		t.Error("%B 미회복이면 false")
	}
	// lookback 밖 터치는 무시
	cur.Close = 100
	setBB(cur, 90, 110)
	if IsBBReboundFromLower(ctx, 1, 0.3) {
		t.Error("lookback 밖 터치는 false")
	}
}

func TestIsBBSqueezeBreakout(t *testing.T) {
	ctx := indicatorCandles(10, func(i int, c *box.Candle) {
		if i <= 7 {
			setBB(c, 99, 101) // width = 2% < 4% 스퀴즈
		} else {
			c.Close = 109
			setBB(c, 90, 110) // width 20%, %B = 0.95
		}
	})
	if !IsBBSqueezeBreakout(ctx, 20, 4.0, 0.8) {
		t.Error("스퀴즈 후 %B 0.95 상방 이탈 → 돌파여야 함")
	}
	// %B 미달이면 false
	cur := ctx.CandleList[ctx.Position]
	cur.Close = 100
	setBB(cur, 90, 110) // %B = 0.5
	if IsBBSqueezeBreakout(ctx, 20, 4.0, 0.8) {
		t.Error("%B 0.5 → 돌파 아님")
	}
}

func TestIsBBUpperBreakout(t *testing.T) {
	ctx := indicatorCandles(5, func(i int, c *box.Candle) { setBB(c, 90, 99) })
	// Close=100 > Upper=99
	if !IsBBUpperBreakout(ctx) {
		t.Error("종가가 상단 밴드 초과 → 돌파여야 함")
	}
	setBB(ctx.CandleList[ctx.Position], 90, 110)
	if IsBBUpperBreakout(ctx) {
		t.Error("종가가 상단 밴드 아래 → 돌파 아님")
	}
}

func TestIsAboveBBMiddle(t *testing.T) {
	ctx := indicatorCandles(10, func(i int, c *box.Candle) {
		setBB(c, 90, 105) // Close=100, %B = 10/15 ≈ 0.67
	})
	if !IsAboveBBMiddle(ctx, 3) {
		t.Error("3캔들 연속 중심선 위 → true여야 함")
	}
	// 직전 캔들이 중심선 아래면 false
	prev := ctx.CandleList[ctx.Position-1]
	prev.Close = 93
	setBB(prev, 90, 105) // %B = 0.2
	if IsAboveBBMiddle(ctx, 3) {
		t.Error("구간 내 %B < 0.5 캔들이 있으면 false")
	}
}

// ─────────────────────────────────────────────
// 이동평균(MA) 조건
// ─────────────────────────────────────────────

func TestIsMaGoldenCross5x20(t *testing.T) {
	ctx := indicatorCandles(5, func(i int, c *box.Candle) {
		if i < 4 {
			c.Ma5, c.Ma20 = 95, 100 // MA5 < MA20
		} else {
			c.Ma5, c.Ma20 = 101, 100 // 상향 돌파
		}
	})
	if !IsMaGoldenCross5x20(ctx) {
		t.Error("MA5가 MA20 상향 돌파 → 골든크로스여야 함")
	}
	// 이미 위에 있었으면 크로스 아님
	ctx.CandleList[3].Ma5 = 102
	if IsMaGoldenCross5x20(ctx) {
		t.Error("직전에 이미 MA5 > MA20이면 크로스 아님")
	}
}

func TestIsMaGoldenCross20x60(t *testing.T) {
	ctx := indicatorCandles(5, func(i int, c *box.Candle) {
		if i < 4 {
			c.Ma20, c.Ma60 = 95, 100
		} else {
			c.Ma20, c.Ma60 = 101, 100
		}
	})
	if !IsMaGoldenCross20x60(ctx) {
		t.Error("MA20이 MA60 상향 돌파여야 함")
	}
	// MA 미계산(0) 가드
	ctx.CandleList[3].Ma60 = 0
	if IsMaGoldenCross20x60(ctx) {
		t.Error("MA 미계산 캔들에서는 false")
	}
}

func TestIsMaProperArrangementNow(t *testing.T) {
	ctx := indicatorCandles(3, func(i int, c *box.Candle) {
		c.Ma5, c.Ma20, c.Ma60 = 105, 102, 100
	})
	if !IsMaProperArrangementNow(ctx) {
		t.Error("MA5 > MA20 > MA60 → 정배열")
	}
	ctx.CandleList[ctx.Position].Ma20 = 106
	if IsMaProperArrangementNow(ctx) {
		t.Error("MA20 > MA5 → 정배열 아님")
	}
}

func TestIsAllMaRising(t *testing.T) {
	ctx := indicatorCandles(3, func(i int, c *box.Candle) {
		c.Ma60 = 100
		c.Gradient, c.Gradient20, c.Gradient60 = 1, 0.5, 0.2
	})
	if !IsAllMaRising(ctx) {
		t.Error("모든 기울기 양수 → true")
	}
	ctx.CandleList[ctx.Position].Gradient60 = -0.1
	if IsAllMaRising(ctx) {
		t.Error("하나라도 음수면 false")
	}
}

func TestIsMaConvergence(t *testing.T) {
	ctx := indicatorCandles(3, func(i int, c *box.Candle) {
		c.Ma5, c.Ma20, c.Ma60 = 101, 100, 99 // (101-99)/99 ≈ 0.0202
	})
	if !IsMaConvergence(ctx, 0.03) {
		t.Error("스프레드 2% ≤ 3% → 수렴")
	}
	ctx.CandleList[ctx.Position].Ma5 = 110 // (110-99)/99 ≈ 0.111
	if IsMaConvergence(ctx, 0.03) {
		t.Error("스프레드 11% → 수렴 아님")
	}
}

func TestIsPriceAboveAllMa(t *testing.T) {
	ctx := indicatorCandles(3, func(i int, c *box.Candle) {
		c.Ma5, c.Ma20, c.Ma60 = 99, 98, 97 // Close=100
	})
	if !IsPriceAboveAllMa(ctx) {
		t.Error("종가가 모든 MA 위 → true")
	}
	ctx.CandleList[ctx.Position].Ma5 = 101
	if IsPriceAboveAllMa(ctx) {
		t.Error("종가가 MA5 아래 → false")
	}
}
