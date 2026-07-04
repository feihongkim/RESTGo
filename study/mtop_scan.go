package study

import (
	"RESTGo/box"
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

// MTopExample 은 상방 M자 매도 신호 1건 (+ 트리거 캔들 기준 forward return).
type MTopExample struct {
	Market      string `json:"market"`
	Shcode      string `json:"shcode"`
	TriggerDate string `json:"trigger_date"` // 음양음 붕괴 캔들 (매도 신호)
	ArmDate     string `json:"arm_date"`     // M 완성 캔들
	P1Date      string `json:"p1_date"`
	P2Date      string `json:"p2_date"`
	ArmToTrig   int    `json:"arm_to_trigger_bars"`
	// forward return % (스케일 종가 기준, 트리거 캔들 종가 대비)
	R1  *float64 `json:"r1,omitempty"`
	R5  *float64 `json:"r5,omitempty"`
	R10 *float64 `json:"r10,omitempty"`
	R20 *float64 `json:"r20,omitempty"`
}

var mtopHorizons = []int{1, 5, 10, 20}

// HandleMTopScan 은 전 종목에서 상방 M자 매도 신호를 수집하고 음의 엣지를 측정한다.
// 사용법: ./RESTGo stock mtop_scan [--foreign-jp|--foreign-cn|--foreign-hk] [--max N] [--candles N] [--out path]
// 기본: hannam 16년(4200봉), 출력 zpicture/mtop_scan.json
// baseline은 전 종목 21봉 간격 표본의 forward return (시장 평균 하락/상승 대비 신호의 초과 하락 측정).
func HandleMTopScan(args []string) {
	mode := "hannam"
	maxStocks := 0
	candleCount := 4200
	outPath := "zpicture/mtop_scan.json"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--foreign-jp":
			mode = "foreign-jp"
		case "--foreign-cn":
			mode = "foreign-cn"
		case "--foreign-hk":
			mode = "foreign-hk"
		case "--hannam":
			mode = "hannam"
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

	db, err := console.MsConn.GetDB("KIS2")
	if mode == "hannam" {
		db, err = console.MsConn.GetDB("han")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mtop_scan] DB 연결 오류: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "[mtop_scan] 종목 목록 오류: %v\n", err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}

	market := map[string]string{"hannam": "KR", "foreign-jp": "JP", "foreign-cn": "CN", "foreign-hk": "HK"}[mode]
	fmt.Printf("[mtop_scan] %s 모드 %d개 종목, 캔들 %d\n", market, len(stocks), candleCount)

	fwdRet := func(candles []*box.Candle, pos, h int) *float64 {
		if pos+h >= len(candles) || candles[pos].Close <= 0 {
			return nil
		}
		r := (candles[pos+h].Close - candles[pos].Close) / candles[pos].Close * 100.0
		return &r
	}

	var mu sync.Mutex
	var examples []MTopExample
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
			if fetchErr != nil || len(candles) < 60 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)

			signals := stg.MPatternAnalyze(candles)

			// baseline: 21봉 간격 표본 forward return
			localBase := map[int][]float64{}
			for pos := 60; pos+20 < len(candles); pos += 21 {
				for _, h := range mtopHorizons {
					if r := fwdRet(candles, pos, h); r != nil {
						localBase[h] = append(localBase[h], *r)
					}
				}
			}

			var localEx []MTopExample
			for _, s := range signals {
				ex := MTopExample{
					Market: market, Shcode: shcode,
					TriggerDate: s.Date, ArmDate: s.ArmDate,
					P1Date: s.P1Date, P2Date: s.P2Date,
					ArmToTrig: s.Pos - s.ArmPos,
					R1:        fwdRet(candles, s.Pos, 1),
					R5:        fwdRet(candles, s.Pos, 5),
					R10:       fwdRet(candles, s.Pos, 10),
					R20:       fwdRet(candles, s.Pos, 20),
				}
				localEx = append(localEx, ex)
			}

			mu.Lock()
			examples = append(examples, localEx...)
			for h, rs := range localBase {
				baseline[h] = append(baseline[h], rs...)
			}
			mu.Unlock()

			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[mtop_scan] 진행 %d/%d (신호 %d)\n", n, len(stocks), len(examples))
			}
		}(code)
	}
	wg.Wait()

	// 결정성: 종목·날짜순 정렬
	sort.Slice(examples, func(i, j int) bool {
		if examples[i].Shcode != examples[j].Shcode {
			return examples[i].Shcode < examples[j].Shcode
		}
		return examples[i].TriggerDate < examples[j].TriggerDate
	})

	// 통계 (매도 신호이므로 "성공" = 전방 수익률 음수)
	type hStat struct {
		Horizon      int     `json:"horizon"`
		N            int     `json:"n"`
		Mean         float64 `json:"mean_pct"`
		Median       float64 `json:"median_pct"`
		DownRate     float64 `json:"down_rate_pct"` // 하락 확률 (매도 신호의 적중률)
		BaselineMean float64 `json:"baseline_mean_pct"`
		Edge         float64 `json:"edge_pct"` // mean - baseline (음수일수록 매도 엣지)
		TStat        float64 `json:"t_stat"`
		BaselineN    int     `json:"baseline_n"`
	}
	var stats []hStat
	for _, h := range mtopHorizons {
		var rs []float64
		for _, ex := range examples {
			var p *float64
			switch h {
			case 1:
				p = ex.R1
			case 5:
				p = ex.R5
			case 10:
				p = ex.R10
			case 20:
				p = ex.R20
			}
			if p != nil {
				rs = append(rs, *p)
			}
		}
		if len(rs) == 0 {
			continue
		}
		sorted := append([]float64(nil), rs...)
		sort.Float64s(sorted)
		down := 0
		for _, r := range rs {
			if r < 0 {
				down++
			}
		}
		bm := meanFloats(baseline[h])
		stats = append(stats, hStat{
			Horizon: h, N: len(rs),
			Mean:         meanFloats(rs),
			Median:       sorted[len(sorted)/2],
			DownRate:     100 * float64(down) / float64(len(rs)),
			BaselineMean: bm,
			Edge:         meanFloats(rs) - bm,
			TStat:        welchTStatFloats(rs, baseline[h]),
			BaselineN:    len(baseline[h]),
		})
	}

	out := struct {
		Market      string        `json:"market"`
		CandleCount int           `json:"candle_count"`
		StockCount  int           `json:"stock_count"`
		SignalCount int           `json:"signal_count"`
		Stats       []hStat       `json:"stats"`
		Examples    []MTopExample `json:"examples"`
	}{market, candleCount, len(stocks), len(examples), stats, examples}

	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[mtop_scan] 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("[mtop_scan] 완료: 종목 %d(실패 %d)  신호 %d  → %s\n",
		scanned.Load(), failed.Load(), len(examples), outPath)
	for _, s := range stats {
		fmt.Printf("[mtop_scan] h%-2d n=%d  mean %+.3f%%  median %+.3f%%  하락률 %.1f%%  baseline %+.3f%%  edge %+.3f%%p  t=%.2f\n",
			s.Horizon, s.N, s.Mean, s.Median, s.DownRate, s.BaselineMean, s.Edge, s.TStat)
	}
}
