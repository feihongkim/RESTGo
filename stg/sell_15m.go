package stg

import (
	"fmt"

	"RESTGo/box"
)

// Evaluate15mExit 는 per_candle 진입 포지션에 E1~E4 청산 규칙을 적용한다.
//
// 우선순위: E1 > E4 > E2 > E3
//
// 인자:
//
//	pos        — 평가할 포지션
//	cur        — 현재 캔들
//	next       — 다음 캔들 (nil 허용 — 마지막 봉일 때)
//	curIdx     — ctx.Position (현재 봉 인덱스, E4 시간 계산용)
//	s          — 분석 설정
//
// 반환값:
//
//	shouldExit  — 청산해야 하면 true
//	reason      — 청산 사유 문자열
//	fillPrice   — 체결 원본가
//	weight      — 청산 비율 (0 < w <= 1.0; E2는 TargetSellWeight, 나머지는 1.0)
//
// 주의: E2는 pos.TargetHit=true 부수효과가 있다.
func Evaluate15mExit(pos *box.TradePosition, cur *box.Candle, next *box.Candle, curIdx int, s Settings) (shouldExit bool, reason string, fillPrice float64, weight float64) {
	if cur == nil || !pos.IsActive {
		return false, "", 0, 0
	}

	// ── E1: 초기 ATR 스탑 (봉 내 도달, cur.LowOrigin 기준) ─────────────
	stopLine := pos.BuyPriceOrigin - s.ATRStopMultiplier*cur.ATR
	if cur.LowOrigin < stopLine {
		// 체결가 = stopLine - ATR*0.05 (슬리피지 근사)
		fill := stopLine - cur.ATR*0.05
		return true, "E1_ATRStop", fill, 1.0
	}

	// ── E4: 시간 청산 (종가 확정 → 다음 봉 시가 체결) ────────────────
	barsHeld := curIdx - pos.BuyPosition
	if s.TimeExitBars > 0 && barsHeld >= s.TimeExitBars {
		fill := 0.0
		if next != nil {
			fill = next.OpenOrigin
		} else {
			fill = cur.CloseOrigin
		}
		return true, "E4_TimeExit", fill, 1.0
	}

	// ── E2: 타겟 부분청산 (봉 내 도달, cur.HighOrigin 기준) ──────────
	if !pos.TargetHit {
		targetLine := pos.BuyPriceOrigin + s.ATRTargetMultiplier*cur.ATR
		if cur.HighOrigin >= targetLine {
			pos.TargetHit = true // E3 활성화 — 부수효과
			return true, "E2_Target", targetLine, s.TargetSellWeight
		}
	}

	// ── E3: 트레일링 EMA (종가 확정 → 다음 봉 시가 체결) ─────────────
	if pos.TargetHit && cur.EMA21 != 0 && cur.CloseOrigin < cur.EMA21 {
		fill := 0.0
		if next != nil {
			fill = next.OpenOrigin
		} else {
			fill = cur.CloseOrigin
		}
		return true, "E3_TrailingEMA", fill, 1.0
	}

	return false, "", 0, 0
}

// smallRemainingThreshold15m 은 소량 잔여 포지션을 전량 청산으로 전환하는 임계값.
const smallRemainingThreshold15m = 0.125

// execute15mExit 는 E1~E4 청산 결과를 포지션에 반영한다.
// weight < 1.0 이면 부분청산(E2), 그 외는 전량 청산.
func execute15mExit(ctx *box.TradingContext, pos *box.TradePosition, reason string, fillPrice float64, weight float64) {
	if fillPrice <= 0 {
		return
	}
	// 소량 잔여 → 전량 청산으로 전환
	if pos.RemainingQuantity <= smallRemainingThreshold15m {
		weight = 1.0
	}

	sellQty := pos.RemainingQuantity * weight
	newRemaining := pos.RemainingQuantity - sellQty
	if newRemaining < 0 {
		newRemaining = 0
	}

	cur := ctx.CandleList[ctx.Position]
	exec := box.SellExecution{
		ExecutionOrder:     len(pos.SellExecutions) + 1,
		SellReason:         reason,
		SellQuantity:       sellQty,
		RemainingAfterSell: newRemaining,
		Weight:             weight,
		SellPrice:          fillPrice, // 원본가와 동일 (15m 단타는 스케일 없음)
		SellPriceOrigin:    fillPrice,
		SellDate:           cur.Date,
		SellPosition:       ctx.Position,
		HoldingDays:        pos.HoldingDays(ctx.Position),
	}

	if pos.BuyPriceOrigin != 0 {
		exec.PartialReturnRate = (fillPrice - pos.BuyPriceOrigin) / pos.BuyPriceOrigin * 100
	}
	if pos.FeeRate > 0 || pos.SlippageRate > 0 {
		costRate := pos.FeeRate + pos.SlippageRate
		exec.SellCost = fillPrice * costRate
		exec.NetPartialReturn = exec.PartialReturnRate - costRate*2*100
	}

	pos.SellExecutions = append(pos.SellExecutions, exec)
	pos.RemainingQuantity = newRemaining

	if pos.IsFullyLiquidated() {
		pos.IsActive = false
		pos.SellDate = exec.SellDate
		pos.SellPrice = fillPrice
		pos.SellPriceOrigin = fillPrice
		pos.SellPosition = ctx.Position
		pos.SellReason = fmt.Sprintf("%s_FINAL", reason)
		pos.ReturnRate = CalculateWeightedAverageReturn(pos)
		if pos.FeeRate > 0 || pos.SlippageRate > 0 {
			costRate := (pos.FeeRate + pos.SlippageRate) * 2 * 100
			pos.NetReturnRate = pos.ReturnRate - costRate
		}
		ctx.SellHelper = pos.SellReason
		if ctx.ActivePositionsCount() == 0 {
			ctx.BuyOn = false
		}
	}
}
