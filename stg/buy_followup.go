package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// ── FollowUp·REST2 매수 처리 ──────────────────────────────────────────────────
// C# 참조: Stock1/biz/Processors/BuyDecisionProcessor.cs (SetBuySignal, ExecuteStrategyWithDuplicatePrevention)
// C# 참조: Stock1/biz/Processors/BuyDecisionProcessor.Options.cs (DetermineBuySignal — REST2 S13~S16)
// C# 참조: Stock1/biz/Processors/BuyDecisionProcessor.FollowUp.cs (S17~S20)
//
// 중복 방지 키는 ctx.LastBuySignalPosition map을 공유한다 (DefBox 변경 시 일괄 리셋):
//   "DetermineBuySignal" / "ShortRange" / "AdditionalBuySignals"
// C# BFunction의 LastBuySignalPosition_DetermineBuySignal/_ShortRange/_AdditionalBuySignals 대응.

// setBuySignalState 는 매수 신호 상태를 컨텍스트에 기록한다.
// C# BuyDecisionProcessor.SetBuySignal 포팅.
func setBuySignalState(ctx *box.TradingContext, bsig, stgName, helperReport string) {
	ctx.Bsig = bsig
	ctx.StgName = stgName
	ctx.MomentumPosition = ctx.Position
	ctx.BuyHelperReport = helperReport
	if helperReport != "매수대기" && helperReport != "multidef매수대기" && helperReport != "!multidef매수대기" {
		ctx.BuyHelper = helperReport
	}
}

// finalizeBuySignal 은 즉시매수 확정 처리: BuySignal 생성 + BuyOn 설정 + Bsig 초기화.
// C# ExecuteBuySignal(VirtualTradingBuy 호출 + Bsig="") 대응.
func finalizeBuySignal(ctx *box.TradingContext, reason string) BuySignal {
	sig := buildTriggeredSignal(ctx, reason)
	sig.Helper = ctx.BuyHelperReport
	ctx.BuyOn = true
	ctx.Bsig = ""
	return sig
}

// determineBuySignal 은 REST2 매수 판별 (S13~S16).
// C# BuyDecisionProcessor.Options.DetermineBuySignal 포팅.
// 후보군1(S13/S14/S16)은 실매수가 아니므로 신호를 반환하지 않고 Bsig 상태만 설정한다
// (C#: ExecuteStrategyWithDuplicatePrevention이 Bsig=="즉시매수"일 때만 실행·기록).
func determineBuySignal(ctx *box.TradingContext, s Settings) *BuySignal {
	// 중복 방지: 이번 DefBox에서 이미 즉시매수가 실행됐으면 평가 자체를 스킵
	if _, fired := ctx.LastBuySignalPosition["DetermineBuySignal"]; fired {
		return nil
	}

	near := cond.IsCloseNearDefboxPrice(ctx, s.DefBoxNearPriceThreshold, s.MainBoxNearPriceThreshold)
	bullish := cond.IsBullishCandle(ctx)
	wick := cond.HasExcessiveUpperWick(ctx, s.DefBoxUpperWickToBodyRatioThreshold)
	simpleMA := cond.IsMa20NearMa60WithSimpleValidation(ctx)

	// Single DefBox 조건 (S13/S14)
	isSingle := ctx.DefCount == 1 &&
		near &&
		cond.IsMainboxDistanceTwiceOrMore(ctx) &&
		cond.IsBoxConditionValid2(ctx) &&
		bullish &&
		cond.IsBoxDensityValidByCount(ctx) &&
		!wick

	// Multi-DefBox with Penetration (S15)
	// 주의: C#의 c.IsPenetrationOptionValid2는 어디서도 할당되지 않는 필드(항상 false)라
	// S15는 현재 C#에서 사문(死文)이다 — 동일하게 보존.
	isPenetrationOptionValid2 := false
	isMultiPen := ctx.DefCount >= 2 &&
		near && bullish && simpleMA &&
		cond.GetMultiDefboxDamcountPen(ctx) <= 2 &&
		isPenetrationOptionValid2 &&
		!wick

	// Multi-DefBox Alternative (S16)
	isMultiAlt := ctx.DefCount >= 2 && near && bullish && !wick

	switch {
	case isSingle:
		ctx.PenPosition = ctx.Position
		if simpleMA {
			setBuySignalState(ctx, "후보군1", "13_SingleDefBoxImmediateBuy_REST2", "매수대기")
		} else {
			setBuySignalState(ctx, "후보군1", "14_candi_SingleDefBoxWeakFoundationBuy_REST2", "연약지반매수")
		}
		return nil // 후보군1: 실매수 아님 (이후 캔들의 ProcessAdditionalBuySignals 활성화용)
	case isMultiPen:
		ctx.PenPosition = ctx.Position
		setBuySignalState(ctx, "즉시매수", "15_MultiDefBoxWithPenetration_REST2", "multidef")
		ctx.LastBuySignalPosition["DetermineBuySignal"] = ctx.Position
		sig := finalizeBuySignal(ctx, "15_MultiDefBoxWithPenetration_REST2")
		return &sig
	case isMultiAlt:
		ctx.PenPosition = ctx.Position
		setBuySignalState(ctx, "후보군1", "16_candi_MultiDefBoxAlternative_REST2", "multidef매수대기")
		return nil
	}
	return nil
}

// processPostBreakoutSignals 는 돌파 이후(DamChecker==2) 캔들의 ShortRange 사후 평가.
// C# BuyDecisionProcessor.FollowUp.ProcessPostBreakoutSignals 포팅 (S19).
func processPostBreakoutSignals(ctx *box.TradingContext) *BuySignal {
	if _, fired := ctx.LastBuySignalPosition["ShortRange"]; fired {
		return nil
	}
	if ctx.PenPosition <= 0 || ctx.Position <= ctx.PenPosition {
		return nil
	}
	if !cond.IsShortRangeValid(ctx) {
		return nil
	}
	setBuySignalState(ctx, "즉시매수", "19_ShortRangePostBreakout_REST2", "SR-진동지지")
	ctx.LastBuySignalPosition["ShortRange"] = ctx.Position
	sig := finalizeBuySignal(ctx, "19_ShortRangePostBreakout_REST2")
	return &sig
}

// processAdditionalBuySignals 는 후보군1 상태에서의 추가 매수 기회 평가.
// C# BuyDecisionProcessor.FollowUp.ProcessAdditionalBuySignalsInternal 포팅 (S20 + multidef대기매수).
// 호출 게이트(!BuyOn && Bsig=="후보군1")는 analyzer가 담당 (C# BFunction.BLogic).
func processAdditionalBuySignals(ctx *box.TradingContext) *BuySignal {
	if _, fired := ctx.LastBuySignalPosition["AdditionalBuySignals"]; fired {
		return nil
	}

	// Multi-DefBox 매수대기 → 즉시매수 전환
	// (게이트 BuyHelper=="multidef매수대기"는 현재 파이프라인에서 설정 경로가 없어 사문 — C# 동일)
	if cond.IsMultiDefWaitToBuyCondition(ctx) {
		ctx.Bsig = "즉시매수"
		ctx.BuyHelper = "multidef대기매수"
		ctx.BuyHelperReport = "multidef대기매수"
		ctx.MomentumPosition = ctx.Position
		ctx.LastBuySignalPosition["AdditionalBuySignals"] = ctx.Position
		sig := finalizeBuySignal(ctx, ctx.StgName) // C#: StgName 미변경 상태로 매수 실행
		return &sig
	}

	// ShortRange 조건
	if cond.IsShortRangeValid(ctx) {
		setBuySignalState(ctx, "즉시매수", "20_ShortRangeAdditionalBuy_REST2", "SR-진동지지")
		ctx.LastBuySignalPosition["AdditionalBuySignals"] = ctx.Position
		sig := finalizeBuySignal(ctx, "20_ShortRangeAdditionalBuy_REST2")
		return &sig
	}
	return nil
}

// processFollowUpBuyDecisions 는 매수 대기 상태에서의 재진입 기회 처리 (매 캔들 호출).
// C# BuyDecisionProcessor.FollowUp.ProcessFollowUpBuyDecisions 포팅 (S17/S18).
//
// 주의: 두 게이트 모두 현재 C# 파이프라인에서 도달 불가(사문)다 —
//
//	S17: BuyHelper에 "multidef매수대기"를 대입하는 코드가 C#에 없음 (SetBuySignal이 명시적으로 제외)
//	S18: SellHelper는 SellReason("...FINAL")로 설정되며 "연약지반fail" 리터럴은 레거시 매도(미포팅 saveSt1) 전용
//
// 충실 포팅 원칙에 따라 게이트 포함 그대로 보존한다.
func processFollowUpBuyDecisions(ctx *box.TradingContext) []BuySignal {
	var out []BuySignal

	// S17: multidef매수대기 재진입 (C# ProcessMultiDefWaitingBuy)
	if ctx.BuyHelper == "multidef매수대기" &&
		(ctx.Position-ctx.PenPosition) < 10 &&
		cond.IsPositionProgressedFromPen(ctx) &&
		cond.IsPriceRecrossedDefBox(ctx) &&
		cond.IsBullishCandle(ctx) &&
		cond.IsHighPriceWithinDefBoxLimit(ctx) {
		setBuySignalState(ctx, "즉시매수", "17_MultiDefWaitingBuy_FollowUp", "MD즉시매수")
		out = append(out, finalizeBuySignal(ctx, "17_MultiDefWaitingBuy_FollowUp"))
	}

	// S18: 연약지반fail 재진입 (C# ProcessWeakGroundRecoveryBuy)
	if ctx.SellHelper == "연약지반fail" &&
		(ctx.Position-ctx.PenPosition) <= 4 &&
		cond.HasOscilloBreakout(ctx) &&
		cond.HasNoBullishCandleSinceMomentum(ctx) &&
		cond.IsBullishCandle(ctx) &&
		cond.IsPriceRecrossedDefBoxForWeakGround(ctx) {
		setBuySignalState(ctx, "즉시매수", "18_WeakGroundRecoveryBuy_FollowUp", "연약지반fail-fail")
		ctx.BuyHelper = "연약지반fail-fail"
		ctx.BuyHelperReport = "연약지반fail-fail"
		ctx.MomentumPosition = ctx.Position
		out = append(out, finalizeBuySignal(ctx, "18_WeakGroundRecoveryBuy_FollowUp"))
	}

	return out
}
