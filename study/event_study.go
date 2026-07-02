package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// strategy1.yaml 8 전략(REST1) + REST2/FollowUp 신호의 사후 가격 분포 측정.
// 2026-06-17 사용자 요청 — hannam DB 16년치 데이터로 전략별 이벤트 스터디.
//
// 각 전략명별로 발화 시점 t에서 t+1/5/10/20일 수익률 측정.
// 매수 룰 평가만 사용 — 매도 시스템(sell_strategy1.yaml) 비활성화 (LoadSellStrategyFile 안 호출).

const (
	sesRoundTripCost = 0.004 // 0.4% 왕복비용 (한국주)
	sesDaysFetch     = 4500  // 종목당 최대 일봉수 (약 18년)
)

var sesHorizons = []int{1, 5, 10, 20}

// SESStratStats 는 전략별 호라이즌별 통계
type SESStratStats struct {
	Strategy      string  `json:"strategy"`
	Horizon       int     `json:"horizon"`
	FireCount     int     `json:"fire_count"`
	MeanNetReturn float64 `json:"mean_net_return_pct"`
	MedianNet     float64 `json:"median_net_return_pct"`
	StdNet        float64 `json:"std_net_return_pct"`
	WinRate       float64 `json:"win_rate"`
	BaselineMean  float64 `json:"baseline_mean_net_pct"`
	Edge          float64 `json:"edge_pct"`
	TStat         float64 `json:"t_stat"`
	P95           float64 `json:"p95"`
	P99           float64 `json:"p99"`
	P5            float64 `json:"p5"`
	P1            float64 `json:"p1"`
}

// SESYearStats 는 전략별 연도별 통계
type SESYearStats struct {
	Strategy  string  `json:"strategy"`
	Year      int     `json:"year"`
	Horizon   int     `json:"horizon"`
	FireCount int     `json:"fire_count"`
	Mean      float64 `json:"mean_net_return_pct"`
	WinRate   float64 `json:"win_rate"`
}

// SESSellStats 는 5-Path 매도 시점 기준 전략별 통계 (--with-sell 모드 전용)
type SESSellStats struct {
	Strategy      string  `json:"strategy"`
	Bucket        string  `json:"bucket"` // "ALL" 또는 SellReason 카테고리
	FireCount     int     `json:"fire_count"`
	MeanNetReturn float64 `json:"mean_net_return_pct"`
	MedianNet     float64 `json:"median_net_return_pct"`
	StdNet        float64 `json:"std_net_return_pct"`
	WinRate       float64 `json:"win_rate"`
	AvgHoldBars   float64 `json:"avg_holding_bars"`
}

// SESSellByYear — 매도 알파의 연도별 분해 (매수 시점 연도 기준)
type SESSellByYear struct {
	Strategy      string  `json:"strategy"`
	Year          int     `json:"year"`
	FireCount     int     `json:"fire_count"`
	MeanNetReturn float64 `json:"mean_net_return_pct"`
	WinRate       float64 `json:"win_rate"`
}

// SESTradeRow 는 발화 trade 1건
type SESTradeRow struct {
	Strategy string  `json:"strategy"`
	Shcode   string  `json:"shcode"`
	BuyDate  string  `json:"buy_date"`
	Year     int     `json:"year"`
	NetH1    float64 `json:"net_h1"`
	NetH5    float64 `json:"net_h5"`
	NetH10   float64 `json:"net_h10"`
	NetH20   float64 `json:"net_h20"`
	// sell 모드일 때만
	SellDate   string  `json:"sell_date,omitempty"`
	SellReason string  `json:"sell_reason,omitempty"`
	NetSell    float64 `json:"net_sell,omitempty"`
}

// StrategyEventStudyOutput 는 전체 결과
type StrategyEventStudyOutput struct {
	GeneratedAt   string               `json:"generated_at"`
	Strategy      string               `json:"strategy_yaml"`
	SellMode      bool                 `json:"sell_mode"`
	StockCount    int                  `json:"stock_count"`
	CandleCount   int64                `json:"total_candle_count"`
	Horizons      []int                `json:"horizons"`
	RoundTripCost float64              `json:"round_trip_cost_pct"`
	Stats         []SESStratStats      `json:"stats"`
	ByYear        []SESYearStats       `json:"by_year"`
	BaselineByH   map[int]float64      `json:"baseline_mean_by_horizon"`
	NetReturnDist map[string][]float64 `json:"net_return_distribution"`
	Trades        []SESTradeRow        `json:"trades"`                  // 상세 trade (최대 ~5000건/전략)
	SellStats     []SESSellStats       `json:"sell_stats,omitempty"`    // 5-Path 매도 통계 (--with-sell 전용)
	SellByYear    []SESSellByYear      `json:"sell_by_year,omitempty"`  // 매도 알파 연도별 분해
}

// HandleStrategyEventStudy 는 "stock strategy_study [--with-sell] [--upbit-15m|--upbit-30m] [yaml] [out]" 명령 진입점.
// --with-sell: sell_strategy1.yaml 활성화 (5-Path 매도)
// --upbit-15m: TUF DB Upbit 4 마켓 15m 캔들
// --upbit-30m: TUF DB Upbit 15m → 30m 집계
func HandleStrategyEventStudy(args []string) {
	withSell := false
	mode := "hannam"
	for len(args) > 0 && (args[0] == "--with-sell" || args[0] == "--upbit-15m" || args[0] == "--upbit-30m" || args[0] == "--foreign-us" || args[0] == "--foreign-jp" || args[0] == "--foreign-cn" || args[0] == "--foreign-hk") {
		switch args[0] {
		case "--with-sell":
			withSell = true
		case "--upbit-15m":
			mode = "upbit15m"
		case "--upbit-30m":
			mode = "upbit30m"
		case "--foreign-us":
			mode = "foreign-us"
		case "--foreign-jp":
			mode = "foreign-jp"
		case "--foreign-cn":
			mode = "foreign-cn"
		case "--foreign-hk":
			mode = "foreign-hk"
		}
		args = args[1:]
	}
	stratPath := "rules/strategy1.yaml"
	outputPath := "zpicture/strategy1_event_study.json"
	if len(args) >= 1 && args[0] != "" {
		stratPath = args[0]
	}
	if len(args) >= 2 && args[1] != "" {
		outputPath = args[1]
	}
	runStrategyEventStudy(stratPath, outputPath, withSell, mode)
}

func runStrategyEventStudy(stratPath, outputPath string, withSell bool, mode string) {
	dbName := "han"
	if mode == "upbit15m" || mode == "upbit30m" {
		dbName = "tuf"
	} else if mode == "foreign-us" || mode == "foreign-jp" || mode == "foreign-cn" || mode == "foreign-hk" {
		dbName = "KIS2"
	}
	db, err := console.MsConn.GetDB(dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ses] %s DB 연결 실패: %v\n", dbName, err)
		return
	}

	if err := stg.LoadStrategy(stratPath); err != nil {
		fmt.Fprintf(os.Stderr, "[ses] 전략 YAML 로드 실패: %v\n", err)
		return
	}
	if withSell {
		sellPath := "rules/sell_strategy1.yaml"
		if p := os.Getenv("RESTGO_SELL_RULES"); p != "" {
			sellPath = p
		}
		if err := stg.LoadSellStrategyFile(sellPath); err != nil {
			fmt.Fprintf(os.Stderr, "[ses] 매도 룰 로드 실패: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[ses] 매도 룰 로드: %s\n", sellPath)
		}
	}
	// withSell=false면 매도 시스템 비활성 (activeSellSettings nil)

	var stocks []string
	if mode == "upbit15m" || mode == "upbit30m" {
		stocks = []string{"KRW-BTC", "KRW-ETH", "KRW-XRP", "KRW-SOL"}
	} else if mode == "foreign-us" || mode == "foreign-jp" || mode == "foreign-cn" || mode == "foreign-hk" {
		var prefixes []string
		switch mode {
		case "foreign-us":
			prefixes = []string{"DNY", "DNA"} // NYSE + NASDAQ
		case "foreign-jp":
			prefixes = []string{"DTS"}
		case "foreign-cn":
			prefixes = []string{"DSZ", "DSH"}
		case "foreign-hk":
			prefixes = []string{"DHK"}
		}
		stocks, err = box.FetchForeignStockList(db, prefixes)
		if err != nil || len(stocks) == 0 {
			fmt.Fprintf(os.Stderr, "[ses] foreign 종목 목록 조회 실패: %v\n", err)
			return
		}
	} else {
		stocks, err = box.FetchHannamStockList(db)
		if err != nil || len(stocks) == 0 {
			fmt.Fprintf(os.Stderr, "[ses] hannam 종목 목록 조회 실패: %v\n", err)
			return
		}
	}
	fmt.Printf("[ses] 모드: %s  전략: %s  종목: %d개  호라이즌: %v  비용: %.1f%%\n",
		mode, stratPath, len(stocks), sesHorizons, sesRoundTripCost*100)

	// 전략명·호라이즌 누적
	type stratKey struct {
		Name string
		H    int
	}
	type yearKey struct {
		Name string
		H    int
		Y    int
	}
	// 5-Path 매도 누적 — strategy x bucket(ALL/Category) → (NetReturn, HoldBars)
	type sellKey struct {
		Name   string
		Bucket string
	}
	type sellRow struct {
		Net  float64
		Hold int
	}
	type sellYearKey struct {
		Name string
		Year int
	}
	var mu sync.Mutex
	stratAcc := make(map[stratKey][]float64)
	yearAcc := make(map[yearKey][]float64)
	sellAcc := make(map[sellKey][]sellRow)
	sellYearAcc := make(map[sellYearKey][]float64)
	tradesAcc := make(map[string][]SESTradeRow) // 전략별 개별 trade 기록
	// 베이스라인 (모든 봉 t+H 수익률 — 호라이즌별)
	baseAcc := make(map[int][]float64)
	for _, h := range sesHorizons {
		baseAcc[h] = nil
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)
	var processed int32
	var totalCandles int64

	for _, sh := range stocks {
		wg.Add(1)
		go func(shcode string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var candles []*box.Candle
			switch mode {
			case "upbit15m":
				candles, err = box.FetchUpbitCandles15m(db, shcode, 400000)
			case "upbit30m":
				c15, e := box.FetchUpbitCandles15m(db, shcode, 400000)
				if e == nil {
					candles = aggregate15mTo30m(c15)
				}
				err = e
			case "foreign-us", "foreign-jp", "foreign-cn", "foreign-hk":
				candles, err = box.FetchCandlesForeign(db, shcode, sesDaysFetch)
			default:
				candles, err = box.FetchCandlesHannam(db, shcode, sesDaysFetch)
			}
			if err != nil || len(candles) < 60+sesHorizons[len(sesHorizons)-1]+2 {
				atomic.AddInt32(&processed, 1)
				return
			}
			indicator.PrepareCandles(candles)

			// stg.Analyze로 매수 신호 추출 (매도 평가는 activeSellSettings nil이라 자동 스킵)
			result := stg.Analyze(candles, stg.GetActiveSettings())

			// 종목별 로컬 누적
			localStrat := make(map[stratKey][]float64)
			localYear := make(map[yearKey][]float64)
			localBase := make(map[int][]float64)
			for _, h := range sesHorizons {
				localBase[h] = nil
			}

			// 베이스라인: 모든 유효 봉의 t+H 수익률 (5봉 워밍업 후)
			endIdx := len(candles) - sesHorizons[len(sesHorizons)-1] - 1
			for i := 5; i < endIdx; i++ {
				entry := candles[i].CloseOrigin
				if entry <= 0 {
					continue
				}
				for _, h := range sesHorizons {
					if i+h >= len(candles) {
						continue
					}
					exit := candles[i+h].CloseOrigin
					if exit <= 0 {
						continue
					}
					ret := (exit-entry)/entry*100.0 - sesRoundTripCost*100.0
					localBase[h] = append(localBase[h], ret)
				}
			}

			// 발화 신호 — result.BuySignals에서 추출
			localTrades := make(map[string][]SESTradeRow)
			for _, sig := range result.BuySignals {
				i := sig.Position
				if i < 5 || i >= len(candles)-sesHorizons[len(sesHorizons)-1]-1 {
					continue
				}
				entry := candles[i].CloseOrigin
				if entry <= 0 {
					continue
				}
				year := 0
				if len(candles[i].Date) >= 4 {
					fmt.Sscanf(candles[i].Date[:4], "%d", &year)
				}
				row := SESTradeRow{
					Strategy: sig.Reason,
					Shcode:   shcode,
					BuyDate:  candles[i].Date,
					Year:     year,
				}
				for _, h := range sesHorizons {
					if i+h >= len(candles) {
						continue
					}
					exit := candles[i+h].CloseOrigin
					if exit <= 0 {
						continue
					}
					ret := (exit-entry)/entry*100.0 - sesRoundTripCost*100.0
					localStrat[stratKey{sig.Reason, h}] = append(localStrat[stratKey{sig.Reason, h}], ret)
					if year > 0 {
						localYear[yearKey{sig.Reason, h, year}] = append(localYear[yearKey{sig.Reason, h, year}], ret)
					}
					switch h {
					case 1:
						row.NetH1 = ret
					case 5:
						row.NetH5 = ret
					case 10:
						row.NetH10 = ret
					case 20:
						row.NetH20 = ret
					}
				}
				localTrades[sig.Reason] = append(localTrades[sig.Reason], row)
			}

			// 5-Path 매도 누적 (withSell 모드 + 청산된 포지션만)
			localSell := make(map[sellKey][]sellRow)
			localSellYear := make(map[sellYearKey][]float64)
			if withSell {
				for _, pos := range result.Positions {
					if pos == nil || pos.SellPosition < 0 || pos.SellPosition <= pos.BuyPosition {
						continue
					}
					hold := pos.SellPosition - pos.BuyPosition
					// NetReturnRate, ReturnRate, PartialReturnRate 모두 이미 percent 단위
					net := pos.NetReturnRate
					if net == 0 && pos.ReturnRate != 0 {
						net = pos.ReturnRate - sesRoundTripCost*100.0
					}
					// 카테고리 추론 (box/sell_types.go Category 로직 인라인)
					bucket := "Technical"
					r := pos.SellReason
					switch {
					case stringContains(r, "Profit"):
						bucket = "ProfitTaking"
					case stringContains(r, "StopLoss"), stringContains(r, "Fail"), stringContains(r, "Breakdown"), stringContains(r, "DeadCross"):
						bucket = "LossCutting"
					case stringContains(r, "Period"):
						bucket = "AutoLiquidation"
					}
					row := sellRow{Net: net, Hold: hold}
					localSell[sellKey{pos.StrategyName, "ALL"}] = append(localSell[sellKey{pos.StrategyName, "ALL"}], row)
					localSell[sellKey{pos.StrategyName, bucket}] = append(localSell[sellKey{pos.StrategyName, bucket}], row)
					// raw SellReason 도 별도 bucket 으로 누적 (cross-strategy 집계용 "_ALL_")
					reason := pos.SellReason
					if reason == "" {
						reason = "(unknown)"
					}
					localSell[sellKey{"_ALL_", "reason:" + reason}] = append(localSell[sellKey{"_ALL_", "reason:" + reason}], row)

					// 매도 알파의 연도별 분해 (매수 시점 연도 기준)
					if len(pos.BuyDate) >= 4 {
						y := 0
						fmt.Sscanf(pos.BuyDate[:4], "%d", &y)
						if y > 0 {
							localSellYear[sellYearKey{pos.StrategyName, y}] = append(localSellYear[sellYearKey{pos.StrategyName, y}], net)
							localSellYear[sellYearKey{"_ALL_", y}] = append(localSellYear[sellYearKey{"_ALL_", y}], net)
						}
					}
				}
			}

			mu.Lock()
			for k, rs := range localStrat {
				stratAcc[k] = append(stratAcc[k], rs...)
			}
			for k, rs := range localYear {
				yearAcc[k] = append(yearAcc[k], rs...)
			}
			for k, rs := range localSell {
				sellAcc[k] = append(sellAcc[k], rs...)
			}
			for k, rs := range localSellYear {
				sellYearAcc[k] = append(sellYearAcc[k], rs...)
			}
			for strat, rows := range localTrades {
				tradesAcc[strat] = append(tradesAcc[strat], rows...)
			}
			for _, h := range sesHorizons {
				baseAcc[h] = append(baseAcc[h], localBase[h]...)
			}
			atomic.AddInt64(&totalCandles, int64(len(candles)))
			mu.Unlock()

			n := atomic.AddInt32(&processed, 1)
			if n%100 == 0 {
				fmt.Printf("[ses] %d/%d 처리...\n", n, len(stocks))
			}
		}(sh)
	}
	wg.Wait()

	// 결과 집계
	out := StrategyEventStudyOutput{
		GeneratedAt:   time.Now().Format(time.RFC3339),
		Strategy:      stratPath,
		SellMode:      withSell,
		StockCount:    len(stocks),
		CandleCount:   totalCandles,
		Horizons:      sesHorizons,
		RoundTripCost: sesRoundTripCost * 100,
		BaselineByH:   make(map[int]float64),
		NetReturnDist: make(map[string][]float64),
	}

	// 베이스라인
	for _, h := range sesHorizons {
		if len(baseAcc[h]) > 0 {
			out.BaselineByH[h] = meanFloats(baseAcc[h])
		}
	}

	// 개별 trade 기록 (전략별 최대 5000건)
	for strat, rows := range tradesAcc {
		if len(rows) > 5000 {
			rows = rows[:5000]
		}
		_ = strat
		out.Trades = append(out.Trades, rows...)
	}

	// 고유 전략명 목록
	stratSet := make(map[string]bool)
	for k := range stratAcc {
		stratSet[k.Name] = true
	}
	strategies := make([]string, 0, len(stratSet))
	for s := range stratSet {
		strategies = append(strategies, s)
	}
	sortStrings(strategies)

	for _, name := range strategies {
		for _, h := range sesHorizons {
			rets := stratAcc[stratKey{name, h}]
			if len(rets) == 0 {
				continue
			}
			stat := SESStratStats{
				Strategy:  name,
				Horizon:   h,
				FireCount: len(rets),
			}
			stat.MeanNetReturn = meanFloats(rets)
			stat.MedianNet = medianFloats(rets)
			stat.StdNet = stdFloats(rets)
			stat.BaselineMean = out.BaselineByH[h]
			stat.Edge = stat.MeanNetReturn - stat.BaselineMean
			stat.TStat = welchTStatFloats(rets, baseAcc[h])
			wins := 0
			for _, r := range rets {
				if r > 0 {
					wins++
				}
			}
			stat.WinRate = float64(wins) / float64(len(rets)) * 100
			// percentile
			sorted := make([]float64, len(rets))
			copy(sorted, rets)
			sortFloats(sorted)
			stat.P95 = percentile(sorted, 95)
			stat.P99 = percentile(sorted, 99)
			stat.P5 = percentile(sorted, 5)
			stat.P1 = percentile(sorted, 1)
			out.Stats = append(out.Stats, stat)
			fmt.Printf("[ses] %s h=%d  fire=%d  mean=%+.3f%%  median=%+.3f%%  win=%.1f%%  edge=%+.3f%%p  t=%.2f\n",
				name, h, stat.FireCount, stat.MeanNetReturn, stat.MedianNet, stat.WinRate, stat.Edge, stat.TStat)
			// raw 분포 저장 (최대 30,000 샘플)
			key := fmt.Sprintf("%s_h%d", name, h)
			if len(rets) > 30000 {
				out.NetReturnDist[key] = rets[:30000]
			} else {
				out.NetReturnDist[key] = rets
			}
		}
	}

	// by_year 집계 (5건 미만 스킵)
	for k, rs := range yearAcc {
		if len(rs) < 5 {
			continue
		}
		wins := 0
		for _, r := range rs {
			if r > 0 {
				wins++
			}
		}
		out.ByYear = append(out.ByYear, SESYearStats{
			Strategy:  k.Name,
			Year:      k.Y,
			Horizon:   k.H,
			FireCount: len(rs),
			Mean:      meanFloats(rs),
			WinRate:   float64(wins) / float64(len(rs)) * 100,
		})
	}

	// 5-Path 매도 통계 집계 (withSell 모드)
	if withSell && len(sellAcc) > 0 {
		for k, rows := range sellAcc {
			if len(rows) == 0 {
				continue
			}
			rets := make([]float64, len(rows))
			holdSum := 0
			wins := 0
			for i, r := range rows {
				rets[i] = r.Net
				holdSum += r.Hold
				if r.Net > 0 {
					wins++
				}
			}
			ss := SESSellStats{
				Strategy:      k.Name,
				Bucket:        k.Bucket,
				FireCount:     len(rows),
				MeanNetReturn: meanFloats(rets),
				MedianNet:     medianFloats(rets),
				StdNet:        stdFloats(rets),
				WinRate:       float64(wins) / float64(len(rows)) * 100,
				AvgHoldBars:   float64(holdSum) / float64(len(rows)),
			}
			out.SellStats = append(out.SellStats, ss)
			if k.Bucket == "ALL" {
				fmt.Printf("[ses-sell] %s ALL  fire=%d  mean=%+.3f%%  win=%.1f%%  avgHold=%.1f\n",
					k.Name, ss.FireCount, ss.MeanNetReturn, ss.WinRate, ss.AvgHoldBars)
			}
		}
	}

	// 매도 알파 연도별 집계
	if withSell {
		for k, rets := range sellYearAcc {
			if len(rets) < 3 {
				continue
			}
			wins := 0
			for _, r := range rets {
				if r > 0 {
					wins++
				}
			}
			out.SellByYear = append(out.SellByYear, SESSellByYear{
				Strategy:      k.Name,
				Year:          k.Year,
				FireCount:     len(rets),
				MeanNetReturn: meanFloats(rets),
				WinRate:       float64(wins) / float64(len(rets)) * 100,
			})
		}
	}

	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[ses] 디렉토리 생성 실패: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ses] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[ses] 파일 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[ses] 완료: %s (전략 %d개)\n", outputPath, len(strategies))
}

// ── 헬퍼 ─────────────────────────────────────────

func stringContains(s, sub string) bool {
	if sub == "" {
		return true
	}
	n, m := len(s), len(sub)
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return true
		}
	}
	return false
}
