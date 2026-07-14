package cond

// buy_volume_wave.go — "선행 거래량 → 고가 횡보 → 2차 거래량 돌파 → MA20 3파동" 전략의
// 순수 조건 모음. 기존 전략 함수는 변경하지 않고, 이미 존재하는 거래량 Z-score·MA20 돌파
// 함수를 조합한다. 모든 판정은 현재 pos 이하의 캔들만 사용한다.

import (
	"RESTGo/box"
	"math"
)

const (
	VolumeWaveAccumulationSpike         = "spike"
	VolumeWaveAccumulationCumulativeOBV = "cumulative_obv"

	VolumeWaveBaseBoxNone           = "none"
	VolumeWaveBaseBoxSRS            = "srs"
	VolumeWaveBaseBoxSRSSupportHeld = "srs_support_held"
)

// VolumeWaveConfig 는 이미지 전략의 모호한 표현을 검증 가능한 수치로 만든 1차 기준값이다.
// 비율 필드는 0.20=20% 형식이다. 연구 스캔 결과로 조정하되 기존 전략 Settings와 분리한다.
type VolumeWaveConfig struct {
	VolumeWindow               int     `json:"volume_window"`
	VolumeZThreshold           float64 `json:"volume_z_threshold"`
	AccumulationMode           string  `json:"accumulation_mode"`
	AccumulationWindow         int     `json:"accumulation_window"`
	AccumulationVolumeRatio    float64 `json:"accumulation_volume_ratio"`
	AccumulationMinOBVPressure float64 `json:"accumulation_min_obv_pressure"`
	AccumulationMinLead        int     `json:"accumulation_min_lead"`
	AccumulationMaxLead        int     `json:"accumulation_max_lead"`
	BreakoutLookback           int     `json:"breakout_lookback"`
	BreakoutMinReturnPct       float64 `json:"breakout_min_return_pct"`
	BaseMinBars                int     `json:"base_min_bars"`
	BaseMaxBars                int     `json:"base_max_bars"`
	BaseMaxDrawdown            float64 `json:"base_max_drawdown"`
	BaseMaxRange               float64 `json:"base_max_range"`
	BaseMaxVolumeRatio         float64 `json:"base_max_volume_ratio"`
	BaseMinCloseRetention      float64 `json:"base_min_close_retention"`
	BaseMaxHighLowRange        float64 `json:"base_max_high_low_range"`
	BaseLateEarlyVolRatio      float64 `json:"base_late_early_volume_ratio"`
	BaseBoxPattern             string  `json:"base_box_pattern"`
	BaseSecondSupportRatio     float64 `json:"base_second_support_ratio"`
	Wave2MinVolumeRatio        float64 `json:"wave2_min_volume_ratio"`
	PullbackMinBars            int     `json:"pullback_min_bars"`
	PullbackMaxBars            int     `json:"pullback_max_bars"`
	MA20TouchTolerance         float64 `json:"ma20_touch_tolerance"`
	MA20MaxCloseBreakdown      float64 `json:"ma20_max_close_breakdown"`
	PullbackMaxVolumeRatio     float64 `json:"pullback_max_volume_ratio"`
	MA20RisingStreak           int     `json:"ma20_rising_streak"`
	ClimaxMaxBars              int     `json:"climax_max_bars"`
	ClimaxMinGain              float64 `json:"climax_min_gain"`
	ClimaxRangeATR             float64 `json:"climax_range_atr"`
	ClimaxUpperWickBody        float64 `json:"climax_upper_wick_body"`
	ClimaxBodyPct              float64 `json:"climax_body_pct"`
}

func DefaultVolumeWaveConfig() VolumeWaveConfig {
	return VolumeWaveConfig{
		VolumeWindow:               20,
		VolumeZThreshold:           2.0,
		AccumulationMode:           VolumeWaveAccumulationSpike,
		AccumulationWindow:         10,
		AccumulationVolumeRatio:    1.20,
		AccumulationMinOBVPressure: 0.0,
		AccumulationMinLead:        40, // 약 2개월
		AccumulationMaxLead:        70, // 약 3개월+
		BreakoutLookback:           20,
		BreakoutMinReturnPct:       3.0,
		BaseMinBars:                5,
		BaseMaxBars:                30,
		BaseMaxDrawdown:            0.20,
		BaseMaxRange:               0.25,
		BaseMaxVolumeRatio:         0.80,
		BaseMinCloseRetention:      0.0, // 0=기존 대조군에서는 미적용
		BaseMaxHighLowRange:        0.0,
		BaseLateEarlyVolRatio:      0.0,
		BaseBoxPattern:             VolumeWaveBaseBoxNone,
		BaseSecondSupportRatio:     0.97,
		Wave2MinVolumeRatio:        1.05,
		PullbackMinBars:            2,
		PullbackMaxBars:            20,
		MA20TouchTolerance:         0.02,
		MA20MaxCloseBreakdown:      0.05,
		PullbackMaxVolumeRatio:     0.70,
		MA20RisingStreak:           1,
		ClimaxMaxBars:              40,
		ClimaxMinGain:              0.15,
		ClimaxRangeATR:             2.0,
		ClimaxUpperWickBody:        1.5,
		ClimaxBodyPct:              0.08,
	}
}

// NormalizeVolumeWaveConfig 는 0/음수 입력만 기본값으로 보완한다.
// 완전한 zero value는 기본 설정으로 해석하고, 명시적으로 만든 설정의 MA20RisingStreak=0은 비활성으로 보존한다.
func NormalizeVolumeWaveConfig(c VolumeWaveConfig) VolumeWaveConfig {
	d := DefaultVolumeWaveConfig()
	if c == (VolumeWaveConfig{}) {
		return d
	}
	if c.VolumeWindow <= 1 {
		c.VolumeWindow = d.VolumeWindow
	}
	if c.VolumeZThreshold <= 0 {
		c.VolumeZThreshold = d.VolumeZThreshold
	}
	if c.AccumulationMode == "" {
		c.AccumulationMode = VolumeWaveAccumulationSpike
	}
	if c.AccumulationWindow <= 1 {
		c.AccumulationWindow = d.AccumulationWindow
	}
	if c.AccumulationVolumeRatio <= 0 {
		c.AccumulationVolumeRatio = d.AccumulationVolumeRatio
	}
	if c.AccumulationMinLead <= 0 {
		c.AccumulationMinLead = d.AccumulationMinLead
	}
	if c.AccumulationMaxLead < c.AccumulationMinLead {
		c.AccumulationMaxLead = d.AccumulationMaxLead
	}
	if c.BreakoutLookback <= 1 {
		c.BreakoutLookback = d.BreakoutLookback
	}
	if c.BreakoutMinReturnPct <= 0 {
		c.BreakoutMinReturnPct = d.BreakoutMinReturnPct
	}
	if c.BaseMinBars <= 0 {
		c.BaseMinBars = d.BaseMinBars
	}
	if c.BaseMaxBars < c.BaseMinBars {
		c.BaseMaxBars = d.BaseMaxBars
	}
	if c.BaseMaxDrawdown <= 0 {
		c.BaseMaxDrawdown = d.BaseMaxDrawdown
	}
	if c.BaseMaxRange <= 0 {
		c.BaseMaxRange = d.BaseMaxRange
	}
	if c.BaseMaxVolumeRatio <= 0 {
		c.BaseMaxVolumeRatio = d.BaseMaxVolumeRatio
	}
	if c.BaseBoxPattern == "" {
		c.BaseBoxPattern = VolumeWaveBaseBoxNone
	}
	if c.BaseSecondSupportRatio <= 0 {
		c.BaseSecondSupportRatio = d.BaseSecondSupportRatio
	}
	if c.Wave2MinVolumeRatio <= 0 {
		c.Wave2MinVolumeRatio = d.Wave2MinVolumeRatio
	}
	if c.PullbackMinBars <= 0 {
		c.PullbackMinBars = d.PullbackMinBars
	}
	if c.PullbackMaxBars < c.PullbackMinBars {
		c.PullbackMaxBars = d.PullbackMaxBars
	}
	if c.MA20TouchTolerance <= 0 {
		c.MA20TouchTolerance = d.MA20TouchTolerance
	}
	if c.MA20MaxCloseBreakdown <= 0 {
		c.MA20MaxCloseBreakdown = d.MA20MaxCloseBreakdown
	}
	if c.PullbackMaxVolumeRatio <= 0 {
		c.PullbackMaxVolumeRatio = d.PullbackMaxVolumeRatio
	}
	if c.MA20RisingStreak < 0 {
		c.MA20RisingStreak = d.MA20RisingStreak
	}
	if c.ClimaxMaxBars <= 0 {
		c.ClimaxMaxBars = d.ClimaxMaxBars
	}
	if c.ClimaxMinGain <= 0 {
		c.ClimaxMinGain = d.ClimaxMinGain
	}
	if c.ClimaxRangeATR <= 0 {
		c.ClimaxRangeATR = d.ClimaxRangeATR
	}
	if c.ClimaxUpperWickBody <= 0 {
		c.ClimaxUpperWickBody = d.ClimaxUpperWickBody
	}
	if c.ClimaxBodyPct <= 0 {
		c.ClimaxBodyPct = d.ClimaxBodyPct
	}
	return c
}

// IsVolumeWaveAccumulation 은 선행 매집 후보다. spike는 기존 단발 이상봉 대조군이고,
// cumulative_obv는 최근 N봉 누적거래량 증가와 정규화 OBV 순증가를 동시에 요구한다.
func IsVolumeWaveAccumulation(ctx *box.TradingContext, cfg VolumeWaveConfig) bool {
	cfg = NormalizeVolumeWaveConfig(cfg)
	c := ctx.GetCurrentCandle()
	if c == nil || c.Volume <= 0 {
		return false
	}
	if cfg.AccumulationMode != VolumeWaveAccumulationCumulativeOBV {
		return IsVolumeZScoreSpike(ctx, cfg.VolumeWindow, cfg.VolumeZThreshold)
	}
	pos, n := ctx.Position, cfg.AccumulationWindow
	if pos < 2*n-1 {
		return false
	}
	var recentVolume, priorVolume float64
	for i := pos - n + 1; i <= pos; i++ {
		recentVolume += ctx.CandleList[i].Volume
	}
	for i := pos - 2*n + 1; i <= pos-n; i++ {
		priorVolume += ctx.CandleList[i].Volume
	}
	if priorVolume <= 0 || recentVolume < priorVolume*cfg.AccumulationVolumeRatio {
		return false
	}
	obvRise := c.OBV - ctx.CandleList[pos-n].OBV
	return recentVolume > 0 && obvRise > 0 && obvRise/recentVolume >= cfg.AccumulationMinOBVPressure
}

// IsVolumeWaveBreakout 은 거래량 이상 + 양봉 + N봉 신고가 종가 + 최소 일간 상승률이다.
func IsVolumeWaveBreakout(ctx *box.TradingContext, cfg VolumeWaveConfig) bool {
	cfg = NormalizeVolumeWaveConfig(cfg)
	pos := ctx.Position
	if pos < cfg.BreakoutLookback || pos < 1 || !IsBullishCandle(ctx) ||
		!IsVolumeZScoreSpike(ctx, cfg.VolumeWindow, cfg.VolumeZThreshold) {
		return false
	}
	candles := ctx.CandleList
	prevClose := candles[pos-1].Close
	if prevClose <= 0 || (candles[pos].Close-prevClose)/prevClose*100 < cfg.BreakoutMinReturnPct {
		return false
	}
	priorHigh := 0.0
	for i := pos - cfg.BreakoutLookback; i < pos; i++ {
		if candles[i].High > priorHigh {
			priorHigh = candles[i].High
		}
	}
	return priorHigh > 0 && candles[pos].Close > priorHigh
}

type VolumeWaveBaseMetrics struct {
	Bars              int     `json:"bars"`
	PeakPrice         float64 `json:"peak_price"`
	LowClose          float64 `json:"low_close"`
	Drawdown          float64 `json:"drawdown"`
	Range             float64 `json:"range"`
	HighLowRange      float64 `json:"high_low_range"`
	CloseRetention    float64 `json:"close_retention"`
	AverageVolume     float64 `json:"average_volume"`
	VolumeRatio       float64 `json:"volume_ratio"`
	LateEarlyVolRatio float64 `json:"late_early_volume_ratio"`
}

// EvaluateVolumeWaveHighBase 는 wave1 다음 봉부터 current 직전까지의 고가 횡보를 판정한다.
func EvaluateVolumeWaveHighBase(candles []*box.Candle, wave1Pos, currentPos int, cfg VolumeWaveConfig) (VolumeWaveBaseMetrics, bool) {
	cfg = NormalizeVolumeWaveConfig(cfg)
	m := VolumeWaveBaseMetrics{Bars: currentPos - wave1Pos - 1}
	if wave1Pos < 0 || currentPos >= len(candles) || m.Bars < cfg.BaseMinBars || m.Bars > cfg.BaseMaxBars {
		return m, false
	}
	peak := candles[wave1Pos].High
	lowClose := math.MaxFloat64
	maxClose := 0.0
	highestHigh, lowestLow := 0.0, math.MaxFloat64
	retained := 0
	var volume, earlyVolume, lateVolume float64
	split := m.Bars / 2
	for i := wave1Pos + 1; i < currentPos; i++ {
		c := candles[i]
		if c.High > peak {
			peak = c.High
		}
		if c.Close < lowClose {
			lowClose = c.Close
		}
		if c.Close > maxClose {
			maxClose = c.Close
		}
		if c.High > highestHigh {
			highestHigh = c.High
		}
		if c.Low < lowestLow {
			lowestLow = c.Low
		}
		if c.Close >= candles[wave1Pos].Close {
			retained++
		}
		volume += c.Volume
		if i-(wave1Pos+1) < split {
			earlyVolume += c.Volume
		} else {
			lateVolume += c.Volume
		}
	}
	if peak <= 0 || lowClose == math.MaxFloat64 || candles[wave1Pos].Volume <= 0 {
		return m, false
	}
	m.PeakPrice = peak
	m.LowClose = lowClose
	m.Drawdown = (peak - lowClose) / peak
	m.Range = (maxClose - lowClose) / peak
	m.HighLowRange = (highestHigh - lowestLow) / candles[wave1Pos].Close
	m.CloseRetention = float64(retained) / float64(m.Bars)
	m.AverageVolume = volume / float64(m.Bars)
	m.VolumeRatio = m.AverageVolume / candles[wave1Pos].Volume
	if split > 0 && m.Bars-split > 0 && earlyVolume > 0 {
		m.LateEarlyVolRatio = (lateVolume / float64(m.Bars-split)) / (earlyVolume / float64(split))
	}
	valid := m.Drawdown <= cfg.BaseMaxDrawdown && m.Range <= cfg.BaseMaxRange &&
		m.VolumeRatio <= cfg.BaseMaxVolumeRatio &&
		(cfg.BaseMinCloseRetention <= 0 || m.CloseRetention >= cfg.BaseMinCloseRetention) &&
		(cfg.BaseMaxHighLowRange <= 0 || m.HighLowRange <= cfg.BaseMaxHighLowRange) &&
		(cfg.BaseLateEarlyVolRatio <= 0 || m.LateEarlyVolRatio <= cfg.BaseLateEarlyVolRatio)
	return m, valid
}

type VolumeWaveBaseBoxMetrics struct {
	PatternFound       bool    `json:"pattern_found"`
	EligibleBoxCount   int     `json:"eligible_box_count"`
	FirstSupportPos    int     `json:"first_support_pos"`
	ResistancePos      int     `json:"resistance_pos"`
	SecondSupportPos   int     `json:"second_support_pos"`
	FirstSupportPrice  float64 `json:"first_support_price"`
	ResistancePrice    float64 `json:"resistance_price"`
	SecondSupportPrice float64 `json:"second_support_price"`
	SecondSupportRatio float64 `json:"second_support_ratio"`
}

// EvaluateVolumeWaveHighBaseBoxes 는 VW1 다음 봉부터 VW2 현재봉 직전까지, VW2 시점에 이미
// 확인된 일반 추세전환 Box 중 연속 S→R→S 세트를 찾는다. BoxPosition뿐 아니라
// CurvePosition도 currentPos 이하로 제한하여 미래의 전환 확인을 소급 사용하지 않는다.
func EvaluateVolumeWaveHighBaseBoxes(boxes []*box.Box, wave1Pos, currentPos int, cfg VolumeWaveConfig) (VolumeWaveBaseBoxMetrics, bool) {
	cfg = NormalizeVolumeWaveConfig(cfg)
	m := VolumeWaveBaseBoxMetrics{FirstSupportPos: -1, ResistancePos: -1, SecondSupportPos: -1}
	if cfg.BaseBoxPattern == VolumeWaveBaseBoxNone {
		return m, true
	}
	eligible := make([]*box.Box, 0, 8)
	for _, b := range boxes {
		if b == nil || b.KindOfBox != box.KindBox || b.BoxPosition <= wave1Pos || b.BoxPosition >= currentPos ||
			b.CurvePosition <= wave1Pos || b.CurvePosition > currentPos {
			continue
		}
		eligible = append(eligible, b)
	}
	m.EligibleBoxCount = len(eligible)
	for i := 0; i+2 < len(eligible); i++ {
		s1, r, s2 := eligible[i], eligible[i+1], eligible[i+2]
		if s1.BoxType != box.BoxTypeSupport || r.BoxType != box.BoxTypeResistance || s2.BoxType != box.BoxTypeSupport ||
			!(s1.BoxPosition < r.BoxPosition && r.BoxPosition < s2.BoxPosition) {
			continue
		}
		m.PatternFound = true
		m.FirstSupportPos, m.ResistancePos, m.SecondSupportPos = s1.BoxPosition, r.BoxPosition, s2.BoxPosition
		m.FirstSupportPrice, m.ResistancePrice, m.SecondSupportPrice = s1.PriceOrigin, r.PriceOrigin, s2.PriceOrigin
		if m.FirstSupportPrice > 0 {
			m.SecondSupportRatio = m.SecondSupportPrice / m.FirstSupportPrice
		}
		if cfg.BaseBoxPattern == VolumeWaveBaseBoxSRSSupportHeld && m.SecondSupportRatio < cfg.BaseSecondSupportRatio {
			continue
		}
		return m, true
	}
	return m, false
}

// IsVolumeWaveSecondBreakout 은 유효 고가 횡보 상단을, 1파동보다 큰 거래량으로 돌파했는지 판정한다.
func IsVolumeWaveSecondBreakout(ctx *box.TradingContext, wave1Pos int, cfg VolumeWaveConfig) (VolumeWaveBaseMetrics, bool) {
	m, _, ok := IsVolumeWaveSecondBreakoutWithBoxes(ctx, wave1Pos, cfg, nil)
	return m, ok
}

// IsVolumeWaveSecondBreakoutWithBoxes 는 기존 가격·거래량 고가놀이에 opt-in Box 패턴을 결합한다.
func IsVolumeWaveSecondBreakoutWithBoxes(ctx *box.TradingContext, wave1Pos int, cfg VolumeWaveConfig, boxes []*box.Box) (VolumeWaveBaseMetrics, VolumeWaveBaseBoxMetrics, bool) {
	m, ok := EvaluateVolumeWaveHighBase(ctx.CandleList, wave1Pos, ctx.Position, cfg)
	boxMetrics, boxesOK := EvaluateVolumeWaveHighBaseBoxes(boxes, wave1Pos, ctx.Position, cfg)
	if !ok || !boxesOK || !IsVolumeWaveBreakout(ctx, cfg) {
		return m, boxMetrics, false
	}
	cur := ctx.GetCurrentCandle()
	wave1 := ctx.CandleList[wave1Pos]
	cfg = NormalizeVolumeWaveConfig(cfg)
	return m, boxMetrics, cur.Close > m.PeakPrice && cur.Volume >= wave1.Volume*cfg.Wave2MinVolumeRatio
}

type VolumeWavePullbackMetrics struct {
	Bars          int     `json:"bars"`
	TouchCount    int     `json:"touch_count"`
	MaxBreakdown  float64 `json:"max_breakdown"`
	AverageVolume float64 `json:"average_volume"`
	VolumeRatio   float64 `json:"volume_ratio"`
}

// EvaluateVolumeWaveMA20Pullback 은 2파동 뒤 MA20 지지와 거래량 수축을 확인한다.
func EvaluateVolumeWaveMA20Pullback(candles []*box.Candle, wave2Pos, currentPos int, cfg VolumeWaveConfig) (VolumeWavePullbackMetrics, bool) {
	cfg = NormalizeVolumeWaveConfig(cfg)
	m := VolumeWavePullbackMetrics{Bars: currentPos - wave2Pos - 1}
	if wave2Pos < 0 || currentPos >= len(candles) || m.Bars < cfg.PullbackMinBars || m.Bars > cfg.PullbackMaxBars ||
		candles[wave2Pos].Volume <= 0 {
		return m, false
	}
	var volume float64
	for i := wave2Pos + 1; i < currentPos; i++ {
		c := candles[i]
		if c.Ma20 <= 0 {
			return m, false
		}
		if c.Low <= c.Ma20*(1+cfg.MA20TouchTolerance) {
			m.TouchCount++
		}
		breakdown := (c.Ma20 - c.Close) / c.Ma20
		if breakdown > m.MaxBreakdown {
			m.MaxBreakdown = breakdown
		}
		volume += c.Volume
	}
	m.AverageVolume = volume / float64(m.Bars)
	m.VolumeRatio = m.AverageVolume / candles[wave2Pos].Volume
	valid := m.TouchCount > 0 && m.MaxBreakdown <= cfg.MA20MaxCloseBreakdown &&
		m.VolumeRatio <= cfg.PullbackMaxVolumeRatio
	return m, valid
}

// IsVolumeWaveThirdRebound 는 MA20 지지 구간 뒤의 양봉 재돌파 edge다.
func IsVolumeWaveThirdRebound(ctx *box.TradingContext, wave2Pos int, cfg VolumeWaveConfig) (VolumeWavePullbackMetrics, bool) {
	m, ok := EvaluateVolumeWaveMA20Pullback(ctx.CandleList, wave2Pos, ctx.Position, cfg)
	if !ok {
		return m, false
	}
	cfg = NormalizeVolumeWaveConfig(cfg)
	if IsMA20BullishBreakout(ctx.CandleList, ctx.Position, cfg.MA20RisingStreak) {
		return m, true
	}
	// MA20을 저가로 지지하고 종가는 계속 위에 있던 경우의 반등 edge.
	pos := ctx.Position
	if pos < 1 || !IsBullishCandle(ctx) {
		return m, false
	}
	cur, prev := ctx.CandleList[pos], ctx.CandleList[pos-1]
	if cur.Ma20 <= 0 || prev.Ma20 <= 0 || cur.Close <= cur.Ma20 || cur.Close <= prev.High ||
		prev.Low > prev.Ma20*(1+cfg.MA20TouchTolerance) ||
		prev.Close < prev.Ma20*(1-cfg.MA20MaxCloseBreakdown) {
		return m, false
	}
	return m, cfg.MA20RisingStreak <= 0 || IsMA20RisingStreak(ctx.CandleList, pos, cfg.MA20RisingStreak)
}

// IsVolumeWaveClimax 은 3파동 이후의 거래량 폭발 + ATR 장대범위 + 윗꼬리/장대몸통 과열이다.
func IsVolumeWaveClimax(ctx *box.TradingContext, wave3Pos int, cfg VolumeWaveConfig) bool {
	cfg = NormalizeVolumeWaveConfig(cfg)
	pos := ctx.Position
	if wave3Pos < 0 || pos <= wave3Pos || pos-wave3Pos > cfg.ClimaxMaxBars ||
		!IsVolumeZScoreSpike(ctx, cfg.VolumeWindow, cfg.VolumeZThreshold) {
		return false
	}
	start, cur := ctx.CandleList[wave3Pos], ctx.CandleList[pos]
	if start.CloseOrigin <= 0 || cur.OpenOrigin <= 0 || cur.ATR <= 0 ||
		(cur.CloseOrigin-start.CloseOrigin)/start.CloseOrigin < cfg.ClimaxMinGain {
		return false
	}
	rangePrice := cur.HighOrigin - cur.LowOrigin
	if rangePrice < cur.ATR*cfg.ClimaxRangeATR {
		return false
	}
	body := math.Abs(cur.CloseOrigin - cur.OpenOrigin)
	upperWick := cur.HighOrigin - math.Max(cur.OpenOrigin, cur.CloseOrigin)
	wickRatio := math.Inf(1)
	if body > 0 {
		wickRatio = upperWick / body
	}
	bodyPct := body / cur.OpenOrigin
	return wickRatio >= cfg.ClimaxUpperWickBody || bodyPct >= cfg.ClimaxBodyPct
}
