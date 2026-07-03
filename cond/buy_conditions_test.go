package cond

import (
	"testing"

	"RESTGo/box"
)

// ─────────────────────────────────────────────
// 테스트 픽스처 헬퍼
// ─────────────────────────────────────────────

// mkCandle 은 스케일된 OHLC만 채운 캔들을 생성 (cond 함수들은 스케일 가격만 사용)
func mkCandle(open, close, high, low float64) *box.Candle {
	return &box.Candle{Open: open, Close: close, High: high, Low: low}
}

// flatCandles 는 n개의 동일한 캔들 리스트 생성 (기본: 종가 close, 몸통 1)
func flatCandles(n int, close float64) []*box.Candle {
	candles := make([]*box.Candle, n)
	for i := range candles {
		candles[i] = mkCandle(close-1, close, close+0.5, close-1.5)
	}
	return candles
}

// ─────────────────────────────────────────────
// IsDefBoxBreakout
// ─────────────────────────────────────────────

func TestIsDefBoxBreakout(t *testing.T) {
	tests := []struct {
		name      string
		prevClose float64
		curOpen   float64
		curClose  float64
		want      bool
	}{
		{"전일 박스 아래 → 당일 종가 돌파", 95, 96, 101, true},
		{"전일 박스 아래 → 당일 갭 시가 돌파", 95, 101, 99, true},
		{"전일 이미 박스 위 (돌파 아님)", 105, 106, 107, false},
		{"전일 박스 아래 → 당일도 아래", 95, 96, 98, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := box.NewTradingContext([]*box.Candle{
				mkCandle(tt.prevClose-1, tt.prevClose, tt.prevClose+1, tt.prevClose-2),
				mkCandle(tt.curOpen, tt.curClose, tt.curClose+1, tt.curOpen-2),
			}, nil)
			ctx.Position = 1
			ctx.DefboxPrice = 100

			if got := IsDefBoxBreakout(ctx); got != tt.want {
				t.Errorf("IsDefBoxBreakout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDefBoxBreakout_PositionZero(t *testing.T) {
	ctx := box.NewTradingContext(flatCandles(2, 50), nil)
	ctx.Position = 0
	if IsDefBoxBreakout(ctx) {
		t.Error("Position 0에서는 항상 false여야 함")
	}
}

// ─────────────────────────────────────────────
// IsCloseNearDefboxPrice
// ─────────────────────────────────────────────

// nearPriceCtx 는 MainBox(idx 0) ← DefBox(idx 1, MainDefLink=[0]) 구조의 ctx 생성
func nearPriceCtx(curClose, defboxPrice, mainboxPrice float64) *box.TradingContext {
	boxList := []*box.Box{
		{KindOfBox: box.KindMainBox, Price: mainboxPrice},
		{KindOfBox: box.KindDefBox, Price: defboxPrice, MainDefLink: []int{0}},
	}
	ctx := box.NewTradingContext([]*box.Candle{
		mkCandle(curClose-1, curClose, curClose+1, curClose-2),
	}, boxList)
	ctx.Position = 0
	ctx.DefboxIndex = 1
	ctx.DefboxPrice = defboxPrice
	return ctx
}

func TestIsCloseNearDefboxPrice(t *testing.T) {
	const threshold, mainThreshold = 0.07, 0.15

	tests := []struct {
		name         string
		curClose     float64
		defboxPrice  float64
		mainboxPrice float64
		want         bool
	}{
		// DefBox 100 기준 7% = 7, MainBox 90 기준 15% = 13.5
		{"DefBox·MainBox 모두 근접", 103, 100, 90, true},
		{"DefBox에서 너무 멀어짐 (+10)", 110, 100, 90, false},
		{"MainBox에서 너무 멀어짐 (+16)", 106, 100, 90, false},
		{"박스 아래는 항상 근접 취급", 80, 100, 90, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := nearPriceCtx(tt.curClose, tt.defboxPrice, tt.mainboxPrice)
			if got := IsCloseNearDefboxPrice(ctx, threshold, mainThreshold); got != tt.want {
				t.Errorf("IsCloseNearDefboxPrice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCloseNearDefboxPrice_NoMainBox(t *testing.T) {
	// MainDefLink가 비면 MainBox 검사는 통과 처리
	boxList := []*box.Box{
		{KindOfBox: box.KindDefBox, Price: 100, MainDefLink: nil},
	}
	ctx := box.NewTradingContext([]*box.Candle{mkCandle(102, 103, 104, 101)}, boxList)
	ctx.Position = 0
	ctx.DefboxIndex = 0
	ctx.DefboxPrice = 100

	if !IsCloseNearDefboxPrice(ctx, 0.07, 0.15) {
		t.Error("MainBox 없으면 DefBox 근접만으로 true여야 함")
	}
}

// ─────────────────────────────────────────────
// IsMainboxDistanceTwiceOrMore
// ─────────────────────────────────────────────

func TestIsMainboxDistanceTwiceOrMore(t *testing.T) {
	tests := []struct {
		name                          string
		mainboxPos, defboxPos, curPos int
		want                          bool
	}{
		// 2*(defbox-mainbox) >= (cur-defbox)
		{"Main~Def 거리 10, Def 이후 20 → 경계 통과", 10, 20, 40, true},
		{"Main~Def 거리 10, Def 이후 10 → 통과", 10, 20, 30, true},
		{"Main~Def 거리 10, Def 이후 30 → 실패", 10, 20, 50, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := box.NewTradingContext(nil, nil)
			ctx.MainboxPosition = tt.mainboxPos
			ctx.DefboxPosition = tt.defboxPos
			ctx.Position = tt.curPos
			if got := IsMainboxDistanceTwiceOrMore(ctx); got != tt.want {
				t.Errorf("IsMainboxDistanceTwiceOrMore() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─────────────────────────────────────────────
// IsBoxDensityValidByCount
// ─────────────────────────────────────────────

func TestIsBoxDensityValidByCount(t *testing.T) {
	mkBoxList := func(n int) []*box.Box {
		list := make([]*box.Box, n)
		for i := range list {
			list[i] = &box.Box{}
		}
		return list
	}

	t.Run("DefBox 없음 → false", func(t *testing.T) {
		ctx := box.NewTradingContext(nil, nil) // DefboxIndex = -1
		if IsBoxDensityValidByCount(ctx) {
			t.Error("DefBox 없으면 false여야 함")
		}
	})

	t.Run("사이 Box 4개 이하 → true", func(t *testing.T) {
		boxList := mkBoxList(6)
		boxList[5] = &box.Box{KindOfBox: box.KindDefBox, MainDefLink: []int{2}} // 5-2=3
		ctx := box.NewTradingContext(nil, boxList)
		ctx.DefboxIndex = 5
		if !IsBoxDensityValidByCount(ctx) {
			t.Error("간격 3이면 true여야 함")
		}
	})

	t.Run("사이 Box 5개 이상 → false", func(t *testing.T) {
		boxList := mkBoxList(6)
		boxList[5] = &box.Box{KindOfBox: box.KindDefBox, MainDefLink: []int{0}} // 5-0=5
		ctx := box.NewTradingContext(nil, boxList)
		ctx.DefboxIndex = 5
		if IsBoxDensityValidByCount(ctx) {
			t.Error("간격 5면 false여야 함")
		}
	})
}

// ─────────────────────────────────────────────
// IsSingleBreakout
// ─────────────────────────────────────────────

func TestIsSingleBreakout(t *testing.T) {
	// MainboxPosition=5, DefboxPosition=8 → 검사 구간: 캔들 1~4
	setup := func() *box.TradingContext {
		ctx := box.NewTradingContext(flatCandles(10, 50), nil)
		ctx.MainboxPosition = 5
		ctx.DefboxPosition = 8
		ctx.Position = 9
		ctx.DefboxPrice = 100
		return ctx
	}

	t.Run("돌파 이력 0회 → true", func(t *testing.T) {
		if !IsSingleBreakout(setup()) {
			t.Error("돌파 이력 없으면 true여야 함")
		}
	})

	t.Run("돌파 이력 1회 → true", func(t *testing.T) {
		ctx := setup()
		ctx.CandleList[2].Close = 110
		if !IsSingleBreakout(ctx) {
			t.Error("돌파 1회까지는 허용해야 함")
		}
	})

	t.Run("돌파 이력 2회 → false", func(t *testing.T) {
		ctx := setup()
		ctx.CandleList[2].Close = 110
		ctx.CandleList[3].Close = 110
		if IsSingleBreakout(ctx) {
			t.Error("돌파 2회면 false여야 함")
		}
	})
}

// ─────────────────────────────────────────────
// HasExcessiveUpperWick
// ─────────────────────────────────────────────

func TestHasExcessiveUpperWick(t *testing.T) {
	const ratio = 4.0

	normal := mkCandle(100, 101, 102, 99)    // 윗꼬리 1 ≤ 몸통 1 × 4
	excessive := mkCandle(100, 101, 110, 99) // 윗꼬리 9 > 몸통 1 × 4
	doji := mkCandle(100, 100, 105, 95)      // 몸통 ≈ 0

	// DefBox 캔들 = index 0, MainBox 캔들 = index 1
	setup := func(defCandle, mainCandle *box.Candle) *box.TradingContext {
		ctx := box.NewTradingContext([]*box.Candle{defCandle, mainCandle}, nil)
		ctx.DefboxPosition = 0
		ctx.MainboxPosition = 1
		return ctx
	}

	tests := []struct {
		name                  string
		defCandle, mainCandle *box.Candle
		want                  bool
	}{
		{"둘 다 정상 → false", normal, normal, false},
		{"DefBox 캔들 윗꼬리 과도 → true", excessive, normal, true},
		{"MainBox 캔들 윗꼬리 과도 → true", normal, excessive, true},
		{"도지(몸통 ≈ 0) → true", doji, normal, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasExcessiveUpperWick(setup(tt.defCandle, tt.mainCandle), ratio); got != tt.want {
				t.Errorf("HasExcessiveUpperWick() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ─────────────────────────────────────────────
// IsBoxConditionValid2
// ─────────────────────────────────────────────

func TestIsBoxConditionValid2(t *testing.T) {
	// MainboxPosition=5, DefboxPosition=7 → 검사 구간: 캔들 1~4
	setup := func() *box.TradingContext {
		ctx := box.NewTradingContext(flatCandles(10, 49), nil)
		ctx.MainboxPosition = 5
		ctx.DefboxPosition = 7
		ctx.DefboxPrice = 100
		return ctx
	}

	t.Run("MainboxPosition 0 → true", func(t *testing.T) {
		ctx := setup()
		ctx.MainboxPosition = 0
		if !IsBoxConditionValid2(ctx) {
			t.Error("MainBox 미설정이면 true여야 함")
		}
	})

	t.Run("패턴 없음 → true", func(t *testing.T) {
		if !IsBoxConditionValid2(setup()) {
			t.Error("박스 위 음봉→양봉 패턴 없으면 true여야 함")
		}
	})

	t.Run("박스 위 음봉→양봉 패턴 존재 → false", func(t *testing.T) {
		ctx := setup()
		ctx.CandleList[1] = mkCandle(106, 105, 107, 104) // 음봉, 박스 위
		ctx.CandleList[2] = mkCandle(104, 106, 107, 103) // 양봉, 박스 위
		if IsBoxConditionValid2(ctx) {
			t.Error("패턴 발견 시 false여야 함")
		}
	})
}

// ─────────────────────────────────────────────
// IsAdditionalBoxConditionValid
// ─────────────────────────────────────────────

func TestIsAdditionalBoxConditionValid_NoDefBox(t *testing.T) {
	ctx := box.NewTradingContext(nil, nil)
	if !IsAdditionalBoxConditionValid(ctx) {
		t.Error("DefBox 없으면 true여야 함")
	}
}

// ─────────────────────────────────────────────
// TODO: 추가 작성 대상 (conditions_extra.go / oscillator.go)
// 실제 C# Stock1 출력과 대조한 골든 케이스로 채울 것.
// ─────────────────────────────────────────────

func TestConditionsExtra_TODO(t *testing.T) {
	t.Skip("TODO: IsMa20NearMa60*, IsMainboxConditionValid, EvaluateMainBoxPositionBasedTiming*, " +
		"IsPenetrationOptionValid, GetMultiDefboxDamCount 테스트 작성 — " +
		"C# 원본 또는 실제 종목(예: 005930) 분석 결과를 골든 데이터로 사용")
}

// ─────────────────────────────────────────────
// HasPullbackOrCorrection (C# CandlePatternEvaluator 정렬)
// ─────────────────────────────────────────────

func TestHasPullbackOrCorrection(t *testing.T) {
	// 캔들 5개: DefboxPosition=1, Position=4 → 스캔 구간은 [1,3]
	mk := func(gradient, low, ma20, open, close float64) *box.Candle {
		return &box.Candle{Gradient: gradient, Low: low, Ma20: ma20, Open: open, Close: close}
	}
	bullish := func(gradient float64) *box.Candle {
		// 양봉 + 저가가 MA20 위 (눌림/조정 어느 쪽도 아님)
		return mk(gradient, 100, 90, 99, 101)
	}

	tests := []struct {
		name    string
		candles []*box.Candle
		want    bool
	}{
		{
			"눌림목: DefBox 이후 Gradient<0 캔들 존재",
			[]*box.Candle{bullish(1), bullish(1), mk(-0.5, 100, 90, 99, 101), bullish(1), bullish(1)},
			true,
		},
		{
			"조정: Low≤MA20 && 음봉 캔들 존재",
			[]*box.Candle{bullish(1), bullish(1), mk(1, 89, 90, 101, 99), bullish(1), bullish(1)},
			true,
		},
		{
			"조정: Low≤MA20 && 도지(Close==Open)도 인정",
			[]*box.Candle{bullish(1), bullish(1), mk(1, 90, 90, 100, 100), bullish(1), bullish(1)},
			true,
		},
		{
			"DefBox 위치 캔들의 Gradient<0은 눌림목 아님 (i > DefboxPosition)",
			[]*box.Candle{bullish(1), mk(-0.5, 100, 90, 99, 101), bullish(1), bullish(1), bullish(1)},
			false,
		},
		{
			"현재 캔들(Position)은 스캔 제외",
			[]*box.Candle{bullish(1), bullish(1), bullish(1), bullish(1), mk(-0.5, 89, 90, 101, 99)},
			false,
		},
		{
			"구간 내 눌림/조정 없음",
			[]*box.Candle{bullish(1), bullish(1), bullish(1), bullish(1), bullish(1)},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := box.NewTradingContext(tt.candles, nil)
			ctx.DefboxPosition = 1
			ctx.Position = 4

			if got := HasPullbackOrCorrection(ctx); got != tt.want {
				t.Errorf("HasPullbackOrCorrection() = %v, want %v", got, tt.want)
			}
		})
	}
}
