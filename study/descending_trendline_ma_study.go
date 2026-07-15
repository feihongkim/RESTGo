package study

// descending_trendline_ma_study.go — apex 수렴 + MA60/120 간격 축소 + 최근/임박 골든크로스 소거.
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

type dtMAFilter struct {
	ID     string                        `json:"id"`
	Config stg.DescendingTrendlineConfig `json:"config"`
}
type DescendingMAResult struct {
	Structure, Filter, Period          string
	Horizon, N                         int
	Mean, Trimmed, Median, WinRate, PF float64
}

func dtMAFilters() []dtMAFilter {
	base := stg.DefaultDescendingTrendlineConfig()
	var v []dtMAFilter
	v = append(v, dtMAFilter{"BASE", base})
	for _, a := range []int{5, 10, 20} {
		c := base
		c.RequireApexProximity = true
		c.MaxApexDistanceBars = a
		v = append(v, dtMAFilter{fmt.Sprintf("APEX_%02d", a), c})
	}
	for _, lb := range []int{10, 20} {
		for _, g := range []float64{.01, .02, .03} {
			c := base
			c.RequireMAConvergence = true
			c.MAGapLookback = lb
			c.MaxMA60120Gap = g
			v = append(v, dtMAFilter{fmt.Sprintf("CONV_L%02d_G%02.0f", lb, g*100), c})
		}
	}
	for _, lb := range []int{10, 20} {
		c := base
		c.GoldenCrossMode = stg.DescendingGoldenRecent
		c.GoldenCrossLookback = lb
		v = append(v, dtMAFilter{fmt.Sprintf("GOLD_RECENT_%02d", lb), c})
	}
	for _, g := range []float64{.01, .02, .03} {
		c := base
		c.GoldenCrossMode = stg.DescendingGoldenImminent
		c.MaxMA60120Gap = g
		v = append(v, dtMAFilter{fmt.Sprintf("GOLD_IMMINENT_%02.0f", g*100), c})
	}
	for _, a := range []int{5, 10, 20} {
		for _, lb := range []int{10, 20} {
			for _, g := range []float64{.01, .02, .03} {
				c := base
				c.RequireApexProximity = true
				c.MaxApexDistanceBars = a
				c.RequireMAConvergence = true
				c.MAGapLookback = lb
				c.MaxMA60120Gap = g
				c.GoldenCrossMode = stg.DescendingGoldenEither
				c.GoldenCrossLookback = 20
				v = append(v, dtMAFilter{fmt.Sprintf("ALL_A%02d_L%02d_G%02.0f", a, lb, g*100), c})
			}
		}
	}
	return v
}
func HandleDescendingTrendlineMAStudy(args []string) {
	maxStocks, nc := 0, 4200
	out, oos := "zpicture/descending_trendline_ma_study.json", "20220101"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--max":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxStocks)
				i++
			}
		case "--candles":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &nc)
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
	structures := []dtVariant{}
	for _, s := range []float64{.05, .08} {
		c := stg.DefaultDescendingTrendlineConfig()
		c.SupportTolerance = s
		c.MinResistanceDrop = .03
		c.MinPatternBars = 60
		c.MaxBreakoutWait = 20
		structures = append(structures, dtVariant{fmt.Sprintf("S%02.0f_D03_B60_W20", s*100), c})
	}
	filters := dtMAFilters()
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
	fmt.Printf("[descending_ma] KR %d × %d구조 × %d필터\n", len(stocks), len(structures), len(filters))
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(sh string) {
			defer wg.Done()
			defer func() { <-sem }()
			c, e := box.FetchCandlesHannam(db, sh, nc)
			if e != nil || len(c) < 140 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(c)
			local := make([]map[string]dtAgg, len(structures))
			for si, s := range structures {
				local[si] = map[string]dtAgg{}
				for _, f := range filters {
					local[si][f.ID] = dtAgg{map[string][]float64{}}
				}
				seen := map[string]bool{}
				for _, sig := range stg.DescendingTrendlineAnalyze(c, s.Config) {
					if !volumeWavePricePathClean(c, sig.Pattern.R1Pos, sig.EntryPos) {
						continue
					}
					for _, f := range filters {
						if !stg.DescendingTrendlinePassesContext(c, sig.Pos, sig.Pattern, f.Config) {
							continue
						}
						k := f.ID + "|" + sig.EntryDate
						if seen[k] {
							continue
						}
						seen[k] = true
						p := volumeWavePeriod(sig.EntryDate, oos)
						a := local[si][f.ID]
						for h, r := range dtEntryReturns(c, sig.EntryPos, sig.EntryPriceOrigin, .30) {
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
				fmt.Printf("[descending_ma] %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	var res []DescendingMAResult
	for si, s := range structures {
		for _, f := range filters {
			for _, p := range []string{"ALL", "IS", "OOS"} {
				for _, h := range dtHorizons {
					x := ag[si][f.ID].r[volumeWaveReturnKey(p, h)]
					r := DescendingMAResult{Structure: s.ID, Filter: f.ID, Period: p, Horizon: h, N: len(x), Mean: meanFloats(x), Trimmed: trimmedMeanFloats(x, .05), Median: medianFloats(x)}
					wins := 0
					pr, ls := 0.0, 0.0
					for _, z := range x {
						if z > 0 {
							wins++
							pr += z
						} else if z < 0 {
							ls -= z
						}
					}
					if len(x) > 0 {
						r.WinRate = 100 * float64(wins) / float64(len(x))
					}
					if ls > 0 {
						r.PF = pr / ls
					}
					res = append(res, r)
				}
			}
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Mean > res[j].Mean })
	payload := struct {
		GeneratedAt     string
		Cost            float64
		Scanned, Failed int64
		Structures      []dtVariant
		Filters         []dtMAFilter
		Results         []DescendingMAResult
	}{time.Now().Format(time.RFC3339), .30, scanned.Load(), failed.Load(), structures, filters, res}
	b, _ := json.MarshalIndent(payload, "", "  ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err = os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[descending_ma] 완료 scan=%d fail=%d → %s\n", scanned.Load(), failed.Load(), out)
	for _, r := range res {
		if r.Period == "OOS" && r.Horizon == 5 && r.N >= 30 && r.Mean > 0 {
			fmt.Printf("  %s %s n=%d mean=%+.3f trim=%+.3f med=%+.3f PF=%.2f\n", r.Structure, r.Filter, r.N, r.Mean, r.Trimmed, r.Median, r.PF)
		}
	}
}
