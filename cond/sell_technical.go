package cond

import "RESTGo/box"

// IsMA5MA20DeadCross 는 매수 후 2개 Box 통과 + MA5/MA20 기울기 역전 + MA20 붕괴 패턴.
// C# TechnicalSellEvaluator.IsMA5MA20DeadCross 포팅.
//
// 조건:
//  1. 매수 후 DefBox 이후 최소 2개 Box (지지선 + 저항선)
//  2. MA5 gradient 연속 2일 음수
//  3. MA20 gradient 연속 2일 양수
//  4. 음봉 + 종가 < MA20
func IsMA5MA20DeadCross(ctx *box.TradingContext, buyPosition, defBoxIndex int) bool {
	pos := ctx.Position
	if pos < 2 {
		return false
	}
	cur := ctx.CandleList[pos]
	prev := ctx.CandleList[pos-1]
	prevPrev := ctx.CandleList[pos-2]

	if !HasTwoBoxesAfterBuy(ctx, buyPosition, defBoxIndex) {
		return false
	}
	if prevPrev.Gradient >= 0 || prev.Gradient >= 0 {
		return false
	}
	if prevPrev.Gradient20 <= 0 || prev.Gradient20 <= 0 {
		return false
	}
	if !IsNegativeCandle(cur) {
		return false
	}
	return cur.Close < cur.Ma20
}

// IsConsecutiveNegativeCandles 는 MA5 아래에서 연속 음봉 패턴을 확인한다.
// C# TechnicalSellEvaluator.IsConsecutiveNegativeCandles 포팅.
//
// 조건:
//  1. 종가 < MA5
//  2. [max(buyPos+1, pos-lookback+1), pos] 구간에서 음봉 개수 >= minCount
//  3. MA5 gradient < 0
//  4. 종가 < mainBoxPrice
func IsConsecutiveNegativeCandles(ctx *box.TradingContext, buyPosition int, mainBoxPrice float64, lookback, minCount int) bool {
	pos := ctx.Position
	cur := ctx.CandleList[pos]
	if cur.Close >= cur.Ma5 {
		return false
	}
	if cur.Gradient >= 0 {
		return false
	}
	if cur.Close >= mainBoxPrice {
		return false
	}
	startPos := pos - lookback + 1
	if startPos < buyPosition+1 {
		startPos = buyPosition + 1
	}
	negCount := 0
	for i := startPos; i <= pos; i++ {
		if IsNegativeCandle(ctx.CandleList[i]) {
			negCount++
		}
	}
	return negCount >= minCount
}
