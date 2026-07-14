package study

// volume_wave_scan.go — 선행 거래량→고가 횡보→2파동→MA20 3파동 전략의 1차 카탈로그/엣지 스캔.
// 기존 strategy1/매도 엔진을 호출하지 않는 독립 연구 러너다.

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
	"sort"
	"sync"
	"sync/atomic"
)

type VolumeWaveExample struct {
	Market           string                         `json:"market"`
	Shcode           string                         `json:"shcode"`
	Kind             string                         `json:"kind"`
	Stage            int                            `json:"stage"`
	SignalDate       string                         `json:"signal_date"`
	SignalPos        int                            `json:"signal_pos"`
	PriceOrigin      float64                        `json:"price_origin"`
	Volume           float64                        `json:"volume"`
	AccumulationDate string                         `json:"accumulation_date"`
	AccumulationLead int                            `json:"accumulation_lead_bars"`
	Wave1Date        string                         `json:"wave1_date,omitempty"`
	Wave2Date        string                         `json:"wave2_date,omitempty"`
	Wave3Date        string                         `json:"wave3_date,omitempty"`
	Wave1Volume      float64                        `json:"wave1_volume"`
	Wave2Volume      float64                        `json:"wave2_volume"`
	Base             cond.VolumeWaveBaseMetrics     `json:"base"`
	Pullback         cond.VolumeWavePullbackMetrics `json:"pullback"`
	ReturnFromWave3  *float64                       `json:"return_from_wave3_pct,omitempty"`
	R1               *float64                       `json:"r1,omitempty"`
	R5               *float64                       `json:"r5,omitempty"`
	R10              *float64                       `json:"r10,omitempty"`
	R20              *float64                       `json:"r20,omitempty"`
}

type VolumeWaveStageStat struct {
	Kind         string  `json:"kind"`
	Stage        int     `json:"stage"`
	Horizon      int     `json:"horizon"`
	N            int     `json:"n"`
	Mean         float64 `json:"mean_pct"`
	Median       float64 `json:"median_pct"`
	WinRate      float64 `json:"win_rate_pct"`
	BaselineMean float64 `json:"baseline_mean_pct"`
	BaselineN    int     `json:"baseline_n"`
	Edge         float64 `json:"edge_pct"`
	TStat        float64 `json:"t_stat"`
}

// HandleVolumeWaveScan 사용법:
// ./RESTGo stock volume_wave_scan [--hannam|--foreign-jp|--foreign-cn|--foreign-hk]
//
//	[--max N] [--candles N] [--out path]
func HandleVolumeWaveScan(args []string) {
	mode := "hannam"
	maxStocks := 0
	candleCount := 4200
	outPath := "zpicture/volume_wave_scan.json"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hannam":
			mode = "hannam"
		case "--foreign-jp":
			mode = "foreign-jp"
		case "--foreign-cn":
			mode = "foreign-cn"
		case "--foreign-hk":
			mode = "foreign-hk"
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
		}
	}

	dbName := "KIS2"
	if mode == "hannam" {
		dbName = "han"
	}
	db, err := console.MsConn.GetDB(dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_scan] DB 연결 오류: %v\n", err)
		return
	}

	var stocks []string
	switch mode {
	case "hannam":
		stocks, err = box.FetchHannamStockList(db)
	case "foreign-jp":
		stocks, err = box.FetchForeignStockList(db, []string{"DTS"})
	case "foreign-cn":
		stocks, err = box.FetchForeignStockList(db, []string{"DSZ", "DSH"})
	case "foreign-hk":
		stocks, err = box.FetchForeignStockList(db, []string{"DHK"})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_scan] 종목 목록 오류: %v\n", err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}
	market := map[string]string{
		"hannam": "KR", "foreign-jp": "JP", "foreign-cn": "CN", "foreign-hk": "HK",
	}[mode]
	cfg := cond.DefaultVolumeWaveConfig()
	fmt.Printf("[volume_wave_scan] %s %d종목, 캔들 %d, 출력 %s\n", market, len(stocks), candleCount, outPath)

	fwdRet := func(candles []*box.Candle, pos, horizon int) *float64 {
		if pos < 0 || pos+horizon >= len(candles) || candles[pos].Close <= 0 {
			return nil
		}
		r := (candles[pos+horizon].Close - candles[pos].Close) / candles[pos].Close * 100
		return &r
	}

	var mu sync.Mutex
	var examples []VolumeWaveExample
	baseline := map[int][]float64{}
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()

			var candles []*box.Candle
			var fetchErr error
			if mode == "hannam" {
				candles, fetchErr = box.FetchCandlesHannam(db, shcode, candleCount)
			} else {
				candles, fetchErr = box.FetchCandlesForeign(db, shcode, candleCount)
			}
			if fetchErr != nil || len(candles) < cfg.AccumulationMaxLead+cfg.VolumeWindow+20 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)
			signals := stg.VolumeWaveAnalyze(candles, cfg)

			localBase := map[int][]float64{}
			for pos := cfg.AccumulationMaxLead + cfg.VolumeWindow; pos+20 < len(candles); pos += 21 {
				for _, h := range mtopHorizons {
					if r := fwdRet(candles, pos, h); r != nil {
						localBase[h] = append(localBase[h], *r)
					}
				}
			}

			localExamples := make([]VolumeWaveExample, 0, len(signals))
			for _, s := range signals {
				ex := VolumeWaveExample{
					Market: market, Shcode: shcode, Kind: s.Kind, Stage: s.Stage,
					SignalDate: s.Date, SignalPos: s.Pos, PriceOrigin: s.PriceOrigin, Volume: s.Volume,
					AccumulationDate: s.AccumulationDate, AccumulationLead: s.AccumulationLead,
					Wave1Date: s.Wave1Date, Wave2Date: s.Wave2Date, Wave3Date: s.Wave3Date,
					Wave1Volume: s.Cycle.Wave1Volume, Wave2Volume: s.Cycle.Wave2Volume,
					Base: s.Cycle.Base, Pullback: s.Cycle.Pullback,
					R1: fwdRet(candles, s.Pos, 1), R5: fwdRet(candles, s.Pos, 5),
					R10: fwdRet(candles, s.Pos, 10), R20: fwdRet(candles, s.Pos, 20),
				}
				if s.Kind == stg.VolumeWaveExit && s.Cycle.Wave3Pos >= 0 && candles[s.Cycle.Wave3Pos].Close > 0 {
					r := (candles[s.Pos].Close - candles[s.Cycle.Wave3Pos].Close) / candles[s.Cycle.Wave3Pos].Close * 100
					ex.ReturnFromWave3 = &r
				}
				localExamples = append(localExamples, ex)
			}

			mu.Lock()
			examples = append(examples, localExamples...)
			for h, values := range localBase {
				baseline[h] = append(baseline[h], values...)
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[volume_wave_scan] 진행 %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()

	sort.Slice(examples, func(i, j int) bool {
		if examples[i].Shcode != examples[j].Shcode {
			return examples[i].Shcode < examples[j].Shcode
		}
		if examples[i].SignalDate != examples[j].SignalDate {
			return examples[i].SignalDate < examples[j].SignalDate
		}
		return examples[i].Stage < examples[j].Stage
	})

	pick := func(ex VolumeWaveExample, horizon int) *float64 {
		switch horizon {
		case 1:
			return ex.R1
		case 5:
			return ex.R5
		case 10:
			return ex.R10
		default:
			return ex.R20
		}
	}
	kinds := []struct {
		name  string
		stage int
	}{{stg.VolumeWaveStage1, 1}, {stg.VolumeWaveStage2, 2}, {stg.VolumeWaveStage3, 3}}
	var stats []VolumeWaveStageStat
	for _, kind := range kinds {
		for _, h := range mtopHorizons {
			var returns []float64
			for _, ex := range examples {
				if ex.Kind == kind.name {
					if r := pick(ex, h); r != nil {
						returns = append(returns, *r)
					}
				}
			}
			if len(returns) == 0 {
				continue
			}
			sorted := append([]float64(nil), returns...)
			sort.Float64s(sorted)
			wins := 0
			for _, r := range returns {
				if r > 0 {
					wins++
				}
			}
			baseMean := meanFloats(baseline[h])
			stats = append(stats, VolumeWaveStageStat{
				Kind: kind.name, Stage: kind.stage, Horizon: h, N: len(returns),
				Mean: meanFloats(returns), Median: medianFloats(sorted),
				WinRate:      100 * float64(wins) / float64(len(returns)),
				BaselineMean: baseMean, BaselineN: len(baseline[h]),
				Edge:  meanFloats(returns) - baseMean,
				TStat: welchTStatFloats(returns, baseline[h]),
			})
		}
	}

	counts := map[string]int{}
	for _, ex := range examples {
		counts[ex.Kind]++
	}
	out := struct {
		Market      string                `json:"market"`
		CandleCount int                   `json:"candle_count"`
		StockCount  int                   `json:"stock_count"`
		FailedCount int64                 `json:"failed_count"`
		Config      cond.VolumeWaveConfig `json:"config"`
		Counts      map[string]int        `json:"counts"`
		Stats       []VolumeWaveStageStat `json:"stats"`
		Examples    []VolumeWaveExample   `json:"examples"`
	}{market, candleCount, len(stocks), failed.Load(), cfg, counts, stats, examples}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_scan] JSON 실패: %v\n", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_scan] 출력 디렉토리 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_scan] 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("[volume_wave_scan] 완료: 스캔 %d, 실패 %d, VW1 %d / VW2 %d / VW3 %d / EXIT %d → %s\n",
		scanned.Load(), failed.Load(), counts[stg.VolumeWaveStage1], counts[stg.VolumeWaveStage2],
		counts[stg.VolumeWaveStage3], counts[stg.VolumeWaveExit], outPath)
	for _, stat := range stats {
		fmt.Printf("[volume_wave_scan] %s h%-2d n=%d mean=%+.3f%% median=%+.3f%% win=%.1f%% edge=%+.3f%%p t=%.2f\n",
			stat.Kind, stat.Horizon, stat.N, stat.Mean, stat.Median, stat.WinRate, stat.Edge, stat.TStat)
	}
}
