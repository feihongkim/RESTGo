package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"RESTGo/study"
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
		"display_days": 180,
		"stocks":       jsonItems,
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
