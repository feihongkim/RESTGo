package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// WPatternSignal 은 W-bottom 패턴 신호 1건
type WPatternSignal struct {
	Date   string // 진입 캔들 날짜
	P1Date string // P1 박스 날짜
	P2Date string // P2 박스 날짜
	Shcode string
	Pos    int // 진입 캔들 인덱스
	P1Pos  int // P1 박스 캔들 인덱스
	P2Pos  int // P2 박스 캔들 인덱스
}

// WPatternAnalyze 는 DefBox 돌파와 무관하게 W-bottom 패턴(MIIIb)을 탐지한다.
// 하단Box(BB이탈) → 상단Box → 하단Box(BB내부) 연속 시퀀스가 완성되는 캔들에서 신호 발화.
//
// 호출 전 indicator.PrepareCandles(candles) 를 반드시 실행해야 한다 (Bollinger 데이터 필요).
// stg.Analyze와 달리 DefBox/DamChecker 게이트 없이 Box 시퀀스만 추적한다.
func WPatternAnalyze(candles []*box.Candle) []WPatternSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	lookback := DefaultSettings().BBWBottomLookback
	var signals []WPatternSignal

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

		// 하락→상승 전환 = 새 하단Box 추가 → W 패턴 체크
		// 진입 캔들 종가가 MA20 아래여야 함
		if prevKey < 0 && candles[i].Curvekey > 0 && len(ctx.BoxList) > prevBoxCount &&
			candles[i].Ma20 > 0 && candles[i].Close < candles[i].Ma20 {
			if p1Pos, p2Pos, ok := cond.FindBBWBottomBoxPattern(ctx, lookback); ok {
				signals = append(signals, WPatternSignal{
					Date:   candles[i].Date,
					P1Date: candles[p1Pos].Date,
					P2Date: candles[p2Pos].Date,
					Shcode: ctx.Shcode,
					Pos:    i,
					P1Pos:  p1Pos,
					P2Pos:  p2Pos,
				})
			}
		}
	}

	return signals
}
