package cond

import "RESTGo/box"

// IsGapUpTakeProfit 은 매수 후 1일차 시가가 매수가 대비 10% 이상 상승했는지 확인한다.
// C# ProfitTakingEvaluator.IsGapUpTakeProfit 포팅.
// 함수명에 "20Percent"가 남아있던 C# 원본 변수와 달리 실제 임계값은 10%다.
func IsGapUpTakeProfit(ctx *box.TradingContext, buyPosition int, buyPrice float64) bool {
	if ctx.Position != buyPosition+1 {
		return false
	}
	if buyPrice == 0 {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	gapUpPct := ((cur.Open - buyPrice) / buyPrice) * 100
	return gapUpPct >= 10.0
}

// IsBBUpperBreakoutProfit 은 BB %B 상단 돌파 + 수익률 임계값 초과 시 익절 신호를 반환한다.
// C# ProfitTakingEvaluator.IsBBUpperBreakoutProfit 포팅.
// minBBPercent: %B 임계 (예: 0.95), minProfitRatio: 수익률 임계(비율, 예: 0.08).
func IsBBUpperBreakoutProfit(ctx *box.TradingContext, buyPrice, minBBPercent, minProfitRatio float64) bool {
	if buyPrice == 0 {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	if cur.BBPercent <= minBBPercent {
		return false
	}
	returnPct := ((cur.Close - buyPrice) / buyPrice) * 100
	return returnPct > minProfitRatio*100
}
