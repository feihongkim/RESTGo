package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// ── 출력 스키마 ────────────────────────────────────────────────────────────

// EdgeStudyOutput 은 Stage 0 이벤트 스터디 전체 결과 JSON
type EdgeStudyOutput struct {
	GeneratedAt string        `json:"generated_at"`
	Period      EdgePeriod    `json:"period"`
	Baseline    []BaselineRow `json:"baseline"`
	Results     []EdgeResult  `json:"results"`
}

// EdgePeriod 는 분석 기간 정보 (IS = In-Sample, OOS = Out-of-Sample)
type EdgePeriod struct {
	IsFrom  string `json:"is_from"`
	IsTo    string `json:"is_to"`
	OOSFrom string `json:"oos_from"`
	OOSTo   string `json:"oos_to"`
}

// BaselineRow 는 무조건 T+H 수익률 베이스라인 (마켓×호라이즌)
type BaselineRow struct {
	Market        string  `json:"market"`
	Horizon       int     `json:"horizon"`
	MeanNetReturn float64 `json:"mean_net_return"`
	N             int     `json:"n"`
}

// EdgeResult 는 조건별·마켓별·호라이즌별 엣지 측정 결과
type EdgeResult struct {
	Condition     string             `json:"condition"`
	Market        string             `json:"market"`
	Horizon       int                `json:"horizon"`
	FireCount     int                `json:"fire_count"`
	MeanNetReturn float64            `json:"mean_net_return"`
	Edge          float64            `json:"edge"`
	TStat         float64            `json:"t_stat"`
	WinRate       float64            `json:"win_rate"`
	EdgeBySession map[string]float64 `json:"edge_by_session"`
}

// ── 조건 목록 ─────────────────────────────────────────────────────────────

// edge37Conditions 는 Stage 0 이벤트 스터디에서 스캔할 조건 목록
var edge37Conditions = []string{
	"IsMACDGoldenCross",
	"IsMACDHistogramRising",
	"IsStochGoldenCross",
	"IsADXTrending",
	"IsDIBullish",
	"IsDIBearish",
	"IsAboveVWAP",
	"IsBelowVWAP",
	"IsVWAPDeviationBelow",
	"IsVWAPReclaim",
	"IsVolumeZScoreSpike",
	"IsOBVRising",
	"IsSuperTrendBullish",
	"IsSuperTrendBearish",
	"IsDonchianBreakout",
	"IsDonchianBreakdown",
	"IsKeltnerBreakout",
	"IsNarrowRange",
	"IsRSIFallingFromOverbought",
	"IsBBUpperReject",
	"IsMaDeadCross5x20",
	"IsMaInverseArrangement",
	"IsPriceBelowAllMa",
	"IsRSIOversold",
	"IsRSIOverbought",
	"IsRSIRecoveringFromOversold",
	"IsRSIRising",
	"IsRSIInBullZone",
	"IsBBLowerTouch",
	"IsBBReboundFromLower",
	"IsBBSqueezeBreakout",
	"IsBBUpperBreakout",
	"IsAboveBBMiddle",
	"IsMaGoldenCross5x20",
	"IsMaGoldenCross20x60",
	"IsMaProperArrangement",
	"IsAllMaRising",
	"IsPriceAboveAllMa",
	"IsEMABullArrangement",
	"IsEMA21PullbackBounce",
	"IsPriceAboveEMA50",
}

// edgeHorizons 는 측정할 선행 봉 수 (T+4 / T+8 / T+16)
var edgeHorizons = []int{4, 8, 16}

// oosMonths 는 Out-of-Sample 제외 기간 (2개월 = 60일 × 96봉/일)
const oosMonths = 5760 // 60일 × 96봉

// warmupBars 는 지표 워밍업 스킵 봉 수
const warmupBars = 100

// roundTripCost 는 왕복 수수료+슬리피지 (0.2%)
const roundTripCost = 0.002

// cooldownBars 는 동일 조건 재발화 금지 봉 수
const cooldownBars = 4

// ── 메인 로직 ─────────────────────────────────────────────────────────────

// HandleEdgeTest 는 "stock edgetest" 명령 진입점
func HandleEdgeTest(args []string) {
	markets := []string{"KRW-BTC", "KRW-ETH", "KRW-XRP", "KRW-SOL"}
	outputPath := "zpicture/stage0_edge.json"
	if len(args) >= 1 && args[0] != "" {
		markets = strings.Split(args[0], ",")
	}
	if len(args) >= 2 {
		outputPath = args[1]
	}
	runEdgeStudy(markets, outputPath)
}

// runEdgeStudy 는 마켓 목록에 대해 Stage 0 이벤트 스터디를 실행하고 결과를 JSON으로 저장한다.
func runEdgeStudy(markets []string, outputPath string) {
	db, err := console.MsConn.GetDB("tuf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[edge] tuf DB 연결 실패: %v\n", err)
		return
	}

	s := stg.DefaultSettings()
	fmt.Printf("[edge] 마켓 %d개, 조건 %d종, 호라이즌 %v\n", len(markets), len(edge37Conditions), edgeHorizons)

	type marketResult struct {
		market   string
		candles  []*box.Candle
		baseline []BaselineRow
		results  []EdgeResult
		isFrom   string
		isTo     string
		oosFrom  string
		oosTo    string
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var allBaselines []BaselineRow
	var allResults []EdgeResult
	var globalIsFrom, globalIsTo, globalOOSFrom, globalOOSTo string

	for _, market := range markets {
		wg.Add(1)
		go func(mkt string) {
			defer wg.Done()

			candles, err := box.FetchUpbitCandles15m(db, mkt, 400000)
			if err != nil || len(candles) < warmupBars+edgeHorizons[len(edgeHorizons)-1]+2 {
				fmt.Printf("[edge] %s 캔들 부족 또는 로드 실패 (%d봉)\n", mkt, len(candles))
				return
			}
			indicator.PrepareCandles(candles)

			// OOS 제외: 마지막 oosMonths 봉 제외
			isEnd := len(candles) - oosMonths
			if isEnd < warmupBars+edgeHorizons[len(edgeHorizons)-1]+2 {
				isEnd = len(candles)
			}

			isFrom := candles[0].Date
			isTo := ""
			if isEnd > 0 && isEnd <= len(candles) {
				isTo = candles[isEnd-1].Date
			}
			oosFrom := ""
			oosTo := ""
			if len(candles) > oosMonths && isEnd < len(candles) {
				oosFrom = candles[isEnd].Date
				oosTo = candles[len(candles)-1].Date
			}

			baselines, results := analyzeMarketEdge(mkt, candles[:isEnd], s)

			mu.Lock()
			allBaselines = append(allBaselines, baselines...)
			allResults = append(allResults, results...)
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

			fmt.Printf("[edge] %s 완료: IS %s~%s, OOS %s~%s, 조건 결과 %d건\n",
				mkt, isFrom, isTo, oosFrom, oosTo, len(results))
		}(market)
	}
	wg.Wait()

	output := EdgeStudyOutput{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Period: EdgePeriod{
			IsFrom:  globalIsFrom,
			IsTo:    globalIsTo,
			OOSFrom: globalOOSFrom,
			OOSTo:   globalOOSTo,
		},
		Baseline: allBaselines,
		Results:  allResults,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[edge] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[edge] 출력 디렉토리 생성 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[edge] 파일 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[edge] 완료: %s (baseline %d, results %d)\n", outputPath, len(allBaselines), len(allResults))
}

// analyzeMarketEdge 는 단일 마켓 캔들에 대해 베이스라인 + 조건별 엣지를 계산한다.
func analyzeMarketEdge(market string, candles []*box.Candle, s stg.Settings) ([]BaselineRow, []EdgeResult) {
	n := len(candles)
	maxHorizon := edgeHorizons[len(edgeHorizons)-1]

	// 유효한 진입 가능 범위: warmupBars ~ n-maxHorizon-2
	// (t+1 시가 진입, t+H 종가 출구 — 인덱스 t+H+1까지 필요)
	endIdx := n - maxHorizon - 1
	if endIdx <= warmupBars {
		return nil, nil
	}

	// ── 베이스라인: 모든 유효 봉에서 T+1 시가 진입 → T+H 종가 수익률 ──
	type horizonSamples struct {
		returns []float64
	}
	baseMap := make(map[int]*horizonSamples)
	for _, h := range edgeHorizons {
		baseMap[h] = &horizonSamples{}
	}
	for t := warmupBars; t < endIdx; t++ {
		if t+1 >= n {
			continue
		}
		entryPrice := candles[t+1].OpenOrigin
		if entryPrice <= 0 {
			continue
		}
		for _, h := range edgeHorizons {
			if t+h >= n {
				continue
			}
			exitPrice := candles[t+h].CloseOrigin
			ret := (exitPrice-entryPrice)/entryPrice - roundTripCost
			baseMap[h].returns = append(baseMap[h].returns, ret)
		}
	}

	var baselines []BaselineRow
	for _, h := range edgeHorizons {
		hs := baseMap[h]
		mean := meanFloats(hs.returns)
		baselines = append(baselines, BaselineRow{
			Market:        market,
			Horizon:       h,
			MeanNetReturn: mean,
			N:             len(hs.returns),
		})
	}

	// ── 조건별 엣지 스캔 ──────────────────────────────────────────────
	// ctx: TradingContext (읽기 전용 캔들 순회용)
	ctx := box.NewTradingContext(candles, nil)

	// condLastFired[condName] = last fired candle index (쿨다운 추적)
	condLastFired := make(map[string]int, len(edge37Conditions))
	for _, name := range edge37Conditions {
		condLastFired[name] = -cooldownBars - 1
	}

	// 조건별·호라이즌별 수익률 샘플 및 세션별 엣지
	type condKey struct {
		cond    string
		horizon int
	}
	type sample struct {
		ret     float64
		session string
	}
	condSamples := make(map[condKey][]sample)

	// 베이스라인 수익률 (전체 풀 — Welch t-test 모집단)
	baseReturns := make(map[int][]float64)
	for _, h := range edgeHorizons {
		baseReturns[h] = baseMap[h].returns
	}

	for t := warmupBars; t < endIdx; t++ {
		ctx.Position = t
		if t+1 >= n {
			continue
		}
		entryPrice := candles[t+1].OpenOrigin
		if entryPrice <= 0 {
			continue
		}
		// KST 세션 분류 (Time 필드: "HH:MM:SS" 또는 "HHMMSS")
		sess := kstSession(candles[t].Time)

		for _, name := range edge37Conditions {
			// 쿨다운 체크
			if t-condLastFired[name] < cooldownBars {
				continue
			}
			fn, ok := stg.ConditionRegistryGet(name)
			if !ok {
				continue
			}
			if !fn(ctx, s) {
				continue
			}
			// 발화
			condLastFired[name] = t
			for _, h := range edgeHorizons {
				if t+h >= n {
					continue
				}
				exitPrice := candles[t+h].CloseOrigin
				ret := (exitPrice-entryPrice)/entryPrice - roundTripCost
				key := condKey{name, h}
				condSamples[key] = append(condSamples[key], sample{ret, sess})
			}
		}
	}

	// ── 결과 집계 ─────────────────────────────────────────────────────
	var results []EdgeResult
	for _, name := range edge37Conditions {
		for _, h := range edgeHorizons {
			key := condKey{name, h}
			samples := condSamples[key]
			if len(samples) == 0 {
				continue
			}
			condReturns := make([]float64, len(samples))
			wins := 0
			sessionSum := make(map[string]float64)
			sessionCount := make(map[string]int)
			baselineMean := meanFloats(baseReturns[h])

			for i, s := range samples {
				condReturns[i] = s.ret
				if s.ret > 0 {
					wins++
				}
				sessionSum[s.session] += s.ret - baselineMean
				sessionCount[s.session]++
			}

			condMean := meanFloats(condReturns)
			edge := condMean - baselineMean
			tStat := edgeWelchTStat(condReturns, baseReturns[h])
			winRate := float64(wins) / float64(len(samples))

			edgeBySession := make(map[string]float64)
			for sess, sum := range sessionSum {
				cnt := sessionCount[sess]
				if cnt > 0 {
					edgeBySession[sess] = sum / float64(cnt)
				}
			}

			results = append(results, EdgeResult{
				Condition:     name,
				Market:        market,
				Horizon:       h,
				FireCount:     len(samples),
				MeanNetReturn: condMean,
				Edge:          edge,
				TStat:         tStat,
				WinRate:       winRate,
				EdgeBySession: edgeBySession,
			})
		}
	}

	return baselines, results
}

// edgeWelchTStat は edge.go 내부 전용 Welch t-통계량 (meanFloats 사용)
func edgeWelchTStat(group1, group2 []float64) float64 {
	n1, n2 := float64(len(group1)), float64(len(group2))
	if n1 < 2 || n2 < 2 {
		return 0
	}
	m1, m2 := meanFloats(group1), meanFloats(group2)
	v1, v2 := varianceFloats(group1), varianceFloats(group2)
	se := math.Sqrt(v1/n1 + v2/n2)
	if se == 0 {
		return 0
	}
	return (m1 - m2) / se
}

// ── KST 세션 분류 ────────────────────────────────────────────────────────

// kstSession 은 KST 시간 문자열을 세션 구간으로 분류한다.
// 입력: "15:30:00" 또는 "153000" 형식
// 반환: "kst_00_08" | "kst_08_16" | "kst_16_24"
func kstSession(timeStr string) string {
	// HH:MM:SS 또는 HHMMSS 형식에서 시(hour) 추출
	h := 0
	clean := strings.ReplaceAll(timeStr, ":", "")
	if len(clean) >= 2 {
		hh := 0
		fmt.Sscanf(clean[:2], "%d", &hh)
		h = hh
	}
	switch {
	case h >= 0 && h < 8:
		return "kst_00_08"
	case h >= 8 && h < 16:
		return "kst_08_16"
	default:
		return "kst_16_24"
	}
}

// ── 유틸 ──────────────────────────────────────────────────────────────────

// dirOf 는 파일 경로에서 디렉토리 부분을 반환한다.
func dirOf(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	return path[:idx]
}

// ── W8-5: 베이스라인 백테스트 ────────────────────────────────────────────────

// BaselineTradeRow 는 거래 1건 상세
type BaselineTradeRow struct {
	Market       string  `json:"market"`
	StrategyName string  `json:"strategy_name"`
	BuyDate      string  `json:"buy_date"`
	BuyTime      string  `json:"buy_time"`
	BuyPrice     float64 `json:"buy_price"`
	SellDate     string  `json:"sell_date"`
	SellTime     string  `json:"sell_time"`
	SellReason   string  `json:"sell_reason"`
	NetReturn    float64 `json:"net_return_pct"`
}

// ByYearRow 는 연도별 분해 통계 (5거래 미만이면 평가 지표 null)
type ByYearRow struct {
	Year         int      `json:"year"`
	Trades       int      `json:"trades"`
	WinRate      *float64 `json:"win_rate"`
	AvgNetReturn *float64 `json:"avg_net_return_pct"`
	ProfitFactor *float64 `json:"pf"`
}

// BaselineStratRow 는 룰×마켓 집계 통계
type BaselineStratRow struct {
	Market       string             `json:"market"`
	Strategy     string             `json:"strategy"`
	TradeCount   int                `json:"trade_count"`
	WinRate      float64            `json:"win_rate"`
	AvgNetReturn float64            `json:"avg_net_return_pct"`
	ProfitFactor float64            `json:"profit_factor"`
	MaxDrawdown  float64            `json:"max_drawdown_pct"`
	ByYear       []ByYearRow        `json:"by_year"`
	Trades       []BaselineTradeRow `json:"trades"`
}

// BaselineOutput 은 베이스라인 백테스트 전체 출력 JSON
type BaselineOutput struct {
	GeneratedAt string             `json:"generated_at"`
	Strategy    string             `json:"strategy"`
	Period      EdgePeriod         `json:"period"`
	Results     []BaselineStratRow `json:"results"`
}

// HandleBaselineBacktest 는 "stock baseline" 명령 진입점
func HandleBaselineBacktest(args []string) {
	stratPath := "rules/buy_crypto_15m.yaml"
	markets := []string{"KRW-BTC", "KRW-ETH", "KRW-XRP", "KRW-SOL"}
	outputPath := "zpicture/baseline_t_rules.json"
	if len(args) >= 1 && args[0] != "" {
		markets = strings.Split(args[0], ",")
	}
	if len(args) >= 2 {
		outputPath = args[1]
	}
	if len(args) >= 3 {
		stratPath = args[2]
	}
	runBaselineBacktest(stratPath, markets, outputPath)
}

func runBaselineBacktest(stratPath string, markets []string, outputPath string) {
	db, err := console.MsConn.GetDB("tuf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[baseline] tuf DB 연결 실패: %v\n", err)
		return
	}

	rules, settings, err := stg.LoadRulesWithSettings(stratPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[baseline] 전략 로드 실패: %v\n", err)
		return
	}
	fmt.Printf("[baseline] 전략: %s  룰: %d개  마켓: %d개\n", stratPath, len(rules), len(markets))

	type marketResult struct {
		rows    []BaselineStratRow
		oosFrom string
		isFrom  string
		isTo    string
	}

	var mu sync.Mutex
	var allRows []BaselineStratRow
	var globalIsFrom, globalIsTo, globalOOSFrom, globalOOSTo string
	var wg sync.WaitGroup

	for _, market := range markets {
		wg.Add(1)
		go func(mkt string) {
			defer wg.Done()

			candles, err := box.FetchUpbitCandles15m(db, mkt, 400000)
			if err != nil || len(candles) < 100 {
				fmt.Printf("[baseline] %s 캔들 부족 또는 로드 실패 (%d봉)\n", mkt, len(candles))
				return
			}
			indicator.PrepareCandles(candles)

			isEnd := len(candles) - oosMonths
			if isEnd < 100 {
				isEnd = len(candles)
			}
			isFrom := candles[0].Date
			isTo := candles[isEnd-1].Date
			oosFrom := ""
			oosTo := ""
			if len(candles) > oosMonths && isEnd < len(candles) {
				oosFrom = candles[isEnd].Date
				oosTo = candles[len(candles)-1].Date
			}

			isCandles := candles[:isEnd]
			result := stg.AnalyzeWithRules(isCandles, rules, settings)

			// 결정성 검증 (2회 실행 비교 — W9-1에서만 필수, baseline은 시간 절약 가능하나 보존)
			result2 := stg.AnalyzeWithRules(isCandles, rules, settings)
			if len(result.Positions) != len(result2.Positions) {
				fmt.Printf("[baseline] %s 결정성 검증 실패: %d vs %d\n", mkt, len(result.Positions), len(result2.Positions))
			}

			// 포지션을 전략별로 집계
			stratMap := make(map[string][]box.TradePosition)
			for _, p := range result.Positions {
				if p.IsActive || len(p.SellExecutions) == 0 {
					continue
				}
				stratMap[p.StrategyName] = append(stratMap[p.StrategyName], *p)
			}

			var rows []BaselineStratRow
			for strat, trades := range stratMap {
				row := computeBaselineStats(mkt, strat, trades)
				rows = append(rows, row)
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

			fmt.Printf("[baseline] %s 완료: IS %s~%s, OOS %s~%s, 청산완료 포지션 %d건\n",
				mkt, isFrom, isTo, oosFrom, oosTo, len(result.Positions))
		}(market)
	}
	wg.Wait()

	output := BaselineOutput{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Strategy:    stratPath,
		Period: EdgePeriod{
			IsFrom:  globalIsFrom,
			IsTo:    globalIsTo,
			OOSFrom: globalOOSFrom,
			OOSTo:   globalOOSTo,
		},
		Results: allRows,
	}

	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[baseline] 디렉토리 생성 실패: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[baseline] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[baseline] 파일 쓰기 실패: %v\n", err)
		return
	}
	fmt.Printf("[baseline] 완료: %s (룰×마켓 결과 %d건)\n", outputPath, len(allRows))
}

func computeBaselineStats(market, strategy string, trades []box.TradePosition) BaselineStratRow {
	row := BaselineStratRow{Market: market, Strategy: strategy}
	var grossWin, grossLoss, cumRet float64
	// 복리 자본곡선 기반 MDD: (peak - equity) / peak — 0~100% 범위 보장
	equity, equityPeak, maxDD := 1.0, 1.0, 0.0

	// by_year 집계용: year → (wins, grossWin, grossLoss, cumRet, count)
	type yearBucket struct {
		wins, count                 int
		grossWin, grossLoss, cumRet float64
	}
	yearMap := make(map[int]*yearBucket)

	for _, t := range trades {
		net := t.NetReturnRate
		if net == 0 {
			net = t.ReturnRate
		}
		cumRet += net
		if net > 0 {
			row.WinRate++
			grossWin += net
		} else {
			grossLoss += -net
		}
		equity *= (1.0 + net/100.0)
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
		var sellDate, sellTime, sellReason string
		if len(t.SellExecutions) > 0 {
			last := t.SellExecutions[len(t.SellExecutions)-1]
			sellDate = last.SellDate
			sellTime = last.ExecutionTime
			sellReason = last.SellReason
		}
		row.Trades = append(row.Trades, BaselineTradeRow{
			Market:       market,
			StrategyName: strategy,
			BuyDate:      t.BuyDate,
			BuyTime:      t.BuyTime,
			BuyPrice:     t.BuyPriceOrigin,
			SellDate:     sellDate,
			SellTime:     sellTime,
			SellReason:   sellReason,
			NetReturn:    net,
		})

		// by_year 집계 — BuyDate 형식: "YYYYMMDD"
		year := 0
		if len(t.BuyDate) >= 4 {
			fmt.Sscanf(t.BuyDate[:4], "%d", &year)
		}
		if year > 0 {
			b, ok := yearMap[year]
			if !ok {
				b = &yearBucket{}
				yearMap[year] = b
			}
			b.count++
			b.cumRet += net
			if net > 0 {
				b.wins++
				b.grossWin += net
			} else {
				b.grossLoss += -net
			}
		}
	}
	n := float64(len(trades))
	row.TradeCount = len(trades)
	if n > 0 {
		row.WinRate = row.WinRate / n * 100
		row.AvgNetReturn = cumRet / n
	}
	if grossLoss > 0 {
		row.ProfitFactor = grossWin / grossLoss
	}
	row.MaxDrawdown = maxDD * 100

	// by_year 정렬 후 추가
	years := make([]int, 0, len(yearMap))
	for y := range yearMap {
		years = append(years, y)
	}
	// 정렬 (삽입 정렬 — 연도 수 소량)
	for i := 1; i < len(years); i++ {
		for j := i; j > 0 && years[j] < years[j-1]; j-- {
			years[j], years[j-1] = years[j-1], years[j]
		}
	}
	for _, y := range years {
		b := yearMap[y]
		yr := ByYearRow{Year: y, Trades: b.count}
		if b.count >= 5 {
			wr := float64(b.wins) / float64(b.count) * 100
			avg := b.cumRet / float64(b.count)
			yr.WinRate = &wr
			yr.AvgNetReturn = &avg
			if b.grossLoss > 0 {
				pf := b.grossWin / b.grossLoss
				yr.ProfitFactor = &pf
			}
		}
		row.ByYear = append(row.ByYear, yr)
	}

	return row
}
