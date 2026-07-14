package stg

// volume_wave_analyze.go — 선행 거래량부터 3파동/과열까지를 한 방향으로 재생하는 독립 분석기.
// 기존 Analyze·전역 activeRules·기본 YAML을 사용하거나 변경하지 않는다.

import (
	"RESTGo/box"
	"RESTGo/cond"
)

const (
	VolumeWaveStage1 = "VW1_FirstWave"
	VolumeWaveStage2 = "VW2_HighBaseBreakout"
	VolumeWaveStage3 = "VW3_MA20Rebound"
	VolumeWaveExit   = "VWX_ClimaxExit"
)

type VolumeWaveCycle struct {
	AccumulationPos int                            `json:"accumulation_pos"`
	Wave1Pos        int                            `json:"wave1_pos"`
	Wave2Pos        int                            `json:"wave2_pos"`
	Wave3Pos        int                            `json:"wave3_pos"`
	Wave1Volume     float64                        `json:"wave1_volume"`
	Wave2Volume     float64                        `json:"wave2_volume"`
	Base            cond.VolumeWaveBaseMetrics     `json:"base"`
	BaseBoxes       cond.VolumeWaveBaseBoxMetrics  `json:"base_boxes"`
	Pullback        cond.VolumeWavePullbackMetrics `json:"pullback"`
}

type VolumeWaveSignal struct {
	Kind             string          `json:"kind"`
	Stage            int             `json:"stage"` // 1/2/3, 과열 매도는 0
	Pos              int             `json:"pos"`
	Date             string          `json:"date"`
	Shcode           string          `json:"shcode"`
	PriceOrigin      float64         `json:"price_origin"`
	Volume           float64         `json:"volume"`
	AccumulationDate string          `json:"accumulation_date"`
	Wave1Date        string          `json:"wave1_date,omitempty"`
	Wave2Date        string          `json:"wave2_date,omitempty"`
	Wave3Date        string          `json:"wave3_date,omitempty"`
	AccumulationLead int             `json:"accumulation_lead_bars"`
	Cycle            VolumeWaveCycle `json:"cycle"`
}

// VolumeWaveAnalyze 는 준비된 일봉을 과거→현재로 한 번만 순회한다.
// 호출 전에 indicator.PrepareCandles(candles)가 필요하다.
func VolumeWaveAnalyze(candles []*box.Candle, config cond.VolumeWaveConfig) []VolumeWaveSignal {
	cfg := cond.NormalizeVolumeWaveConfig(config)
	minBars := cfg.VolumeWindow + cfg.AccumulationMinLead + 1
	if len(candles) < minBars {
		return nil
	}
	var trendBoxes []*box.Box
	if cfg.BaseBoxPattern != cond.VolumeWaveBaseBoxNone {
		trendBoxes = buildVolumeWaveTrendBoxes(candles)
	}
	ctx := box.NewTradingContext(candles, nil)
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	accumulations := make([]int, 0, 16)
	var cycle *VolumeWaveCycle
	var signals []VolumeWaveSignal

	for i := cfg.VolumeWindow; i < len(candles); i++ {
		ctx.Position = i

		// 현재 시점에서 더 이상 2~3개월 선행 후보가 될 수 없는 오래된 거래량봉 제거.
		cut := 0
		for cut < len(accumulations) && i-accumulations[cut] > cfg.AccumulationMaxLead {
			cut++
		}
		if cut > 0 {
			accumulations = accumulations[cut:]
		}

		// 단계별 만료. 만료된 같은 캔들에서 새 1파동을 다시 찾을 수 있다.
		if cycle != nil {
			switch {
			case cycle.Wave2Pos < 0 && i-cycle.Wave1Pos-1 > cfg.BaseMaxBars:
				cycle = nil
			case cycle.Wave2Pos >= 0 && cycle.Wave3Pos < 0 && i-cycle.Wave2Pos-1 > cfg.PullbackMaxBars:
				cycle = nil
			case cycle.Wave3Pos >= 0 && i-cycle.Wave3Pos > cfg.ClimaxMaxBars:
				cycle = nil
			}
		}

		if cycle == nil {
			if accumPos := latestVolumeWaveAccumulation(accumulations, i, cfg); accumPos >= 0 &&
				cond.IsVolumeWaveBreakout(ctx, cfg) {
				cycle = &VolumeWaveCycle{
					AccumulationPos: accumPos,
					Wave1Pos:        i, Wave2Pos: -1, Wave3Pos: -1,
					Wave1Volume: candles[i].Volume,
				}
				signals = append(signals, makeVolumeWaveSignal(ctx, VolumeWaveStage1, 1, *cycle))
			}
		} else if cycle.Wave2Pos < 0 {
			if base, baseBoxes, ok := cond.IsVolumeWaveSecondBreakoutWithBoxes(ctx, cycle.Wave1Pos, cfg, trendBoxes); ok {
				cycle.Wave2Pos = i
				cycle.Wave2Volume = candles[i].Volume
				cycle.Base = base
				cycle.BaseBoxes = baseBoxes
				signals = append(signals, makeVolumeWaveSignal(ctx, VolumeWaveStage2, 2, *cycle))
			}
		} else if cycle.Wave3Pos < 0 {
			if pullback, ok := cond.IsVolumeWaveThirdRebound(ctx, cycle.Wave2Pos, cfg); ok {
				cycle.Wave3Pos = i
				cycle.Pullback = pullback
				signals = append(signals, makeVolumeWaveSignal(ctx, VolumeWaveStage3, 3, *cycle))
			}
		} else if cond.IsVolumeWaveClimax(ctx, cycle.Wave3Pos, cfg) {
			signals = append(signals, makeVolumeWaveSignal(ctx, VolumeWaveExit, 0, *cycle))
			cycle = nil
		}

		// 현재 거래량봉은 다음 캔들부터 선행 후보가 된다.
		if cond.IsVolumeWaveAccumulation(ctx, cfg) {
			accumulations = append(accumulations, i)
		}
	}
	return signals
}

// buildVolumeWaveTrendBoxes 는 strategy1과 같은 MA5 곡률 전환 Box만 결정적으로 재생한다.
// DefBox는 S-R-S 자연 진동 수에 중복 저항을 섞지 않기 위해 만들지 않는다.
func buildVolumeWaveTrendBoxes(candles []*box.Candle) []*box.Box {
	if len(candles) < 7 {
		return nil
	}
	for _, c := range candles {
		c.Curvekey = 0
	}
	ctx := box.NewTradingContext(candles, []*box.Box{})
	if candles[5].Gradient >= 0 {
		candles[5].Curvekey = 1
	} else {
		candles[5].Curvekey = -1
	}
	for i := 6; i < len(candles); i++ {
		ctx.Position = i
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) > 0 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}
	}
	return ctx.BoxList
}

func latestVolumeWaveAccumulation(positions []int, current int, cfg cond.VolumeWaveConfig) int {
	for i := len(positions) - 1; i >= 0; i-- {
		lead := current - positions[i]
		if lead >= cfg.AccumulationMinLead && lead <= cfg.AccumulationMaxLead {
			return positions[i]
		}
	}
	return -1
}

func makeVolumeWaveSignal(ctx *box.TradingContext, kind string, stage int, cycle VolumeWaveCycle) VolumeWaveSignal {
	candles := ctx.CandleList
	pos := ctx.Position
	s := VolumeWaveSignal{
		Kind: kind, Stage: stage, Pos: pos, Date: candles[pos].Date,
		Shcode: ctx.Shcode, PriceOrigin: candles[pos].CloseOrigin, Volume: candles[pos].Volume,
		Cycle: cycle,
	}
	if cycle.AccumulationPos >= 0 && cycle.AccumulationPos < len(candles) {
		s.AccumulationDate = candles[cycle.AccumulationPos].Date
		s.AccumulationLead = cycle.Wave1Pos - cycle.AccumulationPos
	}
	if cycle.Wave1Pos >= 0 && cycle.Wave1Pos < len(candles) {
		s.Wave1Date = candles[cycle.Wave1Pos].Date
	}
	if cycle.Wave2Pos >= 0 && cycle.Wave2Pos < len(candles) {
		s.Wave2Date = candles[cycle.Wave2Pos].Date
	}
	if cycle.Wave3Pos >= 0 && cycle.Wave3Pos < len(candles) {
		s.Wave3Date = candles[cycle.Wave3Pos].Date
	}
	return s
}
