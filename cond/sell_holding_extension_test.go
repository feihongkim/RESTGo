package cond

import (
	"testing"

	"RESTGo/box"
)

func TestIsPeriodExpired(t *testing.T) {
	candles := make([]*box.Candle, 30)
	for i := range candles {
		candles[i] = mkSellCandle("", 0, 0, 0, 0, 0)
	}
	ctx := box.NewTradingContext(candles, nil)
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")

	ctx.Position = 19
	if IsPeriodExpired(ctx, pos, 20) {
		t.Errorf("보유 19일은 < 20 → false 기대")
	}
	ctx.Position = 20
	if !IsPeriodExpired(ctx, pos, 20) {
		t.Errorf("보유 20일 >= 20 → true 기대")
	}
}

func TestCanExtendHoldingOnExpiry(t *testing.T) {
	candles := []*box.Candle{mkSellCandle("", 0, 0, 0, 100, 0)}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 0
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")

	// 종가 > MA5 + 정배열 + MA5 상승 → true
	candles[0].Ma5 = 95
	candles[0].Ma20 = 90
	candles[0].Ma60 = 85
	candles[0].Gradient = 1.0
	if !CanExtendHoldingOnExpiry(ctx, pos) {
		t.Errorf("모든 조건 충족 → true 기대")
	}

	// MA5 하락 → false
	candles[0].Gradient = -0.5
	if CanExtendHoldingOnExpiry(ctx, pos) {
		t.Errorf("MA5 하락 → false 기대")
	}
}

func TestIsMA5BreakdownDuringExtension(t *testing.T) {
	candles := []*box.Candle{mkSellCandle("", 0, 0, 0, 90, 0)}
	candles[0].Ma5 = 95
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 0
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")

	// 연장 비활성 → false
	pos.IsWaitingForSellSignalAfterExpiry = false
	if IsMA5BreakdownDuringExtension(ctx, pos) {
		t.Errorf("연장 비활성 → false 기대")
	}

	// 연장 활성 + 종가<MA5 → true
	pos.IsWaitingForSellSignalAfterExpiry = true
	if !IsMA5BreakdownDuringExtension(ctx, pos) {
		t.Errorf("연장 중 종가<MA5 → true 기대")
	}
}
