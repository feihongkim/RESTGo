package study

// volume_wave_box_study.go — 기존 고가놀이 가격·거래량 게이트에 strategy1 곡률 Box
// S→R→S를 opt-in 결합하고, 원형 및 두 번째 지지 유지형과 비교한다.

import (
	"RESTGo/box"
	"RESTGo/cond"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type volumeWaveBoxMode struct {
	ID    string
	Label string
	Cfg   cond.VolumeWaveConfig
}

type volumeWaveBoxAggregate struct {
	SourceN map[string]int
	Chase   map[string][]float64
	Entry   map[string][]float64
}

type VolumeWaveBoxStudyResult struct {
	Mode            string  `json:"mode"`
	Label           string  `json:"label"`
	Period          string  `json:"period"`
	Horizon         int     `json:"horizon"`
	SourceN         int     `json:"vw2_source_n"`
	SourceRetention float64 `json:"source_retention_pct"`
	EntryN          int     `json:"pullback_entry_n"`
	EntryRate       float64 `json:"pullback_entry_rate_pct"`
	ChaseMeanNet    float64 `json:"chase_mean_net_pct"`
	PullbackMeanNet float64 `json:"pullback_mean_net_pct"`
	PullbackMedian  float64 `json:"pullback_median_net_pct"`
	PullbackWinRate float64 `json:"pullback_win_rate_pct"`
	PullbackPF      float64 `json:"pullback_profit_factor"`
	Improvement     float64 `json:"pullback_vs_chase_pctp"`
	TStatVsChase    float64 `json:"t_stat_vs_chase"`
}

// HandleVolumeWaveBoxStudy 사용법:
// ./RESTGo stock volume_wave_box_study [--max N] [--candles N] [--out path] [--oos-date YYYYMMDD]
func HandleVolumeWaveBoxStudy(args []string) {
	maxStocks, candleCount := 0, 4200
	outPath, oosDate := "zpicture/volume_wave_box_study.json", "20220101"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--max":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxStocks)
				i++
			}
		case "--candles":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &candleCount)
				i++
			}
		case "--out":
			if i+1 < len(args) {
				outPath = args[i+1]
				i++
			}
		case "--oos-date":
			if i+1 < len(args) {
				oosDate = args[i+1]
				i++
			}
		}
	}

	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_box_study] DB 오류: %v\n", err)
		return
	}
	stocks, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_box_study] 종목 오류: %v\n", err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}

	baseCfg := cond.DefaultVolumeWaveConfig()
	srsCfg, heldCfg := baseCfg, baseCfg
	srsCfg.BaseBoxPattern = cond.VolumeWaveBaseBoxSRS
	heldCfg.BaseBoxPattern = cond.VolumeWaveBaseBoxSRSSupportHeld
	heldCfg.BaseSecondSupportRatio = 0.97
	modes := []volumeWaveBoxMode{
		{ID: "BASE", Label: "기존 가격·거래량 고가놀이", Cfg: baseCfg},
		{ID: "SRS", Label: "고가놀이 + 일반Box S-R-S", Cfg: srsCfg},
		{ID: "SRS_HOLD", Label: "S-R-S + 두 번째 지지≥첫 지지 97%", Cfg: heldCfg},
	}
	aggs := make([]volumeWaveBoxAggregate, len(modes))
	for i := range aggs {
		aggs[i] = volumeWaveBoxAggregate{SourceN: map[string]int{}, Chase: map[string][]float64{}, Entry: map[string][]float64{}}
	}
	entryCfg := selectedVolumeWavePullbackConfig()
	const roundTripCostPct = 0.30
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	fmt.Printf("[volume_wave_box_study] KR %d종목 × %d모드, OOS=%s\n", len(stocks), len(modes), oosDate)
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()
			candles, fetchErr := box.FetchCandlesHannam(db, shcode, candleCount)
			if fetchErr != nil || len(candles) < baseCfg.AccumulationMaxLead+baseCfg.VolumeWindow+20 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)
			local := make([]volumeWaveBoxAggregate, len(modes))
			for modeIdx, mode := range modes {
				local[modeIdx] = volumeWaveBoxAggregate{SourceN: map[string]int{}, Chase: map[string][]float64{}, Entry: map[string][]float64{}}
				waves := stg.VolumeWaveAnalyze(candles, mode.Cfg)
				for _, source := range waves {
					if source.Stage != 2 {
						continue
					}
					period := volumeWavePeriod(source.Date, oosDate)
					local[modeIdx].SourceN["ALL"]++
					local[modeIdx].SourceN[period]++
					entryPos := source.Pos + 1
					if entryPos < len(candles) && candles[entryPos].OpenOrigin > 0 {
						for h, r := range volumeWaveEntryReturns(candles, entryPos, candles[entryPos].OpenOrigin, roundTripCostPct) {
							local[modeIdx].Chase[volumeWaveReturnKey("ALL", h)] = append(local[modeIdx].Chase[volumeWaveReturnKey("ALL", h)], r)
							local[modeIdx].Chase[volumeWaveReturnKey(volumeWavePeriod(candles[entryPos].Date, oosDate), h)] = append(local[modeIdx].Chase[volumeWaveReturnKey(volumeWavePeriod(candles[entryPos].Date, oosDate), h)], r)
						}
					}
				}
				for _, entry := range stg.VolumeWavePullbackAnalyze(candles, waves, entryCfg) {
					period := volumeWavePeriod(entry.EntryDate, oosDate)
					for h, r := range volumeWaveEntryReturns(candles, entry.EntryPos, entry.EntryPriceOrigin, roundTripCostPct) {
						local[modeIdx].Entry[volumeWaveReturnKey("ALL", h)] = append(local[modeIdx].Entry[volumeWaveReturnKey("ALL", h)], r)
						local[modeIdx].Entry[volumeWaveReturnKey(period, h)] = append(local[modeIdx].Entry[volumeWaveReturnKey(period, h)], r)
					}
				}
			}
			mu.Lock()
			for i := range modes {
				for k, n := range local[i].SourceN {
					aggs[i].SourceN[k] += n
				}
				for k, v := range local[i].Chase {
					aggs[i].Chase[k] = append(aggs[i].Chase[k], v...)
				}
				for k, v := range local[i].Entry {
					aggs[i].Entry[k] = append(aggs[i].Entry[k], v...)
				}
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[volume_wave_box_study] 진행 %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()

	var results []VolumeWaveBoxStudyResult
	for i, mode := range modes {
		for _, period := range []string{"ALL", "IS", "OOS"} {
			for _, h := range volumeWaveMatrixHorizons {
				chase := aggs[i].Chase[volumeWaveReturnKey(period, h)]
				entries := aggs[i].Entry[volumeWaveReturnKey(period, h)]
				r := VolumeWaveBoxStudyResult{Mode: mode.ID, Label: mode.Label, Period: period, Horizon: h, SourceN: aggs[i].SourceN[period], EntryN: len(entries)}
				baseN := aggs[0].SourceN[period]
				if baseN > 0 {
					r.SourceRetention = 100 * float64(r.SourceN) / float64(baseN)
				}
				if r.SourceN > 0 {
					r.EntryRate = 100 * float64(r.EntryN) / float64(r.SourceN)
				}
				r.ChaseMeanNet = meanFloats(chase)
				r.PullbackMeanNet = meanFloats(entries)
				r.PullbackMedian = medianFloats(entries)
				wins, profit, loss := 0, 0.0, 0.0
				for _, v := range entries {
					if v > 0 {
						wins++
						profit += v
					} else if v < 0 {
						loss -= v
					}
				}
				if len(entries) > 0 {
					r.PullbackWinRate = 100 * float64(wins) / float64(len(entries))
				}
				if loss > 0 {
					r.PullbackPF = profit / loss
				}
				r.Improvement = r.PullbackMeanNet - r.ChaseMeanNet
				r.TStatVsChase = welchTStatFloats(entries, chase)
				results = append(results, r)
			}
		}
	}
	out := struct {
		GeneratedAt      string  `json:"generated_at"`
		OOSDate          string  `json:"oos_date"`
		RoundTripCostPct float64 `json:"round_trip_cost_pct"`
		Scanned, Failed  int64
		EntryConfig      stg.VolumeWavePullbackConfig `json:"entry_config"`
		Modes            []volumeWaveBoxMode          `json:"modes"`
		Results          []VolumeWaveBoxStudyResult   `json:"results"`
	}{GeneratedAt: time.Now().Format(time.RFC3339), OOSDate: oosDate, RoundTripCostPct: roundTripCostPct, Scanned: scanned.Load(), Failed: failed.Load(), EntryConfig: entryCfg, Modes: modes, Results: results}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON 오류: %v\n", err)
		return
	}
	if err = os.MkdirAll(filepath.Dir(outPath), 0755); err == nil {
		err = os.WriteFile(outPath, data, 0644)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "저장 오류: %v\n", err)
		return
	}
	fmt.Printf("[volume_wave_box_study] 완료: 스캔 %d 실패 %d → %s\n", scanned.Load(), failed.Load(), outPath)
	for _, r := range results {
		if r.Period == "OOS" && (r.Horizon == 5 || r.Horizon == 20) {
			fmt.Printf("  %-8s h%-2d source=%-5d retain=%5.1f%% entry=%-4d mean=%+.3f%% med=%+.3f%% PF=%.2f\n", r.Mode, r.Horizon, r.SourceN, r.SourceRetention, r.EntryN, r.PullbackMeanNet, r.PullbackMedian, r.PullbackPF)
		}
	}
}
