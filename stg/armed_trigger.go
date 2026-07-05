package stg

// armed_trigger.go — 장전→발화(Armed) 2단계 트리거 레지스트리 (2026-07-05).
//
// 배경: 기존 트리거 레지스트리는 "한 캔들에서 판정되는 edge"만 지원해, M탑·H&S·눌림돌파처럼
// 상태를 갖는 2단계 패턴(패턴 완성=장전 → 유효기간 내 확인 이벤트=발화)이 전용 분석기에
// 하드코딩됐다 (stg/{mpattern,hns,pullback}_analyze.go — ~80% 동일 루프 중복).
// 이 파일은 그 상태머신을 레지스트리로 일반화해 YAML에서 trigger: 이름으로 자유 조합을 가능케 한다.
//
// 시맨틱 (기존 분석기와 동일):
//   매 캔들: 만료 확인(armPos+WindowBars 초과 시 해제) → 장전 시도(재장전은 상태 갱신)
//            → 발화 확인(장전 캔들 이후, 장전당 1회)
// 상태는 ctx.ArmedTriggerState에 저장돼 분석(종목)마다 자동 초기화된다.
// 엔진 통합: evaluateTriggerSignals 상단에서 룰이 참조하는 armed 트리거를 매 캔들 틱한다 —
// 룰의 once_per/def_count 필터와 무관하게 상태가 갱신되도록 (놓친 장전 방지).
//
// 주의: 엔진 경로에서는 캔들 처리 순서가 CheckAndCreateDefBox → AnalyzeCurvature라
// newBox 판정에 DefBox 생성분이 섞일 수 있다 (전용 분석기와의 미세 차이 — CheckArm이
// 곡률 flip을 함께 요구하므로 실질 영향은 제한적).

import "RESTGo/box"

// ArmedTriggerSpec 은 장전→발화 트리거 정의.
type ArmedTriggerSpec struct {
	// CheckArm: 이 캔들에서 장전되는가. newBox = 직전 틱 이후 BoxList가 늘었는가.
	// 반환 state는 발화 판정에 전달된다 (패턴 위치 등).
	CheckArm func(ctx *box.TradingContext, s Settings, newBox bool) (state interface{}, armed bool)
	// CheckFire: 장전 상태에서 이 캔들에 발화하는가 (armPos 이후 캔들에서만 호출됨).
	CheckFire func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool
	// WindowBars: 장전 후 발화 유효 기간 (거래일 봉).
	WindowBars int
}

type armedTriggerState struct {
	armed        bool
	armPos       int
	state        interface{}
	prevBoxCount int
	initialized  bool
}

var armedTriggerRegistry = map[string]ArmedTriggerSpec{}

// RegisterArmedTrigger 는 armed 트리거를 등록한다. 일반 트리거와 이름 공간을 공유하므로
// 중복 이름 금지 (YAML trigger: 필드에서 동일하게 참조된다).
func RegisterArmedTrigger(name string, spec ArmedTriggerSpec) {
	armedTriggerRegistry[name] = spec
}

// ArmedTriggerRegistryGet 은 armed 트리거 정의를 조회한다 (테스트/검증용).
func ArmedTriggerRegistryGet(name string) (ArmedTriggerSpec, bool) {
	spec, ok := armedTriggerRegistry[name]
	return spec, ok
}

// IsKnownTrigger 는 이름이 일반 또는 armed 트리거로 등록돼 있는지 확인한다.
func IsKnownTrigger(name string) bool {
	if _, ok := triggerRegistry[name]; ok {
		return true
	}
	_, ok := armedTriggerRegistry[name]
	return ok
}

// tickArmedTrigger 는 armed 트리거의 캔들당 1회 상태 갱신 + 발화 판정.
// 반드시 매 캔들 호출돼야 한다 (evaluateTriggerSignals 상단에서 시딩).
func tickArmedTrigger(name string, ctx *box.TradingContext, s Settings) bool {
	spec, ok := armedTriggerRegistry[name]
	if !ok {
		return false
	}
	if ctx.ArmedTriggerState == nil {
		ctx.ArmedTriggerState = map[string]interface{}{}
	}
	st, _ := ctx.ArmedTriggerState[name].(*armedTriggerState)
	if st == nil {
		st = &armedTriggerState{}
		ctx.ArmedTriggerState[name] = st
	}

	newBox := st.initialized && len(ctx.BoxList) > st.prevBoxCount
	st.prevBoxCount = len(ctx.BoxList)
	st.initialized = true

	// 만료
	if st.armed && ctx.Position > st.armPos+spec.WindowBars {
		st.armed = false
	}
	// 장전 (재장전 = 상태 갱신 — 분석기와 동일)
	if state, armed := spec.CheckArm(ctx, s, newBox); armed {
		st.armed = true
		st.armPos = ctx.Position
		st.state = state
	}
	// 발화 (장전 캔들 이후, 장전당 1회)
	if st.armed && ctx.Position > st.armPos && spec.CheckFire(ctx, s, st.state, st.armPos) {
		st.armed = false
		return true
	}
	return false
}

// ArmedFire 는 RunArmedTrigger의 발화 1건.
type ArmedFire struct {
	Pos    int
	ArmPos int
	State  interface{}
}

// RunArmedTrigger 는 armed 트리거를 표준 곡률 루프에서 단독 실행한다 (연구·검증용).
// 전용 분석기(MPatternAnalyze 등)와 동일한 순서(AnalyzeCurvature → newBox 판정 → [DefBox 생성])로
// 돌므로 분석기 결과 재현 검증에 쓸 수 있다. withDefBox: CheckAndCreateDefBox 호출 여부
// (M탑·H&S 분석기는 false, 눌림돌파 분석기는 true였다).
func RunArmedTrigger(candles []*box.Candle, name string, s Settings, withDefBox bool) []ArmedFire {
	spec, ok := armedTriggerRegistry[name]
	if !ok || len(candles) < 60 {
		return nil
	}
	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	var fires []ArmedFire
	st := &armedTriggerState{}

	for i := 5; i < len(candles); i++ {
		ctx.Position = i
		if i == 5 {
			if candles[i].Gradient >= 0.0 {
				candles[i].Curvekey = 1
			} else {
				candles[i].Curvekey = -1
			}
			continue
		}

		prevBoxCount := len(ctx.BoxList)
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)
		newBox := len(ctx.BoxList) > prevBoxCount
		if withDefBox {
			box.CheckAndCreateDefBox(ctx, s.DamOption)
		}
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		if st.armed && i > st.armPos+spec.WindowBars {
			st.armed = false
		}
		if state, armed := spec.CheckArm(ctx, s, newBox); armed {
			st.armed = true
			st.armPos = i
			st.state = state
		}
		if st.armed && i > st.armPos && spec.CheckFire(ctx, s, st.state, st.armPos) {
			fires = append(fires, ArmedFire{Pos: i, ArmPos: st.armPos, State: st.state})
			st.armed = false
		}
	}
	return fires
}
