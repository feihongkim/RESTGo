package cond

import (
	"testing"

	"RESTGo/box"
)

// mkSellCandle 은 테스트용 캔들을 생성한다 (필수 필드만).
func mkSellCandle(date string, open, high, low, close, volume float64) *box.Candle {
	c := &box.Candle{
		Date:        date,
		Open:        open,
		High:        high,
		Low:         low,
		Close:       close,
		Volume:      volume,
		OpenOrigin:  open,
		HighOrigin:  high,
		LowOrigin:   low,
		CloseOrigin: close,
	}
	return c
}

func TestIsMainBoxBreakdownFailure(t *testing.T) {
	cases := []struct {
		name        string
		buyPosition int
		curPos      int
		open, close float64
		mainBox     float64
		want        bool
	}{
		{"D+1 음봉 + MainBox 하향", 0, 1, 100, 90, 95, true},
		{"D+2 음봉 + MainBox 하향", 0, 2, 100, 90, 95, true},
		{"D+3은 기간 초과", 0, 3, 100, 90, 95, false},
		{"D+1 양봉이면 false", 0, 1, 90, 100, 95, false},
		{"D+1 음봉이지만 MainBox 이상", 0, 1, 100, 96, 95, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candles := make([]*box.Candle, tc.curPos+1)
			for i := range candles {
				candles[i] = mkSellCandle("", 0, 0, 0, 0, 0)
			}
			candles[tc.curPos] = mkSellCandle("D+x", tc.open, tc.close, tc.close, tc.close, 0)
			candles[tc.curPos].Open = tc.open
			candles[tc.curPos].Close = tc.close
			ctx := box.NewTradingContext(candles, nil)
			ctx.Position = tc.curPos
			got := IsMainBoxBreakdownFailure(ctx, tc.buyPosition, tc.mainBox)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsStopLoss(t *testing.T) {
	cases := []struct {
		name      string
		buyPos    int
		curPos    int
		close     float64
		buyPrice  float64
		threshold float64
		want      bool
	}{
		{"수익 중이면 false", 0, 5, 110, 100, -10.0, false},
		{"손실 -9.9% (임계 -10) → false", 0, 5, 90.1, 100, -10.0, false},
		{"손실 정확히 -10.0% → true (<=)", 0, 5, 90, 100, -10.0, true},
		{"손실 -15% → true", 0, 5, 85, 100, -10.0, true},
		{"매수 당일은 false", 0, 0, 50, 100, -10.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candles := make([]*box.Candle, tc.curPos+1)
			for i := range candles {
				candles[i] = mkSellCandle("", 0, 0, 0, 0, 0)
			}
			candles[tc.curPos] = mkSellCandle("", 0, 0, 0, tc.close, 0)
			ctx := box.NewTradingContext(candles, nil)
			ctx.Position = tc.curPos
			got := IsStopLoss(ctx, tc.buyPos, tc.buyPrice, tc.threshold)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsMainBoxPersistentBreakdown(t *testing.T) {
	// 매수 후 12일, MainBox 100, 그중 7일 < 100 → 7/12 > 0.5 → true
	mainBox := 100.0
	curPos := 12
	candles := make([]*box.Candle, curPos+1)
	for i := range candles {
		candles[i] = mkSellCandle("", 0, 0, 0, 110, 0)
	}
	// 매수 후 1~7일은 close=90 (MainBox 하향)
	for i := 1; i <= 7; i++ {
		candles[i].Close = 90
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = curPos
	if !IsMainBoxPersistentBreakdown(ctx, 0, mainBox) {
		t.Errorf("7/12 = 58%% > 50%% 이므로 true 기대")
	}

	// 6/12 = 50% 정확히는 false (> 0.5 가 아니라 비초과)
	candles[7].Close = 110
	if IsMainBoxPersistentBreakdown(ctx, 0, mainBox) {
		t.Errorf("6/12 = 50%% 는 50%% 초과가 아니므로 false 기대")
	}

	// 10캔들 이하면 false
	ctx.Position = 9
	if IsMainBoxPersistentBreakdown(ctx, 0, mainBox) {
		t.Errorf("position <= buy+10 이면 false 기대")
	}
}

func TestIsWeakFoundationFailure(t *testing.T) {
	// D+1: 음봉 + 전일 종가 이하
	candles := []*box.Candle{
		mkSellCandle("D+0", 100, 105, 95, 100, 0),
		mkSellCandle("D+1", 105, 105, 90, 95, 0), // 음봉 (95<105) + close 95 <= prev 100
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 1
	if !IsWeakFoundationFailure(ctx, 0) {
		t.Error("D+1 음봉+전일종가이하 → true 기대")
	}

	// D+1 양봉이면 false
	candles[1] = mkSellCandle("D+1", 95, 110, 95, 105, 0)
	if IsWeakFoundationFailure(ctx, 0) {
		t.Error("D+1 양봉 → false 기대")
	}
}
