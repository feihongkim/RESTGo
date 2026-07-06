package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// TriggerScanExample 은 범용 트리거 스캔 발화 1건.
type TriggerScanExample struct {
	Shcode      string   `json:"shcode"`
	TriggerDate string   `json:"trigger_date"`
	R1          *float64 `json:"r1,omitempty"`
	R5          *float64 `json:"r5,omitempty"`
	R10         *float64 `json:"r10,omitempty"`
	R20         *float64 `json:"r20,omitempty"`
}

// HandleTriggerScan 은 임의의 트리거×조건 조합의 전방수익률 엣지를 측정한다 (구조 개선 ②).
// 사용법: ./RESTGo stock trigger_scan --trigger <이름> [--when C1,C2] [--when-not C1,C2]
//         [--cooldown N] [--set K=V[,K=V]] [--max N] [--candles N] [--out path]
// 트리거는 일반(edge)·armed(장전→발화) 모두 지원. 조건·트리거명은 레지스트리 등록명.
// YAML 엔진과 동일한 캔들 처리 순서라 여기서 유효한 조합은 그대로 YAML 전략화 가능 (engine-parity).
func HandleTriggerScan(args []string) {
	var trigger string
	var when, whenNot []string
	cooldown := 0
	maxStocks := 0
	candleCount := 4200
	upbit := false // --upbit: 암호화폐 15분봉 모드 (TUF DB, BTC/ETH/XRP/SOL — 2026-07-05 분봉 전략 크립토 적용)
	foreign := "" // --foreign-us|jp|cn|hk: 해외 일봉 모드 (KIS2 DB — 2026-07-06 전략11 시장 확대)
	var marketsCSV string // --markets: upbit 모드 마켓 목록 오버라이드 (CSV)
	outPath := ""
	overrides := map[string]interface{}{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--trigger":
			if i+1 < len(args) {
				trigger = args[i+1]
				i++
			}
		case "--when":
			if i+1 < len(args) {
				when = append(when, strings.Split(args[i+1], ",")...)
				i++
			}
		case "--when-not":
			if i+1 < len(args) {
				whenNot = append(whenNot, strings.Split(args[i+1], ",")...)
				i++
			}
		case "--cooldown":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cooldown)
				i++
			}
		case "--set":
			if i+1 < len(args) {
				for _, kv := range strings.Split(args[i+1], ",") {
					parts := strings.SplitN(kv, "=", 2)
					if len(parts) == 2 {
						// 숫자로 변환해 저장 — ApplySettingsOverrides의 toInt/toFloat는 문자열을 받지 않는다
						if iv, err := strconv.Atoi(parts[1]); err == nil {
							overrides[parts[0]] = iv
						} else if fv, err := strconv.ParseFloat(parts[1], 64); err == nil {
							overrides[parts[0]] = fv
						} else {
							overrides[parts[0]] = parts[1]
						}
					}
				}
				i++
			}
		case "--max":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxStocks)
				i++
			}
		case "--candles":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &candleCount)
				i++
			}
		case "--upbit":
			upbit = true
			if candleCount == 4200 {
				candleCount = 310000 // 15분봉 전체 이력 (BTC/ETH/XRP ~30만 봉)
			}
		case "--markets":
			if i+1 < len(args) {
				marketsCSV = args[i+1]
				i++
			}
		case "--foreign-us":
			foreign = "us"
		case "--foreign-jp":
			foreign = "jp"
		case "--foreign-cn":
			foreign = "cn"
		case "--foreign-hk":
			foreign = "hk"
		case "--out":
			if i+1 < len(args) {
				outPath = args[i+1]
				i++
			}
		}
	}
	if trigger == "" {
		fmt.Println("사용법: ./RESTGo stock trigger_scan --trigger <이름> [--when C1,C2] [--when-not C1,C2] [--cooldown N] [--set K=V] [--max N] [--candles N] [--upbit] [--out path]")
		return
	}
	if outPath == "" {
		outPath = "zpicture/trigger_scan_" + strings.ToLower(trigger) + ".json"
	}

	s := stg.DefaultSettings()
	stg.ApplySettingsOverrides(&s, overrides)
	cfg := stg.TriggerScanConfig{Trigger: trigger, When: when, WhenNot: whenNot, CooldownBars: cooldown}

	dbName := "han"
	if upbit {
		dbName = "tuf"
	}
	if foreign != "" {
		dbName = "KIS2"
	}
	db, err := console.MsConn.GetDB(dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[trigger_scan] DB 연결 오류: %v\n", err)
		return
	}
	var stocks []string
	if upbit {
		stocks = []string{"KRW-BTC", "KRW-ETH", "KRW-XRP", "KRW-SOL"} // 장기 이력 보유 4종
		if marketsCSV != "" {
			stocks = strings.Split(marketsCSV, ",")
		}
	} else if foreign != "" {
		prefixes := map[string][]string{
			"us": {"DNY", "DNA"}, "jp": {"DTS"}, "cn": {"DSZ", "DSH"}, "hk": {"DHK"},
		}[foreign]
		stocks, err = box.FetchForeignStockList(db, prefixes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[trigger_scan] 해외 종목 목록 오류: %v\n", err)
			return
		}
	} else {
		stocks, err = box.FetchHannamStockList(db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[trigger_scan] 종목 목록 오류: %v\n", err)
			return
		}
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}
	fmt.Printf("[trigger_scan] trigger=%s when=%v when_not=%v cooldown=%d set=%v | %d종목 캔들 %d\n",
		trigger, when, whenNot, cooldown, overrides, len(stocks), candleCount)

	fwdRet := func(candles []*box.Candle, pos, h int) *float64 {
		if pos+h >= len(candles) || candles[pos].Close <= 0 {
			return nil
		}
		r := (candles[pos+h].Close - candles[pos].Close) / candles[pos].Close * 100.0
		return &r
	}

	var mu sync.Mutex
	var examples []TriggerScanExample
	baseline := map[int][]float64{}
	var scanned, failed atomic.Int64
	var unknownOnce sync.Once

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()

			var candles []*box.Candle
			var fetchErr error
			if upbit {
				candles, fetchErr = box.FetchUpbitCandles15m(db, shcode, candleCount)
			} else if foreign != "" {
				candles, fetchErr = box.FetchCandlesForeign(db, shcode, candleCount)
			} else {
				candles, fetchErr = box.FetchCandlesHannam(db, shcode, candleCount)
			}
			if fetchErr != nil || len(candles) < 130 {
				if fetchErr != nil && failed.Add(1) <= 3 {
					fmt.Fprintf(os.Stderr, "[trigger_scan] %s 캔들 조회 실패: %v\n", shcode, fetchErr)
				} else if fetchErr == nil {
					failed.Add(1)
				}
				return
			}
			indicator.PrepareCandles(candles)

			fires, unknown := stg.ScanTrigger(candles, cfg, s)
			if unknown != "" {
				unknownOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "[trigger_scan] 미등록: %s — 중단\n", unknown)
					os.Exit(1)
				})
			}

			localBase := map[int][]float64{}
			for pos := 130; pos+20 < len(candles); pos += 21 {
				for _, h := range mtopHorizons {
					if r := fwdRet(candles, pos, h); r != nil {
						localBase[h] = append(localBase[h], *r)
					}
				}
			}
			var localEx []TriggerScanExample
			for _, pos := range fires {
				localEx = append(localEx, TriggerScanExample{
					Shcode: shcode, TriggerDate: candles[pos].Date,
					R1: fwdRet(candles, pos, 1), R5: fwdRet(candles, pos, 5),
					R10: fwdRet(candles, pos, 10), R20: fwdRet(candles, pos, 20),
				})
			}

			mu.Lock()
			examples = append(examples, localEx...)
			for h, rs := range localBase {
				baseline[h] = append(baseline[h], rs...)
			}
			mu.Unlock()

			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[trigger_scan] 진행 %d/%d (신호 %d)\n", n, len(stocks), len(examples))
			}
		}(code)
	}
	wg.Wait()

	sort.Slice(examples, func(i, j int) bool {
		if examples[i].Shcode != examples[j].Shcode {
			return examples[i].Shcode < examples[j].Shcode
		}
		return examples[i].TriggerDate < examples[j].TriggerDate
	})

	type hStat struct {
		Horizon      int     `json:"horizon"`
		N            int     `json:"n"`
		Mean         float64 `json:"mean_pct"`
		Median       float64 `json:"median_pct"`
		WinRate      float64 `json:"win_rate_pct"`
		BaselineMean float64 `json:"baseline_mean_pct"`
		Edge         float64 `json:"edge_pct"`
		TStat        float64 `json:"t_stat"`
		BaselineN    int     `json:"baseline_n"`
	}
	pick := func(ex TriggerScanExample, h int) *float64 {
		switch h {
		case 1:
			return ex.R1
		case 5:
			return ex.R5
		case 10:
			return ex.R10
		default:
			return ex.R20
		}
	}
	var stats []hStat
	for _, h := range mtopHorizons {
		var rs []float64
		for _, ex := range examples {
			if p := pick(ex, h); p != nil {
				rs = append(rs, *p)
			}
		}
		if len(rs) == 0 {
			continue
		}
		sorted := append([]float64(nil), rs...)
		sort.Float64s(sorted)
		up := 0
		for _, r := range rs {
			if r > 0 {
				up++
			}
		}
		bm := meanFloats(baseline[h])
		stats = append(stats, hStat{
			Horizon: h, N: len(rs), Mean: meanFloats(rs), Median: sorted[len(sorted)/2],
			WinRate: 100 * float64(up) / float64(len(rs)), BaselineMean: bm,
			Edge: meanFloats(rs) - bm, TStat: welchTStatFloats(rs, baseline[h]), BaselineN: len(baseline[h]),
		})
	}

	out := struct {
		Mode        string               `json:"mode"`
		Trigger     string               `json:"trigger"`
		When        []string             `json:"when,omitempty"`
		WhenNot     []string             `json:"when_not,omitempty"`
		Cooldown    int                  `json:"cooldown_bars"`
		Overrides   map[string]interface{} `json:"settings_overrides,omitempty"`
		CandleCount int                  `json:"candle_count"`
		StockCount  int                  `json:"stock_count"`
		SignalCount int                  `json:"signal_count"`
		Stats       []hStat              `json:"stats"`
		Examples    []TriggerScanExample `json:"examples"`
	}{func() string {
		if upbit {
			return "upbit-15m"
		}
		if foreign != "" {
			return "foreign-" + foreign
		}
		return "hannam-daily"
	}(), trigger, when, whenNot, cooldown, overrides, candleCount, len(stocks), len(examples), stats, examples}

	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[trigger_scan] 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("[trigger_scan] 완료: 종목 %d(실패 %d)  신호 %d  → %s\n",
		scanned.Load(), failed.Load(), len(examples), outPath)
	for _, st := range stats {
		fmt.Printf("[trigger_scan] h%-2d n=%d  mean %+.3f%%  median %+.3f%%  승률 %.1f%%  baseline %+.3f%%  edge %+.3f%%p  t=%.2f\n",
			st.Horizon, st.N, st.Mean, st.Median, st.WinRate, st.BaselineMean, st.Edge, st.TStat)
	}
}
