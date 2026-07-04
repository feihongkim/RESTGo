package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// MTopSignal 은 상방 M자 패턴 매도 신호 1건.
// 2단계 구조: M 완성(장전, ArmPos) → 20이평 부근 음양음 붕괴(발화, Pos).
type MTopSignal struct {
	Date    string // 붕괴(트리거) 캔들 날짜 — 매도 신호 시점
	ArmDate string // M 완성(마지막 resist 인지) 캔들 날짜
	P1Date  string // 첫 resist 박스 날짜 (BB 상단 이탈 고점)
	P2Date  string // 마지막 resist 박스 날짜
	Shcode  string
	Pos     int // 트리거 캔들 인덱스
	ArmPos  int // 장전 캔들 인덱스
	P1Pos   int
	P2Pos   int
}

// MTopArmWindowBars 는 M 완성 후 붕괴 트리거 유효 기간 (거래일 봉).
const MTopArmWindowBars = 20

// MPatternAnalyze 는 상방 M자 패턴 매도 신호를 탐지한다 (WPatternAnalyze의 상하 대칭 + 2단계).
//
//	장전: 상승→하락 변곡으로 새 resist 박스 생성 && 종가가 MA20 위 && FindBBMTopBoxPattern 성립
//	발화: 장전 후 20봉 내에 20이평 부근 음-양-음 런 붕괴(cond.FindMTopCollapseRuns) — 장전당 1회
//
// DefBox/DamChecker 게이트 없음. 호출 전 indicator.PrepareCandles(candles) 필수.
func MPatternAnalyze(candles []*box.Candle) []MTopSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	lookback := DefaultSettings().BBWBottomLookback
	var signals []MTopSignal

	armed := false
	var armPos, armP1, armP2 int

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
		if armed && i > armPos+MTopArmWindowBars {
			armed = false
		}

		// 상승→하락 전환 = 새 resist 박스 → M 패턴 체크 (종가가 MA20 위에서 완성)
		if prevKey > 0 && candles[i].Curvekey < 0 && len(ctx.BoxList) > prevBoxCount &&
			candles[i].Ma20 > 0 && candles[i].Close > candles[i].Ma20 {
			if p1Pos, p2Pos, ok := cond.FindBBMTopBoxPattern(ctx, lookback); ok {
				armed = true
				armPos, armP1, armP2 = i, p1Pos, p2Pos
			}
		}

		// 발화: 20이평 부근 음양음 붕괴 (런 시작은 P2 박스 이후여야 함)
		if armed && i > armPos {
			if _, ok := cond.FindMTopCollapseRuns(candles, i, armP2); ok {
				signals = append(signals, MTopSignal{
					Date:    candles[i].Date,
					ArmDate: candles[armPos].Date,
					P1Date:  candles[armP1].Date,
					P2Date:  candles[armP2].Date,
					Shcode:  ctx.Shcode,
					Pos:     i,
					ArmPos:  armPos,
					P1Pos:   armP1,
					P2Pos:   armP2,
				})
				armed = false // 장전당 1회 발화
			}
		}
	}

	return signals
}
