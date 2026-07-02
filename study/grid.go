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
	"time"

	"gopkg.in/yaml.v3"
)

// GridConfig 는 그리드 테스트 정의 파일 구조
type GridConfig struct {
	BaseStrategy string                   `yaml:"base_strategy"` // 베이스 YAML 경로
	Markets      []string                 `yaml:"markets"`       // 대상 마켓 목록 (예: ["KRW-BTC","KRW-ETH"])
	Days         int                      `yaml:"days"`          // 조회 봉 수
	Params       map[string][]interface{} `yaml:"params"`        // 파라미터 그리드
	Workers      int                      `yaml:"workers"`       // 병렬 워커 수 (기본 4)
}

// GridResult 는 파라미터 조합별 백테스트 결과
type GridResult struct {
	Params         map[string]interface{} `json:"params"`
	Market         string                 `json:"market"`
	TotalTrades    int                    `json:"total_trades"`
	WinTrades      int                    `json:"win_trades"`
	WinRate        float64                `json:"win_rate"`
	AvgReturn      float64                `json:"avg_return"`
	AvgNetReturn   float64                `json:"avg_net_return"`
	ProfitFactor   float64                `json:"profit_factor"`
	MaxDrawdown    float64                `json:"max_drawdown"`
	TotalReturn    float64                `json:"total_return"`
	BuySignalCount int                    `json:"buy_signal_count"`
}

// GridRunOutput 는 그리드 전체 출력 JSON
type GridRunOutput struct {
	GeneratedAt  string       `json:"generated_at"`
	BaseStrategy string       `json:"base_strategy"`
	Days         int          `json:"days"`
	Results      []GridResult `json:"results"`
}

// HandleGrid 는 "stock gridtest" 명령 진입점
func HandleGrid(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법: ./RESTGo stock gridtest <grid_yaml> [output_json]")
		fmt.Println("예)  ./RESTGo stock gridtest rules/grid_test.yaml zpicture/grid_results.json")
		return
	}

	gridPath := args[0]
	outputPath := "zpicture/grid_results.json"
	if len(args) >= 2 {
		outputPath = args[1]
	}

	data, err := os.ReadFile(gridPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "그리드 파일 읽기 실패: %v\n", err)
		return
	}
	var cfg GridConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "그리드 YAML 파싱 실패: %v\n", err)
		return
	}

	if cfg.BaseStrategy == "" {
		fmt.Fprintln(os.Stderr, "오류: base_strategy 미지정")
		return
	}
	if cfg.Days <= 0 {
		cfg.Days = 500
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}

	// 파라미터 조합 생성
	combinations := generateCombinations(cfg.Params)
	fmt.Printf("[grid] 베이스 전략: %s  마켓: %d개  파라미터 조합: %d  봉 수: %d\n",
		cfg.BaseStrategy, len(cfg.Markets), len(combinations), cfg.Days)

	// TUF DB 연결 (Upbit 15m 로더용)
	db, err := console.MsConn.GetDB("tuf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: tuf DB 연결 실패: %v\n", err)
		return
	}

	// 마켓별 캔들 미리 로드 (재사용)
	type marketCandles struct {
		market  string
		candles []*box.Candle
	}
	var allCandles []marketCandles
	for _, market := range cfg.Markets {
		candles, err := box.FetchUpbitCandles15m(db, market, cfg.Days)
		if err != nil || len(candles) < 50 {
			fmt.Printf("[grid] %s 캔들 로드 실패 또는 부족 (%d봉) — 건너뜀\n", market, len(candles))
			continue
		}
		indicator.PrepareCandles(candles)
		allCandles = append(allCandles, marketCandles{market, candles})
		fmt.Printf("[grid] %s 캔들 %d봉 로드 완료\n", market, len(candles))
	}

	if len(allCandles) == 0 {
		fmt.Fprintln(os.Stderr, "오류: 유효한 캔들 데이터 없음")
		return
	}

	// 그리드 조합 × 마켓 조합 백테스트
	type workItem struct {
		combo map[string]interface{}
		mc    marketCandles
	}
	var items []workItem
	for _, combo := range combinations {
		for _, mc := range allCandles {
			items = append(items, workItem{combo, mc})
		}
	}

	var (
		results []GridResult
		mu      sync.Mutex
		sem     = make(chan struct{}, cfg.Workers)
		wg      sync.WaitGroup
		done    int32
	)

	total := int32(len(items))
	startTime := time.Now()

	for _, item := range items {
		wg.Add(1)
		go func(it workItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := runGridItem(cfg.BaseStrategy, it.combo, it.mc.market, it.mc.candles)

			mu.Lock()
			results = append(results, res)
			mu.Unlock()

			n := atomic.AddInt32(&done, 1)
			if n%50 == 0 || n == total {
				fmt.Printf("[grid] %d/%d 완료 (%.0fs)\n", n, total, time.Since(startTime).Seconds())
			}
		}(item)
	}
	wg.Wait()

	output := GridRunOutput{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		BaseStrategy: cfg.BaseStrategy,
		Days:         cfg.Days,
		Results:      results,
	}

	outData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, outData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "결과 파일 쓰기 실패: %v\n", err)
		return
	}

	fmt.Printf("\n[grid] 완료: %d개 결과 → %s\n", len(results), outputPath)
	printGridSummary(results)

	// 결정성 검증: 동일 입력 2회 실행 후 일치 확인
	verifyGridDeterminism(cfg.BaseStrategy, combinations[0], allCandles[0].market, allCandles[0].candles)
}

// HandleGridTest 는 stock 패키지 Handle에서 호출되는 진입점
func HandleGridTest(args []string) {
	HandleGrid(args)
}

// runGridItem 은 단일 파라미터 조합 + 마켓에 대해 백테스트를 실행한다.
func runGridItem(basePath string, combo map[string]interface{}, market string, candles []*box.Candle) GridResult {
	res := GridResult{Params: combo, Market: market}

	// 베이스 전략 로드 + 파라미터 오버라이드 적용
	rules, baseSettings, err := stg.LoadRulesWithSettings(basePath)
	if err != nil {
		return res
	}
	applyComboToSettings(&baseSettings, combo)

	analysisResult := stg.AnalyzeWithRules(candles, rules, baseSettings)
	res.BuySignalCount = len(analysisResult.BuySignals)

	// 포지션 기반 성과 계산
	positions := analysisResult.Positions
	res.TotalTrades = 0
	var grossWin, grossLoss float64
	var cumReturn float64
	// 복리 자본곡선 기반 MDD: (peak - equity) / peak — 0~100% 범위 보장
	equity, equityPeak, maxDD := 1.0, 1.0, 0.0

	for _, pos := range positions {
		if pos.IsActive || len(pos.SellExecutions) == 0 {
			continue
		}
		res.TotalTrades++
		ret := pos.NetReturnRate
		if ret == 0 {
			ret = pos.ReturnRate
		}
		cumReturn += ret
		if ret > 0 {
			res.WinTrades++
			grossWin += ret
		} else {
			grossLoss += -ret
		}
		equity *= (1.0 + ret/100.0)
		if equity <= 0 {
			equity = 0
			maxDD = 1.0
		} else {
			if equity > equityPeak {
				equityPeak = equity
			}
			if dd := (equityPeak - equity) / equityPeak; dd > maxDD {
				maxDD = dd
			}
		}
	}

	res.TotalReturn = cumReturn
	res.MaxDrawdown = maxDD * 100
	if res.TotalTrades > 0 {
		res.AvgReturn = cumReturn / float64(res.TotalTrades)
		res.WinRate = float64(res.WinTrades) / float64(res.TotalTrades) * 100
	}
	if grossLoss > 0 {
		res.ProfitFactor = grossWin / grossLoss
	}

	// NetReturnRate 평균 (비용 차감)
	var netSum float64
	netCount := 0
	for _, pos := range positions {
		if pos.IsActive || len(pos.SellExecutions) == 0 {
			continue
		}
		if pos.NetReturnRate != 0 {
			netSum += pos.NetReturnRate
			netCount++
		}
	}
	if netCount > 0 {
		res.AvgNetReturn = netSum / float64(netCount)
	}

	return res
}

// applyComboToSettings 는 파라미터 조합을 Settings에 적용한다.
func applyComboToSettings(s *stg.Settings, combo map[string]interface{}) {
	stg.ApplySettingsOverrides(s, combo)
}

// generateCombinations 은 파라미터 그리드의 모든 조합을 생성한다.
func generateCombinations(params map[string][]interface{}) []map[string]interface{} {
	if len(params) == 0 {
		return []map[string]interface{}{{}}
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}

	result := []map[string]interface{}{{}}
	for _, key := range keys {
		vals := params[key]
		var next []map[string]interface{}
		for _, existing := range result {
			for _, v := range vals {
				combo := make(map[string]interface{}, len(existing)+1)
				for ek, ev := range existing {
					combo[ek] = ev
				}
				combo[key] = v
				next = append(next, combo)
			}
		}
		result = next
	}
	return result
}

// printGridSummary 는 상위 5개 결과를 출력한다.
func printGridSummary(results []GridResult) {
	if len(results) == 0 {
		return
	}
	fmt.Println("\n[grid] 상위 결과 (거래당 순수익 기준):")
	fmt.Printf("  %-20s %-12s %8s %8s %8s %6s\n", "Market", "Params...", "Trades", "WinRate%", "AvgNet%", "PF")

	// 거래 수 최소 조건 통과한 결과 중 AvgNetReturn 내림차순
	type ranked struct {
		r   GridResult
		key float64
	}
	var ranked_ []ranked
	for _, r := range results {
		if r.TotalTrades >= 5 {
			ranked_ = append(ranked_, ranked{r, r.AvgNetReturn})
		}
	}
	// 간단 선택정렬 (상위 5개만)
	for i := 0; i < len(ranked_) && i < 5; i++ {
		best := i
		for j := i + 1; j < len(ranked_); j++ {
			if ranked_[j].key > ranked_[best].key {
				best = j
			}
		}
		ranked_[i], ranked_[best] = ranked_[best], ranked_[i]
		r := ranked_[i].r
		paramsStr := fmt.Sprintf("%v", r.Params)
		if len(paramsStr) > 12 {
			paramsStr = paramsStr[:12] + "..."
		}
		fmt.Printf("  %-20s %-12s %8d %7.1f%% %7.2f%% %6.2f\n",
			r.Market, paramsStr, r.TotalTrades, r.WinRate, r.AvgNetReturn, r.ProfitFactor)
	}
}

// verifyGridDeterminism 은 동일 입력에 대해 2회 실행하여 결과 일치를 확인한다.
func verifyGridDeterminism(basePath string, combo map[string]interface{}, market string, candles []*box.Candle) {
	r1 := runGridItem(basePath, combo, market, candles)
	r2 := runGridItem(basePath, combo, market, candles)
	if r1.TotalTrades == r2.TotalTrades && r1.TotalReturn == r2.TotalReturn {
		fmt.Printf("[grid] 결정성 검증 통과 (거래 수: %d, 수익: %.4f)\n", r1.TotalTrades, r1.TotalReturn)
	} else {
		fmt.Printf("[grid] 결정성 검증 실패: 1차(%d거래, %.4f) != 2차(%d거래, %.4f)\n",
			r1.TotalTrades, r1.TotalReturn, r2.TotalTrades, r2.TotalReturn)
	}
}

// gridDays 는 그리드 YAML의 days 파라미터 파싱 헬퍼
func gridDays(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 500
	}
	return n
}
