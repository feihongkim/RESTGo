package cond

import (
	"RESTGo/box"
)

// Bollinger Band 기반 트리거(edge) 함수 모음.
//
// 트리거는 반드시 edge 시맨틱을 따라야 한다 — 조건 충족이 "발생한 그 캔들"에서만 true를 반환하고,
// 상태가 지속되는 동안(level) 계속 true를 반환하면 안 된다 (매 캔들 발화 방지).
// 따라서 전일 상태와 당일 상태를 비교하여 "순간"을 감지한다.
//
// 모든 함수는 내부적으로 hasValidBollinger로 밴드 미계산 캔들을 가드한다.

// IsBBLowerBreakdownEvent 는 당일 종가가 볼린저 하단을 하향 이탈한 "순간"인지 확인 (edge).
// 전일 종가는 하단 이상이었고 당일 종가가 하단 미만으로 내려온 캔들에서만 true.
func IsBBLowerBreakdownEvent(ctx *box.TradingContext) bool {
	if ctx.Position < 1 {
		return false
	}
	prev := ctx.CandleList[ctx.Position-1]
	cur := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(prev) || !hasValidBollinger(cur) {
		return false
	}
	return prev.Close >= prev.BollingerLower && cur.Close < cur.BollingerLower
}

// IsBBLowerReentryEvent 는 하단 이탈 상태에서 밴드 안으로 복귀한 "순간"인지 확인 (edge).
// John Bollinger Method III W바텀의 재진입 시점. 전일 종가 < 전일 하단 && 당일 종가 >= 당일 하단.
func IsBBLowerReentryEvent(ctx *box.TradingContext) bool {
	if ctx.Position < 1 {
		return false
	}
	prev := ctx.CandleList[ctx.Position-1]
	cur := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(prev) || !hasValidBollinger(cur) {
		return false
	}
	return prev.Close < prev.BollingerLower && cur.Close >= cur.BollingerLower
}

// IsWBottomBoxCompletedEvent 는 5이평 변곡 기반 W패턴(support→resist→support Box 시퀀스)이
// "완성된 순간"인지 확인한다 (edge) — 마지막 support box가 이번 캔들에서 인식된 시점.
// strategy1의 DefBox 돌파와 동일한 지위를 갖는 메인이벤트.
//
// W패턴 정의 (FindBBWBottomBoxPattern, 연구 시리즈에서 정제):
//   P1(첫 support): 직전 구간 BB 하단 이탈 + 밴드폭 팽창 중
//   중간 resist: 종가가 MA20 아래 (여전히 약세 구간)
//   P2(마지막 support): BB 내부 (하단 이탈 아님)
// 추가로 인식 캔들 종가가 MA20 아래여야 한다 (WPatternAnalyze와 동일).
func IsWBottomBoxCompletedEvent(ctx *box.TradingContext, lookback int) bool {
	// 마지막 support box가 이번 캔들에서 생성(인식)되었는지 — edge 보장
	n := len(ctx.BoxList)
	if n == 0 || ctx.Position >= len(ctx.CandleList) {
		return false
	}
	last := ctx.BoxList[n-1]
	if last.KindOfBox != box.KindBox || last.BoxType != box.BoxTypeSupport ||
		last.CurvePosition != ctx.Position {
		return false
	}
	// 인식 캔들이 MA20 아래 (마지막 support가 약세 구간에 위치)
	cur := ctx.CandleList[ctx.Position]
	if cur.Ma20 <= 0 || cur.Close >= cur.Ma20 {
		return false
	}
	// S-R-S W패턴 완성 확인 (P1 하단 이탈 → 상단 → P2 밴드 내부)
	_, _, found := FindBBWBottomBoxPattern(ctx, lookback)
	return found
}

// HasDefBoxBeforeWPattern 은 W패턴의 첫 support box(P1) 이전에 DefBox가 형성되어
// 있는지 확인한다. 전략 가정: 이전에 고점 돌파에 실패한 DefBox는 강한 중력(인력)을
// 가지며, 그 아래에서 완성된 W패턴의 반등 속성과 결합해 강한 상승을 만든다.
func HasDefBoxBeforeWPattern(ctx *box.TradingContext, lookback int) bool {
	p1Pos, _, found := FindBBWBottomBoxPattern(ctx, lookback)
	if !found {
		return false
	}
	for _, b := range ctx.BoxList {
		if b.BoxPosition >= p1Pos {
			break
		}
		if b.KindOfBox == box.KindDefBox {
			return true
		}
	}
	return false
}

// IsBBSqueezeBreakoutEvent 는 스퀴즈 후 상방 돌파가 "발생한 순간"인지 확인 (edge).
// 당일 %B > breakoutPercentB 이고 전일 %B <= breakoutPercentB (돌파 순간),
// 그리고 최근 lookback 구간에 스퀴즈가 존재해야 한다.
func IsBBSqueezeBreakoutEvent(ctx *box.TradingContext, lookback int, widthThreshold, breakoutPercentB float64) bool {
	if ctx.Position < 1 {
		return false
	}
	prev := ctx.CandleList[ctx.Position-1]
	cur := ctx.CandleList[ctx.Position]
	if !hasValidBollinger(prev) || !hasValidBollinger(cur) {
		return false
	}
	return cur.BBPercent > breakoutPercentB && prev.BBPercent <= breakoutPercentB &&
		HasRecentSqueeze(ctx, lookback, widthThreshold)
}
