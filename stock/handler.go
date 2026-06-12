package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	strategyPath     = "rules/strategy1.yaml"
	sellStrategyPath = "rules/sell_strategy1.yaml"
)

func Handle(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법:")
		fmt.Println("  ./RESTGo stock analyze <종목코드> [일수=250]")
		fmt.Println("  ./RESTGo stock batch [일수=250]")
		return
	}

	switch args[0] {
	case "analyze":
		handleAnalyze(args[1:])
	case "batch":
		handleBatch(args[1:])
	default:
		fmt.Printf("알 수 없는 stock 명령: %s\n", args[0])
		fmt.Println("사용법:")
		fmt.Println("  ./RESTGo stock analyze <종목코드> [일수=250]")
		fmt.Println("  ./RESTGo stock batch [일수=250]")
	}
}

func handleAnalyze(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법: ./RESTGo stock analyze <종목코드> [일수=250]")
		return
	}

	shcode := args[0]
	days := 250
	if len(args) >= 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "오류: 일수는 양수여야 합니다 (%s)\n", args[1])
			return
		}
		days = n
	}

	fmt.Printf("[%s] 종목: %s  일수: %d\n", console.GenerateTimestampedString(), shcode, days)

	db, err := console.MsConn.GetDB("KIS2")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: KIS2 DB 연결 실패: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[%s] 캔들 조회 중...\n", console.GenerateTimestampedString())
	candles, err := box.FetchCandles(db, shcode, days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[%s] 조회된 캔들: %d개\n", console.GenerateTimestampedString(), len(candles))

	if len(candles) < 6 {
		fmt.Println("오류: 분석에 필요한 최소 캔들 수(6)가 부족합니다")
		return
	}

	fmt.Printf("[%s] 지표 계산 중...\n", console.GenerateTimestampedString())
	indicator.PrepareCandles(candles)

	fmt.Printf("[%s] Box/매수/매도 분석 중...\n", console.GenerateTimestampedString())
	settings := stg.DefaultSettings()
	_ = stg.LoadStrategy(strategyPath)
	if err := stg.LoadSellStrategyFile(sellStrategyPath); err != nil {
		fmt.Printf("[warn] 매도 룰 로드 실패 — 매도 평가 비활성: %v\n", err)
	}
	result := stg.Analyze(candles, settings)

	nBox, nMain, nDef := 0, 0, 0
	for _, b := range result.BoxList {
		switch b.KindOfBox {
		case box.KindBox:
			nBox++
		case box.KindMainBox:
			nMain++
		case box.KindDefBox:
			nDef++
		}
	}

	fmt.Printf("\n── 분석 결과: %s ──\n", shcode)
	fmt.Printf("  Box: %d  MainBox: %d  DefBox: %d  합계: %d\n", nBox, nMain, nDef, len(result.BoxList))
	fmt.Printf("  매수 신호: %d건\n", len(result.BuySignals))

	kindLabel := []string{"Box    ", "MainBox", "DefBox ", "Multi  "}
	typeLabel := []string{"Support", "Resist ", "Unknown"}
	fmt.Println("\n[전체 Box 목록]")
	fmt.Printf("  %-4s  %-10s  %-5s  %-12s  %-9s  %s\n", "Idx", "Date", "Pos", "원본가격", "Kind", "Type")
	fmt.Println("  " + strings.Repeat("-", 60))
	for i, b := range result.BoxList {
		fmt.Printf("  %-4d  %-10s  %-5d  %-12.0f  %-9s  %s\n",
			i, b.Date, b.BoxPosition, b.PriceOrigin, kindLabel[b.KindOfBox], typeLabel[b.BoxType])
	}

	if len(result.BuySignals) > 0 {
		fmt.Println("\n[매수 신호]")
		for _, s := range result.BuySignals {
			if s.Position >= 0 && s.Position < len(candles) {
				c := candles[s.Position]
				fmt.Printf("  Position:%d  날짜:%s  종가:%.0f  사유:%s\n",
					s.Position, c.Date, c.CloseOrigin, s.Reason)
			}
		}
	}

	if len(result.Positions) > 0 {
		liquidated := 0
		active := 0
		for _, p := range result.Positions {
			if p.IsActive {
				active++
			} else {
				liquidated++
			}
		}
		fmt.Printf("\n[포지션] 활성:%d  청산완료:%d\n", active, liquidated)
		for _, p := range result.Positions {
			if len(p.SellExecutions) == 0 {
				continue
			}
			// 청산 완료 포지션은 매도 시점까지, 활성 포지션은 데이터 끝까지의 보유일
			holdEnd := len(candles) - 1
			if !p.IsActive && p.SellPosition >= 0 {
				holdEnd = p.SellPosition
			}
			fmt.Printf("  TradeId:%s  전략:%s  매수가:%.0f  보유:%d캔들\n",
				p.TradeId, p.StrategyName, p.BuyPriceOrigin, p.HoldingDays(holdEnd))
			for _, e := range p.SellExecutions {
				fmt.Printf("    └ #%d  날짜:%s  사유:%s  비율:%.0f%%  매도가:%.0f  수익률:%.2f%%\n",
					e.ExecutionOrder, e.SellDate, e.SellReason, e.Weight*100, e.SellPriceOrigin, e.PartialReturnRate)
			}
		}
	}
}

func handleBatch(args []string) {
	days := 250
	if len(args) >= 1 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "오류: 일수는 양수여야 합니다 (%s)\n", args[0])
			return
		}
		days = n
	}

	_ = stg.LoadStrategy(strategyPath)
	if err := stg.LoadSellStrategyFile(sellStrategyPath); err != nil {
		fmt.Printf("[warn] 매도 룰 로드 실패 — 매도 평가 비활성: %v\n", err)
	}

	db, err := console.MsConn.GetDB("KIS2")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: KIS2 DB 연결 실패: %v\n", err)
		os.Exit(1)
	}

	rows, err := db.Query(`
		SELECT p.stck_shrn_iscd, ISNULL(RTRIM(k.kor_isnm), p.stck_shrn_iscd) AS hname
		FROM (SELECT DISTINCT stck_shrn_iscd FROM DM.BP_PeriodPrice WHERE period_type='D') p
		LEFT JOIN MS.KospiCode k ON k.shrn_iscd = p.stck_shrn_iscd
		ORDER BY p.stck_shrn_iscd
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: 종목 조회 실패: %v\n", err)
		os.Exit(1)
	}

	type stockInfo struct{ Shcode, Hname string }
	var stocks []stockInfo
	for rows.Next() {
		var s stockInfo
		if err := rows.Scan(&s.Shcode, &s.Hname); err == nil {
			stocks = append(stocks, s)
		}
	}
	rows.Close()

	fmt.Printf("[%s] 분석 대상: %d 종목  일수: %d\n", console.GenerateTimestampedString(), len(stocks), days)

	type resultItem struct {
		Shcode  string          `json:"shcode"`
		Hname   string          `json:"hname"`
		Signals []stg.BuySignal `json:"signals"`
	}

	type signalJSON struct {
		Date         string  `json:"date"`
		Position     int     `json:"position"`
		Reason       string  `json:"reason"`
		DefboxPrice  float64 `json:"defbox_price"`
		MainboxPrice float64 `json:"mainbox_price"`
		MainboxDate  string  `json:"mainbox_date"`
	}

	type resultItemJSON struct {
		Shcode  string       `json:"shcode"`
		Hname   string       `json:"hname"`
		Signals []signalJSON `json:"signals"`
	}

	var results []resultItem
	var mu sync.Mutex
	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup
	var processed int32

	settings := stg.DefaultSettings()

	for _, s := range stocks {
		wg.Add(1)
		go func(s stockInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			candles, err := box.FetchCandles(db, s.Shcode, days)
			if err != nil || len(candles) < 6 {
				atomic.AddInt32(&processed, 1)
				return
			}
			indicator.PrepareCandles(candles)
			result := stg.Analyze(candles, settings)

			n := atomic.AddInt32(&processed, 1)
			if n%100 == 0 {
				fmt.Printf("[batch] %d/%d 처리 중...\n", n, int32(len(stocks)))
			}

			if len(result.BuySignals) > 0 {
				mu.Lock()
				results = append(results, resultItem{s.Shcode, s.Hname, result.BuySignals})
				mu.Unlock()
			}
		}(s)
	}
	wg.Wait()

	fmt.Printf("\n[%s] 매수 신호 종목: %d개\n", console.GenerateTimestampedString(), len(results))

	generatedAt := time.Now().Format("20060102_150405")
	jsonItems := make([]resultItemJSON, 0, len(results))
	for _, r := range results {
		sigs := make([]signalJSON, 0, len(r.Signals))
		for _, sig := range r.Signals {
			sigs = append(sigs, signalJSON{
				Date:         sig.Date,
				Position:     sig.Position,
				Reason:       sig.Reason,
				DefboxPrice:  sig.DefboxPrice,
				MainboxPrice: sig.MainboxPrice,
				MainboxDate:  sig.MainboxDate,
			})
		}
		jsonItems = append(jsonItems, resultItemJSON{r.Shcode, r.Hname, sigs})
	}

	output := map[string]interface{}{
		"generated_at": generatedAt,
		"display_days": 180,
		"stocks":       jsonItems,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: JSON 직렬화 실패: %v\n", err)
		os.Exit(1)
	}

	outPath := "zpicture/batch_signals.json"
	if err := os.MkdirAll("zpicture", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "오류: zpicture 디렉토리 생성 실패: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "오류: JSON 파일 저장 실패: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[%s] 저장 완료: %s\n", console.GenerateTimestampedString(), outPath)
}
