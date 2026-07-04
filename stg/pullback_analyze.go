package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// PullbackSignal 은 20이평 눌림 돌파 매수 신호 1건.
// 2단계: 눌림 R→S 구조 완성(장전, ArmPos) → 양봉 20이평 종가 돌파(발화, Pos).
type PullbackSignal struct {
	Date    string // 돌파 캔들 날짜 — 매수 신호 시점
	ArmDate string // support box 인지 캔들 날짜
	Shcode  string
	Pos     int
	ArmPos  int
	Pattern cond.PullbackPattern
	// 시장국면 속성 (트리거 시점 기록, 2026-07-04 — 게이트 아님, 사후 분석용)
	DefBoxAboveMA20 bool    // 국면a: 최근 DefBox가 MA20 위 (W중력 유사 상방 중력)
	DefBoxExists    bool    // DefBox 존재 여부 (a의 분모 확인용)
	DefBoxDistPct   float64 // DefBox 가격의 MA20 대비 거리 %
	BBExpanding     bool    // 국면b: 볼린저 밴드 확장 중 (스퀴즈 아님)
}

// PullbackArmWindowBars 는 support 인지 후 돌파 유효 기간.
const PullbackArmWindowBars = 20

// PullbackAnalyze 는 20이평 눌림 돌파 매수 신호를 탐지한다 (M/H&S와 동일 계보, 매수 방향).
//
//	장전: 하락→상승 변곡으로 새 support 박스 생성 && cond.FindMA20PullbackPattern 성립
//	발화: 장전 후 20봉 내 cond.IsMA20BullishBreakout (양봉 + 종가 edge 돌파) — 장전당 1회
//
// streak: MA20 연속 상승 요구 봉수 (R box·트리거 양쪽에 적용, 0 = 미적용 — 2026-07-04 +++ 폐지).
// 호출 전 indicator.PrepareCandles(candles) 필수.
func PullbackAnalyze(candles []*box.Candle, streak int) []PullbackSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	settings := DefaultSettings() // DefBox 생성 조건(DamOption)용 — 국면a 속성 기록에 필요
	var signals []PullbackSignal
	armed := false
	var armPos int
	var armPat *cond.PullbackPattern

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

		prevKey := candles[i-1].Curvekey
		prevBoxCount := len(ctx.BoxList)

		candles[i].Curvekey = box.AnalyzeCurvature(ctx)
		// 새 추세 박스 판정은 DefBox 생성 이전 시점으로 고정 (DefBox가 카운트를 오염시키지 않도록)
		newTrendBox := len(ctx.BoxList) > prevBoxCount
		// DefBox 생성 (stg/analyzer.go·WDefBoxAnalyze와 동일 경로) — 국면a 속성용.
		// 패턴 구조 탐지는 FindMA20PullbackPattern에서 DefBox를 제외하므로 기존 신호 불변.
		box.CheckAndCreateDefBox(ctx, settings.DamOption)

		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// 장전 만료
		if armed && i > armPos+PullbackArmWindowBars {
			armed = false
		}

		// 하락→상승 전환 = 새 support 박스 (눌림 바닥 후보) → R→S 구조 체크
		if prevKey < 0 && candles[i].Curvekey > 0 && newTrendBox {
			if p, ok := cond.FindMA20PullbackPattern(ctx, streak); ok {
				armed = true
				armPos = i
				armPat = p
			}
		}

		// 발화: 양봉 + 종가 기준 20이평 상향 돌파 (streak>0이면 MA20 연속 상승 추가)
		if armed && i > armPos && cond.IsMA20BullishBreakout(candles, i, streak) {
			above, dist, exists := cond.LastDefBoxAboveMA20(ctx, i)
			signals = append(signals, PullbackSignal{
				Date:    candles[i].Date,
				ArmDate: candles[armPos].Date,
				Shcode:  ctx.Shcode,
				Pos:     i,
				ArmPos:  armPos,
				Pattern: *armPat,
				DefBoxAboveMA20: above,
				DefBoxExists:    exists,
				DefBoxDistPct:   dist,
				BBExpanding:     cond.IsBBExpanding(candles, i),
			})
			armed = false // 장전당 1회
		}
	}

	return signals
}
