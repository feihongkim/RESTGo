package box

import "math"

// IsLastBoxTypeZero 는 BoxList의 마지막 박스가 지지선(BoxType=0)인지 확인
func IsLastBoxTypeZero(boxList []*Box) bool {
	if len(boxList) == 0 {
		return false
	}
	return boxList[len(boxList)-1].BoxType == BoxTypeSupport
}

// IsCloseBelowMa5Breakout 는 오늘 종가가 MA5 아래로 이탈했는지 확인
func IsCloseBelowMa5Breakout(cur, prev *Candle) bool {
	return cur.Close < cur.Ma5 && prev.Close >= prev.Ma5
}

// IsDefBoxEntryCondition 는 DefBox 체크 진입 조건: MA5 하향 돌파 + 마지막 박스가 지지선
func IsDefBoxEntryCondition(boxList []*Box, cur, prev *Candle) bool {
	return IsCloseBelowMa5Breakout(cur, prev) && IsLastBoxTypeZero(boxList)
}

// IsBoxBreakout 는 박스 돌파 패턴 확인 (고가 터치 + 종가/시가 박스 아래)
func IsBoxBreakout(prices DefBoxPrices, boxPrice float64, boxPosition int) bool {
	return prices.HighBox >= boxPrice &&
		prices.CloseBox <= boxPrice &&
		prices.OpenBox <= boxPrice &&
		boxPosition != prices.HighPosition
}

// CalculateBoxDamage 는 박스 가격을 상향 돌파한 횟수(손상 횟수) 계산
func CalculateBoxDamage(candles []*Candle, boxPrice float64, startPos, endPos int) int {
	damageCount := 0
	for i := startPos + 3; i < endPos && i < len(candles); i++ {
		if i >= 2 &&
			candles[i-2].Close <= boxPrice &&
			candles[i-1].Close > boxPrice &&
			candles[i].Close > boxPrice {
			damageCount++
		}
	}
	return damageCount
}

// ShouldCreateDefBox 는 특정 Box에 대해 DefBox 생성 조건 충족 여부 확인
func ShouldCreateDefBox(b *Box, prices DefBoxPrices, candles []*Candle, currentPos, damageLimit int) bool {
	if b.BoxType != BoxTypeResistance {
		return false
	}
	if !IsBoxBreakout(prices, b.Price, b.BoxPosition) {
		return false
	}
	damage := CalculateBoxDamage(candles, b.Price, b.BoxPosition, currentPos)
	return damage <= damageLimit
}

// ShouldUpdateDefBox 는 기존 DefBox 업데이트가 필요한지 확인
func ShouldUpdateDefBox(boxList []*Box, highPosition int, boxPrice float64) bool {
	if len(boxList) == 0 {
		return false
	}
	last := boxList[len(boxList)-1]
	return last.BoxPosition != highPosition || math.Abs(last.Price-boxPrice) > 0.0001
}
