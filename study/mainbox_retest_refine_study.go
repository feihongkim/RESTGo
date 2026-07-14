package study

// mainbox_retest_refine_study.go — 상승폭 5단계 × retest 3형 × C0/C1/C2 및 strategy1 원 신호 분해.
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

type mainBoxRefineVariant struct {
	ID     string                  `json:"id"`
	Config stg.MainBoxRetestConfig `json:"config"`
}
type MainBoxRefineResult struct {
	VariantID, Group, OriginalStrategy, OriginalSignal, Period string
	Horizon, N                                                 int
	Mean, Trimmed, Median, WinRate, PF                         float64
}

func buildMainBoxRefineVariants() []mainBoxRefineVariant {
	var v []mainBoxRefineVariant
	for _, r := range []float64{.10, .125, .15, .175, .20} {
		for _, m := range []string{stg.MainBoxRetestModeTouch, stg.MainBoxRetestModeUndercutReclaim, stg.MainBoxRetestModeLongWickReclaim} {
			c := stg.DefaultMainBoxRetestConfig()
			c.MinRunupPct = r
			c.TouchTolerance = .04
			c.MaxIntradayUndercut = .03
			c.MaxRetestBars = 80
			c.RetestMode = m
			v = append(v, mainBoxRefineVariant{fmt.Sprintf("R%04.1f_%s", r*100, m), c})
		}
	}
	return v
}
func HandleMainBoxRetestRefineStudy(args []string) {
	maxStocks, nc := 0, 4200
	out, oos := "zpicture/mainbox_retest_refine_study.json", "20220101"
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
	vars := buildMainBoxRefineVariants()
	groups := []string{stg.MainBoxRetestGroupC0, stg.MainBoxRetestGroupC, stg.MainBoxRetestGroupC2}
	agg := make([]map[string]map[string][]float64, len(vars))
	for i := range agg {
		agg[i] = map[string]map[string][]float64{}
	}
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	fmt.Printf("[mainbox_refine] KR %d × %d변형 × C0/C1/C2\n", len(stocks), len(vars))
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
			local := make([]map[string]map[string][]float64, len(vars))
			for vi, v := range vars {
				local[vi] = map[string]map[string][]float64{}
				seen := map[string]bool{}
				for _, s := range stg.MainBoxRetestAnalyzeWithRules(c, v.Config, rules, set) {
					if s.Group != groups[0] && s.Group != groups[1] && s.Group != groups[2] {
						continue
					}
					key := s.Group + "|" + s.EntryDate
					if seen[key] || !volumeWavePricePathClean(c, s.BreakoutPos, s.EntryPos) {
						continue
					}
					seen[key] = true
					p := volumeWavePeriod(s.EntryDate, oos)
					for h, r := range volumeWaveEntryReturnsClean(c, s.EntryPos, s.EntryPriceOrigin, .30) {
						for _, sig := range []string{"ALL", s.OriginalStrategy + "::" + s.OriginalSignal} {
							k := sig + "|" + s.Group
							if local[vi][k] == nil {
								local[vi][k] = map[string][]float64{}
							}
							local[vi][k][volumeWaveReturnKey("ALL", h)] = append(local[vi][k][volumeWaveReturnKey("ALL", h)], r)
							local[vi][k][volumeWaveReturnKey(p, h)] = append(local[vi][k][volumeWaveReturnKey(p, h)], r)
						}
					}
				}
			}
			mu.Lock()
			for vi := range vars {
				for k, m := range local[vi] {
					if agg[vi][k] == nil {
						agg[vi][k] = map[string][]float64{}
					}
					for rk, x := range m {
						agg[vi][k][rk] = append(agg[vi][k][rk], x...)
					}
				}
			}
			mu.Unlock()
			if x := scanned.Add(1); x%500 == 0 {
				fmt.Printf("[mainbox_refine] %d/%d\n", x, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	var res []MainBoxRefineResult
	for vi, v := range vars {
		for k, m := range agg[vi] {
			parts := splitOnce(k, "|")
			sigKey, g := parts[0], parts[1]
			strategy, sig := "ALL", "ALL"
			if sigKey != "ALL" {
				sg := splitOnce(sigKey, "::")
				strategy, sig = sg[0], sg[1]
			}
			for _, p := range []string{"ALL", "IS", "OOS"} {
				for _, h := range volumeWaveMatrixHorizons {
					x := m[volumeWaveReturnKey(p, h)]
					r := MainBoxRefineResult{VariantID: v.ID, Group: g, OriginalStrategy: strategy, OriginalSignal: sig, Period: p, Horizon: h, N: len(x), Mean: meanFloats(x), Trimmed: trimmedMeanFloats(x, .05), Median: medianFloats(x)}
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
	sort.Slice(res, func(i, j int) bool {
		if res[i].Period == res[j].Period && res[i].Horizon == res[j].Horizon {
			return res[i].Mean > res[j].Mean
		}
		return res[i].VariantID < res[j].VariantID
	})
	payload := struct {
		GeneratedAt     string
		Scanned, Failed int64
		Cost            float64
		Variants        []mainBoxRefineVariant
		Results         []MainBoxRefineResult
	}{time.Now().Format(time.RFC3339), scanned.Load(), failed.Load(), .30, vars, res}
	b, _ := json.MarshalIndent(payload, "", "  ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err = os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[mainbox_refine] 완료 scan=%d fail=%d → %s\n", scanned.Load(), failed.Load(), out)
	for _, r := range res {
		if r.OriginalSignal == "ALL" && r.Period == "OOS" && r.Horizon == 5 && r.N >= 30 {
			fmt.Printf("  %s %-20s n=%d mean=%+.3f med=%+.3f PF=%.2f\n", r.VariantID, r.Group, r.N, r.Mean, r.Median, r.PF)
		}
	}
}
func splitOnce(s, sep string) [2]string {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return [2]string{s[:i], s[i+len(sep):]}
		}
	}
	return [2]string{s, ""}
}
