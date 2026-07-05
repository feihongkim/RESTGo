package study

import (
	"RESTGo/box"
	"RESTGo/cond"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

// WGCExample 은 W바텀(완화판) 신호 1건 — 2×2(bb_crash × gc_pending) 분해용.
type WGCExample struct {
	Market     string `json:"market"`
	Shcode     string `json:"shcode"`
	SignalDate string `json:"signal_date"`
	P1Date     string `json:"p1_date"`
	P2Date     string `json:"p2_date"`
	BBCrash    bool   `json:"bb_crash"`
	GCPending  bool   `json:"gc_pending"`
	GCInvert   bool   `json:"gc_invert"`
	GCGapPct   float64 `json:"gc_gap_pct"`
	GCShrink   int     `json:"gc_shrink"`
	HasDefBox  bool    `json:"has_defbox"`
	R1  *float64 `json:"r1,omitempty"`
	R5  *float64 `json:"r5,omitempty"`
	R10 *float64 `json:"r10,omitempty"`
	R20 *float64 `json:"r20,omitempty"`
}

// S1GCExample 은 strategy1 기존 신호에 GC 속성을 후계산으로 붙인 것.
type S1GCExample struct {
	Shcode    string  `json:"shcode"`
	Strategy  string  `json:"strategy"`
	BuyDate   string  `json:"buy_date"`
	GCPending bool    `json:"gc_pending"`
	GCInvert  bool    `json:"gc_invert"`
	GCGapPct  float64 `json:"gc_gap_pct"`
	GCShrink  int     `json:"gc_shrink"`
	R5  *float64 `json:"r5,omitempty"`
	R20 *float64 `json:"r20,omitempty"`
}

// HandleWGCScan 은 ① W바텀 완화판 스캔(BB급락·GC 속성 기록) ② strategy1 기존 신호에 GC 후계산을
// 한 번의 캔들 패스로 수행한다 (2026-07-05 GC-pending 국면 연구).
// 사용법: ./RESTGo stock wgc_scan [--max N] [--candles N] [--out path] [--s1-json path]
// 기본: hannam 4200봉, 출력 zpicture/wgc_scan.json, s1 소스 zpicture/strategy1_density_study.json
func HandleWGCScan(args []string) {
	maxStocks := 0
	candleCount := 4200
	outPath := "zpicture/wgc_scan.json"
	s1Path := "zpicture/strategy1_density_study.json"
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
		case "--s1-json":
			if i+1 < len(args) {
				s1Path = args[i+1]
				i++
			}
		}
	}

	// strategy1 신호 로드 (고유 포지션: strategy, shcode, buy_date)
	s1Sig := map[string][][2]string{} // shcode -> [(strategy, buy_date)]
	if raw, err := os.ReadFile(s1Path); err == nil {
		var doc struct {
			SellTrades []struct {
				Strategy string `json:"strategy"`
				Shcode   string `json:"shcode"`
				BuyDate  string `json:"buy_date"`
			} `json:"sell_trades"`
		}
		if json.Unmarshal(raw, &doc) == nil {
			seen := map[[3]string]bool{}
			for _, r := range doc.SellTrades {
				k := [3]string{r.Strategy, r.Shcode, r.BuyDate}
				if seen[k] {
					continue
				}
				seen[k] = true
				s1Sig[r.Shcode] = append(s1Sig[r.Shcode], [2]string{r.Strategy, r.BuyDate})
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "[wgc_scan] 경고: s1 JSON 없음(%s) — strategy1 후계산 생략\n", s1Path)
	}
	s1Total := 0
	for _, v := range s1Sig {
		s1Total += len(v)
	}

	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wgc_scan] DB 연결 오류: %v\n", err)
		return
	}
	stocks, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wgc_scan] 종목 목록 오류: %v\n", err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}
	fmt.Printf("[wgc_scan] %d개 종목, 캔들 %d, s1 신호 %d건 로드\n", len(stocks), candleCount, s1Total)

	fwdRet := func(candles []*box.Candle, pos, h int) *float64 {
		if pos+h >= len(candles) || candles[pos].Close <= 0 {
			return nil
		}
		r := (candles[pos+h].Close - candles[pos].Close) / candles[pos].Close * 100.0
		return &r
	}

	var mu sync.Mutex
	var wEx []WGCExample
	var s1Ex []S1GCExample
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

			candles, fetchErr := box.FetchCandlesHannam(db, shcode, candleCount)
			if fetchErr != nil || len(candles) < 130 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)

			signals := stg.WGCAnalyze(candles)

			localBase := map[int][]float64{}
			for pos := 130; pos+20 < len(candles); pos += 21 {
				for _, h := range mtopHorizons {
					if r := fwdRet(candles, pos, h); r != nil {
						localBase[h] = append(localBase[h], *r)
					}
				}
			}

			var localW []WGCExample
			for _, s := range signals {
				localW = append(localW, WGCExample{
					Market: "KR", Shcode: shcode,
					SignalDate: s.Date, P1Date: s.P1Date, P2Date: s.P2Date,
					BBCrash: s.BBCrash, GCPending: s.GCPending, GCInvert: s.GCInvert,
					GCGapPct: s.GCGapPct, GCShrink: s.GCShrink, HasDefBox: s.HasDefBox,
					R1:  fwdRet(candles, s.Pos, 1),
					R5:  fwdRet(candles, s.Pos, 5),
					R10: fwdRet(candles, s.Pos, 10),
					R20: fwdRet(candles, s.Pos, 20),
				})
			}

			// strategy1 신호 GC 후계산 (buy_date → 캔들 인덱스)
			var localS1 []S1GCExample
			if list := s1Sig[shcode]; len(list) > 0 {
				idx := map[string]int{}
				for i, c := range candles {
					idx[c.Date] = i
				}
				for _, sd := range list {
					pos, ok := idx[sd[1]]
					if !ok {
						continue
					}
					pending, inv, gapPct, shrink := cond.GoldenCrossPendingInfo(candles, pos)
					localS1 = append(localS1, S1GCExample{
						Shcode: shcode, Strategy: sd[0], BuyDate: sd[1],
						GCPending: pending, GCInvert: inv, GCGapPct: gapPct, GCShrink: shrink,
						R5:  fwdRet(candles, pos, 5),
						R20: fwdRet(candles, pos, 20),
					})
				}
			}

			mu.Lock()
			wEx = append(wEx, localW...)
			s1Ex = append(s1Ex, localS1...)
			for h, rs := range localBase {
				baseline[h] = append(baseline[h], rs...)
			}
			mu.Unlock()

			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[wgc_scan] 진행 %d/%d (W %d, S1 %d)\n", n, len(stocks), len(wEx), len(s1Ex))
			}
		}(code)
	}
	wg.Wait()

	sort.Slice(wEx, func(i, j int) bool {
		if wEx[i].Shcode != wEx[j].Shcode {
			return wEx[i].Shcode < wEx[j].Shcode
		}
		return wEx[i].SignalDate < wEx[j].SignalDate
	})
	sort.Slice(s1Ex, func(i, j int) bool {
		if s1Ex[i].Shcode != s1Ex[j].Shcode {
			return s1Ex[i].Shcode < s1Ex[j].Shcode
		}
		return s1Ex[i].BuyDate < s1Ex[j].BuyDate
	})

	// 요약 출력: W 2×2 h20 평균
	bmean := map[int]float64{}
	for h, rs := range baseline {
		bmean[h] = meanFloats(rs)
	}
	cell := func(bb, gc bool) (int, float64) {
		var rs []float64
		for _, e := range wEx {
			if e.BBCrash == bb && e.GCPending == gc && e.R20 != nil {
				rs = append(rs, *e.R20)
			}
		}
		if len(rs) == 0 {
			return 0, 0
		}
		return len(rs), meanFloats(rs)
	}
	fmt.Printf("[wgc_scan] 완료: 종목 %d(실패 %d)  W신호 %d  S1대응 %d/%d  baseline h20 %+.3f%%\n",
		scanned.Load(), failed.Load(), len(wEx), len(s1Ex), s1Total, bmean[20])
	for _, c := range []struct {
		bb, gc bool
		label  string
	}{{true, false, "BB급락·GC아님(기존 W중력형)"}, {true, true, "BB급락·GC임박"},
		{false, false, "BB완화·GC아님"}, {false, true, "BB완화·GC임박(신규 가설)"}} {
		n, m := cell(c.bb, c.gc)
		fmt.Printf("[wgc_scan]   %-28s n=%-6d h20 mean %+.3f%%\n", c.label, n, m)
	}

	out := struct {
		CandleCount int                `json:"candle_count"`
		StockCount  int                `json:"stock_count"`
		WCount      int                `json:"w_signal_count"`
		S1Count     int                `json:"s1_signal_count"`
		BaselineMean map[int]float64   `json:"baseline_mean_by_horizon"`
		BaselineN   map[int]int        `json:"baseline_n_by_horizon"`
		WExamples   []WGCExample       `json:"w_examples"`
		S1Examples  []S1GCExample      `json:"s1_examples"`
	}{candleCount, len(stocks), len(wEx), len(s1Ex),
		bmean, func() map[int]int {
			m := map[int]int{}
			for h, rs := range baseline {
				m[h] = len(rs)
			}
			return m
		}(), wEx, s1Ex}

	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[wgc_scan] 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[wgc_scan] 저장: %s\n", outPath)
}
