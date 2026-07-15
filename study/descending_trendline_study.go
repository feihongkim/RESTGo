package study

// descending_trendline_study.go — R-S-R-S 하락추세선 돌파 소거·파라미터 매트릭스.
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

type dtVariant struct {
	ID     string                        `json:"id"`
	Config stg.DescendingTrendlineConfig `json:"config"`
}
type dtMode struct {
	ID                 string `json:"id"`
	MA20, MA60, Volume bool
}
type dtAgg struct{ r map[string][]float64 }

var dtHorizons = []int{5, 10, 20, 40, 60}

type DescendingTrendlineResult struct {
	VariantID, Mode, Period            string
	Config                             stg.DescendingTrendlineConfig
	Horizon, N                         int
	Mean, Trimmed, Median, WinRate, PF float64
}
type DescendingTrendlineTop struct {
	VariantID, Mode                  string
	Config                           stg.DescendingTrendlineConfig
	ISN                              int
	ISMean                           float64
	OOSN                             int
	OOSMean, OOSMedian, OOSPF, Score float64
}

func dtVariants() []dtVariant {
	var v []dtVariant
	for _, s := range []float64{.03, .05, .08} {
		for _, d := range []float64{.03, .08} {
			for _, b := range []int{30, 60} {
				for _, w := range []int{20, 40} {
					c := stg.DefaultDescendingTrendlineConfig()
					c.SupportTolerance = s
					c.MinResistanceDrop = d
					c.MinPatternBars = b
					c.MaxBreakoutWait = w
					v = append(v, dtVariant{fmt.Sprintf("S%02.0f_D%02.0f_B%02d_W%02d", s*100, d*100, b, w), c})
				}
			}
		}
	}
	return v
}
func HandleDescendingTrendlineStudy(args []string) {
	maxStocks, nc := 0, 4200
	out, oos := "zpicture/descending_trendline_study.json", "20220101"
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
	vars := dtVariants()
	modes := []dtMode{{"STRUCTURE", false, false, false}, {"MA20", true, false, false}, {"MA20_MA60", true, true, false}, {"VOLUME", false, false, true}, {"MA20_VOLUME", true, false, true}}
	ag := make([]map[string]dtAgg, len(vars))
	for i := range ag {
		ag[i] = map[string]dtAgg{}
		for _, m := range modes {
			ag[i][m.ID] = dtAgg{map[string][]float64{}}
		}
	}
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	fmt.Printf("[descending_trendline] KR %d × %d구조 × %d소거\n", len(stocks), len(vars), len(modes))
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(sh string) {
			defer wg.Done()
			defer func() { <-sem }()
			c, e := box.FetchCandlesHannam(db, sh, nc)
			if e != nil || len(c) < 120 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(c)
			local := make([]map[string]dtAgg, len(vars))
			for vi, v := range vars {
				local[vi] = map[string]dtAgg{}
				for _, m := range modes {
					local[vi][m.ID] = dtAgg{map[string][]float64{}}
				}
				seen := map[string]bool{}
				for _, s := range stg.DescendingTrendlineAnalyze(c, v.Config) {
					if !volumeWavePricePathClean(c, s.Pattern.R1Pos, s.EntryPos) {
						continue
					}
					for _, m := range modes {
						cur := c[s.Pos]
						if m.MA20 && (cur.Ma20Origin <= 0 || cur.CloseOrigin <= cur.Ma20Origin) {
							continue
						}
						if m.MA60 && (cur.Ma60Origin <= 0 || cur.CloseOrigin <= cur.Ma60Origin) {
							continue
						}
						if m.Volume && (cur.VolMa20 <= 0 || cur.Volume < cur.VolMa20*1.5) {
							continue
						}
						k := m.ID + "|" + s.EntryDate
						if seen[k] {
							continue
						}
						seen[k] = true
						p := volumeWavePeriod(s.EntryDate, oos)
						a := local[vi][m.ID]
						for h, r := range dtEntryReturns(c, s.EntryPos, s.EntryPriceOrigin, .30) {
							a.r[volumeWaveReturnKey("ALL", h)] = append(a.r[volumeWaveReturnKey("ALL", h)], r)
							a.r[volumeWaveReturnKey(p, h)] = append(a.r[volumeWaveReturnKey(p, h)], r)
						}
						local[vi][m.ID] = a
					}
				}
			}
			mu.Lock()
			for vi := range vars {
				for _, m := range modes {
					a := ag[vi][m.ID]
					for k, x := range local[vi][m.ID].r {
						a.r[k] = append(a.r[k], x...)
					}
					ag[vi][m.ID] = a
				}
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[descending_trendline] %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	var res []DescendingTrendlineResult
	lookup := map[string]DescendingTrendlineResult{}
	for vi, v := range vars {
		for _, m := range modes {
			for _, p := range []string{"ALL", "IS", "OOS"} {
				for _, h := range dtHorizons {
					x := ag[vi][m.ID].r[volumeWaveReturnKey(p, h)]
					r := DescendingTrendlineResult{VariantID: v.ID, Mode: m.ID, Period: p, Config: v.Config, Horizon: h, N: len(x), Mean: meanFloats(x), Trimmed: trimmedMeanFloats(x, .05), Median: medianFloats(x)}
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
					lookup[fmt.Sprintf("%s|%s|%s|%d", v.ID, m.ID, p, h)] = r
				}
			}
		}
	}
	var top5, top20, top60 []DescendingTrendlineTop
	for _, v := range vars {
		for _, m := range modes {
			for _, pair := range []struct {
				h   int
				dst *[]DescendingTrendlineTop
			}{{5, &top5}, {20, &top20}, {60, &top60}} {
				is := lookup[fmt.Sprintf("%s|%s|IS|%d", v.ID, m.ID, pair.h)]
				oo := lookup[fmt.Sprintf("%s|%s|OOS|%d", v.ID, m.ID, pair.h)]
				if is.N >= 100 && oo.N >= 100 && is.Mean > 0 && oo.Mean > 0 {
					*pair.dst = append(*pair.dst, DescendingTrendlineTop{v.ID, m.ID, v.Config, is.N, is.Mean, oo.N, oo.Mean, oo.Median, oo.PF, minFloat(is.Mean, oo.Mean)})
				}
			}
		}
	}
	sortTop := func(x []DescendingTrendlineTop) {
		sort.Slice(x, func(i, j int) bool { return x[i].Score > x[j].Score })
	}
	sortTop(top5)
	sortTop(top20)
	sortTop(top60)
	if len(top5) > 20 {
		top5 = top5[:20]
	}
	if len(top20) > 20 {
		top20 = top20[:20]
	}
	if len(top60) > 20 {
		top60 = top60[:20]
	}
	payload := struct {
		GeneratedAt           string
		Cost                  float64
		Scanned, Failed       int64
		Variants              []dtVariant
		Modes                 []dtMode
		Results               []DescendingTrendlineResult
		TopH5, TopH20, TopH60 []DescendingTrendlineTop
	}{time.Now().Format(time.RFC3339), .30, scanned.Load(), failed.Load(), vars, modes, res, top5, top20, top60}
	b, _ := json.MarshalIndent(payload, "", "  ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err = os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[descending_trendline] 완료 scan=%d fail=%d robust h5=%d h20=%d h60=%d → %s\n", scanned.Load(), failed.Load(), len(top5), len(top20), len(top60), out)
	for i, x := range top5 {
		if i >= 5 {
			break
		}
		fmt.Printf("  #%d %s %s IS %d %+.3f / OOS %d %+.3f med=%+.3f PF=%.2f\n", i+1, x.VariantID, x.Mode, x.ISN, x.ISMean, x.OOSN, x.OOSMean, x.OOSMedian, x.OOSPF)
	}
}

func dtEntryReturns(c []*box.Candle, entryPos int, entryPrice, cost float64) map[int]float64 {
	out := map[int]float64{}
	for _, h := range dtHorizons {
		end := entryPos + h
		if end >= len(c) || !volumeWavePricePathClean(c, entryPos+1, end) {
			continue
		}
		if c[end].CloseOrigin > 0 {
			out[h] = (c[end].CloseOrigin-entryPrice)/entryPrice*100 - cost
		}
	}
	return out
}
