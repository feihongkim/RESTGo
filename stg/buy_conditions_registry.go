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
	// Method I 고도화: 역사적 스퀴즈 비교 (%B 임계 없음 — DefBox 돌파가 방향 확인)
	RegisterCondition("IsBBSqueezeHistorical", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBSqueezeHistorical(ctx, s.BBSqueezeHistoricalLookback, s.BBSqueezeHistoricalRatio)
	})
	// Method II: Band Walk (상단 밴드 부근 과반 유지)
	RegisterCondition("IsBBWalkingUp", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBWalkingUp(ctx, s.BBWalkDuration, s.BBWalkPercentB)
	})
	// Method III: W바텀 반등
	RegisterCondition("IsBBWBottomPattern", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBWBottomPattern(ctx, s.BBWBottomLookback)
	})
	RegisterCondition("IsBBWBottomBoxPattern", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBWBottomBoxPattern(ctx, s.BBWBottomLookback)
	})
	RegisterCondition("HasDefBoxBeforeWPattern", func(ctx *box.TradingContext, s Settings) bool {
		return cond.HasDefBoxBeforeWPattern(ctx, s.BBWBottomLookback)
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

	// ── 15분봉 P0: 모멘텀·추세 강도 ─────────────────────────────
	RegisterCondition("IsMACDGoldenCross", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMACDGoldenCross(ctx)
	})
	RegisterCondition("IsMACDHistogramRising", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMACDHistogramRising(ctx, s.MACDHistRisingBars)
	})
	RegisterCondition("IsStochGoldenCross", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStochGoldenCross(ctx, s.StochOversoldThreshold)
	})
	RegisterCondition("IsADXTrending", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsADXTrending(ctx, s.ADXTrendThreshold)
	})
	RegisterCondition("IsDIBullish", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsDIBullish(ctx)
	})
	RegisterCondition("IsDIBearish", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsDIBearish(ctx)
	})

	// ── 15분봉 P0: VWAP·거래량 ───────────────────────────────────
	RegisterCondition("IsAboveVWAP", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsAboveVWAP(ctx)
	})
	RegisterCondition("IsBelowVWAP", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBelowVWAP(ctx)
	})
	RegisterCondition("IsVWAPDeviation", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsVWAPDeviation(ctx, s.VWAPDeviationK)
	})
	RegisterCondition("IsVWAPReclaim", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsVWAPReclaim(ctx, 8)
	})
	RegisterCondition("IsVolumeZScoreSpike", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsVolumeZScoreSpike(ctx, s.VolumeZScoreWindow, s.VolumeZScoreThreshold)
	})
	RegisterCondition("IsOBVRising", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsOBVRising(ctx, s.OBVRisingPeriod)
	})

	// ── 15분봉 P0: 변동성 구조 ──────────────────────────────────
	RegisterCondition("IsSuperTrendBullish", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsSuperTrendBullish(ctx)
	})
	RegisterCondition("IsSuperTrendBearish", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsSuperTrendBearish(ctx)
	})
	RegisterCondition("IsDonchianBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsDonchianBreakout(ctx)
	})
	RegisterCondition("IsDonchianBreakdown", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsDonchianBreakdown(ctx)
	})
	RegisterCondition("IsKeltnerBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsKeltnerBreakout(ctx)
	})
	RegisterCondition("IsNarrowRange", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsNarrowRange(ctx, s.NarrowRangePeriod)
	})

	// ── 15분봉 P0: 숏 미러 (when_not 거부 필터용) ────────────────
	RegisterCondition("IsRSIFallingFromOverbought", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsRSIFallingFromOverbought(ctx, s.RSIOverboughtThreshold)
	})
	RegisterCondition("IsBBUpperReject", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBUpperReject(ctx)
	})
	RegisterCondition("IsMaDeadCross5x20", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMaDeadCross5x20(ctx)
	})
	RegisterCondition("IsMaInverseArrangement", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsMaInverseArrangement(ctx)
	})
	RegisterCondition("IsPriceBelowAllMa", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsPriceBelowAllMa(ctx)
	})

	// ── EMA 조건 ──────────────────────────────────────────────
	RegisterCondition("IsEMABullArrangement", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsEMABullArrangement(ctx)
	})
	RegisterCondition("IsEMA9Above21", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsEMA9Above21(ctx)
	})
	RegisterCondition("IsEMA21PullbackBounce", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsEMA21PullbackBounce(ctx, s.EMA21PullbackLookback)
	})
	RegisterCondition("IsPriceAboveEMA50", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsPriceAboveEMA50(ctx)
	})
	RegisterCondition("IsVWAPDeviationBelow", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsVWAPDeviationBelow(ctx, s.VWAPDeviationK)
	})

	// ── 시장 국면 조건 (2026-07-05) ──────────────────────────
	// 골든크로스 임박: MA60<MA120 + 간격 20봉 지속 축소 + 간격 ≤3%.
	// 일봉 단독 엣지는 중립(zpicture/wgc_scan_report.md) — 주봉(거시) 분석과의 결합을 전제로
	// 공식 전략화 등록 (사용자 결정, buy_wdefbox_gc.yaml / strategy1_gc.yaml)
	RegisterCondition("IsGoldenCrossPending", func(ctx *box.TradingContext, s Settings) bool {
		pending, _, _, _ := cond.GoldenCrossPendingInfo(ctx.CandleList, ctx.Position)
		return pending
	})

	// M자(이중천장) 3박스 구조 존재 — 마지막 3개 추세 박스가 R(BB상단 이탈)-S(MA20 위)-R(BB 내부)이고
	// 마지막 R 인지가 15봉 이내 (level 조건 — DefBox 돌파 실패 short 가설의 겹침 판정용, 2026-07-05)
	RegisterCondition("HasMTopStructure", func(ctx *box.TradingContext, s Settings) bool {
		_, _, found := cond.FindBBMTopBoxPattern(ctx, s.BBWBottomLookback)
		return found
	})
}
