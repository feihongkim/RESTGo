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
}
