package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// init 에서 모든 조건 함수를 이름으로 등록
func init() {
	// ── DefBox 돌파 ───────────────────────────────────────────
	RegisterCondition("IsDefBoxBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsDefBoxBreakout(ctx)
	})

	// ── Box 구조 조건 ─────────────────────────────────────────
	RegisterCondition("IsCloseNearDefboxPrice", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsCloseNearDefboxPrice(ctx, s.DefBoxNearPriceThreshold, s.MainBoxNearPriceThreshold)
	})
	RegisterCondition("IsMainboxCloserThanCurrentPosition", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMainboxCloserThanCurrentPosition(ctx)
	})
	RegisterCondition("IsMainboxDistanceTwiceOrMore", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMainboxDistanceTwiceOrMore(ctx)
	})
	RegisterCondition("IsSingleBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsSingleBreakout(ctx)
	})
	RegisterCondition("IsBoxConditionValid", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsAdditionalBoxConditionValid(ctx)
	})
	RegisterCondition("IsBoxConditionValid2", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBoxConditionValid2(ctx)
	})
	RegisterCondition("IsBoxCountBetween2", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBoxCountBetweenDefAndMainLessThanOrEqual(ctx, 2)
	})
	RegisterCondition("IsBoxCountBetween5", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBoxCountBetweenDefAndMainLessThanOrEqual(ctx, 5)
	})
	RegisterCondition("IsBoxDensityValidByCount", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBoxDensityValidByCount(ctx)
	})
	RegisterCondition("IsBoxDensityValidByDistribution", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBoxDensityValidByDistribution(ctx, 30)
	})
	RegisterCondition("HasExcessiveUpperWick", func(ctx *box.TradingContext, s Settings) bool {
		return cond.HasExcessiveUpperWick(ctx, s.DefBoxUpperWickToBodyRatioThreshold)
	})
	RegisterCondition("MultiDefDamCountMax2", func(ctx *box.TradingContext, s Settings) bool {
		return cond.GetMultiDefboxDamCount(ctx) <= 2
	})

	// ── 캔들 패턴 ─────────────────────────────────────────────
	RegisterCondition("IsBullishCandle", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBullishCandle(ctx)
	})
	RegisterCondition("HasPullbackOrCorrection", func(ctx *box.TradingContext, s Settings) bool {
		return cond.HasPullbackOrCorrection(ctx)
	})

	// ── 이동평균선 ─────────────────────────────────────────────
	RegisterCondition("IsMa20NearMa60Complex", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMa20NearMa60WithComplexValidation(ctx)
	})
	RegisterCondition("IsMa20NearMa60Simple", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMa20NearMa60WithSimpleValidation(ctx)
	})
	RegisterCondition("IsMa60StrongerThanMa120By2Percent", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMa60StrongerThanMa120By2Percent(ctx)
	})
	RegisterCondition("IsMainboxPriceAboveMa60OrMa120", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMainboxPriceAboveMa60OrMa120(ctx)
	})
	RegisterCondition("HasLowTouchedMa20", func(ctx *box.TradingContext, s Settings) bool {
		return cond.HasLowTouchedMa20(ctx)
	})
	RegisterCondition("IsMainboxConditionValid", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMainboxConditionValid(ctx)
	})
	RegisterCondition("MainBoxPositionBasedTiming", func(ctx *box.TradingContext, s Settings) bool {
		return cond.EvaluateMainBoxPositionBasedTiming(ctx, s.MaSpreadThreshold)
	})
	RegisterCondition("MainBoxPositionBasedTimingLess", func(ctx *box.TradingContext, s Settings) bool {
		return cond.EvaluateMainBoxPositionBasedTimingLess(ctx, s.MaSpreadThreshold)
	})

	// ── 관통 조건 ─────────────────────────────────────────────
	RegisterCondition("IsPenetrationOptionValid", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsPenetrationOptionValid(ctx)
	})

	// ── MultiDef 완화 조건 ───────────────────────────────────────
	RegisterCondition("IsMultiDefRelaxedDamCondition", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMultiDefRelaxedDamCondition(ctx)
	})

	// ── ATR 변동성 ────────────────────────────────────────────
	RegisterCondition("IsATREntryValid", func(ctx *box.TradingContext, s Settings) bool {
		if !s.ATREntryFilterEnabled {
			return true
		}
		c := ctx.GetCurrentCandle()
		return c != nil && c.ATRPercentage <= s.ATREntryMaxThreshold
	})

	// ── RSI 조건 ──────────────────────────────────────────────
	RegisterCondition("IsRSIOversold", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsRSIOversold(ctx, s.RSIOversoldThreshold)
	})
	RegisterCondition("IsRSIOverbought", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsRSIOverbought(ctx, s.RSIOverboughtThreshold)
	})
	RegisterCondition("IsRSIRecoveringFromOversold", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsRSIRecoveringFromOversold(ctx, s.RSIOversoldThreshold, s.RSIRecoveryLookback)
	})
	RegisterCondition("IsRSIRising", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsRSIRising(ctx, s.RSIRisingPeriod)
	})
	RegisterCondition("IsRSIInBullZone", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsRSIInBullZone(ctx, s.RSIBullZoneLow, s.RSIBullZoneHigh)
	})

	// ── Bollinger Band 조건 ──────────────────────────────────
	RegisterCondition("IsBBLowerTouch", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBLowerTouch(ctx)
	})
	RegisterCondition("IsBBReboundFromLower", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBReboundFromLower(ctx, s.BBReboundLookback, s.BBReboundPercentB)
	})
	RegisterCondition("IsBBSqueezeBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBSqueezeBreakout(ctx, s.BBSqueezeLookback, s.BBSqueezeWidthThreshold, s.BBBreakoutPercentB)
	})
	RegisterCondition("IsBBUpperBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBUpperBreakout(ctx)
	})
	RegisterCondition("IsAboveBBMiddle", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsAboveBBMiddle(ctx, s.BBMiddleHoldDuration)
	})

	// ── 이동평균(MA) 조건 ─────────────────────────────────────
	RegisterCondition("IsMaGoldenCross5x20", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMaGoldenCross5x20(ctx)
	})
	RegisterCondition("IsMaGoldenCross20x60", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMaGoldenCross20x60(ctx)
	})
	RegisterCondition("IsMaProperArrangement", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMaProperArrangementNow(ctx)
	})
	RegisterCondition("IsAllMaRising", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsAllMaRising(ctx)
	})
	RegisterCondition("IsMaConvergence", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMaConvergence(ctx, s.MaConvergenceThreshold)
	})
	RegisterCondition("IsPriceAboveAllMa", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsPriceAboveAllMa(ctx)
	})
}
