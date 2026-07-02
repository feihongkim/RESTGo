package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// WDefBoxSignal 은 W-bottom 패턴 + DefBox 결합 신호 1건
type WDefBoxSignal struct {
	Date   string // W신호 진입 캔들 날짜
	P1Date string // 첫 번째 골(P1) 캔들 날짜
	P2Date string // 두 번째 골(P2) 캔들 날짜
	Shcode string // 종목코드
	Pos    int    // W신호 진입 캔들 인덱스
	P1Pos  int    // 첫 번째 골(P1) 캔들 인덱스
	P2Pos  int    // 두 번째 골(P2) 캔들 인덱스

	// DefBox 정보
	HasDefBox            bool    // P1 이전에 DefBox가 존재했는지
	DefBoxPos            int     // DefBox 박스 위치 인덱스
	DefBoxDate           string  // DefBox 박스 캔들 날짜
	DefBoxPrice          float64 // DefBox 스케일 가격 (돌파 기준)
	DefBoxPriceOrigin    float64 // DefBox 원본 가격 (차트 표시용)
	DefBoxBreakPos       int     // DefBox 돌파 캔들 인덱스 (-1=20일 내 미돌파)
	DefBoxBreakDate      string  // DefBox 돌파 캔들 날짜
}

const defBoxBreakTimeout = 20

// WDefBoxAnalyze 는 W-bottom 패턴을 탐지하되,
// P1 이전에 존재하는 DefBox 유무 및 W신호 이후 20일 내 DefBox 돌파 여부를 추가로 기록한다.
//
// 전략 의도:
//   - W신호 발화 시 50% 진입
//   - 이후 20일 이내 DefBox 돌파 시 나머지 50% 추가 진입
//
// 호출 전 indicator.PrepareCandles(candles) 를 반드시 실행해야 한다.
func WDefBoxAnalyze(candles []*box.Candle) []WDefBoxSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	settings := DefaultSettings()
	lookback := settings.BBWBottomLookback
	var signals []WDefBoxSignal

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

		// DefBox 생성 조건 체크 (stg/analyzer.go CheckAndCreateDefBox와 동일)
		box.CheckAndCreateDefBox(ctx, settings.DamOption)

		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// W-패턴 감지 (WPatternAnalyze와 동일 조건)
		if prevKey < 0 && candles[i].Curvekey > 0 && len(ctx.BoxList) > prevBoxCount &&
			candles[i].Ma20 > 0 && candles[i].Close < candles[i].Ma20 {

			if p1Pos, p2Pos, ok := cond.FindBBWBottomBoxPattern(ctx, lookback); ok {
				sig := WDefBoxSignal{
					Date:            candles[i].Date,
					P1Date:          candles[p1Pos].Date,
					P2Date:          candles[p2Pos].Date,
					Shcode:          ctx.Shcode,
					Pos:             i,
					P1Pos:           p1Pos,
					P2Pos:           p2Pos,
					DefBoxBreakPos:  -1,
				}

				// P1 이전에 존재하는 마지막 DefBox 탐색
				defBoxIdx := findDefBoxBefore(ctx.BoxList, p1Pos)
				if defBoxIdx >= 0 {
					defBox := ctx.BoxList[defBoxIdx]
					sig.HasDefBox = true
					sig.DefBoxPos = defBox.BoxPosition
					sig.DefBoxDate = candles[defBox.BoxPosition].Date
					sig.DefBoxPrice = defBox.Price             // 스케일 가격
					sig.DefBoxPriceOrigin = defBox.PriceOrigin // 원본 가격 (차트용)

					// W신호 이후 20캔들 이내 DefBox 종가 돌파 체크
					for j := i + 1; j <= i+defBoxBreakTimeout && j < len(candles); j++ {
						if candles[j].Close > sig.DefBoxPrice {
							sig.DefBoxBreakPos = j
							sig.DefBoxBreakDate = candles[j].Date
							break
						}
					}
				}

				signals = append(signals, sig)
			}
		}
	}

	return signals
}

// findDefBoxBefore 는 BoxList에서 beforePos 이전에 위치한 마지막 DefBox 인덱스를 반환.
// 없으면 -1 반환.
func findDefBoxBefore(boxList []*box.Box, beforePos int) int {
	result := -1
	for i, b := range boxList {
		if b.BoxPosition >= beforePos {
			break
		}
		if b.KindOfBox == box.KindDefBox {
			result = i
		}
	}
	return result
}
