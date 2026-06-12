package cond

import "RESTGo/box"

// IsMAReversalBoxPattern 은 MA 역배열 + 최근 저항선 형성 + MA5 아래 거래 패턴.
// C# MAReversalEvaluator.IsMAReversalBoxPattern 포팅.
//
// 조건:
//  1. MA5 < MA20 (역배열)
//  2. MA5 gradient < 0
//  3. [pos-lookback, pos] 구간 내 BoxType=Resistance 박스 존재
//  4. 종가 < MA5
func IsMAReversalBoxPattern(ctx *box.TradingContext, lookbackPeriod int) bool {
	pos := ctx.Position
	cur := ctx.CandleList[pos]

	if cur.Ma5 >= cur.Ma20 {
		return false
	}
	if cur.Gradient >= 0 {
		return false
	}

	searchStart := pos - lookbackPeriod
	if searchStart < 0 {
		searchStart = 0
	}
	hasResistance := false
	for _, b := range ctx.BoxList {
		if b.BoxType == box.BoxTypeResistance &&
			b.BoxPosition >= searchStart &&
			b.BoxPosition <= pos {
			hasResistance = true
			break
		}
	}
	if !hasResistance {
		return false
	}
	return cur.Close < cur.Ma5
}
