package box

import "math"

// ShouldReverseToBearish 는 상승->하락 전환 조건 판단
func ShouldReverseToBearish(cur, prev1, prev2 *Candle) bool {
	return (isAcceleratingDowntrend(cur, prev1, prev2) && isStrongDownwardSlope(cur, prev1)) ||
		isShortTermWeaknessWithLongTermUptrend(cur, prev1, prev2)
}

// ShouldReverseToBullish 는 하락->상승 전환 조건 판단
func ShouldReverseToBullish(cur, prev1, prev2 *Candle) bool {
	return isAcceleratingUptrend(cur, prev1, prev2) && isStrongUpwardSlope(cur, prev1)
}

func isAcceleratingDowntrend(cur, prev1, prev2 *Candle) bool {
	curSlope := math.Abs(cur.Gradient) - math.Abs(prev1.Gradient)
	prevSlope := math.Abs(prev1.Gradient) - math.Abs(prev2.Gradient)
	return cur.Gradient < 0.0 &&
		((curSlope > 0.0 && prev1.Gradient < 0.0) ||
			(prevSlope > 0.0 && prev1.Gradient < 0.0))
}

func isStrongDownwardSlope(cur, prev *Candle) bool {
	return cur.Gradient < -0.17 || prev.Gradient < -0.17
}

func isShortTermWeaknessWithLongTermUptrend(cur, prev1, prev2 *Candle) bool {
	allBelowMa5 := prev2.Close < prev2.Ma5 && prev1.Close < prev1.Ma5 && cur.Close < cur.Ma5
	trendReversal := prev2.Gradient >= 0.0 && prev1.Gradient >= 0.0 && cur.Gradient < 0.0
	longTermUp := cur.Gradient20 > cur.Gradient60 && cur.Gradient60 > 0.0
	return allBelowMa5 && trendReversal && longTermUp
}

func isAcceleratingUptrend(cur, prev1, prev2 *Candle) bool {
	curSlope := math.Abs(cur.Gradient) - math.Abs(prev1.Gradient)
	prevSlope := math.Abs(prev1.Gradient) - math.Abs(prev2.Gradient)
	return cur.Gradient > 0.0 &&
		((curSlope > 0.0 && prev1.Gradient > 0.0) ||
			(prevSlope > 0.0 && prev1.Gradient > 0.0))
}

func isStrongUpwardSlope(cur, prev *Candle) bool {
	return cur.Gradient > 0.17 || prev.Gradient > 0.17
}
