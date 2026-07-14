package study

// mainbox_retest_study.go — DefBox 돌파 후 MainBox 수평선 재시험 전략 A/B/C 및 파라미터 비교.

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

type mainBoxRetestVariant struct {
	ID     string                  `json:"id"`
	Config stg.MainBoxRetestConfig `json:"config"`
}
type mainBoxRetestAgg struct{ returns map[string][]float64 }
type MainBoxRetestStudyResult struct {
	VariantID   string                  `json:"variant_id"`
	Group       string                  `json:"group"`
	Config      stg.MainBoxRetestConfig `json:"config"`
	Period      string                  `json:"period"`
	Horizon     int                     `json:"horizon"`
	N           int                     `json:"n"`
	MeanNet     float64                 `json:"mean_net_pct"`
	TrimmedMean float64                 `json:"trimmed_mean_5pct"`
	MedianNet   float64                 `json:"median_net_pct"`
	WinRate     float64                 `json:"win_rate_pct"`
	PF          float64                 `json:"profit_factor"`
	P90         float64                 `json:"p90_net_pct"`
	Max         float64                 `json:"max_net_pct"`
}
type MainBoxRetestTop struct {
	VariantID string                  `json:"variant_id"`
	Group     string                  `json:"group"`
	Config    stg.MainBoxRetestConfig `json:"config"`
	ISN       int                     `json:"is_n"`
	ISMean    float64                 `json:"is_mean_net_pct"`
	OOSN      int                     `json:"oos_n"`
	OOSMean   float64                 `json:"oos_mean_net_pct"`
	OOSMedian float64                 `json:"oos_median_net_pct"`
	OOSPF     float64                 `json:"oos_profit_factor"`
	Score     float64                 `json:"score"`
}

func buildMainBoxRetestVariants() []mainBoxRetestVariant {
	var out []mainBoxRetestVariant
	for _, runup := range []float64{.10, .20} {
		for _, touch := range []float64{.02, .04} {
			for _, under := range []float64{.03, .06} {
				for _, bars := range []int{40, 80} {
					c := stg.DefaultMainBoxRetestConfig()
					c.MinRunupPct = runup
					c.TouchTolerance = touch
					c.MaxIntradayUndercut = under
					c.MaxRetestBars = bars
					id := fmt.Sprintf("R%02.0f_T%02.0f_U%02.0f_W%02d", runup*100, touch*100, under*100, bars)
					out = append(out, mainBoxRetestVariant{id, c})
				}
			}
		}
	}
	return out
}

// HandleMainBoxRetestStudy: ./RESTGo stock mainbox_retest_study [--max N] [--candles N] [--out path]
func HandleMainBoxRetestStudy(args []string) {
	maxStocks, candleCount := 0, 4200
	outPath, oosDate := "zpicture/mainbox_retest_study.json", "20220101"
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
	fmt.Printf("[mainbox_retest_study] KR %d × %d변형 × A/B/C\n", len(stocks), len(variants))
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
				for _, s := range stg.MainBoxRetestAnalyze(candles, v.Config) {
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
				fmt.Printf("[mainbox_retest_study] %d/%d\n", n, len(stocks))
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
		GeneratedAt, OOSDate string
		CostPct              float64
		Scanned, Failed      int64
		Dedup                string
		Variants             []mainBoxRetestVariant
		Results              []MainBoxRetestStudyResult
		TopH5, TopH20        []MainBoxRetestTop
	}{time.Now().Format(time.RFC3339), oosDate, .30, scanned.Load(), failed.Load(), "stock+group+entry_date", variants, results, top5, top20}
	data, _ := json.MarshalIndent(out, "", "  ")
	os.MkdirAll(filepath.Dir(outPath), 0755)
	if err = os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[mainbox_retest_study] 완료 scan=%d fail=%d robust h5=%d h20=%d → %s\n", scanned.Load(), failed.Load(), len(top5), len(top20), outPath)
	base := variants[len(variants)-1]
	_ = base
	for _, r := range results {
		if r.VariantID == "R10_T04_U06_W80" && r.Period == "OOS" && (r.Horizon == 5 || r.Horizon == 20) {
			fmt.Printf("  %s h%d n=%d mean=%+.3f%% med=%+.3f%% PF=%.2f\n", r.Group, r.Horizon, r.N, r.MeanNet, r.MedianNet, r.PF)
		}
	}
}
