package cond

import (
	"math"

	"RESTGo/box"
)

// ── FollowUp 매수 조건 포팅 ────────────────────────────────────────────────────
// C# 참조: Stock1/biz/Processors/BuyDecisionProcessor.FollowUp.cs
// C# 참조: Stock1/biz/Processors/BuyConditionProcessor.cs (EvaluateMultiDefWaiting/WeakGroundRecovery)
// C# 참조: Stock1/biz/Evaluators/PenetrationEvaluator.cs (IsShortRangeValid, GetMultiDefboxDamcount)
// C# 참조: Stock1/biz/Evaluators/OscillatorEvaluator.cs (LastOscilloPosition, OscilloAnalyzer, HighPrice)
// C# 참조: Stock1/biz/Evaluators/VolumeConditionEvaluator.cs (IsVolumeBreakout)

// oscilloScanByPrice 는 boxprice 기준 [start, end] 구간의 상/하향 돌파 위치들을 수집한다.
// C# OscillatorEvaluator.LastOscilloPosition(가격 버전) — first/last/count를 모두 반환.
// (buy_oscillator.go의 LastOscilloPositionByPrice는 last만 반환하는 간략판)
func oscilloScanByPrice(candles []*box.Candle, boxprice float64, start, end int) oscilResult {
	if start < 0 {
		start = 0
	}
	if end >= len(candles) {
		end = len(candles) - 1
	}
	var arr []int
	tempkey := 0
	for i := start; i <= end; i++ {
		c := candles[i]
		if tempkey == -1 && (c.Close > boxprice || c.Open > boxprice) {
			arr = append(arr, i)
		}
		if tempkey == 1 && (c.Close < boxprice || c.Open < boxprice) {
			arr = append(arr, i)
		}
		if c.Close < boxprice {
			tempkey = -1
		}
		if c.Close > boxprice {
			tempkey = 1
		}
	}
	if len(arr) == 0 {
		return oscilResult{}
	}
	return oscilResult{firstPos: arr[0], lastPos: arr[len(arr)-1], count: len(arr)}
}

// OscilloAnalyzer 는 boxprice 기준 진동 패턴을 분석해
// "순정지지"/"순정저항"/"미확인"/"돌파확정"/"붕괴확정"/"무질서횡보"/"진동지지"/"진동저항"/"" 를 반환한다.
// C# OscillatorEvaluator.OscilloAnalyzer 1:1 포팅.
// 주의: C# 원본의 !Cond2 분기는 tempLastResiPosition을 갱신하면서 내부 루프 상한으로는
// tempLastSupportPosition을 사용한다 (의심 코드지만 동일하게 보존).
func OscilloAnalyzer(candles []*box.Candle, boxprice float64, exposition, position int) string {
	if exposition < 0 {
		exposition = 0
	}
	if position >= len(candles) {
		position = len(candles) - 1
	}
	osc := oscilloScanByPrice(candles, boxprice, exposition, position)
	lastOscillo := osc.lastPos
	conclusion := ""
	lastSupport := -1
	lastResi := -1

	if osc.firstPos == 0 {
		for i := exposition; i <= position; i++ {
			c := candles[i]
			if c.Low <= boxprice && c.Open >= boxprice && c.Close >= boxprice {
				lastSupport = i
			} else if c.High >= boxprice && c.Open <= boxprice && c.Close <= boxprice {
				lastResi = i
			}
		}
		cur := candles[position]
		if lastSupport > 0 {
			if cur.Close > cur.Ma5 && cur.Gradient > -0.5 && cur.Low <= cur.Ma5 {
				conclusion = "순정지지"
			}
			return conclusion
		}
		if lastResi > 0 {
			if cur.Close < cur.Ma5 && cur.Gradient < 0.5 && cur.High >= cur.Ma5 {
				conclusion = "순정저항"
			}
			return conclusion
		}
		if lastSupport == 0 && lastResi == 0 {
			conclusion = "미확인"
		}
		return conclusion
	}

	// FirstOscilloPosition > 0
	harmCount, harmCount2 := 0, 0
	for i := osc.firstPos; i <= position; i++ {
		if i < 1 {
			continue
		}
		c, prev := candles[i], candles[i-1]
		if c.Close < boxprice && (c.Close-c.Open) < 0.0 &&
			prev.Close < boxprice && (prev.Close-prev.Open) > 0.0 {
			harmCount++
		}
		if c.Close > boxprice && (c.Close-c.Open) > 0.0 &&
			prev.Close > boxprice && (prev.Close-prev.Open) < 0.0 {
			harmCount2++
		}
	}
	cond1 := harmCount != 0
	if harmCount2 != 0 {
		conclusion = "돌파확정"
	}
	if cond1 {
		conclusion = "붕괴확정"
	}
	if harmCount != 0 && harmCount2 != 0 {
		conclusion = "무질서횡보"
	}

	cur := candles[position]
	if !cond1 {
		for i := exposition; i <= position; i++ {
			c := candles[i]
			if c.Low <= boxprice && c.Open >= boxprice && c.Close >= boxprice {
				lastSupport = i
			}
		}
		lo := candles[lastOscillo]
		checkCont := 0
		if lo.Gradient < 0.0 && lo.Gradient < lo.Gradient20 && lo.Ma5 > boxprice {
			for i := lastOscillo; i <= lastSupport; i++ {
				if (candles[i].Close - candles[i].Open) <= 0.0 {
					checkCont++
				}
			}
		} else {
			for i := lastOscillo - 1; i <= lastSupport; i++ {
				if i < 0 {
					continue
				}
				if (candles[i].Close - candles[i].Open) <= 0.0 {
					checkCont++
				}
			}
		}
		if lastOscillo <= lastSupport && checkCont != 0 &&
			cur.Close > cur.Ma5 &&
			cur.Gradient > (-1.0*math.Abs(cur.Gradient20)) &&
			cur.Low <= cur.Ma5 {
			conclusion = "진동지지"
		}
	}
	if harmCount2 == 0 {
		for i := exposition; i <= position; i++ {
			c := candles[i]
			if c.High >= boxprice && c.Open <= boxprice && c.Close <= boxprice {
				lastResi = i
			}
		}
		lo := candles[lastOscillo]
		checkCont2 := 0
		if lo.Gradient > 0.0 && lo.Gradient > lo.Gradient20 && lo.Ma5 < boxprice {
			for i := lastOscillo; i <= lastSupport; i++ {
				if i < 0 {
					continue
				}
				if (candles[i].Close - candles[i].Open) >= 0.0 {
					checkCont2++
				}
			}
		} else {
			for i := lastOscillo - 1; i <= lastSupport; i++ {
				if i < 0 {
					continue
				}
				if (candles[i].Close - candles[i].Open) >= 0.0 {
					checkCont2++
				}
			}
		}
		if lastOscillo <= lastResi && checkCont2 != 0 &&
			cur.Close < cur.Ma5 &&
			cur.Gradient < math.Abs(cur.Gradient20) &&
			cur.High >= cur.Ma5 {
			conclusion = "진동저항"
		}
	}
	return conclusion
}

// IsShortRangeValid 는 돌파 후 단기 진동지지 패턴(SR-진동지지)을 판정한다.
// C# PenetrationEvaluator.IsShortRangeValid 포팅.
func IsShortRangeValid(ctx *box.TradingContext) bool {
	candles := ctx.CandleList
	position := ctx.Position
	penPosition := ctx.PenPosition
	defboxPrice := ctx.DefboxPrice

	if position-penPosition > 40 {
		return false
	}
	if HighPriceInRange(candles, penPosition, position) >= 1.15*defboxPrice {
		return false
	}

	mGrad5MV := firstGradientChecker(candles, penPosition, position)
	osc := oscilloScanByPrice(candles, defboxPrice, penPosition, position)
	if mGrad5MV <= 0 || osc.firstPos <= 0 {
		return false
	}

	pGrad5MV := firstGradientCheckerUp(candles, mGrad5MV, position)
	tempHigh := HighPriceInRange(candles, penPosition, osc.firstPos)
	highDamChecker := 0
	for i := osc.firstPos; i < position; i++ {
		if candles[i].Close > tempHigh {
			highDamChecker++
		}
	}

	return pGrad5MV > 0 && highDamChecker == 0 &&
		OscilloAnalyzer(candles, defboxPrice, penPosition, position) == "진동지지"
}

// GetMultiDefboxDamcountPen 는 Penetration 버전 Multi-DefBox 돌파 횟수.
// C# PenetrationEvaluator.GetMultiDefboxDamcount 포팅 (St2_dev 동일):
// MainDefLink[DefCount-1] 기준 exposition, i <= position (현재 캔들 포함),
// Close/Open 각각 별도 카운트. (BoxConditionEvaluator 버전 GetMultiDefboxDamCount와 별개 함수)
func GetMultiDefboxDamcountPen(ctx *box.TradingContext) int {
	boxList := ctx.BoxList
	if ctx.DefboxIndex < 0 || ctx.DefboxIndex >= len(boxList) {
		return 0
	}
	defBox := boxList[ctx.DefboxIndex]
	if len(defBox.MainDefLink) == 0 {
		return 0
	}
	mainboxIndex := defBox.MainDefLink[len(defBox.MainDefLink)-1]
	if mainboxIndex < 0 || mainboxIndex >= len(boxList) {
		return 0
	}
	exposition := boxList[mainboxIndex].BoxPosition
	damcount := 0
	for i := exposition; i <= ctx.Position && i < len(ctx.CandleList); i++ {
		if ctx.CandleList[i].Close > ctx.DefboxPrice {
			damcount++
		}
		if ctx.CandleList[i].Open > ctx.DefboxPrice {
			damcount++
		}
	}
	return damcount
}

// IsVolumeBreakout 는 거래대금 기준 돌파 유효성 검사.
// C# VolumeConditionEvaluator.IsVolumeBreakout 포팅:
// Ma5(원본가) × VolMa5 >= volumeLimit × 100,000
func IsVolumeBreakout(c *box.Candle, volumeLimit float64) bool {
	if c == nil || volumeLimit <= 0 {
		return false
	}
	return c.Ma5Origin*c.VolMa5 >= volumeLimit*100000.0
}

// ── MultiDef 대기매수 조건 (C# BuyConditionProcessor.EvaluateMultiDefWaitingConditions) ──

// IsHighPriceWithinDefBoxLimit: PenPosition 이후 최고가가 DefBox 가격의 110% 이하.
func IsHighPriceWithinDefBoxLimit(ctx *box.TradingContext) bool {
	return HighPriceInRange(ctx.CandleList, ctx.PenPosition, ctx.Position) < 1.1*ctx.DefboxPrice
}

// IsPositionProgressedFromPen: 현재가 PenPosition보다 진행됨.
func IsPositionProgressedFromPen(ctx *box.TradingContext) bool {
	return ctx.Position > ctx.PenPosition
}

// IsPriceRecrossedDefBox: DefBox 아래로 내려갔다가(시가 또는 전일 종가) 종가로 재돌파.
func IsPriceRecrossedDefBox(ctx *box.TradingContext) bool {
	cur := ctx.GetCurrentCandle()
	prev := ctx.GetPreviousCandle(1)
	if cur == nil || prev == nil {
		return false
	}
	return (cur.Open < ctx.DefboxPrice || prev.Close < ctx.DefboxPrice) &&
		cur.Close > ctx.DefboxPrice
}

// ── 연약지반 회복 조건 (C# BuyConditionProcessor.EvaluateWeakGroundRecoveryConditions) ──

// HasOscilloBreakout: PenPosition 이후 DefBox 가격 돌파 위치 존재.
func HasOscilloBreakout(ctx *box.TradingContext) bool {
	return LastOscilloPositionByPrice(ctx.CandleList, ctx.DefboxPrice, ctx.PenPosition, ctx.Position) > 0
}

// HasNoBullishCandleSinceMomentum: MomentumPosition 이후 양봉이 없었음.
// C#과 동일하게 "첫 양봉 위치 == 0"으로 판정 (인덱스 0의 양봉은 무시되는 quirk 보존).
func HasNoBullishCandleSinceMomentum(ctx *box.TradingContext) bool {
	firstBullish := 0
	for i := ctx.MomentumPosition; i < ctx.Position && i < len(ctx.CandleList); i++ {
		if i >= 0 && ctx.CandleList[i].Close > ctx.CandleList[i].Open {
			firstBullish = i
			break
		}
	}
	return firstBullish == 0
}

// IsPriceRecrossedDefBoxForWeakGround: 전일 종가는 DefBox 아래, 당일 종가는 DefBox 이상.
func IsPriceRecrossedDefBoxForWeakGround(ctx *box.TradingContext) bool {
	cur := ctx.GetCurrentCandle()
	prev := ctx.GetPreviousCandle(1)
	if cur == nil || prev == nil {
		return false
	}
	return cur.Close >= ctx.DefboxPrice && prev.Close < ctx.DefboxPrice
}

// IsMultiDefWaitToBuyCondition 는 multidef매수대기 → 즉시매수 전환 조건.
// C# BuyDecisionProcessor.FollowUp.IsMultiDefWaitToBuyCondition 포팅.
// 주의: 현재 C# 파이프라인에서 BuyHelper가 "multidef매수대기"로 설정되는 코드 경로가 없어
// 사실상 도달 불가(사문) — 동일하게 보존한다.
func IsMultiDefWaitToBuyCondition(ctx *box.TradingContext) bool {
	if ctx.BuyHelper != "multidef매수대기" {
		return false
	}
	if ctx.Position <= ctx.PenPosition {
		return false
	}
	candles := ctx.CandleList
	penPosPlus1 := ctx.PenPosition + 1
	if penPosPlus1 >= len(candles) {
		return false
	}
	if (candles[penPosPlus1].Close - candles[penPosPlus1].Open) > 0.0 {
		return false // PenPosition+1 캔들이 양봉이면 false
	}
	pos := ctx.Position
	openBelowDefbox := candles[pos].Open < ctx.DefboxPrice
	prevCloseBelowDefbox := pos > 0 && candles[pos-1].Close < ctx.DefboxPrice
	if !openBelowDefbox && !prevCloseBelowDefbox {
		return false
	}
	if (candles[pos].Close - candles[pos].Open) <= 0.0 {
		return false // 음봉이면 false
	}
	return candles[pos].Close >= ctx.DefboxPrice // DefBox 재돌파 양봉
}
