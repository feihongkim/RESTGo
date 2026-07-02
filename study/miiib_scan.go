package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// MIIIbExample 은 MIIIb Box 기반 W바텀 신호 발화 사례 1건
type MIIIbExample struct {
	Market     string `json:"market"`
	Shcode     string `json:"shcode"`
	SignalDate string `json:"signal_date"`
	P1Date     string `json:"p1_date"`
	P2Date     string `json:"p2_date"`
}

// HandleMIIIbScan 은 각 시장에서 MIIIb_WBottomBox 신호 예시를 수집한다.
// 사용법: ./RESTGo stock miiib_scan [--foreign-jp|--foreign-cn|--foreign-hk] [--max N] [--out path]
// 기본 출력: zpicture/miiib_examples.json
func HandleMIIIbScan(args []string) {
	mode := "kr-kis2"
	maxExamples := 2
	outPath := "zpicture/miiib_examples.json"

	for idx := 0; idx < len(args); idx++ {
		switch args[idx] {
		case "--hannam":
			mode = "hannam"
		case "--foreign-jp":
			mode = "foreign-jp"
		case "--foreign-cn":
			mode = "foreign-cn"
		case "--foreign-hk":
			mode = "foreign-hk"
		case "--max":
			if idx+1 < len(args) {
				idx++
				if n, err := strconv.Atoi(args[idx]); err == nil {
					maxExamples = n // 0 = 제한 없음
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
		fmt.Fprintf(os.Stderr, "[miiib_scan] DB 연결 오류: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "[miiib_scan] 종목 목록 오류: %v\n", err)
		return
	}

	market := map[string]string{
		"hannam":     "KR",
		"kr-kis2":    "KR",
		"foreign-jp": "JP",
		"foreign-cn": "CN",
		"foreign-hk": "HK",
	}[mode]

	unlimited := maxExamples == 0
	fmt.Printf("[miiib_scan] %s 모드 %d개 종목 스캔 (max=%d)\n", market, len(stocks), maxExamples)

	var mu sync.Mutex
	var examples []MIIIbExample
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

			for _, sig := range stg.WPatternAnalyze(candles) {
				ex := MIIIbExample{
					Market:     market,
					Shcode:     shcode,
					SignalDate: sig.Date,
					P1Date:     sig.P1Date,
					P2Date:     sig.P2Date,
				}
				mu.Lock()
				if unlimited || len(examples) < maxExamples {
					examples = append(examples, ex)
					fmt.Printf("[miiib_scan] 발견: %s %s signal:%s p1:%s p2:%s\n",
						market, shcode, sig.Date, sig.P1Date, sig.P2Date)
				}
				mu.Unlock()
			}
			scanned.Add(1)
		}(code)
	}
	wg.Wait()

	type output struct {
		Market   string         `json:"market"`
		Examples []MIIIbExample `json:"examples"`
	}
	data, _ := json.MarshalIndent(output{Market: market, Examples: examples}, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[miiib_scan] 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[miiib_scan] 완료: %s (%d개)\n", outPath, len(examples))
}

func fetchKIS2KorStockList(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT stck_shrn_iscd
		FROM DM.BP_PeriodPrice
		WHERE period_type = 'D'
		ORDER BY stck_shrn_iscd
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil {
			codes = append(codes, code)
		}
	}
	return codes, rows.Err()
}
