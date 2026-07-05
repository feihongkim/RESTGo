package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// TriggerFn 은 메인이벤트(트리거) 평가 함수 타입.
// 트리거는 반드시 edge 형태여야 한다 — "돌파/이탈이 발생한 그 캔들"에서만 true를 반환하고,
// 상태가 지속되는 동안(level) 계속 true를 반환하면 안 된다 (매 캔들 발화 방지).
type TriggerFn func(ctx *box.TradingContext, s Settings) bool

// triggerRegistry 는 트리거명 → 평가 함수 매핑.
// 조건(conditionRegistry)과 분리한 이유: 조건은 level(상태), 트리거는 edge(이벤트)로
// 시맨틱이 다르며, YAML 작성 시 trigger 필드에 level 조건을 잘못 쓰는 것을 막기 위함.
var triggerRegistry = map[string]TriggerFn{}

// RegisterTrigger 는 트리거명과 함수를 등록한다.
func RegisterTrigger(name string, fn TriggerFn) {
	triggerRegistry[name] = fn
}

// TriggerRegistryGet 은 트리거명으로 함수를 조회한다 (테스트/검증용).
func TriggerRegistryGet(name string) (TriggerFn, bool) {
	fn, ok := triggerRegistry[name]
	return fn, ok
}

func init() {
	// DefBoxBreakout — stateless 트리거 버전.
	// 기존 on_breakout 경로(evaluateBuySignals의 DamChecker 상태머신)와 달리
	// DefBox당 1회 제한이 없다: 가격이 박스 아래로 내려갔다 다시 돌파하면 재발화한다.
	// DefBox당 1회 시맨틱이 필요하면 룰에 once_per: defbox(기본값)를 사용.
	RegisterTrigger("DefBoxBreakout", func(ctx *box.TradingContext, s Settings) bool {
		if ctx.DefChecker == 0 || ctx.GetDefBox() == nil {
			return false
		}
		return checkDefBoxBreakout(ctx, s)
	})

	// PriceBreakout — 거래대금·ATR 필터 없는 순수 가격 돌파 (분해형).
	// 거래대금·ATR을 when 필터로 조합하고 싶을 때 사용: when: [IsVolumeBreakoutGate, ...]
	RegisterTrigger("PriceBreakout", func(ctx *box.TradingContext, s Settings) bool {
		if ctx.DefChecker == 0 || ctx.GetDefBox() == nil {
			return false
		}
		return cond.IsDefBoxBreakout(ctx)
	})

	// WBottomBox — 5이평 변곡 W패턴(support→resist→support) 완성 순간.
	// 마지막 support box 인식 시점이 메인이벤트 (strategy1의 DefBox 돌파와 동일 지위).
	RegisterTrigger("WBottomBox", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsWBottomBoxCompletedEvent(ctx, s.BBWBottomLookback)
	})

	RegisterTrigger("BBLowerBreakdown", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBLowerBreakdownEvent(ctx)
	})
	RegisterTrigger("BBLowerReentry", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBLowerReentryEvent(ctx)
	})
	RegisterTrigger("BBSqueezeBreakout", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsBBSqueezeBreakoutEvent(ctx, s.BBSqueezeLookback, s.BBSqueezeWidthThreshold, s.BBBreakoutPercentB)
	})

	// r_stg 전략6 이식 (2026-07-05, 크립토 15분봉 대상) — 정배열 유지 + 5<20 조정 + 전고점 터치·미돌파.
	// 명세: zpicture/minute_stg_spec.md §4. 상태 성립 순간(edge)에 발화.
	RegisterTrigger("Stg6PullbackTouch", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStg6PullbackTouchEvent(ctx)
	})

	// r_stg 전략11 크립토 핵심형 (2026-07-05) — 장기 정배열(96봉) 유지 중 60이평 첫 붕괴 순간.
	// 명세: zpicture/minute_stg_spec.md §6 (세션 의존 조건은 제거·변환).
	RegisterTrigger("Stg11MA60Breakdown", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStg11MA60BreakdownEvent(ctx, s.Stg11AlignedBars)
	})
}
