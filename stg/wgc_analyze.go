package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// WGCSignal 은 W바텀(완화판) 신호 1건 + BB급락/GC-pending 속성.
// 2×2 분해용: bb_crash(급락 공황형 W) × gc_pending(골든크로스 임박 국면).
type WGCSignal struct {
	Date   string // 신호(P2 인식) 캔들 날짜
	P1Date string
	P2Date string
	Shcode string
	Pos    int
	// 속성 (게이트 아님)
	BBCrash   bool    // P1 급락 조건(BB 하단 이탈 + BBW 팽창) 충족 = 기존 W중력형
	GCPending bool    // 골든크로스 임박 (MA60<MA120, 20봉 축소, 간격≤3%)
	GCInvert  bool    // MA60 < MA120 (역배열 상태)
	GCGapPct  float64 // (MA120-MA60)/MA120 %
	GCShrink  int     // 연속 축소 체크포인트 수 (5봉 간격)
	HasDefBox bool    // P1 이전 DefBox 존재 (W중력 조건 — 참고 속성)
}

// WGCAnalyze 는 BB 급락 조건을 완화한 W바텀을 탐지하고 국면 속성을 기록한다 (2026-07-05).
// WPatternAnalyze와 동일 루프이되 FindBBWBottomBoxPatternRelaxed 사용 —
// 엄격판 충족 여부(BBCrash)와 GC-pending을 속성으로 남겨 한 스캔에서 2×2 분해가 가능하다.
// DefBox 존재(HasDefBox)도 기록 (W중력 조건과의 결합 확인용 — CheckAndCreateDefBox 경로 포함).
//
// 호출 전 indicator.PrepareCandles(candles) 필수.
func WGCAnalyze(candles []*box.Candle) []WGCSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	settings := DefaultSettings()
	lookback := settings.BBWBottomLookback
	var signals []WGCSignal

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
		newTrendBox := len(ctx.BoxList) > prevBoxCount
		// DefBox 생성 — HasDefBox 속성용이자 운영 경로 정합: stg.Analyze(룰 엔진)·WDefBoxAnalyze 모두
		// DefBox가 BoxList에 있는 상태에서 FindBBWBottomBoxPattern을 평가한다 (WPatternAnalyze만 예외).
		box.CheckAndCreateDefBox(ctx, settings.DamOption)

		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// 하락→상승 전환 = 새 하단Box → W 패턴 체크 (WPatternAnalyze와 동일: MA20 아래 완성)
		if prevKey < 0 && candles[i].Curvekey > 0 && newTrendBox &&
			candles[i].Ma20 > 0 && candles[i].Close < candles[i].Ma20 {
			if p1Pos, p2Pos, bbCrash, ok := cond.FindBBWBottomBoxPatternRelaxed(ctx, lookback); ok {
				pending, inv, gapPct, shrink := cond.GoldenCrossPendingInfo(candles, i)
				hasDef := false
				for _, b := range ctx.BoxList {
					if b.BoxPosition >= p1Pos {
						break
					}
					if b.KindOfBox == box.KindDefBox {
						hasDef = true
						break
					}
				}
				signals = append(signals, WGCSignal{
					Date:   candles[i].Date,
					P1Date: candles[p1Pos].Date,
					P2Date: candles[p2Pos].Date,
					Shcode: ctx.Shcode,
					Pos:    i,
					BBCrash: bbCrash, GCPending: pending, GCInvert: inv,
					GCGapPct: gapPct, GCShrink: shrink, HasDefBox: hasDef,
				})
			}
		}
	}

	return signals
}
