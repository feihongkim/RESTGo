package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// HandleCombinedScan 은 WD(W+DefBox) + S1(DefBox 단독 돌파) 합성 전략 신호를 스캔한다.
// 사용법: ./RESTGo stock combined_scan [--foreign-jp|--foreign-cn|--foreign-hk] [--max N] [--out path]
func HandleCombinedScan(args []string) {
	mode := "kr-kis2"
	maxExamples := 0
	outPath := "zpicture/combined_examples.json"

	for idx := 0; idx < len(args); idx++ {
		switch args[idx] {
		case "--foreign-jp":
			mode = "foreign-jp"
		case "--foreign-cn":
			mode = "foreign-cn"
		case "--foreign-hk":
			mode = "foreign-hk"
		case "--hannam":
			mode = "hannam"
		case "--max":
			if idx+1 < len(args) {
				idx++
				if n, err := strconv.Atoi(args[idx]); err == nil {
					maxExamples = n
				}
			}
		case "--out":
			if idx+1 < len(args) {
				idx++
				outPath = args[idx]
			}
		}
	}

	db, err := console.MsConn.GetDB("KIS2")
	if mode == "hannam" {
		db, err = console.MsConn.GetDB("han")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[combined_scan] DB 연결 오류: %v\n", err)
		return
	}

	var stocks []string
	switch mode {
	case "hannam":
		stocks, err = box.FetchHannamStockList(db)
	case "kr-kis2":
		stocks, err = fetchKIS2KorStockList(db)
	case "foreign-jp":
		stocks, err = box.FetchForeignStockList(db, []string{"DTS"})
	case "foreign-cn":
		stocks, err = box.FetchForeignStockList(db, []string{"DSZ", "DSH"})
	case "foreign-hk":
		stocks, err = box.FetchForeignStockList(db, []string{"DHK"})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[combined_scan] 종목 목록 오류: %v\n", err)
		return
	}

	market := map[string]string{
		"hannam": "KR", "kr-kis2": "KR",
		"foreign-jp": "JP", "foreign-cn": "CN", "foreign-hk": "HK",
	}[mode]

	unlimited := maxExamples == 0
	fmt.Printf("[combined_scan] %s 모드 %d개 종목 스캔 (max=%d)\n", market, len(stocks), maxExamples)

	var mu sync.Mutex
	var examples []stg.CombinedSignal
	var scanned atomic.Int64

	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup

	for _, code := range stocks {
		if !unlimited {
			mu.Lock()
			full := len(examples) >= maxExamples
			mu.Unlock()
			if full {
				break
			}
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()

			var candles []*box.Candle
			var fetchErr error
			switch mode {
			case "hannam":
				candles, fetchErr = box.FetchCandlesHannam(db, shcode, 500)
			case "kr-kis2":
				candles, fetchErr = box.FetchCandles(db, shcode, 500)
			default:
				candles, fetchErr = box.FetchCandlesForeign(db, shcode, 500)
			}
			if fetchErr != nil || len(candles) < 60 {
				scanned.Add(1)
				return
			}
			indicator.PrepareCandles(candles)

			sigs := stg.CombinedAnalyze(candles)
			if len(sigs) == 0 {
				scanned.Add(1)
				return
			}

			mu.Lock()
			for _, sig := range sigs {
				if unlimited || len(examples) < maxExamples {
					sig.Shcode = shcode
					examples = append(examples, sig)
				}
			}
			mu.Unlock()
			scanned.Add(1)
		}(code)
	}
	wg.Wait()

	type output struct {
		Market   string             `json:"market"`
		Examples []stg.CombinedSignal `json:"examples"`
	}
	data, _ := json.MarshalIndent(output{Market: market, Examples: examples}, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[combined_scan] 저장 실패: %v\n", err)
		return
	}

	wdCount, wdBreak, s1Count := 0, 0, 0
	for _, e := range examples {
		if e.Type == "WD" {
			wdCount++
			if e.DefBoxBreakDate != "" {
				wdBreak++
			}
		} else {
			s1Count++
		}
	}
	fmt.Printf("[combined_scan] 완료: %s (%d건 — WD:%d(돌파:%d) S1:%d)\n",
		outPath, len(examples), wdCount, wdBreak, s1Count)
}
