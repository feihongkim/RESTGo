package study

// mainbox_retest_s1_study.go — 최초 돌파가 strategy1 YAML 룰에도 실제 매칭된 cohort만 분리 검증.

import (
	"RESTGo/box"
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
	"time"
)

// HandleMainBoxRetestS1Study: ./RESTGo stock mainbox_retest_s1_study [--max N] [--candles N] [--out path]
func HandleMainBoxRetestS1Study(args []string) {
	maxStocks, candleCount := 0, 4200
	outPath, oosDate := "zpicture/mainbox_retest_s1_study.json", "20220101"
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
	rules, settings, err := stg.LoadRulesWithSettings("rules/strategy1.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	stocks, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}
	variants := buildMainBoxRetestVariants()
	groups := []string{stg.MainBoxRetestGroupA, stg.MainBoxRetestGroupB, stg.MainBoxRetestGroupC}
	aggs := make([]map[string]mainBoxRetestAgg, len(variants))
	for i := range aggs {
		aggs[i] = map[string]mainBoxRetestAgg{}
		for _, g := range groups {
			aggs[i][g] = mainBoxRetestAgg{map[string][]float64{}}
		}
	}
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	fmt.Printf("[mainbox_retest_s1] KR %d × %d변형 × A/B/C, strategy1-qualified\n", len(stocks), len(variants))
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()
			candles, e := box.FetchCandlesHannam(db, shcode, candleCount)
			if e != nil || len(candles) < 120 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)
			local := make([]map[string]mainBoxRetestAgg, len(variants))
			for vi, v := range variants {
				local[vi] = map[string]mainBoxRetestAgg{}
				for _, g := range groups {
					local[vi][g] = mainBoxRetestAgg{map[string][]float64{}}
				}
				seen := map[string]bool{}
				for _, s := range stg.MainBoxRetestAnalyzeWithRules(candles, v.Config, rules, settings) {
					key := s.Group + "|" + s.EntryDate
					if seen[key] || !volumeWavePricePathClean(candles, s.BreakoutPos, s.EntryPos) {
						continue
					}
					seen[key] = true
					p := volumeWavePeriod(s.EntryDate, oosDate)
					a := local[vi][s.Group]
					for h, r := range volumeWaveEntryReturnsClean(candles, s.EntryPos, s.EntryPriceOrigin, .30) {
						a.returns[volumeWaveReturnKey("ALL", h)] = append(a.returns[volumeWaveReturnKey("ALL", h)], r)
						a.returns[volumeWaveReturnKey(p, h)] = append(a.returns[volumeWaveReturnKey(p, h)], r)
					}
					local[vi][s.Group] = a
				}
			}
			mu.Lock()
			for vi := range variants {
				for _, g := range groups {
					a := aggs[vi][g]
					for k, x := range local[vi][g].returns {
						a.returns[k] = append(a.returns[k], x...)
					}
					aggs[vi][g] = a
				}
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[mainbox_retest_s1] %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	var results []MainBoxRetestStudyResult
	lookup := map[string]MainBoxRetestStudyResult{}
	for vi, v := range variants {
		for _, g := range groups {
			for _, p := range []string{"ALL", "IS", "OOS"} {
				for _, h := range volumeWaveMatrixHorizons {
					x := aggs[vi][g].returns[volumeWaveReturnKey(p, h)]
					r := MainBoxRetestStudyResult{VariantID: v.ID, Group: g, Config: v.Config, Period: p, Horizon: h, N: len(x), MeanNet: meanFloats(x), TrimmedMean: trimmedMeanFloats(x, .05), MedianNet: medianFloats(x), P90: percentileFloats(x, .9), Max: percentileFloats(x, 1)}
					wins := 0
					profit, loss := 0.0, 0.0
					for _, z := range x {
						if z > 0 {
							wins++
							profit += z
						} else if z < 0 {
							loss -= z
						}
					}
					if len(x) > 0 {
						r.WinRate = 100 * float64(wins) / float64(len(x))
					}
					if loss > 0 {
						r.PF = profit / loss
					}
					results = append(results, r)
					lookup[fmt.Sprintf("%s|%s|%s|%d", v.ID, g, p, h)] = r
				}
			}
		}
	}
	var top5, top20 []MainBoxRetestTop
	for _, v := range variants {
		for _, g := range groups {
			for _, pair := range []struct {
				h   int
				dst *[]MainBoxRetestTop
			}{{5, &top5}, {20, &top20}} {
				is := lookup[fmt.Sprintf("%s|%s|IS|%d", v.ID, g, pair.h)]
				oo := lookup[fmt.Sprintf("%s|%s|OOS|%d", v.ID, g, pair.h)]
				if is.N >= 100 && oo.N >= 100 && is.MeanNet > 0 && oo.MeanNet > 0 {
					*pair.dst = append(*pair.dst, MainBoxRetestTop{v.ID, g, v.Config, is.N, is.MeanNet, oo.N, oo.MeanNet, oo.MedianNet, oo.PF, minFloat(is.MeanNet, oo.MeanNet)})
				}
			}
		}
	}
	sortTop := func(x []MainBoxRetestTop) { sort.Slice(x, func(i, j int) bool { return x[i].Score > x[j].Score }) }
	sortTop(top5)
	sortTop(top20)
	if len(top5) > 20 {
		top5 = top5[:20]
	}
	if len(top20) > 20 {
		top20 = top20[:20]
	}
	out := struct {
		GeneratedAt, OOSDate, Cohort string
		CostPct                      float64
		Scanned, Failed              int64
		Variants                     []mainBoxRetestVariant
		Results                      []MainBoxRetestStudyResult
		TopH5, TopH20                []MainBoxRetestTop
	}{time.Now().Format(time.RFC3339), oosDate, "strategy1.yaml matched at original DefBox breakout", .30, scanned.Load(), failed.Load(), variants, results, top5, top20}
	data, _ := json.MarshalIndent(out, "", "  ")
	os.MkdirAll(filepath.Dir(outPath), 0755)
	if err = os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[mainbox_retest_s1] 완료 scan=%d fail=%d robust h5=%d h20=%d → %s\n", scanned.Load(), failed.Load(), len(top5), len(top20), outPath)
	for _, r := range results {
		if r.VariantID == "R10_T04_U06_W80" && r.Period == "OOS" && (r.Horizon == 5 || r.Horizon == 20) {
			fmt.Printf("  %s h%d n=%d mean=%+.3f%% med=%+.3f%% PF=%.2f\n", r.Group, r.Horizon, r.N, r.MeanNet, r.MedianNet, r.PF)
		}
	}
}
