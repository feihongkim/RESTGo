package study

// volume_wave_chart.go — 선정된 VW2 첫 눌림 변형의 대표 OOS 사례(P90/P50/P10)를
// 차트용 JSON으로 덤프한다. 매수=반등 확인 다음 봉 시가, 매도=20봉 뒤 종가.

import (
	"RESTGo/box"
	"RESTGo/cond"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
)

type volumeWaveChartCandidate struct {
	Shcode      string
	SourceDate  string
	PullbackDate string
	FireDate    string
	EntryDate   string
	ExitDate    string
	NetReturn   float64
}

type VolumeWaveChartCandle struct {
	Date    string  `json:"date"`
	Open    float64 `json:"open"`
	High    float64 `json:"high"`
	Low     float64 `json:"low"`
	Close   float64 `json:"close"`
	Volume  float64 `json:"volume"`
	MA20    float64 `json:"ma20"`
	VolMA20 float64 `json:"vol_ma20"`
}

type VolumeWaveChartSample struct {
	Label        string                  `json:"label"`
	Shcode       string                  `json:"shcode"`
	SourceDate   string                  `json:"source_date"`
	PullbackDate string                  `json:"pullback_date"`
	FireDate     string                  `json:"fire_date"`
	EntryDate    string                  `json:"entry_date"`
	ExitDate     string                  `json:"exit_date"`
	EntryPrice   float64                 `json:"entry_price"`
	ExitPrice    float64                 `json:"exit_price"`
	NetReturn    float64                 `json:"net_return_pct"`
	Candles      []VolumeWaveChartCandle `json:"candles"`
}

func selectedVolumeWavePullbackConfig() stg.VolumeWavePullbackConfig {
	return stg.VolumeWavePullbackConfig{
		SourceStage: 2, MinWaitBars: 1, MaxWaitBars: 15,
		MinDepth: 0.01, MaxDepth: 0.08,
		MaxEntryVolumeRatio: 0.50, MaxPullbackAverageVolumeRatio: 0.50,
		Confirmation: stg.VolumeWaveConfirmPrevHigh,
		Structure: stg.VolumeWaveStructureNone, StructureTolerance: 0.02,
	}
}

// HandleVolumeWaveChartSamples 사용법:
// ./RESTGo stock volume_wave_charts [--max N] [--candles N] [--out path] [--oos-date YYYYMMDD]
func HandleVolumeWaveChartSamples(args []string) {
	maxStocks := 0
	candleCount := 4200
	outPath := "zpicture/volume_wave_chart_samples.json"
	oosDate := "20220101"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--max":
			if i+1 < len(args) { fmt.Sscanf(args[i+1], "%d", &maxStocks); i++ }
		case "--candles":
			if i+1 < len(args) { fmt.Sscanf(args[i+1], "%d", &candleCount); i++ }
		case "--out":
			if i+1 < len(args) { outPath = args[i+1]; i++ }
		case "--oos-date":
			if i+1 < len(args) { oosDate = args[i+1]; i++ }
		}
	}

	db, err := console.MsConn.GetDB("han")
	if err != nil { fmt.Fprintf(os.Stderr, "[volume_wave_charts] DB 오류: %v\n", err); return }
	stocks, err := box.FetchHannamStockList(db)
	if err != nil { fmt.Fprintf(os.Stderr, "[volume_wave_charts] 종목 오류: %v\n", err); return }
	if maxStocks > 0 && len(stocks) > maxStocks { stocks = stocks[:maxStocks] }

	waveCfg := cond.DefaultVolumeWaveConfig()
	entryCfg := selectedVolumeWavePullbackConfig()
	const roundTripCostPct = 0.30
	var candidates []volumeWaveChartCandidate
	var mu sync.Mutex
	var scanned atomic.Int64
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, code := range stocks {
		wg.Add(1); sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done(); defer func(){ <-sem }()
			candles, fetchErr := box.FetchCandlesHannam(db, shcode, candleCount)
			if fetchErr != nil || len(candles) < 120 { return }
			indicator.PrepareCandles(candles)
			waves := stg.VolumeWaveAnalyze(candles, waveCfg)
			entries := stg.VolumeWavePullbackAnalyze(candles, waves, entryCfg)
			var local []volumeWaveChartCandidate
			for _, e := range entries {
				exitPos := e.EntryPos + 20
				if e.EntryDate < oosDate || exitPos >= len(candles) || e.EntryPriceOrigin <= 0 { continue }
				exitPrice := candles[exitPos].CloseOrigin
				if exitPrice <= 0 { continue }
				local = append(local, volumeWaveChartCandidate{
					Shcode: shcode, SourceDate: e.SourceDate,
					PullbackDate: e.PullbackStartDate, FireDate: e.FireDate,
					EntryDate: e.EntryDate, ExitDate: candles[exitPos].Date,
					NetReturn: (exitPrice-e.EntryPriceOrigin)/e.EntryPriceOrigin*100-roundTripCostPct,
				})
			}
			mu.Lock(); candidates = append(candidates, local...); mu.Unlock()
			scanned.Add(1)
		}(code)
	}
	wg.Wait()
	if len(candidates) < 3 { fmt.Fprintf(os.Stderr, "[volume_wave_charts] 후보 부족: %d\n", len(candidates)); return }
	sort.Slice(candidates, func(i,j int) bool { return candidates[i].NetReturn < candidates[j].NetReturn })
	selected := selectVolumeWaveChartPercentiles(candidates)

	labels := []string{"P90 winner", "P50 median", "P10 loser"}
	var samples []VolumeWaveChartSample
	for i, candidate := range selected {
		candles, fetchErr := box.FetchCandlesHannam(db, candidate.Shcode, candleCount)
		if fetchErr != nil { continue }
		indicator.PrepareCandles(candles)
		index := map[string]int{}
		for pos, c := range candles { index[c.Date] = pos }
		sourcePos, ok1 := index[candidate.SourceDate]
		entryPos, ok2 := index[candidate.EntryDate]
		exitPos, ok3 := index[candidate.ExitDate]
		if !ok1 || !ok2 || !ok3 { continue }
		start, end := sourcePos-25, exitPos+10
		if start < 0 { start = 0 }
		if end >= len(candles) { end = len(candles)-1 }
		window := make([]VolumeWaveChartCandle, 0, end-start+1)
		for pos := start; pos <= end; pos++ {
			c := candles[pos]
			window = append(window, VolumeWaveChartCandle{
				Date:c.Date, Open:c.OpenOrigin, High:c.HighOrigin, Low:c.LowOrigin,
				Close:c.CloseOrigin, Volume:c.Volume, MA20:c.Ma20Origin, VolMA20:c.VolMa20,
			})
		}
		samples = append(samples, VolumeWaveChartSample{
			Label:labels[i], Shcode:candidate.Shcode,
			SourceDate:candidate.SourceDate, PullbackDate:candidate.PullbackDate,
			FireDate:candidate.FireDate, EntryDate:candidate.EntryDate, ExitDate:candidate.ExitDate,
			EntryPrice:candles[entryPos].OpenOrigin, ExitPrice:candles[exitPos].CloseOrigin,
			NetReturn:candidate.NetReturn, Candles:window,
		})
	}

	out := struct {
		Strategy string                       `json:"strategy"`
		Selection string                      `json:"selection"`
		CandidateCount int                    `json:"candidate_count"`
		ScannedCount int64                    `json:"scanned_count"`
		Config stg.VolumeWavePullbackConfig   `json:"config"`
		Samples []VolumeWaveChartSample       `json:"samples"`
	}{
		Strategy:"VW2 first pullback: W15 D1-8% V50 PH, next-open buy, +20 close sell",
		Selection:"OOS net-return P90/P50/P10; one distinct stock per percentile",
		CandidateCount:len(candidates), ScannedCount:scanned.Load(), Config:entryCfg, Samples:samples,
	}
	data, err := json.MarshalIndent(out,"","  ")
	if err != nil { fmt.Fprintf(os.Stderr,"[volume_wave_charts] JSON 오류: %v\n",err); return }
	if err := os.MkdirAll(filepath.Dir(outPath),0755); err != nil { fmt.Fprintf(os.Stderr,"[volume_wave_charts] 디렉토리 오류: %v\n",err); return }
	if err := os.WriteFile(outPath,data,0644); err != nil { fmt.Fprintf(os.Stderr,"[volume_wave_charts] 저장 오류: %v\n",err); return }
	fmt.Printf("[volume_wave_charts] 후보 %d건 중 P90/P50/P10 %d개 저장 → %s\n",len(candidates),len(samples),outPath)
	for _, s := range samples { fmt.Printf("  %s %s buy=%s sell=%s net=%+.2f%%\n",s.Label,s.Shcode,s.EntryDate,s.ExitDate,s.NetReturn) }
}

func selectVolumeWaveChartPercentiles(sorted []volumeWaveChartCandidate) []volumeWaveChartCandidate {
	targets := []float64{0.90, 0.50, 0.10}
	used := map[string]bool{}
	out := make([]volumeWaveChartCandidate,0,len(targets))
	for _, q := range targets {
		idx := int(math.Round(float64(len(sorted)-1)*q))
		for radius:=0; radius<len(sorted); radius++ {
			for _, candidateIdx := range []int{idx-radius,idx+radius} {
				if candidateIdx<0 || candidateIdx>=len(sorted) { continue }
				c := sorted[candidateIdx]
				if used[c.Shcode] { continue }
				used[c.Shcode]=true; out=append(out,c); radius=len(sorted); break
			}
		}
	}
	return out
}
