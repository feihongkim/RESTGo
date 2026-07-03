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

// ─────────────────────────────────────────────
// edge 재발동: 상태가 끊겼다 다시 충족되면 새 edge (o o o x o 패턴)
// ─────────────────────────────────────────────

func TestIsBBLowerBreakdownEvent_RefiresAfterInterruption(t *testing.T) {
	// 이탈 상태 패턴: [위, 아래, 아래, 위, 아래] → edge는 idx1과 idx4에서만
	below := []bool{false, true, true, false, true}
	ctx := triggerCandles(5, func(i int, c *box.Candle) {
		setBB(c, 100, 110)
		if below[i] {
			c.Close = 98 // 하단(100) 아래
		} else {
			c.Close = 102 // 하단 위
		}
	})

	want := []bool{false, true, false, false, true} // idx0은 prev 없음
	for pos := 0; pos < 5; pos++ {
		ctx.Position = pos
		if got := IsBBLowerBreakdownEvent(ctx); got != want[pos] {
			t.Errorf("Position %d: got %v, want %v (패턴 o o o x o의 재발동 검증)", pos, got, want[pos])
		}
	}
}

// ─────────────────────────────────────────────
// IsWBottomBoxCompletedEvent / HasDefBoxBeforeWPattern
// ─────────────────────────────────────────────

// wBottomFixture 는 S-R-S W패턴이 완성된 컨텍스트를 구성한다.
//   P1(support, pos30): 직전 10봉(20~29) 저가≤BB하단 + 밴드폭 팽창(25~30 상단 확대)
//   resist(pos38): 종가 110 < MA20 115
//   P2(support, pos45): 직전 10봉 하단 이탈 없음 (BB 내부)
//   인식 캔들(pos46): 종가 110 < MA20 115, 마지막 box CurvePosition=46
func wBottomFixture(withDefBox bool) *box.TradingContext {
	candles := make([]*box.Candle, 60)
	for i := range candles {
		c := &box.Candle{Open: 109, Close: 110, High: 111, Low: 108, Ma20: 115}
		setBB(c, 100, 110)
		if i >= 20 && i <= 29 {
			c.Low = 99 // P1 직전 구간 BB 하단 이탈
		}
		if i >= 25 && i <= 34 {
			setBB(c, 100, 110+2*float64(i-24)) // 밴드폭 팽창
		}
		candles[i] = c
	}
	var boxes []*box.Box
	if withDefBox {
		boxes = append(boxes, &box.Box{BoxPosition: 15, BoxType: box.BoxTypeResistance, KindOfBox: box.KindDefBox})
	}
	boxes = append(boxes,
		&box.Box{BoxPosition: 30, BoxType: box.BoxTypeSupport, KindOfBox: box.KindBox, CurvePosition: 31},
		&box.Box{BoxPosition: 38, BoxType: box.BoxTypeResistance, KindOfBox: box.KindBox, CurvePosition: 39},
		&box.Box{BoxPosition: 45, BoxType: box.BoxTypeSupport, KindOfBox: box.KindBox, CurvePosition: 46},
	)
	ctx := box.NewTradingContext(candles, boxes)
	ctx.Position = 46
	return ctx
}

func TestIsWBottomBoxCompletedEvent_Fires(t *testing.T) {
	ctx := wBottomFixture(false)
	if !IsWBottomBoxCompletedEvent(ctx, 50) {
		t.Fatal("W패턴 완성 순간인데 트리거 미발동")
	}
}

func TestIsWBottomBoxCompletedEvent_NotNewBox(t *testing.T) {
	ctx := wBottomFixture(false)
	ctx.Position = 47 // 마지막 box 인식 다음 캔들 → CurvePosition(46) != Position → edge 아님
	if IsWBottomBoxCompletedEvent(ctx, 50) {
		t.Fatal("인식 캔들이 지났는데 발동 (edge 위반)")
	}
}

func TestIsWBottomBoxCompletedEvent_AboveMa20Blocked(t *testing.T) {
	ctx := wBottomFixture(false)
	ctx.CandleList[46].Close = 120 // MA20(115) 위 → 약세 구간 아님
	if IsWBottomBoxCompletedEvent(ctx, 50) {
		t.Fatal("MA20 위인데 발동")
	}
}

func TestHasDefBoxBeforeWPattern(t *testing.T) {
	if !HasDefBoxBeforeWPattern(wBottomFixture(true), 50) {
		t.Error("P1 이전 DefBox 있는데 false")
	}
	if HasDefBoxBeforeWPattern(wBottomFixture(false), 50) {
		t.Error("DefBox 없는데 true")
	}
}
