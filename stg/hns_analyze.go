package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// HNSSignal 은 해드앤숄더 매도 신호 1건.
// 2단계: 오른어깨 박스 인지(장전, ArmPos) → 넥라인 종가 하향 이탈(발화, Pos).
type HNSSignal struct {
	Date    string // 넥라인 이탈 캔들 날짜 — 매도 신호 시점
	ArmDate string // 오른어깨 인지 캔들 날짜
	Shcode  string
	Pos     int
	ArmPos  int
	Pattern cond.HNSPattern // 5박스 위치·가격 + 사후 분석 속성
}

// HNSArmWindowBars 는 오른어깨 인지 후 넥라인 이탈 유효 기간.
const HNSArmWindowBars = 20

// HNSAnalyze 는 해드앤숄더 천장 패턴 매도 신호를 탐지한다 (MPatternAnalyze와 동일 계보).
//
//	장전: 상승→하락 변곡으로 새 resist 박스(오른어깨) 생성 && cond.FindHNSPattern 성립
//	발화: 장전 후 20봉 내 종가가 넥라인(두 골 저점 연결 직선) 아래로 마감 — 장전당 1회
//
// 호출 전 indicator.PrepareCandles(candles) 필수.
func HNSAnalyze(candles []*box.Candle) []HNSSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	var signals []HNSSignal
	armed := false
	var armPos int
	var armPat *cond.HNSPattern

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

		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// 장전 만료
		if armed && i > armPos+HNSArmWindowBars {
			armed = false
		}

		// 상승→하락 전환 = 새 resist 박스 (오른어깨 후보) → H&S 5박스 체크
		if prevKey > 0 && candles[i].Curvekey < 0 && len(ctx.BoxList) > prevBoxCount {
			if p, ok := cond.FindHNSPattern(ctx, cond.HNSLookbackBars); ok {
				armed = true
				armPos = i
				armPat = p
			}
		}

		// 발화: 종가가 넥라인 아래로 마감
		if armed && i > armPos && candles[i].Close < armPat.NecklineValue(i) {
			signals = append(signals, HNSSignal{
				Date:    candles[i].Date,
				ArmDate: candles[armPos].Date,
				Shcode:  ctx.Shcode,
				Pos:     i,
				ArmPos:  armPos,
				Pattern: *armPat,
			})
			armed = false // 장전당 1회
		}
	}

	return signals
}
