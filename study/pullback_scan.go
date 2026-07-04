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

// PullbackExample 은 20이평 눌림 돌파 매수 신호 1건 (+ forward return + 패턴 속성).
type PullbackExample struct {
	Market      string `json:"market"`
	Shcode      string `json:"shcode"`
	TriggerDate string `json:"trigger_date"` // 양봉 20이평 돌파 캔들 (매수)
	ArmDate     string `json:"arm_date"`     // support box 인지 캔들
	RDate       string `json:"r_date"`
	SDate       string `json:"s_date"`
	ArmToTrig   int    `json:"arm_to_trigger_bars"`
	// 사후 분석용 속성
	TouchCount   int     `json:"touch_count"`    // R±3봉 중 고가>MA20 봉 수
	DepthPct     float64 `json:"depth_pct"`      // 눌림 깊이 (MA20 대비 %)
	MA20SlopePct float64 `json:"ma20_slope_pct"` // R 시점 5봉 기울기 %
	RToSBars     int     `json:"r_to_s_bars"`
	// 시장국면 속성 (트리거 시점, 2026-07-04)
	DefBoxAboveMA20 bool    `json:"defbox_above_ma20"` // 국면a
	DefBoxExists    bool    `json:"defbox_exists"`
	DefBoxDistPct   float64 `json:"defbox_dist_pct"`
	BBExpanding     bool    `json:"bb_expanding"` // 국면b
	// forward return % (스케일 종가, 트리거 캔들 대비)
	R1  *float64 `json:"r1,omitempty"`
	R5  *float64 `json:"r5,omitempty"`
	R10 *float64 `json:"r10,omitempty"`
	R20 *float64 `json:"r20,omitempty"`
}

// HandlePullbackScan 은 전 종목에서 눌림 돌파 매수 신호를 수집하고 양의 엣지를 측정한다.
// 사용법: ./RESTGo stock pullback_scan [--foreign-jp|--foreign-cn|--foreign-hk] [--max N] [--candles N] [--out path]
// 기본: hannam 16년(4200봉), 출력 zpicture/pullback_scan.json
func HandlePullbackScan(args []string) {
	mode := "hannam"
	maxStocks := 0
	candleCount := 4200
	streak := 0 // MA20 연속 상승 요구 (기본 0 = 미적용, 2026-07-04 +++ 폐지. --streak 3으로 구버전 재현)
	outPath := "zpicture/pullback_scan.json"
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
		case "--streak":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &streak)
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
		fmt.Fprintf(os.Stderr, "[pullback_scan] DB 연결 오류: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "[pullback_scan] 종목 목록 오류: %v\n", err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}

	market := map[string]string{"hannam": "KR", "foreign-jp": "JP", "foreign-cn": "CN", "foreign-hk": "HK"}[mode]
	fmt.Printf("[pullback_scan] %s 모드 %d개 종목, 캔들 %d, streak=%d\n", market, len(stocks), candleCount, streak)

	fwdRet := func(candles []*box.Candle, pos, h int) *float64 {
		if pos+h >= len(candles) || candles[pos].Close <= 0 {
			return nil
		}
		r := (candles[pos+h].Close - candles[pos].Close) / candles[pos].Close * 100.0
		return &r
	}

	var mu sync.Mutex
	var examples []PullbackExample
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

			signals := stg.PullbackAnalyze(candles, streak)

			localBase := map[int][]float64{}
			for pos := 60; pos+20 < len(candles); pos += 21 {
				for _, h := range mtopHorizons {
					if r := fwdRet(candles, pos, h); r != nil {
						localBase[h] = append(localBase[h], *r)
					}
				}
			}

			var localEx []PullbackExample
			for _, s := range signals {
				localEx = append(localEx, PullbackExample{
					Market: market, Shcode: shcode,
					TriggerDate: s.Date, ArmDate: s.ArmDate,
					RDate:        candles[s.Pattern.RPos].Date,
					SDate:        candles[s.Pattern.SPos].Date,
					ArmToTrig:    s.Pos - s.ArmPos,
					TouchCount:   s.Pattern.TouchCount,
					DepthPct:     s.Pattern.DepthPct,
					MA20SlopePct: s.Pattern.MA20SlopePct,
					RToSBars:     s.Pattern.RToSBars,
					DefBoxAboveMA20: s.DefBoxAboveMA20,
					DefBoxExists:    s.DefBoxExists,
					DefBoxDistPct:   s.DefBoxDistPct,
					BBExpanding:     s.BBExpanding,
					R1:           fwdRet(candles, s.Pos, 1),
					R5:           fwdRet(candles, s.Pos, 5),
					R10:          fwdRet(candles, s.Pos, 10),
					R20:          fwdRet(candles, s.Pos, 20),
				})
			}

			mu.Lock()
			examples = append(examples, localEx...)
			for h, rs := range localBase {
				baseline[h] = append(baseline[h], rs...)
			}
			mu.Unlock()

			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[pullback_scan] 진행 %d/%d (신호 %d)\n", n, len(stocks), len(examples))
			}
		}(code)
	}
	wg.Wait()

	sort.Slice(examples, func(i, j int) bool {
		if examples[i].Shcode != examples[j].Shcode {
			return examples[i].Shcode < examples[j].Shcode
		}
		return examples[i].TriggerDate < examples[j].TriggerDate
	})

	type hStat struct {
		Horizon      int     `json:"horizon"`
		N            int     `json:"n"`
		Mean         float64 `json:"mean_pct"`
		Median       float64 `json:"median_pct"`
		WinRate      float64 `json:"win_rate_pct"` // 매수 신호: 상승 확률
		BaselineMean float64 `json:"baseline_mean_pct"`
		Edge         float64 `json:"edge_pct"` // 양수일수록 매수 엣지
		TStat        float64 `json:"t_stat"`
		BaselineN    int     `json:"baseline_n"`
	}
	pick := func(ex PullbackExample, h int) *float64 {
		switch h {
		case 1:
			return ex.R1
		case 5:
			return ex.R5
		case 10:
			return ex.R10
		default:
			return ex.R20
		}
	}
	var stats []hStat
	for _, h := range mtopHorizons {
		var rs []float64
		for _, ex := range examples {
			if p := pick(ex, h); p != nil {
				rs = append(rs, *p)
			}
		}
		if len(rs) == 0 {
			continue
		}
		sorted := append([]float64(nil), rs...)
		sort.Float64s(sorted)
		up := 0
		for _, r := range rs {
			if r > 0 {
				up++
			}
		}
		bm := meanFloats(baseline[h])
		stats = append(stats, hStat{
			Horizon: h, N: len(rs), Mean: meanFloats(rs), Median: sorted[len(sorted)/2],
			WinRate: 100 * float64(up) / float64(len(rs)), BaselineMean: bm,
			Edge: meanFloats(rs) - bm, TStat: welchTStatFloats(rs, baseline[h]), BaselineN: len(baseline[h]),
		})
	}

	out := struct {
		Market      string            `json:"market"`
		CandleCount int               `json:"candle_count"`
		Streak      int               `json:"streak"`
		StockCount  int               `json:"stock_count"`
		SignalCount int               `json:"signal_count"`
		Stats       []hStat           `json:"stats"`
		Examples    []PullbackExample `json:"examples"`
	}{market, candleCount, streak, len(stocks), len(examples), stats, examples}

	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[pullback_scan] 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("[pullback_scan] 완료: 종목 %d(실패 %d)  신호 %d  → %s\n",
		scanned.Load(), failed.Load(), len(examples), outPath)
	for _, s := range stats {
		fmt.Printf("[pullback_scan] h%-2d n=%d  mean %+.3f%%  median %+.3f%%  승률 %.1f%%  baseline %+.3f%%  edge %+.3f%%p  t=%.2f\n",
			s.Horizon, s.N, s.Mean, s.Median, s.WinRate, s.BaselineMean, s.Edge, s.TStat)
	}
}
