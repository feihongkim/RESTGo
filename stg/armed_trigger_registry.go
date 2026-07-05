package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// armed_trigger_registry.go — armed(장전→발화) 트리거 등록 (2026-07-05).
// 기존 전용 분석기(stg/{mpattern,hns,pullback}_analyze.go)의 상태머신을 레지스트리화한 것.
// 검증: RunArmedTrigger가 각 분석기와 동일 신호를 내는지 실데이터 대조 (armed_trigger 도입 검증 스크립트).
//
// ⚠️ 세 패턴 모두 단독 엣지가 기각된 실험 신호다 (zpicture/{mtop,hns,pullback_regime}_scan_report.md).
// 등록 목적은 "패턴 × 상황 조건 자유 조합" 구조 — 향후 국면 조건과의 결합 실험을 YAML로 가능케 함.

func init() {
	// M탑 붕괴 — M자(R-S-R, P1 BB상단 이탈) 완성이 장전, 20이평 부근 음-양-음 반등 실패가 발화.
	RegisterArmedTrigger("MTopCollapse", ArmedTriggerSpec{
		WindowBars: MTopArmWindowBars,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			i := ctx.Position
			c := ctx.CandleList
			if i < 1 || !(c[i-1].Curvekey > 0 && c[i].Curvekey < 0) || !newBox {
				return nil, false
			}
			if c[i].Ma20 <= 0 || c[i].Close <= c[i].Ma20 { // MA20 위에서 M 완성
				return nil, false
			}
			_, p2Pos, ok := cond.FindBBMTopBoxPattern(ctx, s.BBWBottomLookback)
			if !ok {
				return nil, false
			}
			return p2Pos, true
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			p2Pos, _ := state.(int)
			_, ok := cond.FindMTopCollapseRuns(ctx.CandleList, ctx.Position, p2Pos)
			return ok
		},
	})

	// 해드앤숄더 넥라인 이탈 — 오른어깨 박스 인지가 장전, 종가가 넥라인(골1-골2 직선) 아래 마감이 발화.
	RegisterArmedTrigger("HNSNecklineBreak", ArmedTriggerSpec{
		WindowBars: HNSArmWindowBars,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			i := ctx.Position
			c := ctx.CandleList
			if i < 1 || !(c[i-1].Curvekey > 0 && c[i].Curvekey < 0) || !newBox {
				return nil, false
			}
			p, ok := cond.FindHNSPattern(ctx, cond.HNSLookbackBars)
			if !ok {
				return nil, false
			}
			return p, true
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			p, _ := state.(*cond.HNSPattern)
			if p == nil {
				return false
			}
			return ctx.CandleList[ctx.Position].Close < p.NecklineValue(ctx.Position)
		},
	})

	// 20이평 눌림 돌파 — 눌림 R→S 구조 완성(support box 인지)이 장전, 양봉 MA20 종가 돌파가 발화.
	// streak(MA20 연속 상승 요구)는 s.PullbackStreak (기본 0 = +++ 폐지판).
	RegisterArmedTrigger("MA20PullbackBreakout", ArmedTriggerSpec{
		WindowBars: PullbackArmWindowBars,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			i := ctx.Position
			c := ctx.CandleList
			if i < 1 || !(c[i-1].Curvekey < 0 && c[i].Curvekey > 0) || !newBox {
				return nil, false
			}
			p, ok := cond.FindMA20PullbackPattern(ctx, s.PullbackStreak)
			if !ok {
				return nil, false
			}
			return p, true
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			return cond.IsMA20BullishBreakout(ctx.CandleList, ctx.Position, s.PullbackStreak)
		},
	})
}
