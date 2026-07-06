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

	// DefBox 돌파 실패(재붕괴) — strategy1의 트리거인 DefBox 돌파(가격+거래대금+ATR 게이트)가 장전,
	// 유효기간 내 종가가 DefBox 가격 아래로 재붕괴하면 발화. "실패한 돌파"를 short/청산 신호로
	// 뒤집는 가설 (2026-07-05 사용자). 창 내 재붕괴가 없으면 만료 = 돌파 성공 케이스.
	RegisterArmedTrigger("DefBoxBreakoutFailure", ArmedTriggerSpec{
		WindowBars: 20,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			if ctx.DefChecker == 0 || ctx.GetDefBox() == nil {
				return nil, false
			}
			if !checkDefBoxBreakout(ctx, s) {
				return nil, false
			}
			return ctx.GetDefBox().Price, true // 돌파 시점의 박스 가격(스케일)을 상태로
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			price, _ := state.(float64)
			if price <= 0 {
				return false
			}
			return ctx.CandleList[ctx.Position].Close < price // 재붕괴 (장전당 1회라 첫 이탈에서만 발화)
		},
	})

	// Double Bump 재시도 (r_stg 전략3 이식, 2026-07-05) — 거래량 수반 1차 범프(1개월 신고가)가 장전,
	// 5봉 경과 후 "고점 아래 + 되돌림 35% 회복 + 고점 종가돌파 2회 미만 + 거래대금" 성립일이 발화.
	// 원본 SQL 코어 충실 이식 (R 후처리 제외조건 미포함 — zpicture/r_stg_catalog.md 2-2 참조).
	RegisterArmedTrigger("DoubleBumpRetest", ArmedTriggerSpec{
		WindowBars: 50, // 범프 후 ~50봉 내 (SQL rn2 < 50)
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			info, ok := cond.FindDoubleBump(ctx.CandleList, ctx.Position)
			if !ok {
				return nil, false
			}
			return &doubleBumpState{info: info}, true
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			st, _ := state.(*doubleBumpState)
			if st == nil {
				return false
			}
			i := ctx.Position
			c := ctx.CandleList[i]
			if c.Close > st.info.High { // 범프 고점 종가 돌파 카운트 (2회 이상이면 실격)
				st.higherCloses++
			}
			if st.higherCloses >= 2 {
				return false
			}
			if i-st.info.BumpPos <= 5 { // 범프 직후 5봉은 발화 금지 (SQL rn2 > 5)
				return false
			}
			return cond.IsDoubleBumpRetestDay(ctx.CandleList, i, st.info)
		},
	})

	// Double Bump 2차 돌파 변형 — 같은 장전(1차 범프)에서, 발화를 "범프 고점 첫 종가 돌파"로 정의.
	// 원본 R은 준비일 스크리닝이지만 전략명(이중 돌파)의 본래 의도는 2차 돌파 진입일 수 있어 함께 측정.
	RegisterArmedTrigger("DoubleBumpBreakout2", ArmedTriggerSpec{
		WindowBars: 50,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			info, ok := cond.FindDoubleBump(ctx.CandleList, ctx.Position)
			if !ok {
				return nil, false
			}
			return &doubleBumpState{info: info}, true
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			st, _ := state.(*doubleBumpState)
			if st == nil {
				return false
			}
			i := ctx.Position
			if ctx.CandleList[i].Close > st.info.High {
				st.higherCloses++
				// 첫 번째 종가 돌파 = 2차 돌파 진입 (범프 5봉 이후에만)
				return st.higherCloses == 1 && i-st.info.BumpPos > 5
			}
			return false
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

func init() {
	// r_stg 전략2 크립토 핵심형 (2026-07-05) — 완전 역배열(48봉)+120 무접촉 유지 중 120 첫 관통이 장전,
	// 24봉 내 되돌림(2연속 종가<MA20 + MA5≥(MA20+MA60)/2)이 발화. 명세: minute_stg_spec.md §1.
	RegisterArmedTrigger("Stg2Inverted120Retreat", ArmedTriggerSpec{
		WindowBars: 24,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			if cond.IsStg2FirstPierce120(ctx.CandleList, ctx.Position) {
				return nil, true
			}
			return nil, false
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			return cond.IsStg2RetreatEntry(ctx.CandleList, ctx.Position)
		},
	})
}

func init() {
	// DefBox 돌파 생존 지연 진입 (2026-07-06, 사용자 가설) — 돌파(장전) 후 N봉(기본 5) 동안
	// 종가가 박스가 아래로 내려가지 않고 "생존"하면 N봉째에 진입. 실패의 53%가 3일, 75%가
	// 7일 내 발생하므로 생존 확인으로 다수의 실패를 걸러내고, 그 대가로 초기 상승분을 포기한다.
	// N은 settings.DefBoxSurvivalBars (--set DefBoxSurvivalBars=N 스윕 가능).
	RegisterArmedTrigger("DefBoxBreakoutSurvival", ArmedTriggerSpec{
		WindowBars: 30,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			if ctx.DefChecker == 0 || ctx.GetDefBox() == nil {
				return nil, false
			}
			if !checkDefBoxBreakout(ctx, s) {
				return nil, false
			}
			return &survivalState{price: ctx.GetDefBox().Price}, true
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			st, _ := state.(*survivalState)
			if st == nil || st.price <= 0 {
				return false
			}
			if ctx.CandleList[ctx.Position].Close < st.price {
				st.failed = true // 생존 실패 — 이 장전은 영구 불발
			}
			if st.failed {
				return false
			}
			n := s.DefBoxSurvivalBars
			if n <= 0 {
				n = 5
			}
			return ctx.Position-armPos == n
		},
	})
}

// survivalState 는 DefBoxBreakoutSurvival의 장전 상태.
type survivalState struct {
	price  float64 // 돌파 시점 DefBox 가격 (스케일)
	failed bool    // 생존 확인 중 재붕괴 발생 여부
}

// doubleBumpState 는 DoubleBumpRetest 트리거의 장전 상태.
type doubleBumpState struct {
	info         *cond.DoubleBumpInfo
	higherCloses int // 범프 고점을 종가로 넘은 횟수 (2회 이상 = 실격)
}
