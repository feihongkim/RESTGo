package cond

import (
	"math"

	"RESTGo/box"
)

// IsMainBoxBreakdownFailure 은 매수 후 1~2일 이내 음봉으로 MainBox 가격 아래로 하락했는지 확인한다.
// C# LossCuttingEvaluator.IsMainBoxBreakdownFailure 포팅.
func IsMainBoxBreakdownFailure(ctx *box.TradingContext, buyPosition int, mainBoxPrice float64) bool {
	if ctx.Position > buyPosition+2 {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	return IsNegativeCandle(cur) && cur.Close < mainBoxPrice
}

// IsWeakFoundationFailure 는 연약지반 매수 직후 반등 실패 패턴을 확인한다.
// C# LossCuttingEvaluator.IsWeakFoundationFailure 포팅.
// - 매수 후 1일차: 음봉 + 전일 종가 이하 마감
// - 매수 후 2일차: 1일차 음봉 + 1일차가 매수일보다 높게 마감 + 2일차 음봉 + 2일차가 매수일 종가 이하
func IsWeakFoundationFailure(ctx *box.TradingContext, buyPosition int) bool {
	pos := ctx.Position
	candles := ctx.CandleList
	if pos == buyPosition+1 {
		return IsNegativeCandle(candles[pos]) && candles[pos].Close <= candles[pos-1].Close
	}
	if pos == buyPosition+2 {
		return IsNegativeCandle(candles[pos-1]) &&
			candles[pos-1].Close > candles[pos-2].Close &&
			IsNegativeCandle(candles[pos]) &&
			candles[pos].Close <= candles[pos-2].Close
	}
	return false
}

// IsTrendEntryFailure1 은 이벤트 고점 돌파 실패 패턴을 확인한다.
// C# LossCuttingEvaluator.IsTrendEntryFailure1 포팅.
func IsTrendEntryFailure1(ctx *box.TradingContext, eventHighPrice float64) bool {
	pos := ctx.Position
	candles := ctx.CandleList
	if pos < 1 {
		return false
	}
	prev := candles[pos-1]
	cur := candles[pos]
	return IsPositiveCandle(prev) &&
		prev.High >= eventHighPrice &&
		prev.Close <= eventHighPrice &&
		cur.Open > eventHighPrice &&
		cur.Close < eventHighPrice
}

// IsTrendEntryFailure2 는 임시 고점 돌파 실패 패턴을 확인한다.
// C# LossCuttingEvaluator.IsTrendEntryFailure2 포팅.
func IsTrendEntryFailure2(ctx *box.TradingContext, tempHighPrice float64) bool {
	pos := ctx.Position
	candles := ctx.CandleList
	if pos < 1 {
		return false
	}
	prev := candles[pos-1]
	cur := candles[pos]
	return prev.High >= tempHighPrice &&
		prev.Open < tempHighPrice &&
		cur.Open > tempHighPrice &&
		cur.Close < tempHighPrice
}

// IsTrendEntryFailure1WithPosition 은 포지션의 EventHighPrice를 받아서 IsTrendEntryFailure1 평가.
// C#의 VirtualTrading.BuyPosition 임시 변경 트릭 대신 EventHighPrice를 명시적으로 받는다.
func IsTrendEntryFailure1WithPosition(ctx *box.TradingContext, pos *box.TradePosition, eventHighPrice float64) bool {
	return IsTrendEntryFailure1(ctx, eventHighPrice)
}

// IsTrendEntryFailure2WithPosition 은 LastOscilloPosition·TempHighPrice 계산을 포함한 래퍼.
// C# LossCuttingEvaluator.IsTrendEntryFailure2WithPosition 포팅.
func IsTrendEntryFailure2WithPosition(ctx *box.TradingContext, pos *box.TradePosition) bool {
	lastOscilloPos := LastOscilloPositionByPrice(ctx.CandleList, pos.DefBoxPrice, pos.PenPosition, ctx.Position)
	delta := ctx.Position - lastOscilloPos
	if delta > 2 || delta <= 0 {
		return false
	}
	tempHighPrice := HighPriceInRange(ctx.CandleList, pos.PenPosition, lastOscilloPos-1)
	return IsTrendEntryFailure2(ctx, tempHighPrice)
}

// IsMainBoxPersistentBreakdown 은 매수 이후 종가 < MainBoxPrice인 캔들 비율이 50% 초과인지 확인한다.
// C# LossCuttingEvaluator.IsMainBoxPersistentBreakdown 포팅.
func IsMainBoxPersistentBreakdown(ctx *box.TradingContext, buyPosition int, mainBoxPrice float64) bool {
	pos := ctx.Position
	if pos <= buyPosition+10 {
		return false
	}
	candles := ctx.CandleList
	total := 0
	breakdown := 0
	for i := buyPosition + 1; i <= pos; i++ {
		total++
		if candles[i].Close < mainBoxPrice {
			breakdown++
		}
	}
	if total == 0 {
		return false
	}
	return float64(breakdown)/float64(total) > 0.50
}

// IsStopLoss 는 수익률이 stopLossThresholdPct(%) 이하면 손절 신호를 반환한다.
// C# LossCuttingEvaluator.IsStopLoss 포팅. 기본 임계값 -20.0% (C# Settings.StopLossThreshold.
// C# 평가자 doc 주석의 -10.0%는 낡은 값, 실제 Settings 기본값은 -20.0).
func IsStopLoss(ctx *box.TradingContext, buyPosition int, buyPrice, stopLossThresholdPct float64) bool {
	if ctx.Position <= buyPosition {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	returnPct := CalculateReturnPercentage(cur.Close, buyPrice)
	return returnPct <= stopLossThresholdPct
}

// IsMainBoxRecoveryFailure 는 MainBox 붕괴 후 반등 시도 실패 패턴을 확인한다.
// C# LossCuttingEvaluator.IsMainBoxRecoveryFailure 포팅.
// 조건: 매수 후 checkPeriod 이내 + MainBox 아래 하락 이력 + 최근 3일 내 양봉 + MA5<MA20 + 현재 종가 < MainBox
func IsMainBoxRecoveryFailure(ctx *box.TradingContext, buyPosition int, mainBoxPrice float64, checkPeriod int) bool {
	pos := ctx.Position
	if pos <= buyPosition || pos-buyPosition > checkPeriod {
		return false
	}
	candles := ctx.CandleList

	hasDroppedBelow := false
	for i := buyPosition + 1; i < pos; i++ {
		if candles[i].Close < mainBoxPrice {
			hasDroppedBelow = true
			break
		}
	}
	if !hasDroppedBelow {
		return false
	}

	hasBullish := false
	lookback := int(math.Min(3, float64(pos-buyPosition)))
	for i := pos - lookback; i < pos; i++ {
		if i > buyPosition && candles[i].Close > candles[i].Open {
			hasBullish = true
			break
		}
	}
	if !hasBullish {
		return false
	}

	cur := candles[pos]
	return IsPartialReversal(cur) && cur.Close < mainBoxPrice
}

// IsMainBoxBreakdownWithBB 는 MainBox 붕괴 + BB %B 하단 확인 손절 신호.
// C# LossCuttingEvaluator.IsMainBoxBreakdownWithBB 포팅.
// minDaysAfterBuy/maxDaysAfterBuy: 일자 범위, bbPercentThreshold: %B 임계.
func IsMainBoxBreakdownWithBB(ctx *box.TradingContext, buyPosition int, mainBoxPrice float64, minDaysAfterBuy, maxDaysAfterBuy int, bbPercentThreshold float64) bool {
	days := ctx.Position - buyPosition
	if days < minDaysAfterBuy || days > maxDaysAfterBuy {
		return false
	}
	cur := ctx.CandleList[ctx.Position]
	if cur.Close >= mainBoxPrice {
		return false
	}
	return cur.BBPercent < bbPercentThreshold
}
