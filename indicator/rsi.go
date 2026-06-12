package indicator

import "RESTGo/box"

// CalculateRSI 는 Wilder 평활법 RSI(period=14)를 계산하여 Candle.RSI에 저장합니다.
// 스케일된 종가(Close) 기준이며 값은 0~100 범위입니다.
func CalculateRSI(candles []*box.Candle, period int) {
	if len(candles) <= period {
		return
	}

	// 첫 period 구간: 단순 평균으로 초기 avgGain/avgLoss 계산
	var sumGain, sumLoss float64
	for i := 1; i <= period; i++ {
		diff := candles[i].Close - candles[i-1].Close
		if diff > 0 {
			sumGain += diff
		} else {
			sumLoss -= diff
		}
	}
	avgGain := sumGain / float64(period)
	avgLoss := sumLoss / float64(period)

	setRSI(candles, period, avgGain, avgLoss)

	// 이후: Wilder 평활 (EMA 방식)
	for i := period + 1; i < len(candles); i++ {
		diff := candles[i].Close - candles[i-1].Close
		gain, loss := 0.0, 0.0
		if diff > 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
		setRSI(candles, i, avgGain, avgLoss)
	}
}

func setRSI(candles []*box.Candle, i int, avgGain, avgLoss float64) {
	if avgLoss == 0 {
		candles[i].RSI = 100
		return
	}
	rs := avgGain / avgLoss
	candles[i].RSI = 100 - (100 / (1 + rs))
}
