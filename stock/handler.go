package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"RESTGo/study"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	strategyPath     = "rules/strategy1.yaml"
	sellStrategyPath = "rules/sell_default.yaml"
)

// buyRulesPath 는 매수 전략 YAML 경로를 반환한다.
// RESTGO_BUY_RULES 환경변수로 대안 전략 파일(예: rules/buy_indicator.yaml)을 지정할 수 있다.
func buyRulesPath() string {
	if p := os.Getenv("RESTGO_BUY_RULES"); p != "" {
		return p
	}
	return strategyPath
}

// sellRulesPath 는 매도 전략 YAML 경로를 반환한다.
// RESTGO_SELL_RULES 환경변수로 대안 전략 파일(예: rules/sell_positive_only_mh25.yaml)을 지정할 수 있다.
func sellRulesPath() string {
	if p := os.Getenv("RESTGO_SELL_RULES"); p != "" {
		return p
	}
	return sellStrategyPath
}

func Handle(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법:")
		fmt.Println("  ./RESTGo stock analyze <종목코드> [일수=250]")
		fmt.Println("  ./RESTGo stock batch [일수=250] [출력경로=zpicture/batch_signals.json]")
		fmt.Println("  ./RESTGo stock gridtest <grid_yaml> [output_json]")
		fmt.Println("  ./RESTGo stock edgetest [markets_csv] [output_json]")
		return
	}

	switch args[0] {
	case "analyze": // 단일 종목 Box/매수/매도 분석 → 신호·포지션 콘솔 출력
		handleAnalyze(args[1:])
	case "candlesjson": // 캔들을 boxcalc 입력 JSON으로 출력 (boxcalc 정합 검증·차트 패리티용)
		handleCandlesJSON(args[1:])
	case "batch": // 전 종목 배치 분석 → zpicture/batch_signals.json 저장
		handleBatch(args[1:])
	case "gridtest": // 전략 파라미터 그리드 서치 백테스트
		study.HandleGridTest(args[1:])
	case "edgetest": // 돌파 엣지(우위) 검증 백테스트
		study.HandleEdgeTest(args[1:])
	case "baseline": // 베이스라인 전략 백테스트
		study.HandleBaselineBacktest(args[1:])
	case "walkforward": // 워크포워드(시계열 분할) 검증
		study.HandleWalkForward(args[1:])
	case "pairtest": // 페어 트레이딩 검증
		study.HandlePairTest(args[1:])
	case "baseline30m": // 30분봉 타임프레임 베이스라인 백테스트
		study.HandleBaseline30m(args[1:])
	case "breakdown_study": // 돌파/이탈/회복 이벤트 사후 분석
		study.HandleBreakdownStudy(args[1:])
	case "strategy_study": // YAML 전략의 매수/매도 이벤트 스터디
		study.HandleStrategyEventStudy(args[1:])
	case "wbottom_scan": // 전 종목에서 W바텀(캔들 %B) 패턴 예시 수집
		study.HandleWBottomScan(args[1:])
	case "miiib_scan": // 전 종목에서 MIIIb_WBottomBox(Box W바텀) 신호 예시 수집
		study.HandleMIIIbScan(args[1:])
	case "wdefbox_scan": // W패턴+DefBox 결합 신호 스캔
		study.HandleWDefBoxScan(args[1:])
	case "combined_scan": // WD+S1 합성 전략 신호 스캔
		study.HandleCombinedScan(args[1:])
	case "densitygate": // W중력 밀도 게이트 판정 (hannam StrategySignalDaily 기반)
		handleDensityGate(args[1:])
	case "mtop_scan": // 상방 M자 패턴 매도 신호 스캔 + 음의 엣지 측정
		study.HandleMTopScan(args[1:])
	case "hns_scan": // 해드앤숄더 패턴 매도 신호 스캔 + 음의 엣지 측정
		study.HandleHNSScan(args[1:])
	case "pullback_scan": // 20이평 눌림 돌파 매수 신호 스캔 + 양의 엣지 측정
		study.HandlePullbackScan(args[1:])
	case "wgc_scan": // W바텀(BB완화) × 골든크로스 임박 2×2 스캔 + strategy1 GC 후계산
		study.HandleWGCScan(args[1:])
	case "volume_wave_scan": // 선행 거래량→고가 횡보→2파동→MA20 3파동 독립 스캔
		study.HandleVolumeWaveScan(args[1:])
	case "volume_wave_matrix": // VW1/VW2 첫 눌림 next-open 진입 소거·IS/OOS 매트릭스
		study.HandleVolumeWaveMatrix(args[1:])
	case "volume_wave_charts": // 선정 VW2 첫 눌림 전략의 OOS P90/P50/P10 차트 데이터
		study.HandleVolumeWaveChartSamples(args[1:])
	case "volume_wave_box_study": // 고가놀이에 일반Box S-R-S를 결합한 소거 비교
		study.HandleVolumeWaveBoxStudy(args[1:])
	case "volume_wave_strict_study": // 누적거래량·OBV + 좁은 고가놀이 소거·매트릭스
		study.HandleVolumeWaveStrictStudy(args[1:])
	case "mainbox_retest_study": // DefBox 돌파→MainBox retest A/B/C 독립 전략 연구
		study.HandleMainBoxRetestStudy(args[1:])
	case "mainbox_retest_s1_study": // 최초 돌파가 strategy1 YAML에도 매칭된 cohort
		study.HandleMainBoxRetestS1Study(args[1:])
	case "mainbox_retest_refine_study": // 상승폭×C0/C1/C2×retest형×원 signal 분해
		study.HandleMainBoxRetestRefineStudy(args[1:])
	case "mainbox_retest_temporal": // 고정 20%+Touch C0/C1 시간 안정성 kill test
		study.HandleMainBoxRetestTemporal(args[1:])
	case "descending_trendline_study": // R-S-R-S 장기 바닥 하락추세선 돌파 연구
		study.HandleDescendingTrendlineStudy(args[1:])
	case "descending_trendline_ma_study": // apex + MA60/120 수렴·골든크로스 소거
		study.HandleDescendingTrendlineMAStudy(args[1:])
	case "descending_trendline_sideways_study": // apex + 돌파 직전 가격 횡보·압축 소거
		study.HandleDescendingTrendlineSidewaysStudy(args[1:])
	case "descending_trendline_charts": // 하락추세선 돌파 P90/P75/P50/P25/P10 차트 샘플 JSON 생성
		study.HandleDescendingTrendlineChartSamples(args[1:])
	case "trigger_scan": // 범용 트리거×조건 조합 전방수익률 측정 (일반·armed 트리거 모두)
		study.HandleTriggerScan(args[1:])
	case "listen": // 실시간 큐 소비 모드 — 가상 금일봉 250봉 수신 → 즉시 신호 평가 (2026-07-09)
		handleListen(args[1:])
	case "feed": // 발신 모드 — KIS2 일봉을 발신측 스키마로 큐에 발행 (셀프 테스트·패리티 검증용)
		handleFeed(args[1:])
	case "paper_wd": // B슬리브 WD Paper 트레이딩 — 4국(3시장) 일일 스캔 + 알림 + 원장
		HandlePaperWD(args[1:])
	case "paper_wd_report": // B슬리브 WD Paper 월간 리포트 (비용 차감 통계)
		HandlePaperWDReport(args[1:])
	default:
		fmt.Printf("알 수 없는 stock 명령: %s\n", args[0])
		fmt.Println("사용법:")
		fmt.Println("  ./RESTGo stock analyze <종목코드> [일수=250]")
		fmt.Println("  ./RESTGo stock batch [일수=250] [출력경로=zpicture/batch_signals.json]")
		fmt.Println("  ./RESTGo stock gridtest <grid_yaml> [output_json]")
		fmt.Println("  ./RESTGo stock edgetest [markets_csv] [output_json]")
		fmt.Println("  ./RESTGo stock baseline [markets_csv] [output_json] [strategy_yaml]")
		fmt.Println("  ./RESTGo stock walkforward <market> [output_json] [strategy_yaml]")
		fmt.Println("  ./RESTGo stock volume_wave_scan [--hannam|--foreign-*] [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock volume_wave_matrix [--max N] [--candles N] [--out path] [--oos-date YYYYMMDD]")
		fmt.Println("  ./RESTGo stock volume_wave_charts [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock volume_wave_box_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock volume_wave_strict_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock mainbox_retest_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock mainbox_retest_s1_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock mainbox_retest_refine_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock mainbox_retest_temporal [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock descending_trendline_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock descending_trendline_ma_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock descending_trendline_sideways_study [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock descending_trendline_charts [--max N] [--candles N] [--out path]")
		fmt.Println("  ./RESTGo stock paper_wd [--date YYYYMMDD]")
		fmt.Println("  ./RESTGo stock paper_wd_report [--month YYYYMM]")
	}
}

// handleDensityGate 는 "stock densitygate [일자=오늘] [overlay_yaml]" — 밀도 게이트 판정 출력.
// 신호 이력은 hannam DB StrategySignalDaily에서 로드. 설정 기본값: rules/overlay_wdefbox.yaml
// (RESTGO_OVERLAY_RULES 환경변수로 교체 가능).
func handleDensityGate(args []string) {
	date := time.Now().Format("20060102")
	if len(args) >= 1 && args[0] != "" {
		if _, err := time.Parse("20060102", args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "오류: 일자 형식은 YYYYMMDD (%s)\n", args[0])
			return
		}
		date = args[0]
	}
	cfgPath := "rules/overlay_wdefbox.yaml"
	if p := os.Getenv("RESTGO_OVERLAY_RULES"); p != "" {
		cfgPath = p
	}
	if len(args) >= 2 && args[1] != "" {
		cfgPath = args[1]
	}

	cfg, err := stg.LoadOverlayConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: han DB 연결 실패: %v\n", err)
		return
	}
	history, err := stg.FetchSignalDailyCounts(db, cfg.Strategies)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}
	gate, err := stg.NewDensityGate(cfg.GateConfig(), history)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}
	dec, err := gate.Evaluate(date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	fmt.Printf("[densitygate] 설정: %s  전략: %s  이력: %d일\n",
		cfgPath, strings.Join(cfg.Strategies, ","), len(history))
	fmt.Printf("[densitygate] 일자 %s  밀도(%d일) %d  임계값(q%.2f/%d년, 표본 %d) %d\n",
		dec.Date, cfg.WindowDays, dec.Density, cfg.Quantile, cfg.LookbackYears, dec.HistoryDays, dec.Threshold)
	if dec.Pass {
		fmt.Printf("[densitygate] 판정: PASS — W신호 진입 허용, 신호당 제안 비중 %.2f%% (equity/%d)\n",
			dec.SuggestedWeight*100, cfg.SizingK)
	} else {
		fmt.Printf("[densitygate] 판정: HOLD — 밀도 미달, W신호 진입 보류 (휴면은 설계임)\n")
	}
	if dec.HistoryDays == 0 {
		fmt.Println("[densitygate] 경고: 임계값 표본이 없음 — 신호 이력 백필/적재 상태를 확인하라")
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

	// 국내 일봉 소스: hannam (2026-07-09 사용자 지시 — KIS2 일봉 적재 지연으로 전환)
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: han DB 연결 실패: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[%s] 캔들 조회 중...\n", console.GenerateTimestampedString())
	candles, err := box.FetchCandlesHannam(db, shcode, days)
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
	_ = stg.LoadStrategy(buyRulesPath())
	if err := stg.LoadSellStrategyFile(sellRulesPath()); err != nil {
		fmt.Printf("[warn] 매도 룰 로드 실패 — 매도 평가 비활성: %v\n", err)
	}
	result := stg.Analyze(candles, stg.GetActiveSettings())

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

// fetchStockNames 는 KIS2 KospiCode에서 종목명 맵을 만든다 (이름 전용 — 시세는 hannam 사용).
// KIS2 접속 실패 시 빈 맵 반환 (이름 없이 진행).
func fetchStockNames() map[string]string {
	m := map[string]string{}
	db, err := console.MsConn.GetDB("KIS2")
	if err != nil {
		return m
	}
	rows, err := db.Query(`SELECT shrn_iscd, RTRIM(kor_isnm) FROM MS.KospiCode`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var c, n string
		if err := rows.Scan(&c, &n); err == nil && c != "" {
			m[c] = n
		}
	}
	return m
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
	batchOut := "zpicture/batch_signals.json" // 기본 출력 (py box_chart 등 기존 소비자 호환)
	if len(args) >= 2 && args[1] != "" {
		batchOut = args[1] // 일일 운용 배치 등 다중 전략 병행 시 충돌 방지용 (2026-07-07)
	}

	_ = stg.LoadStrategy(buyRulesPath())
	if err := stg.LoadSellStrategyFile(sellRulesPath()); err != nil {
		fmt.Printf("[warn] 매도 룰 로드 실패 — 매도 평가 비활성: %v\n", err)
	}

	// 국내 일봉 소스: hannam (2026-07-09 사용자 지시). 종목명은 KIS2 KospiCode에서 보조 조회
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: han DB 연결 실패: %v\n", err)
		os.Exit(1)
	}

	codes, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: 종목 조회 실패: %v\n", err)
		os.Exit(1)
	}
	names := fetchStockNames()

	type stockInfo struct{ Shcode, Hname string }
	stocks := make([]stockInfo, 0, len(codes))
	for _, c := range codes {
		nm := names[c]
		if nm == "" {
			nm = c
		}
		stocks = append(stocks, stockInfo{c, nm})
	}

	fmt.Printf("[%s] 분석 대상: %d 종목  일수: %d\n", console.GenerateTimestampedString(), len(stocks), days)

	type sellEventJSON struct {
		BuyDate   string  `json:"buy_date"`
		SellDate  string  `json:"sell_date"`
		Reason    string  `json:"reason"`
		Weight    float64 `json:"weight"`
		NetReturn float64 `json:"net_return_pct"`
		Holding   int     `json:"holding_days"`
	}

	type resultItem struct {
		Shcode  string          `json:"shcode"`
		Hname   string          `json:"hname"`
		Signals []stg.BuySignal `json:"signals"`
		Sells   []sellEventJSON `json:"sells"`
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
		Shcode  string          `json:"shcode"`
		Hname   string          `json:"hname"`
		Signals []signalJSON    `json:"signals"`
		Sells   []sellEventJSON `json:"sells"` // 시뮬레이션 포지션의 매도 실행 (2026-07-07 일일 운용 매도 알림용)
	}

	var results []resultItem
	var mu sync.Mutex
	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup
	var processed int32
	// lastDates: 종목별 마지막 봉 날짜 분포. dataDate 산출에 쓴다.
	// 전 종목 최댓값을 쓰면 안 된다 — 일봉 적재는 종목코드 순으로 진행되므로,
	// 적재 중에는 선두 몇 종목이 전체를 대표해 버린다 (2026-07-30: 23/4298 종목만 최신).
	// 그 상태로 적재하면 StrategySignalDaily에 소수 종목 기준 count가 박히고,
	// 기존 행 보존 규약 때문에 나중에 데이터가 다 차도 덮어써지지 않는다.
	lastDates := map[string]int{}

	batchSettings := stg.GetActiveSettings()

	for _, s := range stocks {
		wg.Add(1)
		go func(s stockInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			candles, err := box.FetchCandlesHannam(db, s.Shcode, days)
			if err != nil || len(candles) < 6 {
				atomic.AddInt32(&processed, 1)
				return
			}
			indicator.PrepareCandles(candles)
			result := stg.Analyze(candles, batchSettings)

			if last := candles[len(candles)-1].Date; last != "" {
				mu.Lock()
				lastDates[last]++
				mu.Unlock()
			}

			n := atomic.AddInt32(&processed, 1)
			if n%100 == 0 {
				fmt.Printf("[batch] %d/%d 처리 중...\n", n, int32(len(stocks)))
			}

			// 매도 실행 수집 (시뮬레이션 포지션 — "신호대로 매수했다면"의 매도 알림)
			var sells []sellEventJSON
			for _, pos := range result.Positions {
				for _, ex := range pos.SellExecutions {
					sells = append(sells, sellEventJSON{
						BuyDate: pos.BuyDate, SellDate: ex.SellDate, Reason: ex.SellReason,
						Weight: ex.Weight, NetReturn: ex.NetPartialReturn, Holding: ex.HoldingDays,
					})
				}
			}
			if len(result.BuySignals) > 0 || len(sells) > 0 {
				mu.Lock()
				results = append(results, resultItem{s.Shcode, s.Hname, result.BuySignals, sells})
				mu.Unlock()
			}
		}(s)
	}
	wg.Wait()

	fmt.Printf("\n[%s] 매수 신호 종목: %d개\n", console.GenerateTimestampedString(), len(results))

	dataDate, dataCoverage, maxDate := resolveDataDate(lastDates, dataDateCoverageThreshold())
	if dataDate == "" {
		fmt.Printf("[%s] [경고] data_date 산출 불가 — 분석된 캔들이 없습니다\n", console.GenerateTimestampedString())
	} else {
		fmt.Printf("[%s] 기준일(data_date) %s  커버리지 %.1f%% (임계 %.0f%%)\n",
			console.GenerateTimestampedString(), dataDate, dataCoverage*100, dataDateCoverageThreshold()*100)
		if dataDate != maxDate {
			fmt.Printf("[%s] [경고] 일봉 적재 진행 중 — 최신 봉은 %s이나 커버리지 미달로 %s를 기준일로 사용합니다\n",
				console.GenerateTimestampedString(), maxDate, dataDate)
		}
	}

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
		jsonItems = append(jsonItems, resultItemJSON{r.Shcode, r.Hname, sigs, r.Sells})
	}

	output := map[string]interface{}{
		"generated_at": generatedAt,
		// data_date: 종목 커버리지가 임계 이상인 최신 봉 날짜 (달력 오늘도, 단순 최댓값도 아님)
		"data_date":          dataDate,
		"data_date_coverage": math.Round(dataCoverage*1000) / 1000,
		"data_date_max":      maxDate, // 참고용 — 적재 진행 중이면 data_date보다 앞선다
		"display_days":       180,
		"stocks":             jsonItems,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: JSON 직렬화 실패: %v\n", err)
		os.Exit(1)
	}

	outPath := batchOut
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

// handleCandlesJSON 은 캔들을 boxcalc 입력 JSON으로 출력한다.
// 용도: boxcalc ↔ stock analyze 정합 검증, makesql_chart 등 외부 소비자 패리티 테스트.
// console.Init() 로그가 stdout에 섞이므로 파이프 대신 --out 파일 경유를 권장:
//
//	./RESTGo stock candlesjson 005930 250 --out /tmp/c.json && ./boxcalc < /tmp/c.json
func handleCandlesJSON(args []string) {
	if len(args) < 1 {
		fmt.Println("사용법: ./RESTGo stock candlesjson <종목코드> [일수=250] [--out 경로]")
		return
	}
	shcode := args[0]
	days := 250
	outPath := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "--out" && i+1 < len(rest):
			outPath = rest[i+1]
			i++
		default:
			n, err := strconv.Atoi(rest[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(os.Stderr, "오류: 잘못된 인수: %s\n", rest[i])
				os.Exit(1)
			}
			days = n
		}
	}

	// 국내 일봉 소스: hannam (analyze와 동일 — 정합 검증이 목적이므로 반드시 같은 로더 사용)
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: han DB 연결 실패: %v\n", err)
		os.Exit(1)
	}
	candles, err := box.FetchCandlesHannam(db, shcode, days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}

	type candleJSON struct {
		Date   string  `json:"date"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	}
	out := struct {
		Shcode  string       `json:"shcode"`
		Candles []candleJSON `json:"candles"`
	}{Shcode: shcode, Candles: make([]candleJSON, 0, len(candles))}
	for _, c := range candles {
		out.Candles = append(out.Candles, candleJSON{
			Date: c.Date, Open: c.OpenOrigin, High: c.HighOrigin,
			Low: c.LowOrigin, Close: c.CloseOrigin, Volume: c.Volume,
		})
	}

	data, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: JSON 직렬화 실패: %v\n", err)
		os.Exit(1)
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "오류: 파일 저장 실패: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[%s] 저장 완료: %s (%d 캔들)\n", console.GenerateTimestampedString(), outPath, len(candles))
	} else {
		fmt.Println(string(data))
	}
}

// dataDateCoverageThreshold 는 data_date 판정에 요구하는 종목 커버리지 비율.
// RESTGO_DATA_DATE_COVERAGE로 조정 가능 (0 초과 1 이하). 기본 0.8.
func dataDateCoverageThreshold() float64 {
	if v := os.Getenv("RESTGO_DATA_DATE_COVERAGE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1 {
			return f
		}
		fmt.Fprintf(os.Stderr, "[warn] RESTGO_DATA_DATE_COVERAGE 무시 (0<x<=1 아님): %s\n", v)
	}
	return 0.8
}

// resolveDataDate 는 종목별 마지막 봉 날짜 분포에서 기준일을 고른다.
//
// 커버리지(D) = (마지막 봉이 D 이상인 종목 수) / (분석된 종목 수) 로 정의하고,
// 커버리지가 threshold 이상인 **가장 최근** 날짜를 반환한다. 커버리지는 D가 커질수록
// 단조 감소하므로 최신 날짜부터 내려오며 첫 충족 지점을 찾으면 된다.
//
// 일봉 적재가 종목코드 순으로 진행되는 동안에는 선두 소수 종목의 최신 날짜가
// 커버리지 미달로 걸러지고 직전 완전일이 선택된다. 적재가 끝나면 자연히 최신일로 넘어간다.
//
// 반환: (기준일, 그 날짜의 커버리지, 전체 최댓값). 입력이 비면 모두 zero value.
func resolveDataDate(lastDates map[string]int, threshold float64) (string, float64, string) {
	total := 0
	dates := make([]string, 0, len(lastDates))
	for d, n := range lastDates {
		dates = append(dates, d)
		total += n
	}
	if total == 0 {
		return "", 0, ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates))) // 날짜 문자열은 YYYYMMDD라 사전순 = 시간순

	cum := 0
	for _, d := range dates {
		cum += lastDates[d] // d 이상인 종목 누적 (내림차순 순회이므로)
		if cov := float64(cum) / float64(total); cov >= threshold {
			return d, cov, dates[0]
		}
	}
	// 임계 미달이면(분포가 극단적으로 흩어진 경우) 가장 오래된 날짜 — 전 종목이 커버하는 유일한 지점
	oldest := dates[len(dates)-1]
	return oldest, 1.0, dates[0]
}
