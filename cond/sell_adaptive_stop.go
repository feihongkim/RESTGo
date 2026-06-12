package cond

import (
	"math"

	"RESTGo/box"
)

// AdaptiveStopLossParams 는 적응형 손절 임계값 계산에 필요한 파라미터 묶음 (C# Settings 매핑).
type AdaptiveStopLossParams struct {
	BaseThreshold             float64 // 기본 임계값 (음수 %, 예: -20.0)
	HighVolatilityMultiplier  float64 // 고변동성 배수 (예: 1.5)
	LowVolatilityMultiplier   float64 // 저변동성 배수 (예: 1.0)
	MinThreshold              float64 // 하한 (예: -30.0)
	MaxThreshold              float64 // 상한 (예: -10.0)
	VolatilityLookbackDays    int     // 변동성 계산 일수 (예: 20)
	HighVolatilityThreshold   float64 // 고변동성 기준 (예: 0.10)
	MediumVolatilityThreshold float64 // 중변동성 기준 (예: 0.05)
	StrongStructureThreshold  float64 // 강구조 기준 (예: 0.15)
	MediumStructureThreshold  float64 // 중구조 기준 (예: 0.10)
}

// CalculateAdaptiveStopLossThreshold 는 변동성과 Box 구조 강도를 반영한 손절 임계값(%)을 반환한다.
// C# AdaptiveStopLossEvaluator.CalculateAdaptiveThreshold 포팅.
func CalculateAdaptiveStopLossThreshold(ctx *box.TradingContext, pos *box.TradePosition, p AdaptiveStopLossParams) float64 {
	volatility := calculateVolatility(ctx, pos.BuyPosition, p.VolatilityLookbackDays)
	boxStrength := calculateBoxStrength(pos)

	var volMult float64
	switch {
	case volatility > p.HighVolatilityThreshold:
		volMult = p.HighVolatilityMultiplier
	case volatility > p.MediumVolatilityThreshold:
		volMult = 1.2
	default:
		volMult = p.LowVolatilityMultiplier
	}

	var structMult float64
	switch {
	case boxStrength > p.StrongStructureThreshold:
		structMult = 1.3
	case boxStrength > p.MediumStructureThreshold:
		structMult = 1.1
	default:
		structMult = 0.9
	}

	adaptive := p.BaseThreshold * volMult * structMult
	if adaptive < p.MinThreshold {
		adaptive = p.MinThreshold
	}
	if adaptive > p.MaxThreshold {
		adaptive = p.MaxThreshold
	}
	return adaptive
}

// IsAdaptiveStopLoss 은 매수 후 현재 수익률이 적응형 임계값 이하면 손절 신호.
// C# AdaptiveStopLossEvaluator.IsAdaptiveStopLoss 포팅.
func IsAdaptiveStopLoss(ctx *box.TradingContext, pos *box.TradePosition, p AdaptiveStopLossParams) bool {
	if ctx.Position <= pos.BuyPosition {
		return false
	}
	threshold := CalculateAdaptiveStopLossThreshold(ctx, pos, p)
	cur := ctx.CandleList[ctx.Position]
	return CalculateReturnPercentage(cur.Close, pos.BuyPrice) <= threshold
}

// calculateVolatility 는 [start, start+days] 구간 일일 수익률의 표준편차를 반환한다.
// 데이터 부족 시 기본값 0.05.
func calculateVolatility(ctx *box.TradingContext, startPos, days int) float64 {
	candles := ctx.CandleList
	end := startPos + days
	if end >= len(candles) {
		end = len(candles) - 1
	}
	begin := startPos
	if begin < 1 {
		begin = 1
	}

	returns := make([]float64, 0, end-begin+1)
	for i := begin; i <= end; i++ {
		if candles[i-1].Close == 0 {
			continue
		}
		r := (candles[i].Close - candles[i-1].Close) / candles[i-1].Close
		returns = append(returns, r)
	}
	if len(returns) == 0 {
		return 0.05
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(returns))
	return math.Sqrt(variance)
}

// calculateBoxStrength 는 |MainBoxPrice - DefBoxPrice| / DefBoxPrice 를 반환한다.
// DefBoxPrice가 0이면 기본값 0.1.
func calculateBoxStrength(pos *box.TradePosition) float64 {
	if pos.DefBoxPrice == 0 {
		return 0.1
	}
	return math.Abs(pos.MainBoxPrice-pos.DefBoxPrice) / pos.DefBoxPrice
}

// IsTimeDelayedStopLoss 는 손절 조건이 requiredDays 연속 지속되는지 확인한다.
// C# TimeDelayedStopLossEvaluator.IsTimeDelayedStopLoss 포팅.
// stopLossThresholdPct: IsStopLoss용 임계값(기본 -20.0, C# Settings.StopLossThreshold).
func IsTimeDelayedStopLoss(ctx *box.TradingContext, pos *box.TradePosition, requiredDays int, stopLossThresholdPct float64) bool {
	consecutive := 0
	for i := ctx.Position; i > pos.BuyPosition && consecutive < requiredDays; i-- {
		if isStopLossAt(ctx, i, pos.BuyPosition, pos.BuyPrice, stopLossThresholdPct) {
			consecutive++
		} else {
			break
		}
	}
	return consecutive >= requiredDays
}

// isStopLossAt 는 임시 position 기준으로 손절 조건을 평가한다 (ctx 변경 없이).
func isStopLossAt(ctx *box.TradingContext, position, buyPosition int, buyPrice, stopLossThresholdPct float64) bool {
	if position <= buyPosition || position >= len(ctx.CandleList) {
		return false
	}
	c := ctx.CandleList[position]
	return CalculateReturnPercentage(c.Close, buyPrice) <= stopLossThresholdPct
}

// CountDaysBelowMainBox 은 현재부터 역순으로 MainBox 아래 연속 일수를 센다.
// C# TimeDelayedStopLossEvaluator.CountDaysBelowMainBox 포팅.
func CountDaysBelowMainBox(ctx *box.TradingContext, pos *box.TradePosition) int {
	consecutive := 0
	for i := ctx.Position; i > pos.BuyPosition; i-- {
		if ctx.CandleList[i].Close < pos.MainBoxPrice {
			consecutive++
		} else {
			break
		}
	}
	return consecutive
}
