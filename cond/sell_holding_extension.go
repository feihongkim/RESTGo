package cond

import "RESTGo/box"

// CanExtendHoldingOnExpiry 는 기간 만료 시 홀딩 연장 가능 여부를 판단한다.
// C# HoldingExtensionEvaluator.CanExtendHoldingOnExpiry 포팅.
//
// 조건 (모두 충족 시 연장 가능):
//  1. 종가 > MA5
//  2. MA 정배열 (MA5 > MA20 > MA60)
//  3. MA5 상승 추세 (Gradient > 0)
func CanExtendHoldingOnExpiry(ctx *box.TradingContext, pos *box.TradePosition) bool {
	cur := ctx.CandleList[ctx.Position]
	return cur.Close > cur.Ma5 &&
		IsProperArrangement(cur) &&
		IsMA5Rising(cur)
}

// IsMA5BreakdownDuringExtension 는 홀딩 연장 중 MA5 이탈 (종가 < MA5) 패턴을 감지한다.
// 연장 대기 상태가 아니면 항상 false.
// C# HoldingExtensionEvaluator.IsMA5BreakdownDuringExtension 포팅.
func IsMA5BreakdownDuringExtension(ctx *box.TradingContext, pos *box.TradePosition) bool {
	if !pos.IsWaitingForSellSignalAfterExpiry {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	return cur.Close < cur.Ma5
}

// IsExtensionActive 는 포지션이 홀딩 연장 대기 상태인지 단순 확인 (YAML 룰 가드용).
func IsExtensionActive(pos *box.TradePosition) bool {
	return pos.IsWaitingForSellSignalAfterExpiry
}

// IsPeriodExpired 는 보유 기간이 maxHoldingPeriod 이상인지 확인한다.
// C# SFunction.Helpers.CheckHoldingPeriodExpiry 포팅 (Settings.AutoLiquidateOnExpiry 가드는 룰 레이어에서 처리).
func IsPeriodExpired(ctx *box.TradingContext, pos *box.TradePosition, maxHoldingPeriod int) bool {
	return ctx.Position-pos.BuyPosition >= maxHoldingPeriod
}
