package cond

import (
	"testing"

	"RESTGo/box"
)

func TestIsEarlyDrop(t *testing.T) {
	cases := []struct {
		name      string
		buyPos    int
		curPos    int
		close     float64
		buyPrice  float64
		days      int
		threshold float64
		want      bool
	}{
		{"D+1 -5% 정확히 → true (<=)", 0, 1, 95, 100, 3, -5.0, true},
		{"D+3 -10% → true", 0, 3, 90, 100, 3, -5.0, true},
		{"D+4는 기간 초과 → false", 0, 4, 50, 100, 3, -5.0, false},
		{"D+1 -4% → false", 0, 1, 96, 100, 3, -5.0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candles := make([]*box.Candle, tc.curPos+1)
			for i := range candles {
				candles[i] = mkSellCandle("", 0, 0, 0, 0, 0)
			}
			candles[tc.curPos].Close = tc.close
			ctx := box.NewTradingContext(candles, nil)
			ctx.Position = tc.curPos
			got := IsEarlyDrop(ctx, tc.buyPos, tc.buyPrice, tc.days, tc.threshold)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsEarlyMainBoxBreak(t *testing.T) {
	candles := []*box.Candle{
		mkSellCandle("", 0, 0, 0, 0, 0),
		mkSellCandle("", 0, 0, 0, 90, 0),
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 1

	// 종가 < MainBox + MA 역배열
	candles[1].Ma5 = 95
	candles[1].Ma20 = 100
	if !IsEarlyMainBoxBreak(ctx, 0, 92, 2) {
		t.Errorf("MainBox=92 + 종가90 + MA5<MA20 → true 기대")
	}

	// MA5 >= MA20 이면 false
	candles[1].Ma5 = 105
	candles[1].Ma20 = 100
	if IsEarlyMainBoxBreak(ctx, 0, 92, 2) {
		t.Errorf("MA 정배열 → false 기대")
	}
}
