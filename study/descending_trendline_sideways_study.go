package study

// descending_trendline_sideways_study.go — MA를 배제하고 돌파 직전 가격 횡보·압축을 소거한다.
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

type dtSidewaysFilter struct {
	ID     string                        `json:"id"`
	Config stg.DescendingTrendlineConfig `json:"config"`
}

type DescendingSidewaysResult struct {
	Structure, Filter, Period          string
	Horizon, N                         int
	Mean, Trimmed, Median, WinRate, PF float64
}

func dtSidewaysFilters() []dtSidewaysFilter {
	base := stg.DefaultDescendingTrendlineConfig()
	var out []dtSidewaysFilter
	out = append(out, dtSidewaysFilter{"BASE", base})
	for _, apex := range []int{5, 10, 20} {
		c := base
		c.RequireApexProximity = true
		c.MaxApexDistanceBars = apex
		out = append(out, dtSidewaysFilter{fmt.Sprintf("APEX_%02d", apex), c})
	}
	for _, lb := range []int{10, 20, 30} {
		for _, width := range []float64{.06, .08, .10, .12} {
			for _, drift := range []float64{.03, .05} {
				c := base
				c.RequireLateSideways = true
				c.SidewaysLookback = lb
				c.MaxSidewaysRange = width
				c.MaxSidewaysNetChange = drift
				out = append(out, dtSidewaysFilter{fmt.Sprintf("SIDE_L%02d_W%02.0f_D%02.0f", lb, width*100, drift*100), c})
			}
		}
	}
	for _, apex := range []int{5, 10, 20} {
		for _, lb := range []int{10, 20, 30} {
			for _, width := range []float64{.06, .08, .10, .12} {
				for _, drift := range []float64{.03, .05} {
					c := base
					c.RequireApexProximity = true
					c.MaxApexDistanceBars = apex
					c.RequireLateSideways = true
					c.SidewaysLookback = lb
					c.MaxSidewaysRange = width
					c.MaxSidewaysNetChange = drift
					out = append(out, dtSidewaysFilter{fmt.Sprintf("BOTH_A%02d_L%02d_W%02.0f_D%02.0f", apex, lb, width*100, drift*100), c})
				}
			}
		}
	}
	return out
}

func HandleDescendingTrendlineSidewaysStudy(args []string) {
	maxStocks, candleN := 0, 4200
	out, oosStart := "zpicture/descending_trendline_sideways_study.json", "20220101"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--max":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxStocks)
				i++
			}
		case "--candles":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &candleN)
				i++
			}
		case "--out":
			if i+1 < len(args) {
				out = args[i+1]
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
	var structures []dtVariant
	for _, support := range []float64{.05, .08} {
		for _, bars := range []int{30, 60} {
			c := stg.DefaultDescendingTrendlineConfig()
			c.SupportTolerance = support
			c.MinResistanceDrop = .03
			c.MinPatternBars = bars
			c.MaxBreakoutWait = 20
			structures = append(structures, dtVariant{fmt.Sprintf("S%02.0f_D03_B%02d_W20", support*100, bars), c})
		}
	}
	filters := dtSidewaysFilters()
	ag := make([]map[string]dtAgg, len(structures))
	for i := range ag {
		ag[i] = map[string]dtAgg{}
		for _, f := range filters {
			ag[i][f.ID] = dtAgg{map[string][]float64{}}
		}
	}
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	fmt.Printf("[descending_sideways] KR %d × %d구조 × %d필터\n", len(stocks), len(structures), len(filters))
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(sh string) {
			defer wg.Done()
			defer func() { <-sem }()
			candles, e := box.FetchCandlesHannam(db, sh, candleN)
			if e != nil || len(candles) < 140 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)
			local := make([]map[string]dtAgg, len(structures))
			for si, s := range structures {
				local[si] = map[string]dtAgg{}
				for _, f := range filters {
					local[si][f.ID] = dtAgg{map[string][]float64{}}
				}
				seen := map[string]bool{}
				for _, sig := range stg.DescendingTrendlineAnalyze(candles, s.Config) {
					if !volumeWavePricePathClean(candles, sig.Pattern.R1Pos, sig.EntryPos) {
						continue
					}
					for _, f := range filters {
						if !stg.DescendingTrendlinePassesContext(candles, sig.Pos, sig.Pattern, f.Config) {
							continue
						}
						key := f.ID + "|" + sig.EntryDate
						if seen[key] {
							continue
						}
						seen[key] = true
						p := volumeWavePeriod(sig.EntryDate, oosStart)
						a := local[si][f.ID]
						for h, r := range dtEntryReturns(candles, sig.EntryPos, sig.EntryPriceOrigin, .30) {
							a.r[volumeWaveReturnKey("ALL", h)] = append(a.r[volumeWaveReturnKey("ALL", h)], r)
							a.r[volumeWaveReturnKey(p, h)] = append(a.r[volumeWaveReturnKey(p, h)], r)
						}
						local[si][f.ID] = a
					}
				}
			}
			mu.Lock()
			for si := range structures {
				for _, f := range filters {
					a := ag[si][f.ID]
					for k, x := range local[si][f.ID].r {
						a.r[k] = append(a.r[k], x...)
					}
					ag[si][f.ID] = a
				}
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[descending_sideways] %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	var results []DescendingSidewaysResult
	for si, s := range structures {
		for _, f := range filters {
			for _, p := range []string{"ALL", "IS", "OOS"} {
				for _, h := range dtHorizons {
					x := ag[si][f.ID].r[volumeWaveReturnKey(p, h)]
					r := DescendingSidewaysResult{Structure: s.ID, Filter: f.ID, Period: p, Horizon: h, N: len(x), Mean: meanFloats(x), Trimmed: trimmedMeanFloats(x, .05), Median: medianFloats(x)}
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
				}
			}
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Mean > results[j].Mean })
	payload := struct {
		GeneratedAt     string
		Cost            float64
		Scanned, Failed int64
		Structures      []dtVariant
		Filters         []dtSidewaysFilter
		Results         []DescendingSidewaysResult
	}{time.Now().Format(time.RFC3339), .30, scanned.Load(), failed.Load(), structures, filters, results}
	b, _ := json.MarshalIndent(payload, "", "  ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err = os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[descending_sideways] 완료 scan=%d fail=%d → %s\n", scanned.Load(), failed.Load(), out)
	for _, r := range results {
		if r.Period == "OOS" && r.Horizon == 5 && r.N >= 100 && r.Mean > 0 {
			fmt.Printf("  %s %s n=%d mean=%+.3f trim=%+.3f med=%+.3f PF=%.2f\n", r.Structure, r.Filter, r.N, r.Mean, r.Trimmed, r.Median, r.PF)
		}
	}
}
