package cond

import (
	"testing"

	"RESTGo/box"
)

func TestIsGapUpTakeProfit(t *testing.T) {
	cases := []struct {
		name        string
		buyPosition int
		curPos      int
		open        float64
		buyPrice    float64
		want        bool
	}{
		{"D+1 시가 +10% 정확히 → true", 0, 1, 110, 100, true},
		{"D+1 시가 +15% → true", 0, 1, 115, 100, true},
		{"D+1 시가 +9.9% → false", 0, 1, 109.9, 100, false},
		{"D+2는 false", 0, 2, 130, 100, false},
		{"buyPrice 0이면 false", 0, 1, 110, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candles := make([]*box.Candle, tc.curPos+1)
			for i := range candles {
				candles[i] = mkSellCandle("", 0, 0, 0, 0, 0)
			}
			candles[tc.curPos].Open = tc.open
			ctx := box.NewTradingContext(candles, nil)
			ctx.Position = tc.curPos
			got := IsGapUpTakeProfit(ctx, tc.buyPosition, tc.buyPrice)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsBBUpperBreakoutProfit(t *testing.T) {
	candles := []*box.Candle{
		mkSellCandle("", 0, 0, 0, 0, 0),
		mkSellCandle("", 0, 0, 0, 110, 0),
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 1

	// %B > 0.95 + 수익률 > 8% → true
	candles[1].BBPercent = 0.96
	if !IsBBUpperBreakoutProfit(ctx, 100, 0.95, 0.08) {
		t.Errorf("%%B=0.96 + 수익률 10%% → true 기대")
	}

	// %B = 0.95 (>=가 아니라 >) → false
	candles[1].BBPercent = 0.95
	if IsBBUpperBreakoutProfit(ctx, 100, 0.95, 0.08) {
		t.Errorf("%%B 정확히 0.95는 false 기대")
	}

	// %B 충족하지만 수익률 미달 → false
	candles[1].BBPercent = 0.99
	if IsBBUpperBreakoutProfit(ctx, 105, 0.95, 0.08) {
		t.Errorf("수익률 %%5 < 임계 %%8 → false 기대")
	}
}
