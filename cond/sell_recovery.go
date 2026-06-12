package cond

import (
	"math"

	"RESTGo/box"
)

// RecoveryParams 는 회복 가능성 평가에 사용되는 임계값들 (C# Settings 매핑).
type RecoveryParams struct {
	HighMaxDaysBelow          int     // 0.RecoveryHighMaxDaysBelow (예: 2)
	HighMaxDropRate           float64 // 0.RecoveryHighMaxDropRate (예: 0.05)
	HighMinRecoveryRate       float64 // 0.RecoveryHighMinRecoveryRate (예: 0.02)
	MediumMaxDaysBelow        int     // 0.RecoveryMediumMaxDaysBelow (예: 5)
	VolumeSupportBearishRatio float64 // 0.VolumeSupport_BearishVolumeRatio (예: 0.8)
	MA5Tolerance              float64 // 0.MaSupport_MA5Tolerance (예: 0.02)
	MA20Tolerance             float64 // 0.MaSupport_MA20Tolerance (예: 0.03)
}

// CriticalFailureParams 는 IsCriticalFailure 평가에 사용되는 임계값들.
type CriticalFailureParams struct {
	DailyDropThreshold      float64 // 0.CriticalFailure_DailyDropThreshold (음수 비율, 예: -0.10)
	PanicVolumeMultiplier   float64 // 0.CriticalFailure_PanicVolumeMultiplier (예: 2.0)
	PanicMinDropRate        float64 // 0.CriticalFailure_PanicMinDropRate (음수 비율, 기본 -0.05)
	CumulativeDropThreshold float64 // 0.CriticalFailure_CumulativeDropThreshold (음수 비율, 예: -0.15)
	CumulativeDropDays      int     // 0.CriticalFailure_CumulativeDropDays (예: 5)
	MAReversalDays          int     // 0.CriticalFailure_MAReversalDays (예: 3)
}

// EvaluateRecoveryPotential 은 MainBox 이탈 후 회복 가능성을 분류한다.
// C# RecoveryPotentialEvaluator.EvaluateRecoveryPotential 포팅.
//
// 주의: C# 원본에는 "IsCriticalFailure 문제가 많음. 다시 체크 필요." 주석이 달려 있다.
// 동일 로직을 그대로 포팅하며, 향후 별도 분석·수정 대상으로 남긴다.
func EvaluateRecoveryPotential(ctx *box.TradingContext, pos *box.TradePosition, p RecoveryParams) box.RecoveryPotential {
	daysBelow := CountDaysBelowMainBox(ctx, pos)
	dropDepth := calculateDropDepth(ctx, pos)
	recoveryAttempt := calculateRecoveryAttempt(ctx, pos)
	hasVolSupport := checkVolumeSupport(ctx, pos, p.VolumeSupportBearishRatio)
	hasMaSupport := checkMASupport(ctx, p.MA5Tolerance, p.MA20Tolerance)

	if daysBelow <= p.HighMaxDaysBelow &&
		dropDepth < p.HighMaxDropRate &&
		recoveryAttempt > p.HighMinRecoveryRate {
		return box.RecoveryHigh
	}

	if daysBelow <= p.MediumMaxDaysBelow && (hasMaSupport || hasVolSupport) {
		return box.RecoveryMedium
	}

	return box.RecoveryLow
}

// IsCriticalFailure 는 치명적 실패(즉시 100% 청산) 패턴을 감지한다.
// C# RecoveryPotentialEvaluator.IsCriticalFailure 포팅.
// 5가지 조건 중 하나라도 충족 시 true.
func IsCriticalFailure(ctx *box.TradingContext, pos *box.TradePosition, p CriticalFailureParams) bool {
	candles := ctx.CandleList
	curPos := ctx.Position
	cur := candles[curPos]

	// 1. 폭락 후 3일 경과 + MainBox 미회복
	checkRange := 3
	if checkRange > curPos+1 {
		checkRange = curPos + 1
	}
	crashPos := -1
	for i := 0; i < checkRange; i++ {
		checkPos := curPos - i
		c := candles[checkPos]
		if c.Open == 0 {
			continue
		}
		dailyReturn := (c.Close - c.Open) / c.Open
		if dailyReturn <= p.DailyDropThreshold {
			crashPos = checkPos
			break
		}
	}
	if crashPos >= 0 && curPos-crashPos >= 3 {
		recovered := false
		for i := crashPos + 1; i <= curPos; i++ {
			if candles[i].Close >= pos.MainBoxPrice {
				recovered = true
				break
			}
		}
		if !recovered {
			return true
		}
	}

	// 2. 모든 MA 아래 + 역배열
	if IsMA5BelowAll(cur) && IsReversedArrangement(cur) {
		return true
	}

	// 3. 패닉 매도 (대량 거래량 + 음봉)
	// C#: 매수 당일(holdingPeriod==0)은 평균 거래량 계산 불가로 여기서 함수 전체 종료
	// (조건 4·5도 평가하지 않음 — RecoveryPotentialEvaluator.cs:133-138)
	holdingPeriod := curPos - pos.BuyPosition
	if holdingPeriod == 0 {
		return false
	}
	volSum := 0.0
	count := 0
	for i := pos.BuyPosition; i < pos.BuyPosition+holdingPeriod && i < len(candles); i++ {
		volSum += candles[i].Volume
		count++
	}
	if count > 0 {
		avgVolume := volSum / float64(count)
		highVolumeRed := cur.Volume > avgVolume*p.PanicVolumeMultiplier && cur.Close < cur.Open
		if highVolumeRed && cur.Open != 0 {
			dailyReturn := (cur.Close - cur.Open) / cur.Open
			if dailyReturn <= p.PanicMinDropRate {
				return true
			}
		}
	}

	// 4. 5일 누적 하락률 임계 초과
	if curPos >= p.CumulativeDropDays {
		startPos := curPos - p.CumulativeDropDays
		startPrice := candles[startPos].Close
		if startPrice != 0 {
			cumulativeDrop := (cur.Close - startPrice) / startPrice
			if cumulativeDrop <= p.CumulativeDropThreshold {
				return true
			}
		}
	}

	// 5. MA 역배열 N일 이상 지속
	if countConsecutiveMAReversalDays(candles, curPos) >= p.MAReversalDays {
		return true
	}

	return false
}

// calculateDropDepth 는 (MainBoxPrice - 최저점) / MainBoxPrice 를 반환한다.
func calculateDropDepth(ctx *box.TradingContext, pos *box.TradePosition) float64 {
	if pos.MainBoxPrice == 0 {
		return 0
	}
	lowest := getLowestPriceAfterBuy(ctx, pos)
	return (pos.MainBoxPrice - lowest) / pos.MainBoxPrice
}

// calculateRecoveryAttempt 는 (현재가 - 최저점) / 최저점 을 반환한다.
func calculateRecoveryAttempt(ctx *box.TradingContext, pos *box.TradePosition) float64 {
	lowest := getLowestPriceAfterBuy(ctx, pos)
	if lowest == 0 {
		return 0
	}
	curPrice := ctx.CandleList[ctx.Position].Close
	return (curPrice - lowest) / lowest
}

// checkVolumeSupport 는 최근 5일 내 음봉 평균 거래량이 보유 기간 평균 × ratio 미만인지 확인한다.
func checkVolumeSupport(ctx *box.TradingContext, pos *box.TradePosition, ratio float64) bool {
	candles := ctx.CandleList
	curPos := ctx.Position
	start := curPos - 5
	if start < pos.BuyPosition {
		start = pos.BuyPosition
	}
	bearishVol := 0.0
	bearishCount := 0
	for i := start; i <= curPos; i++ {
		c := candles[i]
		if c.Close < c.Open {
			bearishVol += c.Volume
			bearishCount++
		}
	}
	if bearishCount == 0 {
		return false
	}
	bearishAvg := bearishVol / float64(bearishCount)

	overallVol := 0.0
	overallCount := 0
	for i := pos.BuyPosition; i <= curPos; i++ {
		overallVol += candles[i].Volume
		overallCount++
	}
	if overallCount == 0 {
		return false
	}
	overallAvg := overallVol / float64(overallCount)
	if overallAvg == 0 {
		return false
	}
	return bearishAvg < overallAvg*ratio
}

// checkMASupport 는 종가가 MA5 또는 MA20 근처(허용오차 이내)에 있고 추세를 유지하는지 확인한다.
func checkMASupport(ctx *box.TradingContext, ma5Tol, ma20Tol float64) bool {
	cur := ctx.CandleList[ctx.Position]
	ma5Support := false
	if cur.Ma5 != 0 {
		ma5Support = math.Abs(cur.Close-cur.Ma5)/cur.Ma5 < ma5Tol
	}
	ma20Support := false
	if cur.Ma20 != 0 {
		ma20Support = math.Abs(cur.Close-cur.Ma20)/cur.Ma20 < ma20Tol
	}
	ma5Rising := cur.Gradient > 0
	return (ma5Support && ma5Rising) || ma20Support
}

func getLowestPriceAfterBuy(ctx *box.TradingContext, pos *box.TradePosition) float64 {
	candles := ctx.CandleList
	lowest := math.MaxFloat64
	for i := pos.BuyPosition + 1; i <= ctx.Position; i++ {
		if candles[i].Low < lowest {
			lowest = candles[i].Low
		}
	}
	if lowest == math.MaxFloat64 {
		return pos.BuyPrice
	}
	return lowest
}

func countConsecutiveMAReversalDays(candles []*box.Candle, curPos int) int {
	consecutive := 0
	for i := curPos; i >= 0; i-- {
		c := candles[i]
		if c.Ma5 < c.Ma20 && c.Ma20 < c.Ma60 {
			consecutive++
		} else {
			break
		}
	}
	return consecutive
}
