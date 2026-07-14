package study

// mainbox_retest_temporal.go — 고정 20%+Touch C0/C1의 시간 안정성 kill test. 재최적화 금지.
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

type mainBoxTemporalTrade struct {
	Group, Shcode, EntryDate string
	Returns                  map[int]float64
}
type MainBoxTemporalStat struct {
	Group, Slice                       string
	Horizon, N                         int
	Mean, Median, WinRate, PF, Trimmed float64
}
type MainBoxTemporalDecision struct {
	Group                                                           string
	PositiveEras, TotalEras                                         int
	H5Mean, H5Median, H5PF, MaxStockProfitShare, MaxYearProfitShare float64
	Pass                                                            bool
	Reasons                                                         []string
}

func temporalSlice(d string) string {
	switch {
	case d < "20140101":
		return "2009-2013"
	case d < "20180101":
		return "2014-2017"
	case d < "20220101":
		return "2018-2021"
	default:
		return "2022-2026"
	}
}
func HandleMainBoxRetestTemporal(args []string) {
	maxStocks, nc := 0, 4200
	out := "zpicture/mainbox_retest_temporal.json"
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
	rules, set, err := stg.LoadRulesWithSettings("rules/strategy1.yaml")
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
	cfg := stg.DefaultMainBoxRetestConfig()
	cfg.MinRunupPct = .20
	cfg.TouchTolerance = .04
	cfg.MaxIntradayUndercut = .03
	cfg.MaxRetestBars = 80
	cfg.RetestMode = stg.MainBoxRetestModeTouch
	groups := map[string]bool{stg.MainBoxRetestGroupC0: true, stg.MainBoxRetestGroupC: true}
	var trades []mainBoxTemporalTrade
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	fmt.Printf("[mainbox_temporal] KR %d fixed 20%% Touch C0/C1\n", len(stocks))
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
			seen := map[string]bool{}
			var local []mainBoxTemporalTrade
			for _, s := range stg.MainBoxRetestAnalyzeWithRules(c, cfg, rules, set) {
				if !groups[s.Group] {
					continue
				}
				k := s.Group + "|" + s.EntryDate
				if seen[k] || !volumeWavePricePathClean(c, s.BreakoutPos, s.EntryPos) {
					continue
				}
				seen[k] = true
				rs := volumeWaveEntryReturnsClean(c, s.EntryPos, s.EntryPriceOrigin, .30)
				if len(rs) > 0 {
					local = append(local, mainBoxTemporalTrade{s.Group, sh, s.EntryDate, rs})
				}
			}
			mu.Lock()
			trades = append(trades, local...)
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[mainbox_temporal] %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	slices := []string{"ALL", "2009-2013", "2014-2017", "2018-2021", "2022-2026"}
	var stats []MainBoxTemporalStat
	for g := range groups {
		for _, sl := range slices {
			for _, h := range volumeWaveMatrixHorizons {
				var x []float64
				for _, t := range trades {
					if t.Group != g || (sl != "ALL" && temporalSlice(t.EntryDate) != sl) {
						continue
					}
					if r, ok := t.Returns[h]; ok {
						x = append(x, r)
					}
				}
				s := MainBoxTemporalStat{Group: g, Slice: sl, Horizon: h, N: len(x), Mean: meanFloats(x), Median: medianFloats(x), Trimmed: trimmedMeanFloats(x, .05)}
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
					s.WinRate = 100 * float64(wins) / float64(len(x))
				}
				if ls > 0 {
					s.PF = pr / ls
				}
				stats = append(stats, s)
			}
		}
	}
	lookup := map[string]MainBoxTemporalStat{}
	for _, s := range stats {
		lookup[fmt.Sprintf("%s|%s|%d", s.Group, s.Slice, s.Horizon)] = s
	}
	var decisions []MainBoxTemporalDecision
	for g := range groups {
		d := MainBoxTemporalDecision{Group: g, TotalEras: 4}
		for _, sl := range slices[1:] {
			if lookup[fmt.Sprintf("%s|%s|5", g, sl)].Mean > 0 {
				d.PositiveEras++
			}
		}
		all := lookup[fmt.Sprintf("%s|ALL|5", g)]
		d.H5Mean, d.H5Median, d.H5PF = all.Mean, all.Median, all.PF
		d.MaxStockProfitShare, d.MaxYearProfitShare = temporalProfitConcentration(trades, g)
		if d.PositiveEras < 3 {
			d.Reasons = append(d.Reasons, "positive eras < 3/4")
		}
		if d.H5Mean < .30 {
			d.Reasons = append(d.Reasons, "h5 mean < 0.30%")
		}
		if d.H5PF < 1.15 {
			d.Reasons = append(d.Reasons, "PF < 1.15")
		}
		if d.H5Median < 0 {
			d.Reasons = append(d.Reasons, "median < 0")
		}
		if d.MaxStockProfitShare > .25 {
			d.Reasons = append(d.Reasons, "stock concentration > 25%")
		}
		if d.MaxYearProfitShare > .25 {
			d.Reasons = append(d.Reasons, "year concentration > 25%")
		}
		d.Pass = len(d.Reasons) == 0
		decisions = append(decisions, d)
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].Group < decisions[j].Group })
	payload := struct {
		GeneratedAt             string
		Config                  stg.MainBoxRetestConfig
		Cost                    float64
		Scanned, Failed, Trades int64
		Stats                   []MainBoxTemporalStat
		Decisions               []MainBoxTemporalDecision
	}{time.Now().Format(time.RFC3339), cfg, .30, scanned.Load(), failed.Load(), int64(len(trades)), stats, decisions}
	b, _ := json.MarshalIndent(payload, "", "  ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err = os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[mainbox_temporal] 완료 trades=%d → %s\n", len(trades), out)
	for _, d := range decisions {
		fmt.Printf("  %s pass=%v eras=%d/4 h5=%+.3f med=%+.3f PF=%.2f stock=%.1f%% year=%.1f%% reasons=%v\n", d.Group, d.Pass, d.PositiveEras, d.H5Mean, d.H5Median, d.H5PF, d.MaxStockProfitShare*100, d.MaxYearProfitShare*100, d.Reasons)
	}
}
func temporalProfitConcentration(t []mainBoxTemporalTrade, g string) (float64, float64) {
	byS, byY := map[string]float64{}, map[string]float64{}
	total := 0.0
	for _, x := range t {
		r, ok := x.Returns[5]
		if x.Group != g || !ok || r <= 0 {
			continue
		}
		total += r
		byS[x.Shcode] += r
		y := x.EntryDate
		if len(y) > 4 {
			y = y[:4]
		}
		byY[y] += r
	}
	if total <= 0 {
		return 0, 0
	}
	max := func(m map[string]float64) float64 {
		z := 0.0
		for _, v := range m {
			if v > z {
				z = v
			}
		}
		return z / total
	}
	return max(byS), max(byY)
}
