package indicator

import (
	"RESTGo/box"
	"math"
)

// PairBar 은 페어 스프레드 1봉 — log(A/B) 및 z-score.
type PairBar struct {
	Index   int
	Date    string
	Time    string
	PriceA  float64
	PriceB  float64
	LogSpr  float64 // log(A) − log(B) = log(A/B)
	Mean    float64 // 롤링 평균 (window 직전 봉까지, look-ahead 방지)
	Std     float64 // 롤링 표준편차
	Z       float64 // (LogSpr − Mean) / Std
	Valid   bool    // 워밍업 완료 여부
}

// ComputePairBars 는 페어 캔들 슬라이스로부터 log-spread + 롤링 z-score를 계산한다.
//
// window: 롤링 mean/std 윈도우 봉 수 (기본 480 = 5일)
// Mean/Std는 t-window ~ t-1 범위로 계산 (현재 봉 미포함, look-ahead 방지).
func ComputePairBars(paired []box.PairedCandle, window int) []PairBar {
	bars := make([]PairBar, len(paired))
	for i, p := range paired {
		bars[i].Index = i
		bars[i].Date = p.Date
		bars[i].Time = p.Time
		bars[i].PriceA = p.A.CloseOrigin
		bars[i].PriceB = p.B.CloseOrigin
		bars[i].LogSpr = math.Log(p.A.CloseOrigin) - math.Log(p.B.CloseOrigin)
	}
	// 롤링 mean/std
	for i := window; i < len(bars); i++ {
		sum, sumSq := 0.0, 0.0
		for j := i - window; j < i; j++ {
			sum += bars[j].LogSpr
			sumSq += bars[j].LogSpr * bars[j].LogSpr
		}
		mean := sum / float64(window)
		variance := sumSq/float64(window) - mean*mean
		if variance < 0 {
			variance = 0
		}
		std := math.Sqrt(variance)
		bars[i].Mean = mean
		bars[i].Std = std
		if std > 0 {
			bars[i].Z = (bars[i].LogSpr - mean) / std
			bars[i].Valid = true
		}
	}
	return bars
}

// PearsonCorrelation 는 두 시계열의 Pearson 상관계수.
// 코인티그레이션 1차 점검용 (강한 상관 = 페어 후보 자격).
func PearsonCorrelation(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0
	}
	n := float64(len(xs))
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}
	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumX2-sumX*sumX)*(n*sumY2-sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}

// ADFTStat 는 간이 ADF(Augmented Dickey-Fuller, lag=0) 통계량.
// 회귀: Δy_t = α + ρ·y_{t-1} + ε
// t-stat = ρ / SE(ρ). 통계량이 음수 큰 값일수록 정상성 강함.
// 임계값 (lag=0, α 포함): 1%=-3.43, 5%=-2.86, 10%=-2.57 (대략)
// 정상성 인정 기준: t-stat < -2.86 (5% 유의수준)
func ADFTStat(ys []float64) float64 {
	n := len(ys)
	if n < 30 {
		return 0
	}
	// Δy = y_t - y_{t-1}, lag y = y_{t-1}
	dy := make([]float64, n-1)
	ly := make([]float64, n-1)
	for i := 1; i < n; i++ {
		dy[i-1] = ys[i] - ys[i-1]
		ly[i-1] = ys[i-1]
	}
	// OLS: Δy = α + ρ·ly + ε
	m := float64(len(dy))
	var sumX, sumY, sumXY, sumX2 float64
	for i := range dy {
		sumX += ly[i]
		sumY += dy[i]
		sumXY += ly[i] * dy[i]
		sumX2 += ly[i] * ly[i]
	}
	meanX := sumX / m
	meanY := sumY / m
	cov := sumXY/m - meanX*meanY
	varX := sumX2/m - meanX*meanX
	if varX <= 0 {
		return 0
	}
	rho := cov / varX
	alpha := meanY - rho*meanX
	// 잔차 분산
	var ssr float64
	for i := range dy {
		resid := dy[i] - (alpha + rho*ly[i])
		ssr += resid * resid
	}
	sigma2 := ssr / (m - 2)
	if varX*m <= 0 || sigma2 <= 0 {
		return 0
	}
	seRho := math.Sqrt(sigma2 / (m * varX))
	if seRho == 0 {
		return 0
	}
	return rho / seRho
}
