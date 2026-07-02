package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
)

// CombinedSignal 은 WD(W+DefBox) 또는 S1(DefBox 단독 돌파) 신호 1건.
// WD: W-신호 진입 50% + DefBox 돌파 진입 50% (돌파 없으면 50%만 배치)
// S1: W-패턴 없이 DefBox 돌파 진입 100%
type CombinedSignal struct {
	Type              string  `json:"type"` // "WD" or "S1"
	Shcode            string  `json:"shcode"`
	WSignalDate       string  `json:"w_signal_date,omitempty"`
	P1Date            string  `json:"p1_date,omitempty"`
	P2Date            string  `json:"p2_date,omitempty"`
	DefBoxDate        string  `json:"defbox_date,omitempty"`
	DefBoxPrice       float64 `json:"defbox_price,omitempty"`       // 스케일 가격
	DefBoxPriceOrigin float64 `json:"defbox_price_origin,omitempty"` // 원본 가격
	DefBoxBreakDate   string  `json:"defbox_break_date,omitempty"`
}

type wdPending struct {
	signalPos         int
	signalDate        string
	p1Date            string
	p2Date            string
	defBoxListIdx     int // ctx.BoxList 인덱스
	defBoxPrice       float64
	defBoxPriceOrigin float64
	defBoxDate        string
}

// CombinedAnalyze 는 W+DefBox(WD)와 DefBox 단독 돌파(S1) 신호를 단일 패스로 탐지한다.
// indicator.PrepareCandles(candles) 호출 후 사용해야 한다.
func CombinedAnalyze(candles []*box.Candle) []CombinedSignal {
	if len(candles) < 60 {
		return nil
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	settings := DefaultSettings()
	lookback := settings.BBWBottomLookback

	var out []CombinedSignal
	var pending []*wdPending

	// S1 추적 상태
	curDefBoxListIdx := -1
	curDefBoxBroken := false

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

		// 1. 곡률 + DefBox 생성
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)
		box.CheckAndCreateDefBox(ctx, settings.DamOption)

		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// 2. W-패턴 탐지 → pending 추가 (S1 체크보다 먼저: WD가 S1보다 우선)
		if prevKey < 0 && candles[i].Curvekey > 0 && len(ctx.BoxList) > prevBoxCount &&
			candles[i].Ma20 > 0 && candles[i].Close < candles[i].Ma20 {

			if p1Pos, p2Pos, ok := cond.FindBBWBottomBoxPattern(ctx, lookback); ok {
				defIdx := findDefBoxBefore(ctx.BoxList, p1Pos)
				if defIdx >= 0 {
					defBox := ctx.BoxList[defIdx]
					pending = append(pending, &wdPending{
						signalPos:         i,
						signalDate:        candles[i].Date,
						p1Date:            candles[p1Pos].Date,
						p2Date:            candles[p2Pos].Date,
						defBoxListIdx:     defIdx,
						defBoxPrice:       defBox.Price,
						defBoxPriceOrigin: defBox.PriceOrigin,
						defBoxDate:        candles[defBox.BoxPosition].Date,
					})
				}
			}
		}

		// 3. S1 체크: 이전 반복에서 갱신된 curDefBox 기준 (이번 반복 신규 DefBox 제외)
		if curDefBoxListIdx >= 0 && !curDefBoxBroken {
			// 동일 DefBox를 타겟으로 하는 pending WD가 있으면 WD가 처리
			wdClaimed := false
			for _, pw := range pending {
				if pw.defBoxListIdx == curDefBoxListIdx {
					wdClaimed = true
					break
				}
			}
			curDB := ctx.BoxList[curDefBoxListIdx]
			if !wdClaimed && candles[i].Close > curDB.Price {
				curDefBoxBroken = true
				out = append(out, CombinedSignal{
					Type:              "S1",
					Shcode:            ctx.Shcode,
					DefBoxDate:        candles[curDB.BoxPosition].Date,
					DefBoxPrice:       curDB.Price,
					DefBoxPriceOrigin: curDB.PriceOrigin,
					DefBoxBreakDate:   candles[i].Date,
				})
			}
		}

		// 4. Pending WD 처리 (만료 또는 돌파): age >= 1 필요 (W-신호 당일 돌파 불가)
		nextPending := pending[:0]
		for _, pw := range pending {
			age := i - pw.signalPos
			if age > defBoxBreakTimeout {
				// 만료 — 돌파 없는 WD
				out = append(out, CombinedSignal{
					Type:              "WD",
					Shcode:            ctx.Shcode,
					WSignalDate:       pw.signalDate,
					P1Date:            pw.p1Date,
					P2Date:            pw.p2Date,
					DefBoxDate:        pw.defBoxDate,
					DefBoxPrice:       pw.defBoxPrice,
					DefBoxPriceOrigin: pw.defBoxPriceOrigin,
				})
				continue
			}
			if age > 0 && candles[i].Close > pw.defBoxPrice {
				// DefBox 돌파 — WD with break
				out = append(out, CombinedSignal{
					Type:              "WD",
					Shcode:            ctx.Shcode,
					WSignalDate:       pw.signalDate,
					P1Date:            pw.p1Date,
					P2Date:            pw.p2Date,
					DefBoxDate:        pw.defBoxDate,
					DefBoxPrice:       pw.defBoxPrice,
					DefBoxPriceOrigin: pw.defBoxPriceOrigin,
					DefBoxBreakDate:   candles[i].Date,
				})
				continue
			}
			nextPending = append(nextPending, pw)
		}
		pending = nextPending

		// 5. curDefBox 갱신 (이번 반복에서 새로 생성된 DefBox 포함)
		latestDefIdx := findLastDefBoxIndex(ctx.BoxList)
		if latestDefIdx > curDefBoxListIdx {
			curDefBoxListIdx = latestDefIdx
			curDefBoxBroken = false
		}
	}

	// 루프 종료 후 미완료 pending WD 처리
	for _, pw := range pending {
		out = append(out, CombinedSignal{
			Type:              "WD",
			Shcode:            ctx.Shcode,
			WSignalDate:       pw.signalDate,
			P1Date:            pw.p1Date,
			P2Date:            pw.p2Date,
			DefBoxDate:        pw.defBoxDate,
			DefBoxPrice:       pw.defBoxPrice,
			DefBoxPriceOrigin: pw.defBoxPriceOrigin,
		})
	}

	return out
}
