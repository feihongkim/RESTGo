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
