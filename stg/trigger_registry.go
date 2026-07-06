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
	// 터치 변형 (2026-07-06) — 종가 붕괴 대신 저가 첫 터치에 발화 (비교 측정용)
	RegisterTrigger("Stg11MA60FirstTouch", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStg11MA60FirstTouchEvent(ctx, s.Stg11AlignedBars)
	})

	// r_stg 전략9·7·14 이식 (2026-07-05, 일봉 대상) — 명세: zpicture/r_stg_catalog.md
	RegisterTrigger("Stg9ApexPerch", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStg9ApexPerchEvent(ctx)
	})
	RegisterTrigger("Stg7GCAccel", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStg7GCAccelEvent(ctx)
	})
	RegisterTrigger("Stg14Oversold", func(ctx *box.TradingContext, s Settings) bool {
		return cond.IsStg14OversoldEvent(ctx)
	})

	// DefBox 선매수 접근 (2026-07-06, 사용자 구상 "돌파 전 박스 아래 매수") —
	// 미돌파 DefBox의 가격 -δ(기본 2.5%) 밴드에 종가가 아래에서 처음 진입하는 순간.
	// strategy1 조건들을 when으로 얹어 "돌파 전 선매수"를 측정/전략화하기 위한 트리거.
	RegisterTrigger("DefBoxApproach", func(ctx *box.TradingContext, s Settings) bool {
		if ctx.DefChecker == 0 {
			return false
		}
		db := ctx.GetDefBox()
		pos := ctx.Position
		if db == nil || pos < 1 || db.Price <= 0 {
			return false
		}
		delta := s.DefBoxApproachPct
		if delta <= 0 {
			delta = 0.025
		}
		price := db.Price
		lo := price * (1 - delta)
		cur := ctx.CandleList[pos]
		prev := ctx.CandleList[pos-1]
		if !(cur.Close >= lo && cur.Close < price) { // 접근 밴드 [박스-δ, 박스)
			return false
		}
		if prev.Close >= lo { // edge: 아래에서 처음 진입
			return false
		}
		// 미돌파: 박스 형성 이후 종가 돌파 이력 없어야 (재돌파 상황 배제 — 재돌파 열등함이 기측정됨)
		start := db.BoxPosition
		if start < 0 {
			start = 0
		}
		for j := start; j < pos; j++ {
			if ctx.CandleList[j].Close > price {
				return false
			}
		}
		return true
	})
}
