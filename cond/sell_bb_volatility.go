package cond

import "RESTGo/box"

// 주의: 이 파일의 BBWidth 임계값은 box.Candle.BollingerWidth 단위(percent, 예: 4.0 = 4%)에 맞춰야 한다.
// C# Settings는 ratio(0.04 = 4%)로 저장하므로 등록 레이어에서 × 100 변환이 필요하다.

// IsBBSqueeze 는 최근 minDuration 캔들이 모두 BBWidth < widthThreshold 인 BB 스퀴즈 패턴을 감지한다.
// C# BBVolatilityEvaluator.IsBBSqueeze 포팅.
func IsBBSqueeze(ctx *box.TradingContext, minDuration int, widthThreshold float64) bool {
	pos := ctx.Position
	if pos < minDuration {
		return false
	}
	count := 0
	for i := pos - minDuration + 1; i <= pos; i++ {
		if ctx.CandleList[i].BollingerWidth < widthThreshold {
			count++
		}
	}
	return count >= minDuration
}

// IsBBExpansion 은 최근 lookback 구간에 Squeeze가 존재했고 현재 BBWidth가 squeezeMinWidth × expansionRatio 이상이면 true.
// 두번째 반환값은 squeezeMinWidth (squeeze 미발견 시 0).
// C# BBVolatilityEvaluator.IsBBExpansion 포팅.
func IsBBExpansion(ctx *box.TradingContext, lookback int, widthThreshold, expansionRatio float64) (float64, bool) {
	pos := ctx.Position
	startPos := pos - lookback
	if startPos < 0 {
		startPos = 0
	}
	minWidth := -1.0
	for i := startPos; i < pos; i++ {
		w := ctx.CandleList[i].BollingerWidth
		if w < widthThreshold {
			if minWidth < 0 || w < minWidth {
				minWidth = w
			}
		}
	}
	if minWidth < 0 {
		return 0, false
	}
	cur := ctx.CandleList[pos].BollingerWidth
	return minWidth, cur >= minWidth*expansionRatio
}

// IsBBUpperWalking 은 최근 minDuration 캔들이 모두 %B > upperThreshold(0.80 등)인 강한 상승 추세 지속 패턴.
// C# BBVolatilityEvaluator.IsBBUpperWalking 포팅.
func IsBBUpperWalking(ctx *box.TradingContext, minDuration int, upperThreshold float64) bool {
	pos := ctx.Position
	if pos < minDuration {
		return false
	}
	count := 0
	for i := pos - minDuration + 1; i <= pos; i++ {
		if ctx.CandleList[i].BBPercent > upperThreshold {
			count++
		}
	}
	return count >= minDuration
}

// IsBBLowerWalking 은 최근 minDuration 캔들이 모두 %B < lowerThreshold(0.20 등)인 강한 하락 추세 지속 패턴.
// C# BBVolatilityEvaluator.IsBBLowerWalking 포팅.
func IsBBLowerWalking(ctx *box.TradingContext, minDuration int, lowerThreshold float64) bool {
	pos := ctx.Position
	if pos < minDuration {
		return false
	}
	count := 0
	for i := pos - minDuration + 1; i <= pos; i++ {
		if ctx.CandleList[i].BBPercent < lowerThreshold {
			count++
		}
	}
	return count >= minDuration
}

// HasRecentSqueeze 는 [pos-lookbackPeriod, pos] 구간 내 BBWidth < widthThreshold 캔들이 하나라도 있으면 true.
// C# BBVolatilityEvaluator.HasRecentSqueeze 포팅.
func HasRecentSqueeze(ctx *box.TradingContext, lookbackPeriod int, widthThreshold float64) bool {
	pos := ctx.Position
	startPos := pos - lookbackPeriod
	if startPos < 0 {
		startPos = 0
	}
	for i := startPos; i <= pos; i++ {
		if ctx.CandleList[i].BollingerWidth < widthThreshold {
			return true
		}
	}
	return false
}

// VolatilityRegime 분류값
type VolatilityRegime string

const (
	VolatilityHigh   VolatilityRegime = "High"
	VolatilityMedium VolatilityRegime = "Medium"
	VolatilityLow    VolatilityRegime = "Low"
)

// GetVolatilityRegime 은 현재 BBWidth 기준으로 변동성 체제를 분류한다.
// C# BBVolatilityEvaluator.GetVolatilityRegime 포팅.
func GetVolatilityRegime(ctx *box.TradingContext, highThreshold, lowThreshold float64) VolatilityRegime {
	w := ctx.CandleList[ctx.Position].BollingerWidth
	switch {
	case w > highThreshold:
		return VolatilityHigh
	case w < lowThreshold:
		return VolatilityLow
	default:
		return VolatilityMedium
	}
}
