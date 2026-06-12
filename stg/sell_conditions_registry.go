package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// SellConditionFn 은 매도 조건 평가 함수 타입.
// ctx + 활성 포지션 + 매도 설정을 받아 bool 반환.
type SellConditionFn func(ctx *box.TradingContext, pos *box.TradePosition, s SellSettings) bool

var sellConditionRegistry = map[string]SellConditionFn{}

// RegisterSellCondition 은 매도 조건명과 함수를 등록한다.
func RegisterSellCondition(name string, fn SellConditionFn) {
	sellConditionRegistry[name] = fn
}

// SellConditionRegistryGet 은 조건명으로 함수를 조회 (테스트/검증용).
func SellConditionRegistryGet(name string) (SellConditionFn, bool) {
	fn, ok := sellConditionRegistry[name]
	return fn, ok
}

// init 은 cond/sell_*.go의 모든 매도 조건을 이름으로 등록한다.
func init() {
	// Composite Path용 RecoveryPotential 평가 hook 주입
	recoveryEvaluatorHook = func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) box.RecoveryPotential {
		return cond.EvaluateRecoveryPotential(ctx, p, s.Recovery)
	}

	// ── Critical ────────────────────────────────────────────────
	RegisterSellCondition("IsCriticalFailure", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsCriticalFailure(ctx, p, s.Critical)
	})

	// ── Profit Taking ──────────────────────────────────────────
	RegisterSellCondition("IsGapUpTakeProfit", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsGapUpTakeProfit(ctx, p.BuyPosition, p.BuyPrice)
	})
	RegisterSellCondition("IsBBUpperBreakoutProfit", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsBBUpperBreakoutProfit(ctx, p.BuyPrice, s.BBUpperBreakoutMinBBPercent, s.BBUpperBreakoutMinProfitRatio)
	})

	// ── Loss Cutting ────────────────────────────────────────────
	RegisterSellCondition("IsMainBoxBreakdownFailure", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMainBoxBreakdownFailure(ctx, p.BuyPosition, p.MainBoxPrice)
	})
	RegisterSellCondition("IsMainBoxPersistentBreakdown", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMainBoxPersistentBreakdown(ctx, p.BuyPosition, p.MainBoxPrice)
	})
	RegisterSellCondition("IsMainBoxRecoveryFailure", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMainBoxRecoveryFailure(ctx, p.BuyPosition, p.MainBoxPrice, s.MainBoxRecoveryCheckPeriod)
	})
	RegisterSellCondition("IsMainBoxBBBreakdown", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMainBoxBreakdownWithBB(ctx, p.BuyPosition, p.MainBoxPrice,
			s.MainBoxBBBreakdownMinDays, s.MainBoxBBBreakdownMaxDays, s.MainBoxBBBreakdownBBPercentThreshold)
	})
	RegisterSellCondition("IsWeakFoundationFailure", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsWeakFoundationFailure(ctx, p.BuyPosition)
	})
	RegisterSellCondition("IsTrendEntryFailure1", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		eventHigh := cond.CalculateEventHighPriceForPosition(ctx, p)
		return cond.IsTrendEntryFailure1WithPosition(ctx, p, eventHigh)
	})
	RegisterSellCondition("IsTrendEntryFailure2", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsTrendEntryFailure2WithPosition(ctx, p)
	})
	RegisterSellCondition("IsWithin10Days", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return ctx.Position-p.BuyPosition < 10
	})
	RegisterSellCondition("IsStopLoss", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsStopLoss(ctx, p.BuyPosition, p.BuyPrice, s.StopLossThreshold)
	})

	// ── Adaptive / TimeDelayed ──────────────────────────────────
	RegisterSellCondition("IsAdaptiveStopLoss", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsAdaptiveStopLoss(ctx, p, s.Adaptive)
	})
	RegisterSellCondition("IsTimeDelayedStopLoss", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsTimeDelayedStopLoss(ctx, p, s.TimeDelayedStopLossRequiredDays, s.StopLossThreshold)
	})
	RegisterSellCondition("IsTimeDelayedStopLossEnabled", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return s.TimeDelayedStopLossEnabled
	})

	// ── Early Warning ───────────────────────────────────────────
	RegisterSellCondition("IsEarlyDrop", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsEarlyDrop(ctx, p.BuyPosition, p.BuyPrice, s.EarlyDropDaysAfterBuy, s.EarlyDropThresholdPercent)
	})
	RegisterSellCondition("IsEarlyMainBoxBreak", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsEarlyMainBoxBreak(ctx, p.BuyPosition, p.MainBoxPrice, s.EarlyMainBoxBreakDaysAfterBuy)
	})
	RegisterSellCondition("IsBBSqueezeExpansionWarning", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsBBSqueezeExpansionWarning(ctx,
			s.BBSqueezeExpansionLookback,
			s.BBSqueezeWidthThreshold,
			s.BBSqueezeExpansionWidthIncreaseRatio,
			s.BBSqueezeExpansionBBPercentThreshold,
			s.BBSqueezeExpansionRequiredCandles,
		)
	})

	// ── Technical / MA Reversal ─────────────────────────────────
	RegisterSellCondition("IsMA5MA20DeadCross", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMA5MA20DeadCross(ctx, p.BuyPosition, p.DefBoxIndex)
	})
	RegisterSellCondition("IsConsecutiveNegativeCandles", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsConsecutiveNegativeCandles(ctx, p.BuyPosition, p.MainBoxPrice, s.ConsecutiveNegativeLookback, s.ConsecutiveNegativeMinCount)
	})
	RegisterSellCondition("IsMAReversalBoxPattern", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMAReversalBoxPattern(ctx, s.MAReversalBoxLookbackPeriod)
	})

	// ── Period Expiry / Extension ───────────────────────────────
	RegisterSellCondition("IsPeriodExpired", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		if !s.AutoLiquidateOnExpiry {
			return false
		}
		return cond.IsPeriodExpired(ctx, p, s.MaxHoldingPeriod)
	})
	RegisterSellCondition("IsExtensionActive", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsExtensionActive(p)
	})
	RegisterSellCondition("IsMA5BreakdownDuringExtension", func(ctx *box.TradingContext, p *box.TradePosition, s SellSettings) bool {
		return cond.IsMA5BreakdownDuringExtension(ctx, p)
	})
	// 주의: PeriodExpiryPlusSellSignal 룰은 별도 조건이 필요 없다 — 룰 엔진의 5-Path 결정에서
	// IsExtensionActive + 다른 Individual 신호 발생 여부를 직접 검사한다 (makeSellDecision의 Extension Path 분기).
}
