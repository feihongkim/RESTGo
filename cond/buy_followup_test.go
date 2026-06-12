package cond

import (
	"testing"

	"RESTGo/box"
)

// ─────────────────────────────────────────────
// IsVolumeBreakout — Ma5(원본) × VolMa5 ≥ limit × 100,000
// ─────────────────────────────────────────────

func TestIsVolumeBreakout(t *testing.T) {
	tests := []struct {
		name      string
		ma5Origin float64
		volMa5    float64
		limit     float64
		want      bool
	}{
		{"거래대금 충분 (10000×100=1M ≥ 0.5M)", 10000, 100, 5, true},
		{"거래대금 부족 (1000×100=0.1M < 0.5M)", 1000, 100, 5, false},
		{"경계값 (정확히 0.5M)", 5000, 100, 5, true},
		{"limit 0 → false", 10000, 100, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &box.Candle{Ma5Origin: tt.ma5Origin, VolMa5: tt.volMa5}
			if got := IsVolumeBreakout(c, tt.limit); got != tt.want {
				t.Errorf("IsVolumeBreakout() = %v, want %v", got, tt.want)
			}
		})
	}
	if IsVolumeBreakout(nil, 5) {
		t.Error("nil 캔들은 false여야 함")
	}
}

// ─────────────────────────────────────────────
// oscilloScanByPrice — 상/하향 돌파 위치 수집
// ─────────────────────────────────────────────

func TestOscilloScanByPrice(t *testing.T) {
	mk := func(close float64) *box.Candle {
		return &box.Candle{Open: close, Close: close, High: close, Low: close}
	}

	t.Run("교차 3회", func(t *testing.T) {
		// 90(아래) → 110(상향돌파) → 90(하향돌파) → 110(상향돌파)
		candles := []*box.Candle{mk(90), mk(110), mk(90), mk(110)}
		osc := oscilloScanByPrice(candles, 100, 0, 3)
		if osc.count != 3 || osc.firstPos != 1 || osc.lastPos != 3 {
			t.Errorf("got count=%d first=%d last=%d, want 3/1/3", osc.count, osc.firstPos, osc.lastPos)
		}
	})

	t.Run("교차 없음", func(t *testing.T) {
		candles := []*box.Candle{mk(110), mk(115), mk(120)}
		osc := oscilloScanByPrice(candles, 100, 0, 2)
		if osc.count != 0 || osc.firstPos != 0 {
			t.Errorf("교차 없으면 zero값이어야 함: %+v", osc)
		}
	})
}

// ─────────────────────────────────────────────
// IsMultiDefWaitToBuyCondition
// ─────────────────────────────────────────────

func TestIsMultiDefWaitToBuyCondition(t *testing.T) {
	// PenPosition=1, 캔들2(pen+1)=음봉, 캔들3=DefBox 아래 시가 → 위로 마감하는 양봉
	setup := func() *box.TradingContext {
		candles := []*box.Candle{
			mkCandle(100, 101, 102, 99),  // 0
			mkCandle(100, 102, 103, 99),  // 1 (Pen)
			mkCandle(105, 104, 106, 103), // 2 (pen+1, 음봉)
			mkCandle(95, 106, 107, 94),   // 3 (시가<DefBox, 종가>DefBox, 양봉)
		}
		ctx := box.NewTradingContext(candles, nil)
		ctx.Position = 3
		ctx.PenPosition = 1
		ctx.DefboxPrice = 100
		ctx.BuyHelper = "multidef매수대기"
		return ctx
	}

	t.Run("전환 조건 충족 → true", func(t *testing.T) {
		if !IsMultiDefWaitToBuyCondition(setup()) {
			t.Error("true여야 함")
		}
	})

	t.Run("BuyHelper 게이트 미충족 → false", func(t *testing.T) {
		ctx := setup()
		ctx.BuyHelper = ""
		if IsMultiDefWaitToBuyCondition(ctx) {
			t.Error("BuyHelper 미설정이면 false여야 함")
		}
	})

	t.Run("pen+1 캔들이 양봉 → false", func(t *testing.T) {
		ctx := setup()
		ctx.CandleList[2] = mkCandle(103, 105, 106, 102)
		if IsMultiDefWaitToBuyCondition(ctx) {
			t.Error("pen+1 양봉이면 false여야 함")
		}
	})

	t.Run("현재 캔들이 음봉 → false", func(t *testing.T) {
		ctx := setup()
		ctx.CandleList[3] = mkCandle(95, 94, 96, 93)
		if IsMultiDefWaitToBuyCondition(ctx) {
			t.Error("현재 음봉이면 false여야 함")
		}
	})
}

// ─────────────────────────────────────────────
// 연약지반 회복 조건들
// ─────────────────────────────────────────────

func TestHasNoBullishCandleSinceMomentum(t *testing.T) {
	t.Run("모멘텀 이후 전부 음봉 → true", func(t *testing.T) {
		candles := []*box.Candle{
			mkCandle(100, 105, 106, 99), // 0 (양봉이지만 C# quirk: 인덱스 0은 무시)
			mkCandle(105, 104, 106, 103),
			mkCandle(104, 103, 105, 102),
			mkCandle(103, 102, 104, 101),
		}
		ctx := box.NewTradingContext(candles, nil)
		ctx.MomentumPosition = 1
		ctx.Position = 3
		if !HasNoBullishCandleSinceMomentum(ctx) {
			t.Error("양봉 없으면 true여야 함")
		}
	})

	t.Run("모멘텀 이후 양봉 존재 → false", func(t *testing.T) {
		candles := []*box.Candle{
			mkCandle(100, 99, 101, 98),
			mkCandle(99, 98, 100, 97),
			mkCandle(98, 103, 104, 97), // 양봉
			mkCandle(103, 102, 104, 101),
		}
		ctx := box.NewTradingContext(candles, nil)
		ctx.MomentumPosition = 1
		ctx.Position = 3
		if HasNoBullishCandleSinceMomentum(ctx) {
			t.Error("양봉 있으면 false여야 함")
		}
	})
}

func TestIsPriceRecrossedDefBoxForWeakGround(t *testing.T) {
	candles := []*box.Candle{
		mkCandle(100, 95, 101, 94), // prev: DefBox 아래 마감
		mkCandle(96, 101, 102, 95), // cur: DefBox 이상 마감
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 1
	ctx.DefboxPrice = 100
	if !IsPriceRecrossedDefBoxForWeakGround(ctx) {
		t.Error("재돌파 패턴이면 true여야 함")
	}
	ctx.DefboxPrice = 90 // prev.Close(95) > 90 → 재돌파 아님
	if IsPriceRecrossedDefBoxForWeakGround(ctx) {
		t.Error("전일이 이미 박스 위면 false여야 함")
	}
}

// ─────────────────────────────────────────────
// IsShortRangeValid — 게이트(부정 경로) 검증
// ─────────────────────────────────────────────

func TestIsShortRangeValid_Gates(t *testing.T) {
	t.Run("PenPosition 이후 40캔들 초과 → false", func(t *testing.T) {
		ctx := box.NewTradingContext(flatCandles(60, 50), nil)
		ctx.PenPosition = 1
		ctx.Position = 50
		ctx.DefboxPrice = 100
		if IsShortRangeValid(ctx) {
			t.Error("40캔들 초과면 false여야 함")
		}
	})

	t.Run("최고가가 DefBox 115% 이상 → false", func(t *testing.T) {
		candles := flatCandles(10, 50)
		candles[5].High = 130 // 1.15 × 100 = 115 초과
		ctx := box.NewTradingContext(candles, nil)
		ctx.PenPosition = 1
		ctx.Position = 9
		ctx.DefboxPrice = 100
		if IsShortRangeValid(ctx) {
			t.Error("최고가 초과면 false여야 함")
		}
	})
}

// ─────────────────────────────────────────────
// TODO: IsShortRangeValid 양성 케이스 / OscilloAnalyzer 분기별 검증
// 실제 종목 캔들 스냅샷(C# 결과 대조)으로 골든 테스트 작성 필요.
// ─────────────────────────────────────────────

func TestShortRangePositive_TODO(t *testing.T) {
	t.Skip("TODO: IsShortRangeValid '진동지지' 양성 케이스 — C# 동일 입력 골든 데이터 필요")
}
