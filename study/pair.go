package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// 페어 트레이딩 long-only 변형 (Upbit short 불가):
// - z < -EntryK : A 저평가 → A 매수
// - z > +EntryK : B 저평가 → B 매수
// - 청산: |z| ≤ ExitZ (회귀) OR TimeExitBars 경과 OR |z| > StopZ (확장 손절)
// - 비용: FeeRate + SlippageRate 양쪽 차감 (1 leg only — long-only이므로)

// PairTradeRow 는 거래 1건 상세
type PairTradeRow struct {
	PairName    string  `json:"pair"`
	EntryDate   string  `json:"entry_date"`
	EntryTime   string  `json:"entry_time"`
	EntryZ      float64 `json:"entry_z"`
	BuySide     string  `json:"buy_side"`
	BuyPrice    float64 `json:"buy_price"`
	ExitDate    string  `json:"exit_date"`
	ExitTime    string  `json:"exit_time"`
	ExitZ       float64 `json:"exit_z"`
	ExitPrice   float64 `json:"exit_price"`
	ExitReason  string  `json:"exit_reason"`
	HoldingBars int     `json:"holding_bars"`
	NetReturn   float64 `json:"net_return_pct"`
}

// PairResult 는 페어별 종합 결과
type PairResult struct {
	PairName     string         `json:"pair"`
	MarketA      string         `json:"market_a"`
	MarketB      string         `json:"market_b"`
	PairedBars   int            `json:"paired_bars"`
	Correlation  float64        `json:"correlation"`
	ADFTStat     float64        `json:"adf_t_stat"`
	IsStationary bool           `json:"is_stationary"` // ADFTStat < -2.86
	TradeCount   int            `json:"trade_count"`
	WinRate      float64        `json:"win_rate"`
	AvgNetReturn float64        `json:"avg_net_return_pct"`
	ProfitFactor float64        `json:"profit_factor"`
	MaxDrawdown  float64        `json:"max_drawdown_pct"`
	ISFrom       string         `json:"is_from"`
	ISTo         string         `json:"is_to"`
	OOSFrom      string         `json:"oos_from"`
	OOSTo        string         `json:"oos_to"`
	BuyACount    int            `json:"buy_a_count"`
	BuyBCount    int            `json:"buy_b_count"`
	Trades       []PairTradeRow `json:"trades"`
}

// PairBaselineOutput 는 페어 베이스라인 전체 출력
type PairBaselineOutput struct {
	GeneratedAt  string       `json:"generated_at"`
	ZWindow      int          `json:"z_window"`
	EntryK       float64      `json:"entry_k"`
	ExitZ        float64      `json:"exit_z"`
	StopZ        float64      `json:"stop_z"`
	TimeExit     int          `json:"time_exit_bars"`
	FeeRate      float64      `json:"fee_rate"`
	SlippageRate float64      `json:"slippage_rate"`
	Pairs        []PairResult `json:"pairs"`
}

// 페어 트레이딩 기본 파라미터
const (
	pairZWindow      = 480 // 5일 롤링 (96×5)
	pairEntryK       = 2.0 // 진입 |z| ≥ 2
	pairExitZ        = 0.5 // 청산 |z| ≤ 0.5
	pairStopZ        = 4.0 // 손절 |z| ≥ 4
	pairTimeExitBars = 192 // 2일 (96×2)
	pairFeeRate      = 0.0005
	pairSlipRate     = 0.0005
)

// HandlePairTest 는 "stock pairtest" 명령 진입점
//
// 사용법:
//
//	./RESTGo stock pairtest <output_json>            — 4 마켓 6 페어 모두
//	./RESTGo stock pairtest <output_json> <A> <B>    — 단일 페어
func HandlePairTest(args []string) {
	outputPath := "zpicture/pair_baseline.json"
	if len(args) >= 1 && args[0] != "" {
		outputPath = args[0]
	}
	pairs := defaultPairs()
	if len(args) >= 3 {
		pairs = [][2]string{{args[1], args[2]}}
	}
	runPairBaseline(pairs, outputPath)
}

// defaultPairs 는 기본 페어 6개 (4 마켓 C(4,2))
func defaultPairs() [][2]string {
	return [][2]string{
		{"KRW-BTC", "KRW-ETH"},
		{"KRW-BTC", "KRW-XRP"},
		{"KRW-BTC", "KRW-SOL"},
		{"KRW-ETH", "KRW-XRP"},
		{"KRW-ETH", "KRW-SOL"},
		{"KRW-XRP", "KRW-SOL"},
	}
}

func runPairBaseline(pairs [][2]string, outputPath string) {
	db, err := console.MsConn.GetDB("tuf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[pair] tuf DB 연결 실패: %v\n", err)
		return
	}

	out := PairBaselineOutput{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		ZWindow:      pairZWindow,
		EntryK:       pairEntryK,
		ExitZ:        pairExitZ,
		StopZ:        pairStopZ,
		TimeExit:     pairTimeExitBars,
		FeeRate:      pairFeeRate,
		SlippageRate: pairSlipRate,
	}

	for _, p := range pairs {
		mktA, mktB := p[0], p[1]
		name := mktA + "/" + mktB
		fmt.Printf("[pair] %s 로딩 + 분석...\n", name)
		paired, err := box.FetchUpbitPair15m(db, mktA, mktB, 400000)
		if err != nil || len(paired) < pairZWindow+pairTimeExitBars+10 {
			fmt.Printf("[pair] %s 캔들 부족 또는 로드 실패 (%d봉)\n", name, len(paired))
			continue
		}
		res := analyzePair(name, mktA, mktB, paired)
		out.Pairs = append(out.Pairs, res)
		fmt.Printf("[pair] %s 완료: trades=%d, PF=%.2f, avgNet=%.3f, corr=%.3f, ADF=%.2f %s\n",
			name, res.TradeCount, res.ProfitFactor, res.AvgNetReturn, res.Correlation, res.ADFTStat,
			condText(res.IsStationary, "정상", "비정상"))
	}

	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[pair] 디렉토리 생성 실패: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[pair] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[pair] 파일 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[pair] 완료: %s (페어 %d개)\n", outputPath, len(out.Pairs))
}

// analyzePair 는 단일 페어의 룰 평가 + 메트릭 계산
func analyzePair(name, mktA, mktB string, paired []box.PairedCandle) PairResult {
	bars := indicator.ComputePairBars(paired, pairZWindow)
	n := len(bars)

	// OOS 보존: 마지막 oosMonths(5760) 봉 제외
	isEnd := n - 5760
	if isEnd < pairZWindow+pairTimeExitBars {
		isEnd = n
	}

	// 코인티그레이션 메트릭 (IS 구간만으로 검증)
	pricesA := make([]float64, isEnd)
	pricesB := make([]float64, isEnd)
	logSpr := make([]float64, isEnd)
	for i := 0; i < isEnd; i++ {
		pricesA[i] = bars[i].PriceA
		pricesB[i] = bars[i].PriceB
		logSpr[i] = bars[i].LogSpr
	}
	corr := indicator.PearsonCorrelation(pricesA, pricesB)
	adf := indicator.ADFTStat(logSpr)
	isStationary := adf < -2.86

	res := PairResult{
		PairName:     name,
		MarketA:      mktA,
		MarketB:      mktB,
		PairedBars:   n,
		Correlation:  corr,
		ADFTStat:     adf,
		IsStationary: isStationary,
	}
	res.ISFrom = bars[0].Date
	if isEnd > 0 && isEnd <= n {
		res.ISTo = bars[isEnd-1].Date
	}
	if isEnd < n {
		res.OOSFrom = bars[isEnd].Date
		res.OOSTo = bars[n-1].Date
	}

	// 트레이드 시뮬레이션 (IS 구간만)
	var trades []PairTradeRow
	var inPos bool
	var entryIdx int
	var entryZ, entryPrice float64
	var buySide string // "A" or "B"

	for i := pairZWindow; i < isEnd; i++ {
		bar := &bars[i]
		if !bar.Valid {
			continue
		}

		if inPos {
			// 보유 중 — 청산 평가
			barsHeld := i - entryIdx
			var exitPrice float64
			if buySide == "A" {
				exitPrice = bar.PriceA
			} else {
				exitPrice = bar.PriceB
			}
			exitReason := ""
			if math.Abs(bar.Z) <= pairExitZ {
				exitReason = "z_reverted"
			} else if math.Abs(bar.Z) >= pairStopZ {
				exitReason = "z_extreme_stop"
			} else if barsHeld >= pairTimeExitBars {
				exitReason = "time_exit"
			}
			if exitReason != "" {
				netReturn := (exitPrice-entryPrice)/entryPrice*100 - (pairFeeRate+pairSlipRate)*2*100
				trades = append(trades, PairTradeRow{
					PairName:    name,
					EntryDate:   bars[entryIdx].Date,
					EntryTime:   bars[entryIdx].Time,
					EntryZ:      entryZ,
					BuySide:     buySide,
					BuyPrice:    entryPrice,
					ExitDate:    bar.Date,
					ExitTime:    bar.Time,
					ExitZ:       bar.Z,
					ExitPrice:   exitPrice,
					ExitReason:  exitReason,
					HoldingBars: barsHeld,
					NetReturn:   netReturn,
				})
				inPos = false
			}
			continue
		}

		// 미보유 — 진입 평가
		if bar.Z <= -pairEntryK {
			// A 저평가 → A 매수
			inPos = true
			entryIdx = i
			entryZ = bar.Z
			entryPrice = bar.PriceA
			buySide = "A"
			res.BuyACount++
		} else if bar.Z >= pairEntryK {
			// B 저평가 → B 매수
			inPos = true
			entryIdx = i
			entryZ = bar.Z
			entryPrice = bar.PriceB
			buySide = "B"
			res.BuyBCount++
		}
	}

	// 활성 포지션은 결과에서 제외 (베이스라인 관례)
	res.TradeCount = len(trades)
	res.Trades = trades

	// 메트릭 계산
	if len(trades) == 0 {
		return res
	}
	var grossWin, grossLoss, cumRet float64
	equity, peak, maxDD := 1.0, 1.0, 0.0
	wins := 0
	for _, t := range trades {
		net := t.NetReturn
		cumRet += net
		if net > 0 {
			wins++
			grossWin += net
		} else {
			grossLoss += -net
		}
		equity *= 1.0 + net/100.0
		if equity <= 0 {
			equity = 0
			maxDD = 1.0
		} else {
			if equity > peak {
				peak = equity
			}
			if dd := (peak - equity) / peak; dd > maxDD {
				maxDD = dd
			}
		}
	}
	res.WinRate = float64(wins) / float64(len(trades)) * 100
	res.AvgNetReturn = cumRet / float64(len(trades))
	if grossLoss > 0 {
		res.ProfitFactor = grossWin / grossLoss
	}
	res.MaxDrawdown = maxDD * 100
	return res
}

func condText(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}
