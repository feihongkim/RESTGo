package cond

import (
	"testing"

	"RESTGo/box"
)

// triggerCandles 는 n개의 캔들을 생성하고 setup으로 볼린저 밴드 필드를 채운다.
// indicatorCandles와 달리 Position을 호출자가 제어할 수 있도록 ctx만 반환.
func triggerCandles(n int, setup func(i int, c *box.Candle)) *box.TradingContext {
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

// ─────────────────────────────────────────────
// IsBBLowerBreakdownEvent
// ─────────────────────────────────────────────

func TestIsBBLowerBreakdownEvent_EdgeFires(t *testing.T) {
	// 전일: 종가 102 >= 하단 100 → 하단 위
	// 당일: 종가 98 < 하단 99 → 하향 이탈 순간
	ctx := triggerCandles(5, func(i int, c *box.Candle) {
		if i == 3 {
			c.Close = 102
			setBB(c, 100, 110) // Lower=100
		} else if i == 4 {
			c.Close = 98
			setBB(c, 99, 110) // Lower=99
		} else {
			setBB(c, 100, 110)
		}
	})
	if !IsBBLowerBreakdownEvent(ctx) {
		t.Error("전일 종가 >= 하단, 당일 종가 < 하단 → 하향 이탈 이벤트여야 함")
	}
}

func TestIsBBLowerBreakdownEvent_LevelBlocked(t *testing.T) {
	// 전일도 이미 하단 아래, 당일도 하단 아래 → edge 아님
	ctx := triggerCandles(5, func(i int, c *box.Candle) {
		if i == 3 {
			c.Close = 97
			setBB(c, 100, 110) // 이미 이탈 상태
		} else if i == 4 {
			c.Close = 96
			setBB(c, 99, 110) // 여전히 이탈
		} else {
			setBB(c, 100, 110)
		}
	})
	if IsBBLowerBreakdownEvent(ctx) {
		t.Error("전일부터 이미 이탈 상태면 level 차단 — false여야 함")
	}
}

func TestIsBBLowerBreakdownEvent_PositionZero(t *testing.T) {
	ctx := triggerCandles(3, func(i int, c *box.Candle) {
		setBB(c, 100, 110)
	})
	ctx.Position = 0
	if IsBBLowerBreakdownEvent(ctx) {
		t.Error("Position 0 → 비교할 전일이 없으므로 false")
	}
}

func TestIsBBLowerBreakdownEvent_NoBollinger(t *testing.T) {
	// 전일 밴드 미계산 (Upper/Lower == 0)
	ctx := triggerCandles(5, nil)
	ctx.CandleList[3].Close = 102
	// 밴드 0 → hasValidBollinger false
	ctx.CandleList[4].Close = 98
	setBB(ctx.CandleList[4], 99, 110)
	if IsBBLowerBreakdownEvent(ctx) {
		t.Error("전일 밴드 미계산이면 false")
	}
}

// ─────────────────────────────────────────────
// IsBBLowerReentryEvent
// ─────────────────────────────────────────────

func TestIsBBLowerReentryEvent_EdgeFires(t *testing.T) {
	// 전일: 종가 97 < 하단 100 → 하단 아래
	// 당일: 종가 101 >= 하단 100 → 밴드 안으로 복귀
	ctx := triggerCandles(5, func(i int, c *box.Candle) {
		if i == 3 {
			c.Close = 97
			setBB(c, 100, 110)
		} else if i == 4 {
			c.Close = 101
			setBB(c, 100, 110)
		} else {
			setBB(c, 100, 110)
		}
	})
	if !IsBBLowerReentryEvent(ctx) {
		t.Error("전일 종가 < 하단, 당일 종가 >= 하단 → 재진입 이벤트여야 함")
	}
}

func TestIsBBLowerReentryEvent_LevelBlocked(t *testing.T) {
	// 전일도 이미 하단 위, 당일도 하단 위 → 이탈 이력 없음 → edge 아님
	ctx := triggerCandles(5, func(i int, c *box.Candle) {
		if i == 3 {
			c.Close = 101
			setBB(c, 100, 110)
		} else if i == 4 {
			c.Close = 102
			setBB(c, 100, 110)
		} else {
			setBB(c, 100, 110)
		}
	})
	if IsBBLowerReentryEvent(ctx) {
		t.Error("이탈 이력 없이 계속 하단 위면 level 차단 — false여야 함")
	}
}

func TestIsBBLowerReentryEvent_PositionZero(t *testing.T) {
	ctx := triggerCandles(3, func(i int, c *box.Candle) {
		setBB(c, 100, 110)
	})
	ctx.Position = 0
	if IsBBLowerReentryEvent(ctx) {
		t.Error("Position 0 → false")
	}
}

func TestIsBBLowerReentryEvent_NoBollinger(t *testing.T) {
	ctx := triggerCandles(5, nil)
	ctx.CandleList[3].Close = 97
	setBB(ctx.CandleList[3], 100, 110)
	ctx.CandleList[4].Close = 101
	// 밴드 0 → hasValidBollinger false
	if IsBBLowerReentryEvent(ctx) {
		t.Error("당일 밴드 미계산이면 false")
	}
}

// ─────────────────────────────────────────────
// IsBBSqueezeBreakoutEvent
// ─────────────────────────────────────────────

func TestIsBBSqueezeBreakoutEvent_EdgeFires(t *testing.T) {
	// 전일: %B 0.79 <= 0.8
	// 당일: %B 0.85 > 0.8, lookback에 스퀴즈 존재
	ctx := triggerCandles(10, func(i int, c *box.Candle) {
		if i <= 7 {
			setBB(c, 99, 101) // width ≈ 2% < 4% → 스퀴즈
			c.Close = 100
			setBB(c, 99, 101) // %B = (100-99)/(101-99) = 0.5
		} else if i == 8 {
			setBB(c, 96, 110) // width ≈ 13.6%
			c.Close = 107     // %B ≈ (107-96)/(110-96) = 11/14 ≈ 0.786
			setBB(c, 96, 110)
		} else if i == 9 {
			c.Close = 109 // %B = (109-90)/(110-90) = 19/20 = 0.95
			setBB(c, 90, 110)
		} else {
			setBB(c, 100, 110)
		}
	})
	if !IsBBSqueezeBreakoutEvent(ctx, 20, 4.0, 0.8) {
		t.Error("전일 %B <= 0.8, 당일 %B > 0.8, 스퀴즈 이력 → 돌파 이벤트여야 함")
	}
}

func TestIsBBSqueezeBreakoutEvent_LevelBlocked(t *testing.T) {
	// 전일도 이미 %B > 0.8 → 어제 이미 돌파 발생 → 오늘은 edge 아님
	ctx := triggerCandles(10, func(i int, c *box.Candle) {
		if i <= 7 {
			setBB(c, 99, 101) // 스퀴즈
		} else if i == 8 {
			c.Close = 109 // %B = 0.95 > 0.8 → 전일도 돌파 상태
			setBB(c, 90, 110)
		} else if i == 9 {
			c.Close = 109
			setBB(c, 90, 110)
		} else {
			setBB(c, 100, 110)
		}
	})
	if IsBBSqueezeBreakoutEvent(ctx, 20, 4.0, 0.8) {
		t.Error("전일부터 이미 돌파 상태 → level 차단 — false여야 함")
	}
}

func TestIsBBSqueezeBreakoutEvent_NoSqueeze(t *testing.T) {
	// %B 돌파는 있지만 lookback에 스퀴즈 없음 → false
	ctx := triggerCandles(10, func(i int, c *box.Candle) {
		if i == 8 {
			c.Close = 107
			setBB(c, 96, 110) // %B ≈ 0.786
		} else if i == 9 {
			c.Close = 109
			setBB(c, 90, 110) // %B = 0.95
		} else {
			setBB(c, 95, 110) // width ≈ 15% > 4% → 스퀴즈 아님
		}
	})
	if IsBBSqueezeBreakoutEvent(ctx, 20, 4.0, 0.8) {
		t.Error("lookback에 스퀴즈 없으면 false")
	}
}

func TestIsBBSqueezeBreakoutEvent_PositionZero(t *testing.T) {
	ctx := triggerCandles(3, func(i int, c *box.Candle) {
		setBB(c, 100, 110)
	})
	ctx.Position = 0
	if IsBBSqueezeBreakoutEvent(ctx, 20, 4.0, 0.8) {
		t.Error("Position 0 → false")
	}
}

func TestIsBBSqueezeBreakoutEvent_NoBollinger(t *testing.T) {
	ctx := triggerCandles(10, nil)
	for i := 0; i <= 7; i++ {
		setBB(ctx.CandleList[i], 99, 101) // 스퀴즈
	}
	ctx.CandleList[8].Close = 107
	setBB(ctx.CandleList[8], 96, 110)
	// 당일 밴드 미계산(Upper/Lower==0)
	ctx.CandleList[9].Close = 109
	// 밴드 0 → hasValidBollinger false
	if IsBBSqueezeBreakoutEvent(ctx, 20, 4.0, 0.8) {
		t.Error("당일 밴드 미계산이면 false")
	}
}
