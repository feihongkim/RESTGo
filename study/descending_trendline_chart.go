package study

// descending_trendline_chart.go — 하락추세선 돌파 OOS P90/P75/P50/P25/P10 샘플 차트 JSON 생성.
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
)

type dtChartCandle struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	MA20   float64 `json:"ma20"`
	MA60   float64 `json:"ma60"`
}

type dtChartSample struct {
	Label         string          `json:"label"`
	Percentile    string          `json:"percentile"`
	Shcode        string          `json:"shcode"`
	R1Date        string          `json:"r1_date"`
	S1Date        string          `json:"s1_date"`
	R2Date        string          `json:"r2_date"`
	S2Date        string          `json:"s2_date"`
	R1Price       float64         `json:"r1_price"`
	S1Price       float64         `json:"s1_price"`
	R2Price       float64         `json:"r2_price"`
	S2Price       float64         `json:"s2_price"`
	FloorPrice    float64         `json:"floor_price"`
	Slope         float64         `json:"slope"`
	PatternBars   int             `json:"pattern_bars"`
	BreakoutDate  string          `json:"breakout_date"`
	BreakoutPos   int             `json:"breakout_pos"`
	BreakoutPrice float64         `json:"breakout_price"`
	TrendlineDate float64          `json:"trendline_price"`
	EntryDate     string          `json:"entry_date"`
	EntryPos      int             `json:"entry_pos"`
	EntryPrice    float64         `json:"entry_price"`
	ExitDate      string          `json:"exit_date"`
	ExitPos       int             `json:"exit_pos"`
	ExitPrice     float64         `json:"exit_price"`
	NetReturnPct  float64         `json:"net_return_pct"`
	Candles       []dtChartCandle `json:"candles"`
	PivotR1Idx    int             `json:"pivot_r1_idx"`
	PivotS1Idx    int             `json:"pivot_s1_idx"`
	PivotR2Idx    int             `json:"pivot_r2_idx"`
	PivotS2Idx    int             `json:"pivot_s2_idx"`
	BreakoutIdx   int             `json:"breakout_idx"`
	EntryIdx      int             `json:"entry_idx"`
	ExitIdx       int             `json:"exit_idx"`
}

type dtChartPayload struct {
	VariantID string           `json:"variant_id"`
	Config    json.RawMessage   `json:"config"`
	Samples   []dtChartSample  `json:"samples"`
}

// dtTrade 는 수익률 정렬을 위한 임시 구조체
type dtTrade struct {
	Shcode       string
	Signal       stg.DescendingTrendlineSignal
	NetReturnPct float64
}

func HandleDescendingTrendlineChartSamples(args []string) {
	maxStocks, nc := 0, 4200
	out := "zpicture/descending_trendline_chart_samples.json"
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

	// 고정 설정: S08_D03_B60_W20 STRUCTURE
	cfg := stg.DescendingTrendlineConfig{
		SupportTolerance:   .08,
		MinResistanceDrop:  .03,
		MinPatternBars:     60,
		MaxPatternBars:     180,
		MaxBreakoutWait:    20,
		BreakoutBuffer:     0,
		MaxFloorBreakdown:  .05,
		RequireMA20Recovery: false,
		RequireMA60Recovery: false,
		RequireVolume:      false,
		BreakoutVolumeRatio: 1.5,
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

	oosCutoff := "20220101"
	var mu sync.Mutex
	var allTrades []dtTrade
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	fmt.Printf("[dt_chart] KR %d종목 → OOS(>=%s) h20 샘플링\n", len(stocks), oosCutoff)
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
			sigs := stg.DescendingTrendlineAnalyze(c, cfg)
			for _, sig := range sigs {
				if sig.EntryDate < oosCutoff {
					continue
				}
				// 기업행위 필터: R1→entry
				if !volumeWavePricePathClean(c, sig.Pattern.R1Pos, sig.EntryPos) {
					continue
				}
				// 기업행위 필터: entry→entry+20
				exitPos := sig.EntryPos + 20
				if exitPos >= len(c) || !volumeWavePricePathClean(c, sig.EntryPos+1, exitPos) {
					continue
				}
				if c[exitPos].CloseOrigin <= 0 {
					continue
				}
				netRet := (c[exitPos].CloseOrigin/sig.EntryPriceOrigin-1)*100 - 0.30
				mu.Lock()
				allTrades = append(allTrades, dtTrade{Shcode: sh, Signal: sig, NetReturnPct: netRet})
				mu.Unlock()
			}
			if n := scanned.Add(1); n%500 == 0 {
				mu.Lock()
				nt := len(allTrades)
				mu.Unlock()
				fmt.Printf("[dt_chart] %d/%d scanned (OOS trades=%d)\n", n, len(stocks), nt)
			}
		}(code)
	}
	wg.Wait()

	fmt.Printf("[dt_chart] scan=%d fail=%d OOS valid=%d\n", scanned.Load(), failed.Load(), len(allTrades))
	if len(allTrades) == 0 {
		fmt.Fprintln(os.Stderr, "유효 OOS 트레이드 없음")
		return
	}

	// h20 수익률 기준 오름차순 정렬
	sort.Slice(allTrades, func(i, j int) bool {
		return allTrades[i].NetReturnPct < allTrades[j].NetReturnPct
	})

	n := len(allTrades)
	type pqTarget struct {
		q     float64
		label string
	}
	targets := []pqTarget{
		{90, "P90"},
		{75, "P75"},
		{50, "P50"},
		{25, "P25"},
		{10, "P10"},
	}

	// 각 퍼센타일 인덱스에서 시작해 서로 다른 종목으로 결정적 선택
	type selectedTrade struct {
		trade     dtTrade
		label     string
		origIdx   int
	}
	var picks []selectedTrade
	usedCodes := map[string]bool{}

	for _, tgt := range targets {
		baseIdx := int(float64(n-1) * tgt.q / 100.0)
		// baseIdx 주변에서 유일한 종목 찾기 (최대 ±40 탐색)
		found := false
		for offset := 0; offset <= 40; offset++ {
			for _, d := range []int{0, 1, -1, 2, -2, 3, -3, 4, -4, 5, -5, 6, -6, 7, -7, 8, -8, 9, -9, 10, -10,
				11, -11, 12, -12, 13, -13, 14, -14, 15, -15, 16, -16, 17, -17, 18, -18, 19, -19, 20, -20,
				21, -21, 22, -22, 23, -23, 24, -24, 25, -25, 26, -26, 27, -27, 28, -28, 29, -29, 30, -30,
				31, -31, 32, -32, 33, -33, 34, -34, 35, -35, 36, -36, 37, -37, 38, -38, 39, -39, 40, -40} {
				idx := baseIdx + d
				if idx < 0 || idx >= n {
					continue
				}
				tr := allTrades[idx]
				if !usedCodes[tr.Shcode] {
					picks = append(picks, selectedTrade{trade: tr, label: tgt.label, origIdx: baseIdx})
					usedCodes[tr.Shcode] = true
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			fmt.Printf("[dt_chart] 경고: %s에 할당할 유일 종목 없음 (baseIdx=%d)\n", tgt.label, baseIdx)
		}
	}

	// 각 샘플의 차트 데이터 생성
	samples := make([]dtChartSample, 0, len(picks))
	for _, pk := range picks {
		tr := pk.trade
		sig := tr.Signal

		db2, _ := console.MsConn.GetDB("han")
		c, err := box.FetchCandlesHannam(db2, tr.Shcode, nc)
		if err != nil || len(c) == 0 {
			continue
		}
		indicator.PrepareCandles(c)

		// 차트 윈도우: R1Pos 30봉 전부터 exit+20봉 후까지
		winStart := sig.Pattern.R1Pos - 30
		if winStart < 0 {
			winStart = 0
		}
		exitPos := sig.EntryPos + 20
		winEnd := exitPos + 20
		if winEnd >= len(c) {
			winEnd = len(c) - 1
		}

		cl := make([]dtChartCandle, 0, winEnd-winStart+1)
		for i := winStart; i <= winEnd; i++ {
			cd := c[i]
			cl = append(cl, dtChartCandle{
				Date: cd.Date, Open: cd.OpenOrigin, High: cd.HighOrigin,
				Low: cd.LowOrigin, Close: cd.CloseOrigin, Volume: cd.Volume,
				MA20: cd.Ma20Origin, MA60: cd.Ma60Origin,
			})
		}

		exitPrice := 0.0
		exitDate := ""
		if exitPos < len(c) {
			exitPrice = c[exitPos].CloseOrigin
			exitDate = c[exitPos].Date
		}

		// candle 배열에서의 상대 인덱스 계산 (winStart 오프셋)
		samples = append(samples, dtChartSample{
			Label:      pk.label,
			Percentile: pk.label,
			Shcode:     tr.Shcode,
			R1Date:     c[sig.Pattern.R1Pos].Date,
			S1Date:     c[sig.Pattern.S1Pos].Date,
			R2Date:     c[sig.Pattern.R2Pos].Date,
			S2Date:     c[sig.Pattern.S2Pos].Date,
			R1Price:    sig.Pattern.R1Price,
			S1Price:    sig.Pattern.S1Price,
			R2Price:    sig.Pattern.R2Price,
			S2Price:    sig.Pattern.S2Price,
			FloorPrice: sig.Pattern.FloorPrice,
			Slope:      sig.Pattern.Slope,
			PatternBars: sig.Pattern.PatternBars,
			BreakoutDate:  sig.Date,
			BreakoutPos:   sig.Pos,
			BreakoutPrice: c[sig.Pos].CloseOrigin,
			TrendlineDate: sig.TrendlinePrice,
			EntryDate:     sig.EntryDate,
			EntryPos:      sig.EntryPos,
			EntryPrice:    sig.EntryPriceOrigin,
			ExitDate:      exitDate,
			ExitPos:       exitPos,
			ExitPrice:     exitPrice,
			NetReturnPct:  tr.NetReturnPct,
			Candles:       cl,
			PivotR1Idx:  sig.Pattern.R1Pos - winStart,
			PivotS1Idx:  sig.Pattern.S1Pos - winStart,
			PivotR2Idx:  sig.Pattern.R2Pos - winStart,
			PivotS2Idx:  sig.Pattern.S2Pos - winStart,
			BreakoutIdx: sig.Pos - winStart,
			EntryIdx:    sig.EntryPos - winStart,
			ExitIdx:     exitPos - winStart,
		})
	}

	cfgJSON, _ := json.Marshal(cfg)
	payload := dtChartPayload{VariantID: "S08_D03_B60_W20", Config: cfgJSON, Samples: samples}
	b, _ := json.MarshalIndent(payload, "", "  ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err := os.WriteFile(out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[dt_chart] %d samples → %s\n", len(samples), out)
	for _, s := range samples {
		fmt.Printf("  %s %s BO=%s ENTRY=%s EXIT=%s net=%.2f%%\n",
			s.Percentile, s.Shcode, s.BreakoutDate, s.EntryDate, s.ExitDate, s.NetReturnPct)
	}
}
