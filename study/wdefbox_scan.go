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

// WDefBoxExample 은 W패턴+DefBox 결합 신호 1건 (JSON 출력용)
type WDefBoxExample struct {
	Market               string   `json:"market"`
	Shcode               string   `json:"shcode"`
	SignalDate           string   `json:"signal_date"`
	P1Date               string   `json:"p1_date"`
	P2Date               string   `json:"p2_date"`
	HasDefBox            bool     `json:"has_defbox"`
	DefBoxDate           string   `json:"defbox_date,omitempty"`
	DefBoxPriceOrigin    float64  `json:"defbox_price,omitempty"` // 원본 가격 (차트 수평선용)
	DefBoxBreakDate      string   `json:"defbox_break_date,omitempty"` // 비어있으면 20일 내 미돌파
	DefBoxBreakDays      int      `json:"defbox_break_days,omitempty"` // W신호→돌파 거래일 수
	// forward return (Go가 캔들 보유 중 직접 계산, 스케일 가격 기준)
	RW5   *float64 `json:"r_w5,omitempty"`
	RW10  *float64 `json:"r_w10,omitempty"`
	RW20  *float64 `json:"r_w20,omitempty"`
	RDef5  *float64 `json:"r_def5,omitempty"`
	RDef10 *float64 `json:"r_def10,omitempty"`
	RDef20 *float64 `json:"r_def20,omitempty"`
	// 필터용 지표 (W신호 발화일 기준)
	MA200AtSignal float64 `json:"ma200_at_signal,omitempty"` // 원본가 MA200 (0이면 미계산)
	RSI14AtSignal float64 `json:"rsi14_at_signal,omitempty"` // RSI14
	CloseAtSignal float64 `json:"close_at_signal,omitempty"` // 원본 종가
}

// HandleWDefBoxScan 은 W패턴+DefBox 결합 신호를 스캔한다.
// 사용법: ./RESTGo stock wdefbox_scan [--foreign-jp|--foreign-cn|--foreign-hk|--hannam] [--max N] [--candles N] [--out path] [--defbox-only]
// 기본 출력: zpicture/wdefbox_examples.json
func HandleWDefBoxScan(args []string) {
	mode := "kr-kis2"
	maxExamples := 0
	candleCount := 500
	outPath := "zpicture/wdefbox_examples.json"
	defboxOnly := false

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
		case "--defbox-only":
			defboxOnly = true
		case "--max":
			if idx+1 < len(args) {
				idx++
				if n, err := strconv.Atoi(args[idx]); err == nil {
					maxExamples = n
				}
			}
		case "--candles":
			if idx+1 < len(args) {
				idx++
				if n, err := strconv.Atoi(args[idx]); err == nil {
					candleCount = n
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
		fmt.Fprintf(os.Stderr, "[wdefbox_scan] DB 연결 오류: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "[wdefbox_scan] 종목 목록 오류: %v\n", err)
		return
	}

	market := map[string]string{
		"hannam": "KR", "kr-kis2": "KR",
		"foreign-jp": "JP", "foreign-cn": "CN", "foreign-hk": "HK",
	}[mode]

	unlimited := maxExamples == 0
	fmt.Printf("[wdefbox_scan] %s 모드 %d개 종목 스캔 (max=%d candles=%d defbox-only=%v)\n",
		market, len(stocks), maxExamples, candleCount, defboxOnly)

	var mu sync.Mutex
	var examples []WDefBoxExample
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
				candles, fetchErr = box.FetchCandlesHannam(db, shcode, candleCount)
			case "kr-kis2":
				candles, fetchErr = box.FetchCandles(db, shcode, candleCount)
			default:
				candles, fetchErr = box.FetchCandlesForeign(db, shcode, candleCount)
			}
			if fetchErr != nil || len(candles) < 60 {
				scanned.Add(1)
				return
			}
			indicator.PrepareCandles(candles)

			n := len(candles)
			for _, sig := range stg.WDefBoxAnalyze(candles) {
				if defboxOnly && !sig.HasDefBox {
					continue
				}
				breakDays := 0
				if sig.DefBoxBreakPos > 0 {
					breakDays = sig.DefBoxBreakPos - sig.Pos
				}
				ex := WDefBoxExample{
					Market:            market,
					Shcode:            shcode,
					SignalDate:        sig.Date,
					P1Date:            sig.P1Date,
					P2Date:            sig.P2Date,
					HasDefBox:         sig.HasDefBox,
					DefBoxDate:        sig.DefBoxDate,
					DefBoxPriceOrigin: sig.DefBoxPriceOrigin,
					DefBoxBreakDate:   sig.DefBoxBreakDate,
					DefBoxBreakDays:   breakDays,
				}
				// W-신호 기준 forward return (스케일 종가)
				base := candles[sig.Pos].Close
				if base > 0 {
					for _, h := range []int{5, 10, 20} {
						end := sig.Pos + h
						if end < n {
							r := (candles[end].Close - base) / base
							switch h {
							case 5:  ex.RW5 = &r
							case 10: ex.RW10 = &r
							case 20: ex.RW20 = &r
							}
						}
					}
				}
				// W신호 발화일 필터 지표 (⑤ 강세장 필터용)
				ex.CloseAtSignal = candles[sig.Pos].CloseOrigin
				ex.MA200AtSignal = candles[sig.Pos].Ma200Origin
				ex.RSI14AtSignal = candles[sig.Pos].RSI

				// DefBox 돌파 기준 forward return
				if sig.DefBoxBreakPos > 0 && sig.DefBoxBreakPos < n {
					bbase := candles[sig.DefBoxBreakPos].Close
					if bbase > 0 {
						for _, h := range []int{5, 10, 20} {
							end := sig.DefBoxBreakPos + h
							if end < n {
								r := (candles[end].Close - bbase) / bbase
								switch h {
								case 5:  ex.RDef5 = &r
								case 10: ex.RDef10 = &r
								case 20: ex.RDef20 = &r
								}
							}
						}
					}
				}
				mu.Lock()
				if unlimited || len(examples) < maxExamples {
					examples = append(examples, ex)
				}
				mu.Unlock()
			}
			scanned.Add(1)
		}(code)
	}
	wg.Wait()

	type output struct {
		Market   string           `json:"market"`
		Examples []WDefBoxExample `json:"examples"`
	}
	data, _ := json.MarshalIndent(output{Market: market, Examples: examples}, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[wdefbox_scan] 저장 실패: %v\n", err)
		return
	}

	total := len(examples)
	withDefBox := 0
	withBreak := 0
	for _, e := range examples {
		if e.HasDefBox {
			withDefBox++
		}
		if e.DefBoxBreakDate != "" {
			withBreak++
		}
	}
	fmt.Printf("[wdefbox_scan] 완료: %s (%d개 신호, DefBox있음:%d, DefBox돌파:%d)\n",
		outPath, total, withDefBox, withBreak)
}
