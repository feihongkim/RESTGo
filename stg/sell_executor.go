package stg

import (
	"fmt"

	"RESTGo/box"
)

// ExecutePartialSell 은 weight 비율에 따라 부분 매도를 실행하고
// SellExecution 기록을 추가, RemainingQuantity 감소, 완전 청산 시 IsActive=false 설정한다.
// C# SFunction.PositionManagement.ExecutePartialSell 포팅.
//
// 반환값: 실제로 실행되었는지 여부 (잔량이 minimum_execution_size 미만이거나 비활성 포지션이면 false).
func ExecutePartialSell(ctx *box.TradingContext, pos *box.TradePosition, reason string, weight float64, s SellSettings) bool {
	if !pos.IsActive || pos.RemainingQuantity <= 0 {
		return false
	}

	// 소량 잔여 → 전량 청산으로 전환
	if pos.RemainingQuantity <= s.SmallRemainingThreshold {
		weight = 1.0
	}

	sellQty := pos.RemainingQuantity * weight
	newRemaining := pos.RemainingQuantity - sellQty
	if newRemaining < 0 {
		newRemaining = 0
	}

	// 최소 실행 크기 미만 → 스킵
	if sellQty < s.MinimumExecutionSize {
		return false
	}

	cur := ctx.CandleList[ctx.Position]
	exec := box.SellExecution{
		ExecutionOrder:     len(pos.SellExecutions) + 1,
		SellReason:         reason,
		SellQuantity:       sellQty,
		RemainingAfterSell: newRemaining,
		Weight:             weight,
		SellPrice:          cur.Close,
		SellPriceOrigin:    cur.CloseOrigin,
		SellDate:           cur.Date,
		SellPosition:       ctx.Position,
		HoldingDays:        pos.HoldingDays(ctx.Position),
	}
	if pos.BuyPrice != 0 {
		exec.PartialReturnRate = (cur.Close - pos.BuyPrice) / pos.BuyPrice * 100
	}
	// 비용 차감 수익률 계산 (포지션에 FeeRate/SlippageRate가 설정된 경우)
	// 주의: 현재 체결 방식은 same-candle fill (신호 발생 캔들 = 체결 캔들).
	// next-candle open 체결 방식은 향후 개선 예정.
	if pos.FeeRate > 0 || pos.SlippageRate > 0 {
		costRate := pos.FeeRate + pos.SlippageRate
		exec.SellCost = cur.CloseOrigin * costRate
		exec.NetPartialReturn = exec.PartialReturnRate - costRate*2*100 // round-trip cost in %
	}

	pos.SellExecutions = append(pos.SellExecutions, exec)
	pos.RemainingQuantity = newRemaining

	if pos.IsFullyLiquidated() {
		pos.IsActive = false
		pos.SellDate = exec.SellDate
		pos.SellPrice = exec.SellPrice
		pos.SellPriceOrigin = exec.SellPriceOrigin
		pos.SellPosition = exec.SellPosition
		pos.SellReason = fmt.Sprintf("%s_FINAL", reason)
		// C# BacktestUtilities.CalculateWeightedAverageReturn: 부분 매도 수량 가중평균
		// (마지막 체결가 기준이 아님 — 부분 매도가 여러 번이면 결과가 다름)
		pos.ReturnRate = CalculateWeightedAverageReturn(pos)
		// 최종 NetReturnRate: 매수 비용 포함 전체 왕복 비용 차감
		if pos.FeeRate > 0 || pos.SlippageRate > 0 {
			costRate := (pos.FeeRate + pos.SlippageRate) * 2 * 100 // round-trip in %
			pos.NetReturnRate = pos.ReturnRate - costRate
		}
		// C# VirtualTrading: 완전 청산 시 SellHelper = SellReason (FollowUp 재진입 게이트용)
		ctx.SellHelper = pos.SellReason
		// C# VirtualTrading.cs:410 — 모든 포지션 청산 완료 시 BuyOn 리셋
		// (후보군1 추가매수 경로의 !BuyOn 게이트가 다시 열린다)
		if ctx.ActivePositionsCount() == 0 {
			ctx.BuyOn = false
		}
	}
	return true
}

// CalculateWeightedAverageReturn 은 매도 실행들의 수량 가중평균 수익률(%).
// C# helpers/BacktestUtilities.CalculateWeightedAverageReturn 포팅.
func CalculateWeightedAverageReturn(pos *box.TradePosition) float64 {
	if len(pos.SellExecutions) == 0 {
		return 0
	}
	weightedSum := 0.0
	totalQty := 0.0
	for _, e := range pos.SellExecutions {
		weightedSum += e.SellQuantity * e.PartialReturnRate
		totalQty += e.SellQuantity
	}
	if totalQty == 0 {
		return 0
	}
	return weightedSum / totalQty
}
