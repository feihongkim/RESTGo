package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// 15분봉 → 30분봉 집계 (시간프레임 회생 시도용).
//
// 표준 OHLC 집계: 첫 봉 시가, 둘째 봉 종가, 고가 max, 저가 min, 거래량 합.
// 30분 경계 정렬: 첫 봉이 HH:30 시작이 아닌 경우 (HH:15·HH:45) 앞부분 1봉 스킵.

// aggregate15mTo30m 는 시간순 정렬된 15분 캔들을 30분 캔들로 집계한다.
// 30분 경계(00, 30분)부터 시작하도록 정렬한다.
func aggregate15mTo30m(candles []*box.Candle) []*box.Candle {
	if len(candles) == 0 {
		return nil
	}
	// 30분 경계 정렬: 첫 봉의 minute가 0/30이 아닌 경우 1봉 스킵
	start := 0
	first := candles[0].Time
	// Time format "HH:MM:SS" or "HHMMSS"
	mm := parseMinute(first)
	if mm != 0 && mm != 30 {
		start = 1
	}
	out := make([]*box.Candle, 0, (len(candles)-start)/2)
	for i := start; i+1 < len(candles); i += 2 {
		a, b := candles[i], candles[i+1]
		hi, lo := a.HighOrigin, a.LowOrigin
		if b.HighOrigin > hi {
			hi = b.HighOrigin
		}
		if b.LowOrigin < lo {
			lo = b.LowOrigin
		}
		c := &box.Candle{
			Shcode:      a.Shcode,
			Hname:       a.Hname,
			Date:        a.Date,
			Time:        a.Time, // 30분 시작시각
			OpenOrigin:  a.OpenOrigin,
			HighOrigin:  hi,
			LowOrigin:   lo,
			CloseOrigin: b.CloseOrigin,
			Volume:      a.Volume + b.Volume,
		}
		out = append(out, c)
	}
	return out
}

// parseMinute 은 "HH:MM:SS" 또는 "HHMMSS"에서 분(minute)을 추출한다.
func parseMinute(t string) int {
	clean := strings.ReplaceAll(t, ":", "")
	if len(clean) < 4 {
		return 0
	}
	m := 0
	fmt.Sscanf(clean[2:4], "%d", &m)
	return m
}

// HandleBaseline30m 는 "stock baseline30m" 명령 진입점.
//
// 사용법: ./RESTGo stock baseline30m [markets_csv] [output_json] [strategy_yaml]
func HandleBaseline30m(args []string) {
	stratPath := "rules/strategy3_stage1.yaml" // W11 9 전략 기본
	markets := []string{"KRW-BTC", "KRW-ETH", "KRW-XRP", "KRW-SOL"}
	outputPath := "zpicture/baseline_30m.json"
	if len(args) >= 1 && args[0] != "" {
		markets = strings.Split(args[0], ",")
	}
	if len(args) >= 2 && args[1] != "" {
		outputPath = args[1]
	}
	if len(args) >= 3 && args[2] != "" {
		stratPath = args[2]
	}
	runBaseline30m(stratPath, markets, outputPath)
}

func runBaseline30m(stratPath string, markets []string, outputPath string) {
	db, err := console.MsConn.GetDB("tuf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[30m] tuf DB 연결 실패: %v\n", err)
		return
	}

	rules, settings, err := stg.LoadRulesWithSettings(stratPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[30m] 전략 로드 실패: %v\n", err)
		return
	}
	fmt.Printf("[30m] 전략: %s  룰: %d개  마켓: %d개\n", stratPath, len(rules), len(markets))

	var mu sync.Mutex
	var allRows []BaselineStratRow
	var globalIsFrom, globalIsTo, globalOOSFrom, globalOOSTo string
	var wg sync.WaitGroup

	for _, market := range markets {
		wg.Add(1)
		go func(mkt string) {
			defer wg.Done()

			candles15m, err := box.FetchUpbitCandles15m(db, mkt, 400000)
			if err != nil || len(candles15m) < 200 {
				fmt.Printf("[30m] %s 캔들 부족 (%d봉)\n", mkt, len(candles15m))
				return
			}
			// 15m → 30m 집계 (봉수 절반)
			candles := aggregate15mTo30m(candles15m)
			if len(candles) < 100 {
				fmt.Printf("[30m] %s 30분 집계 후 캔들 부족 (%d봉)\n", mkt, len(candles))
				return
			}
			indicator.PrepareCandles(candles)

			// OOS 보존: 30m 기준 60일 = 60×48봉 = 2880봉
			oos30m := 2880
			isEnd := len(candles) - oos30m
			if isEnd < 100 {
				isEnd = len(candles)
			}
			isFrom := candles[0].Date
			isTo := candles[isEnd-1].Date
			oosFrom := ""
			oosTo := ""
			if len(candles) > oos30m && isEnd < len(candles) {
				oosFrom = candles[isEnd].Date
				oosTo = candles[len(candles)-1].Date
			}

			isCandles := candles[:isEnd]
			result := stg.AnalyzeWithRules(isCandles, rules, settings)

			stratMap := make(map[string][]box.TradePosition)
			for _, p := range result.Positions {
				if p.IsActive || len(p.SellExecutions) == 0 {
					continue
				}
				stratMap[p.StrategyName] = append(stratMap[p.StrategyName], *p)
			}
			var rows []BaselineStratRow
			for strat, trades := range stratMap {
				rows = append(rows, computeBaselineStats(mkt, strat, trades))
			}

			mu.Lock()
			allRows = append(allRows, rows...)
			if globalIsFrom == "" || isFrom < globalIsFrom {
				globalIsFrom = isFrom
			}
			if globalIsTo == "" || isTo > globalIsTo {
				globalIsTo = isTo
			}
			if globalOOSFrom == "" || (oosFrom != "" && oosFrom < globalOOSFrom) {
				globalOOSFrom = oosFrom
			}
			if globalOOSTo == "" || oosTo > globalOOSTo {
				globalOOSTo = oosTo
			}
			mu.Unlock()

			fmt.Printf("[30m] %s 완료: 15m %d→30m %d봉, IS %s~%s, OOS %s~%s, 청산 포지션 %d건\n",
				mkt, len(candles15m), len(candles), isFrom, isTo, oosFrom, oosTo, len(result.Positions))
		}(market)
	}
	wg.Wait()

	out := BaselineOutput{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Strategy:    stratPath + " (30m aggregated)",
		Period: EdgePeriod{
			IsFrom:  globalIsFrom,
			IsTo:    globalIsTo,
			OOSFrom: globalOOSFrom,
			OOSTo:   globalOOSTo,
		},
		Results: allRows,
	}

	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[30m] 디렉토리 생성 실패: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[30m] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[30m] 파일 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[30m] 완료: %s (룰×마켓 결과 %d건)\n", outputPath, len(allRows))
}
