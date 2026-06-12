package cond

import (
	"math"

	"RESTGo/box"
)

// ============================================================================
// 캔들 패턴 헬퍼 (C# CandlePatternHelper.cs 포팅)
// ============================================================================

// IsNegativeCandle 은 음봉(종가 < 시가)인지 확인
func IsNegativeCandle(c *box.Candle) bool {
	return c.Close < c.Open
}

// IsPositiveCandle 은 양봉(종가 > 시가)인지 확인
func IsPositiveCandle(c *box.Candle) bool {
	return c.Close > c.Open
}

// IsDoji 는 도지 패턴(시가≈종가, 기본 허용오차 0.1%)인지 확인
func IsDoji(c *box.Candle, threshold float64) bool {
	if c.Open == 0 {
		return false
	}
	return math.Abs(c.Close-c.Open)/c.Open < threshold
}

// GetBodySize 는 캔들 몸통 크기 (|close - open|)
func GetBodySize(c *box.Candle) float64 {
	return math.Abs(c.Close - c.Open)
}

// GetBodyRatio 는 캔들 몸통 비율 (|close - open| / open)
func GetBodyRatio(c *box.Candle) float64 {
	if c.Open == 0 {
		return 0
	}
	return math.Abs(c.Close-c.Open) / c.Open
}

// ============================================================================
// 이동평균 정렬 헬퍼 (C# MovingAverageHelper.cs 포팅)
// ============================================================================

// IsProperArrangement 는 MA 정배열 (MA5 > MA20 > MA60)
func IsProperArrangement(c *box.Candle) bool {
	return c.Ma5 > c.Ma20 && c.Ma20 > c.Ma60
}

// IsReversedArrangement 는 MA 역배열 (MA5 < MA20 < MA60)
func IsReversedArrangement(c *box.Candle) bool {
	return c.Ma5 < c.Ma20 && c.Ma20 < c.Ma60
}

// IsPartialReversal 은 MA 부분 역배열 (MA5 < MA20)
func IsPartialReversal(c *box.Candle) bool {
	return c.Ma5 < c.Ma20
}

// IsMA5BelowAll 은 MA5가 모든 MA 아래 (Close < MA5 < MA20 < MA60)
func IsMA5BelowAll(c *box.Candle) bool {
	return c.Close < c.Ma5 && c.Ma5 < c.Ma20 && c.Ma20 < c.Ma60
}

// IsMA5Rising 은 MA5 상승 추세 (Gradient > 0)
func IsMA5Rising(c *box.Candle) bool {
	return c.Gradient > 0
}

// IsMA20Rising 은 MA20 상승 추세 (Gradient20 > 0)
func IsMA20Rising(c *box.Candle) bool {
	return c.Gradient20 > 0
}

// IsMA5Falling 은 MA5 하락 추세 (Gradient < 0)
func IsMA5Falling(c *box.Candle) bool {
	return c.Gradient < 0
}

// IsMA20Falling 은 MA20 하락 추세 (Gradient20 < 0)
func IsMA20Falling(c *box.Candle) bool {
	return c.Gradient20 < 0
}

// ============================================================================
// 수익률 계산 헬퍼 (C# ReturnCalculator.cs 포팅)
// ============================================================================

// CalculateReturnPercentage 는 수익률(%) — 예: 5.5 = 5.5%
func CalculateReturnPercentage(currentPrice, buyPrice float64) float64 {
	if buyPrice == 0 {
		return 0
	}
	return ((currentPrice - buyPrice) / buyPrice) * 100
}

// CalculateDailyReturn 은 일간 수익률(%) — 예: 1.2 = 1.2%
func CalculateDailyReturn(closePrice, previousClose float64) float64 {
	if previousClose == 0 {
		return 0
	}
	return ((closePrice - previousClose) / previousClose) * 100
}

// CalculateReturnRatio 는 수익률(비율) — 예: 0.055 = 5.5%
func CalculateReturnRatio(currentPrice, buyPrice float64) float64 {
	if buyPrice == 0 {
		return 0
	}
	return (currentPrice - buyPrice) / buyPrice
}

// CalculateProfitLoss 는 손익금 = (현재가 - 매수가) × 수량
func CalculateProfitLoss(currentPrice, buyPrice, quantity float64) float64 {
	return (currentPrice - buyPrice) * quantity
}

// ============================================================================
// 매도 평가 공통 헬퍼
// (공용 오실로 헬퍼 HighPriceInRange/LastOscilloPositionByPrice는 buy_oscillator.go로 이동)
// ============================================================================

// HasTwoBoxesAfterBuy 는 매수 이후 DefBox 다음에 지지선(BoxType=0) + 저항선(BoxType=1) 순서로
// 2개의 Box가 생성되었는지 확인한다.
// C# PositionTrackingHelper.HasTwoBoxesAfterBuy 포팅.
func HasTwoBoxesAfterBuy(ctx *box.TradingContext, buyPosition, defBoxIndex int) bool {
	hasSupport := false
	hasResistance := false
	for i := defBoxIndex + 1; i < len(ctx.BoxList); i++ {
		b := ctx.BoxList[i]
		if b.BoxPosition <= buyPosition {
			continue
		}
		if b.BoxType == box.BoxTypeSupport && !hasSupport {
			hasSupport = true
		} else if b.BoxType == box.BoxTypeResistance && hasSupport && !hasResistance {
			hasResistance = true
			break
		}
	}
	return hasSupport && hasResistance
}

// CalculateEventHighPriceForPosition 은 매도 평가에서 사용하는 구간 내 최고가를 반환한다.
// 검색 시작 위치는 ctx.Position - 3*(currentPos - MainboxPos), 최소 0.
// DefBox 위치까지의 [searchStart, defboxPosition] 구간 최고가(High).
// C# SFunction.CalculateEventHighPriceForPosition 포팅.
func CalculateEventHighPriceForPosition(ctx *box.TradingContext, pos *box.TradePosition) float64 {
	if pos == nil || pos.DefBoxIndex < 0 || pos.DefBoxIndex >= len(ctx.BoxList) {
		return 0
	}
	distance := ctx.Position - pos.MainboxPosition
	searchStart := ctx.Position - 3*distance
	if searchStart < 0 {
		searchStart = 0
	}
	defBox := ctx.BoxList[pos.DefBoxIndex]
	return HighPriceInRange(ctx.CandleList, searchStart, defBox.BoxPosition)
}
