package stg

// volume_wave_pullback.go — VW1/VW2 거래량 돌파 당일을 추격하지 않고 첫 눌림 뒤 반등에서
// 다음 봉 시가로 진입하는 독립 연구 분석기. 기존 Analyze/YAML/포지션 엔진에는 연결하지 않는다.

import (
	"RESTGo/box"
	"math"
)

const (
	VolumeWaveConfirmCloseUp  = "close_up"
	VolumeWaveConfirmPrevHigh = "prev_high"

	VolumeWaveStructureNone          = "none"
	VolumeWaveStructureBreakoutFloor = "breakout_floor"
	VolumeWaveStructureMA20          = "ma20"
)

type VolumeWavePullbackConfig struct {
	SourceStage                   int     `json:"source_stage"` // 1 또는 2
	MinWaitBars                   int     `json:"min_wait_bars"`
	MaxWaitBars                   int     `json:"max_wait_bars"`
	MinDepth                      float64 `json:"min_depth"` // peak 대비 최대 종가 눌림, 0.03=3%
	MaxDepth                      float64 `json:"max_depth"`
	MaxEntryVolumeRatio           float64 `json:"max_entry_volume_ratio"`        // 0=미적용
	MaxPullbackAverageVolumeRatio float64 `json:"max_pullback_avg_volume_ratio"` // 0=미적용
	Confirmation                  string  `json:"confirmation"`
	Structure                     string  `json:"structure"`
	StructureTolerance            float64 `json:"structure_tolerance"`
}

func DefaultVolumeWavePullbackConfig(stage int) VolumeWavePullbackConfig {
	return VolumeWavePullbackConfig{
		SourceStage: stage, MinWaitBars: 1, MaxWaitBars: 10,
		MinDepth: 0.02, MaxDepth: 0.15,
		MaxEntryVolumeRatio: 0.70, MaxPullbackAverageVolumeRatio: 0.60,
		Confirmation:       VolumeWaveConfirmPrevHigh,
		Structure:          VolumeWaveStructureBreakoutFloor,
		StructureTolerance: 0.02,
	}
}

type VolumeWavePullbackEntry struct {
	SourceKind             string          `json:"source_kind"`
	SourceStage            int             `json:"source_stage"`
	SourcePos              int             `json:"source_pos"`
	SourceDate             string          `json:"source_date"`
	SourcePriceOrigin      float64         `json:"source_price_origin"`
	SourceVolume           float64         `json:"source_volume"`
	PullbackStartPos       int             `json:"pullback_start_pos"`
	PullbackStartDate      string          `json:"pullback_start_date"`
	FirePos                int             `json:"fire_pos"`
	FireDate               string          `json:"fire_date"`
	EntryPos               int             `json:"entry_pos"`
	EntryDate              string          `json:"entry_date"`
	EntryPriceOrigin       float64         `json:"entry_price_origin"`
	WaitBars               int             `json:"wait_bars"`
	MaxDepth               float64         `json:"max_depth"`
	EntryVolumeRatio       float64         `json:"entry_volume_ratio"`
	PullbackAvgVolumeRatio float64         `json:"pullback_avg_volume_ratio"`
	MaxMA20Breakdown       float64         `json:"max_ma20_breakdown"`
	MinCloseToFloor        float64         `json:"min_close_to_floor"`
	Cycle                  VolumeWaveCycle `json:"cycle"`
}

// VolumeWavePullbackAnalyze 는 각 VW1/VW2 신호마다 최초로 cfg를 만족하는 반등 1건만 반환한다.
// 신호 확인은 FirePos 종가, 체결은 look-ahead 방지를 위해 EntryPos=FirePos+1 시가다.
func VolumeWavePullbackAnalyze(candles []*box.Candle, waveSignals []VolumeWaveSignal, cfg VolumeWavePullbackConfig) []VolumeWavePullbackEntry {
	if len(candles) < 3 || (cfg.SourceStage != 1 && cfg.SourceStage != 2) {
		return nil
	}
	if cfg.MinWaitBars < 1 {
		cfg.MinWaitBars = 1
	}
	if cfg.MaxWaitBars < cfg.MinWaitBars {
		return nil
	}
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 1
	}
	if cfg.Confirmation == "" {
		cfg.Confirmation = VolumeWaveConfirmCloseUp
	}
	if cfg.Structure == "" {
		cfg.Structure = VolumeWaveStructureNone
	}

	var entries []VolumeWavePullbackEntry
	for signalIdx, source := range waveSignals {
		if source.Stage != cfg.SourceStage || source.Pos < 1 || source.Pos+2 >= len(candles) {
			continue
		}
		end := source.Pos + cfg.MaxWaitBars
		if end >= len(candles)-1 { // fire 다음 봉 시가 필요
			end = len(candles) - 2
		}
		end = volumeWavePullbackDeadline(waveSignals, signalIdx, source, end)
		if end < source.Pos+cfg.MinWaitBars {
			continue
		}

		sourceCandle := candles[source.Pos]
		if sourceCandle.Volume <= 0 || sourceCandle.CloseOrigin <= 0 {
			continue
		}
		floor := candles[source.Pos-1].Close
		peak := sourceCandle.High
		minClose := sourceCandle.Close
		maxDepth := 0.0
		maxMA20Breakdown := 0.0
		pullbackStart := -1
		var pullbackVolume float64
		pullbackVolumeBars := 0

		for i := source.Pos + 1; i <= end; i++ {
			cur, prev := candles[i], candles[i-1]
			if prev.High > peak {
				peak = prev.High
			}
			if cur.Close < minClose {
				minClose = cur.Close
			}
			if peak > 0 {
				depth := (peak - cur.Close) / peak
				if depth > maxDepth {
					maxDepth = depth
				}
			}
			if cur.Ma20 > 0 {
				breakdown := (cur.Ma20 - cur.Close) / cur.Ma20
				if breakdown > maxMA20Breakdown {
					maxMA20Breakdown = breakdown
				}
			}

			isPullbackBar := cur.Close < prev.Close || cur.Close <= cur.Open
			if pullbackStart < 0 && isPullbackBar {
				pullbackStart = i
			}
			if pullbackStart >= 0 && i < end+1 {
				// 반등 확인봉 직전까지의 평균은 아래에서 현재 거래량을 빼서 계산한다.
				pullbackVolume += cur.Volume
				pullbackVolumeBars++
			}
			if pullbackStart < 0 || i-source.Pos < cfg.MinWaitBars || !volumeWaveReboundConfirmed(candles, i, cfg.Confirmation) {
				continue
			}

			entryVolumeRatio := cur.Volume / sourceCandle.Volume
			avgVolume := pullbackVolume - cur.Volume
			avgBars := pullbackVolumeBars - 1
			if avgBars <= 0 {
				avgVolume, avgBars = candles[pullbackStart].Volume, 1
			}
			pullbackAvgRatio := (avgVolume / float64(avgBars)) / sourceCandle.Volume
			if maxDepth < cfg.MinDepth || maxDepth > cfg.MaxDepth ||
				(cfg.MaxEntryVolumeRatio > 0 && entryVolumeRatio > cfg.MaxEntryVolumeRatio) ||
				(cfg.MaxPullbackAverageVolumeRatio > 0 && pullbackAvgRatio > cfg.MaxPullbackAverageVolumeRatio) ||
				!volumeWaveStructureHeld(candles, source.Pos+1, i, floor, cfg) {
				continue
			}

			entry := candles[i+1]
			if entry.OpenOrigin <= 0 || math.IsNaN(entry.OpenOrigin) || math.IsInf(entry.OpenOrigin, 0) {
				continue
			}
			floorRatio := 0.0
			if floor > 0 {
				floorRatio = minClose/floor - 1
			}
			entries = append(entries, VolumeWavePullbackEntry{
				SourceKind: source.Kind, SourceStage: source.Stage,
				SourcePos: source.Pos, SourceDate: source.Date,
				SourcePriceOrigin: sourceCandle.CloseOrigin, SourceVolume: sourceCandle.Volume,
				PullbackStartPos: pullbackStart, PullbackStartDate: candles[pullbackStart].Date,
				FirePos: i, FireDate: cur.Date,
				EntryPos: i + 1, EntryDate: entry.Date, EntryPriceOrigin: entry.OpenOrigin,
				WaitBars: i - source.Pos, MaxDepth: maxDepth,
				EntryVolumeRatio: entryVolumeRatio, PullbackAvgVolumeRatio: pullbackAvgRatio,
				MaxMA20Breakdown: maxMA20Breakdown, MinCloseToFloor: floorRatio,
				Cycle: source.Cycle,
			})
			break
		}
	}
	return entries
}

// VW1은 동일 사이클의 VW2 직전까지만, VW2는 VW3 발화일까지를 첫 눌림 탐색 구간으로 쓴다.
func volumeWavePullbackDeadline(signals []VolumeWaveSignal, sourceIdx int, source VolumeWaveSignal, end int) int {
	for i := sourceIdx + 1; i < len(signals); i++ {
		next := signals[i]
		if next.Pos <= source.Pos {
			continue
		}
		if next.Cycle.Wave1Pos != source.Cycle.Wave1Pos {
			break
		}
		if source.Stage == 1 && next.Stage == 2 && next.Pos-1 < end {
			return next.Pos - 1
		}
		if source.Stage == 2 && next.Stage == 3 && next.Pos < end {
			return next.Pos
		}
	}
	return end
}

func volumeWaveReboundConfirmed(candles []*box.Candle, pos int, confirmation string) bool {
	if pos < 1 {
		return false
	}
	cur, prev := candles[pos], candles[pos-1]
	if cur.Close <= cur.Open || cur.Close <= prev.Close {
		return false
	}
	switch confirmation {
	case VolumeWaveConfirmPrevHigh:
		return cur.Close > prev.High
	default:
		return true
	}
}

func volumeWaveStructureHeld(candles []*box.Candle, start, end int, floor float64, cfg VolumeWavePullbackConfig) bool {
	switch cfg.Structure {
	case VolumeWaveStructureBreakoutFloor:
		if floor <= 0 {
			return false
		}
		for i := start; i <= end; i++ {
			if candles[i].Close < floor*(1-cfg.StructureTolerance) {
				return false
			}
		}
	case VolumeWaveStructureMA20:
		for i := start; i <= end; i++ {
			if candles[i].Ma20 <= 0 || candles[i].Close < candles[i].Ma20*(1-cfg.StructureTolerance) {
				return false
			}
		}
	}
	return true
}
