package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Paper WD (W-Bottom + DefBox) — B 슬리브 Paper Trading 시스템
//
// 4국(3시장) 확장: KR(hannam) + CN(KIS2 DSZ/DSH) + HK(KIS2 DHK). JP 제외.
// 가상 자본 4.5억, 종목당 슬리브의 10%, 2-stage 진입(W 50% + DefBox 돌파 50%).
// 비용 모델: 편도 KR 15bp / CN 20bp / HK 25bp.
// 20일 시간청산, 최대 30일 연장.
// ──────────────────────────────────────────────────────────────────────────────

// ── 설정 상수 ────────────────────────────────────────────────────────────────

const (
	paperWDCapital        = 450_000_000.0
	paperWDPerPositionPct = 0.10
	paperWDMaxPositions   = 8
	paperWDStage1Pct      = 0.50
	paperWDStage2Pct      = 0.50
	paperWDDefaultHolding = 20
	paperWDMaxHolding     = 30
	paperWDDefBoxTimeout  = 20
	paperWDCandleDays     = 500

	paperWDLedgerDir  = "zpicture/paper_wd"
	paperWDLedgerFile = "zpicture/paper_wd/ledger.json"
)

var paperWDCostBP = map[string]float64{"KR": 15, "CN": 20, "HK": 25}
var paperWDFX = map[string]float64{"KR": 1.0, "CN": 180.0, "HK": 140.0}

type marketCfg struct {
	Label    string
	DBLabel  string // "han" or "KIS2"
	Mode     string // "hannam", "foreign-cn", "foreign-hk"
	Prefixes []string
}

var paperWDMarkets = []marketCfg{
	{Label: "KR", DBLabel: "han", Mode: "hannam"},
	{Label: "CN", DBLabel: "KIS2", Mode: "foreign-cn", Prefixes: []string{"DSZ", "DSH"}},
	{Label: "HK", DBLabel: "KIS2", Mode: "foreign-hk", Prefixes: []string{"DHK"}},
}

// ── 포지션 원장 구조체 ─────────────────────────────────────────────────────

type PaperWDPosition struct {
	ID         string `json:"id"`
	Market     string `json:"market"`
	Shcode     string `json:"shcode"`
	Hname      string `json:"hname"`
	SignalDate string `json:"signal_date"`

	HasDefBox       bool    `json:"has_defbox"`
	DefBoxDate      string  `json:"defbox_date,omitempty"`
	DefBoxPrice     float64 `json:"defbox_price"`
	DefBoxPriceRaw  float64 `json:"defbox_price_raw"`
	DefBoxBreakDate string  `json:"defbox_break_date,omitempty"` // WDefBoxAnalyze가 계산한 돌파일

	Stage1Date   string  `json:"stage1_date"`
	Stage1Price  float64 `json:"stage1_price"`
	Stage1Qty    int     `json:"stage1_qty"`
	Stage1Amount float64 `json:"stage1_amount"`
	Stage1Cost   float64 `json:"stage1_cost"`

	Stage2Date   string  `json:"stage2_date,omitempty"`
	Stage2Price  float64 `json:"stage2_price,omitempty"`
	Stage2Qty    int     `json:"stage2_qty,omitempty"`
	Stage2Amount float64 `json:"stage2_amount,omitempty"`
	Stage2Cost   float64 `json:"stage2_cost,omitempty"`

	ExitDate   string  `json:"exit_date,omitempty"`
	ExitPrice  float64 `json:"exit_price,omitempty"`
	ExitReason string  `json:"exit_reason,omitempty"`
	ExitCost   float64 `json:"exit_cost,omitempty"`

	RealizedPL    float64 `json:"realized_pl,omitempty"`
	RealizedPLPct float64 `json:"realized_pl_pct,omitempty"`

	Status string `json:"status"` // "stage1" | "stage2" | "closed"
}

type PaperWDLedger struct {
	Version     int                `json:"version"`
	Capital     float64            `json:"capital"`
	StartDate   string             `json:"start_date"`
	LastUpdated string             `json:"last_updated"`
	Positions   []*PaperWDPosition `json:"positions"`
	ClosedPL    float64            `json:"closed_pl"`
	TradeCount  int                `json:"trade_count"`
}

// scanResult 는 종목별 당일 신호와 그 시점까지의 캔들 스냅샷이다.
type scanResult struct {
	market  string
	shcode  string
	hname   string
	sigs    []stg.WDefBoxSignal
	candles []*box.Candle
}

type paperWDSnapshot struct {
	candles []*box.Candle
}

// ── CLI 진입점 ──────────────────────────────────────────────────────────────

// HandlePaperWD 는 "stock paper_wd [--date YYYYMMDD]" — 일일 paper 스캔.
func HandlePaperWD(args []string) {
	scanDate := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--date" && i+1 < len(args) {
			scanDate = args[i+1]
			i++
		}
	}
	if scanDate == "" {
		scanDate = time.Now().Format("20060102")
	}
	if err := validatePaperWDDate(scanDate); err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd] 잘못된 --date: %v\n", err)
		return
	}

	ledger, err := loadPaperWDLedger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd] 원장 로드 실패 — 변경 없이 중단: %v\n", err)
		return
	}
	if ledger.StartDate == "" {
		ledger.StartDate = scanDate
	}

	fmt.Printf("[paper_wd] ===== B슬리브 Paper 스캔 %s =====\n", scanDate)
	fmt.Printf("[paper_wd] 자본: %.0f억  종목당: %.0f만원 (%.0f%%)  동시최대: %d\n",
		ledger.Capital/1e8, ledger.Capital*paperWDPerPositionPct/1e4, paperWDPerPositionPct*100, paperWDMaxPositions)
	fmt.Printf("[paper_wd] 비용: KR %dbp / CN %dbp / HK %dbp (편도)\n",
		int(paperWDCostBP["KR"]), int(paperWDCostBP["CN"]), int(paperWDCostBP["HK"]))

	var kis2Skipped []string
	if !console.MsConn.IsAvailable("KIS2") {
		for _, mkt := range paperWDMarkets {
			if mkt.DBLabel == "KIS2" {
				kis2Skipped = append(kis2Skipped, mkt.Label)
			}
		}
		if len(kis2Skipped) > 0 {
			skipMsg := fmt.Sprintf("[paper_wd] ⚠️ KIS2 DB unavailable — %s 시장 SKIP (KR hannam 단독 모드)", strings.Join(kis2Skipped, "/"))
			fmt.Println(skipMsg)
			console.LogInfo(skipMsg)
			console.Tele("⚠️ *B슬리브 Paper WD* — KIS2 DB unavailable\n%s 시장 스킵 — KR(hannam) 단독 진행", strings.Join(kis2Skipped, " + "))
		}
	}

	openKeys := make(map[string]struct{})
	for _, p := range ledger.Positions {
		if p.Status != "closed" {
			openKeys[paperWDStockKey(p.Market, p.Shcode)] = struct{}{}
		}
	}
	openSnapshots := make(map[string]paperWDSnapshot)
	var snapshotMu sync.Mutex
	var todaySignals []scanResult
	var totalStocks, totalScanned int32

	for _, mkt := range paperWDMarkets {
		db, dbErr := console.MsConn.GetDB(mkt.DBLabel)
		if dbErr != nil {
			fmt.Fprintf(os.Stderr, "[paper_wd] %s DB(%s) 연결 실패: %v\n", mkt.Label, mkt.DBLabel, dbErr)
			continue
		}
		stocks, listErr := fetchStockList(db, mkt)
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "[paper_wd] %s 종목 목록 실패: %v\n", mkt.Label, listErr)
			continue
		}
		atomic.AddInt32(&totalStocks, int32(len(stocks)))
		fmt.Printf("[paper_wd] %s: %d개 종목 스캔 중...\n", mkt.Label, len(stocks))

		var mu sync.Mutex
		var marketSignals []scanResult
		sem := make(chan struct{}, 20)
		var wg sync.WaitGroup
		for _, code := range stocks {
			wg.Add(1)
			sem <- struct{}{}
			go func(shcode string) {
				defer wg.Done()
				defer func() { <-sem }()
				defer atomic.AddInt32(&totalScanned, 1)

				candles, fetchErr := fetchCandles(db, mkt, shcode, paperWDCandleDays)
				if fetchErr != nil {
					return
				}
				candles = candlesThroughDate(candles, scanDate)
				if len(candles) < 60 {
					return
				}
				indicator.PrepareCandles(candles)

				key := paperWDStockKey(mkt.Label, shcode)
				if _, ok := openKeys[key]; ok {
					snapshotMu.Lock()
					openSnapshots[key] = paperWDSnapshot{candles: candles}
					snapshotMu.Unlock()
				}

				var todaySigs []stg.WDefBoxSignal
				for _, s := range stg.WDefBoxAnalyze(candles) {
					if s.Date == scanDate {
						todaySigs = append(todaySigs, s)
					}
				}
				if len(todaySigs) == 0 {
					return
				}
				// 신규 진입 포지션의 당일 보유 거래일 표시에도 같은 스냅샷을 사용한다.
				snapshotMu.Lock()
				openSnapshots[key] = paperWDSnapshot{candles: candles}
				snapshotMu.Unlock()
				name := shcode
				if mkt.Label == "KR" {
					name = lookupKRName(shcode)
				}
				mu.Lock()
				marketSignals = append(marketSignals, scanResult{
					market: mkt.Label, shcode: shcode, hname: name,
					sigs: todaySigs, candles: candles,
				})
				mu.Unlock()
			}(code)
		}
		wg.Wait()
		fmt.Printf("[paper_wd] %s: 완료 — %d개 종목 중 %d건 신호\n", mkt.Label, len(stocks), len(marketSignals))
		todaySignals = append(todaySignals, marketSignals...)
	}

	// 종목 마스터에서 빠진 기존 보유 종목도 직접 조회해 좀비 포지션을 방지한다.
	fillMissingOpenSnapshots(ledger, openSnapshots, scanDate)
	fmt.Printf("[paper_wd] 전체 %d종목 중 %d종목 스캔, 오늘(%s) W신호 %d건\n",
		totalStocks, totalScanned, scanDate, len(todaySignals))

	stage2Entered, closedToday, processErrors := processOpenPaperWDPositions(ledger, openSnapshots, scanDate)
	for _, processErr := range processErrors {
		fmt.Fprintf(os.Stderr, "[paper_wd] 포지션 갱신 보류: %v\n", processErr)
	}

	// DefBox 우선, 이후 시장/종목/날짜 순으로 정렬해 동시 포지션 제한의 결과를 결정적으로 만든다.
	sort.Slice(todaySignals, func(i, j int) bool {
		iHas := len(todaySignals[i].sigs) > 0 && todaySignals[i].sigs[0].HasDefBox
		jHas := len(todaySignals[j].sigs) > 0 && todaySignals[j].sigs[0].HasDefBox
		if iHas != jHas {
			return iHas
		}
		ik := todaySignals[i].market + "|" + todaySignals[i].shcode
		jk := todaySignals[j].market + "|" + todaySignals[j].shcode
		return ik < jk
	})

	canOpen := paperWDMaxPositions - countOpenPositions(ledger)
	var entered []*PaperWDPosition
	duplicateCount := 0
	for _, sr := range todaySignals {
		for _, sig := range sr.sigs {
			id := paperWDPositionID(sr.market, sr.shcode, sig.Date)
			if ledgerHasPositionID(ledger, id) {
				duplicateCount++
				continue
			}
			if canOpen <= 0 {
				continue
			}
			pos, createErr := createPaperPosition(sr.market, sr.shcode, sr.hname, sig, sr.candles, ledger.Capital)
			if createErr != nil {
				fmt.Fprintf(os.Stderr, "[paper_wd] 신규 진입 거부 (%s): %v\n", id, createErr)
				continue
			}
			ledger.Positions = append(ledger.Positions, pos)
			entered = append(entered, pos)
			canOpen--
		}
	}
	fmt.Printf("[paper_wd] 신규 진입: %d건  중복 스킵: %d건\n", len(entered), duplicateCount)

	ledger.LastUpdated = scanDate
	if err := savePaperWDLedger(ledger); err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd] 원장 저장 실패 — 알림 전송 없이 중단: %v\n", err)
		return
	}

	// 원장 commit 이후에만 거래 알림을 보낸다.
	for _, p := range stage2Entered {
		tgPaperWDAlert("🔵 DefBox 돌파 (stage2)", p, "stage2")
	}
	for _, p := range closedToday {
		tgPaperWDAlert("🔴 만기청산", p, "exit")
	}
	for _, p := range entered {
		tgPaperWDAlert("🟢 W신호 진입", p, "stage1")
	}

	openNow := countOpenPositions(ledger)
	stage1Only, stage2 := countByStatus(ledger)
	fmt.Printf("\n[paper_wd] ===== 요약 %s =====\n", scanDate)
	fmt.Printf("  신규 W신호: %d건  신규 진입: %d건  stage2: %d건  만기청산: %d건\n",
		len(todaySignals), len(entered), len(stage2Entered), len(closedToday))
	fmt.Printf("  현재 보유: %d건 (stage1:%d stage2:%d)  누적 청산: %d건  누적 손익: %+.0f원\n",
		openNow, stage1Only, stage2, ledger.TradeCount, ledger.ClosedPL)

	if openNow > 0 {
		fmt.Println("\n[보유 포지션]")
		for _, p := range ledger.Positions {
			if p.Status == "closed" {
				continue
			}
			held := "?"
			if snap, ok := openSnapshots[paperWDStockKey(p.Market, p.Shcode)]; ok {
				if bars, barErr := tradingBarsBetween(snap.candles, p.SignalDate, scanDate); barErr == nil {
					held = fmt.Sprintf("%d", bars)
				}
			}
			fmt.Printf("  %s %-10s %-12s stage=%s 거래일=%s", p.Market, p.Shcode, p.Hname, p.Status, held)
			if p.HasDefBox && p.Status == "stage1" {
				fmt.Printf(" DefBox=%s(돌파대기중)", p.DefBoxDate)
			}
			fmt.Println()
		}
	}
}

// HandlePaperWDReport 는 "stock paper_wd_report [--month YYYYMM]".
func HandlePaperWDReport(args []string) {
	month := time.Now().Format("200601")
	for i := 0; i < len(args); i++ {
		if args[i] == "--month" && i+1 < len(args) {
			month = args[i+1]
			i++
		}
	}
	if _, err := time.Parse("200601", month); err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd_report] 잘못된 --month (%s): YYYYMM 형식이어야 합니다\n", month)
		return
	}

	ledger, err := loadPaperWDLedger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd_report] 원장 로드 실패: %v\n", err)
		return
	}
	if len(ledger.Positions) == 0 {
		fmt.Println("[paper_wd_report] 원장이 비어 있습니다")
		return
	}

	closed := filterClosed(ledger.Positions)
	monthlyClosed := filterByMonth(closed, month)
	cum := buildStats(closed, ledger.Capital)
	mon := buildStats(monthlyClosed, ledger.Capital)

	monthlyGross := toFloat(mon["gross_realized_pl"])
	monthlyNet := toFloat(mon["net_realized_pl"])
	gatePass := paperWDMonthlyGate(monthlyClosed, mon)
	report := map[string]interface{}{
		"title":        "B슬리브 WD Paper 트레이딩 리포트",
		"month":        month,
		"generated_at": time.Now().Format("2006-01-02 15:04:05"),
		"capital":      ledger.Capital,
		"period":       map[string]string{"start": ledger.StartDate, "end": ledger.LastUpdated},
		"cumulative":   cum,
		"monthly":      mon,
		"cost_model": map[string]float64{
			"kr_bp_per_side": paperWDCostBP["KR"],
			"cn_bp_per_side": paperWDCostBP["CN"],
			"hk_bp_per_side": paperWDCostBP["HK"],
		},
		"gate_metrics": map[string]interface{}{
			"criteria":                  "monthly_gross_realized_pl > 0 AND monthly_net_after_cost > 0",
			"monthly_gross_realized_pl": monthlyGross,
			"monthly_net_after_cost":    monthlyNet,
			"closed_trades":             len(monthlyClosed),
			"gate_pass":                 gatePass,
			"note":                      "Sharpe는 참고 지표이며 게이트 판정에 사용하지 않음",
		},
	}

	outPath := fmt.Sprintf("zpicture/paper_wd/report_%s.json", month)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd_report] JSON 생성 실패: %v\n", err)
		return
	}
	if err := atomicWriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[paper_wd_report] 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("\n[paper_wd_report] ===== B슬리브 Paper 리포트 %s =====\n", month)
	fmt.Printf("  기간: %s ~ %s  자본: %.0f억\n", ledger.StartDate, ledger.LastUpdated, ledger.Capital/1e8)
	fmt.Println("\n── 누적 ──")
	printStats(cum)
	fmt.Println("\n── 당월 ──")
	printStats(mon)
	fmt.Printf("\n  운용 게이트: %v (월 총실현 %+.0f원 / 비용후 %+.0f원)\n", gatePass, monthlyGross, monthlyNet)
	fmt.Printf("  리포트 저장: %s\n", outPath)
}

// ── 내부: DB/캔들 ────────────────────────────────────────────────────────────

func fetchStockList(db *sql.DB, mkt marketCfg) ([]string, error) {
	switch mkt.Mode {
	case "hannam":
		return box.FetchHannamStockList(db)
	case "foreign-cn":
		return box.FetchForeignStockList(db, []string{"DSZ", "DSH"})
	case "foreign-hk":
		return box.FetchForeignStockList(db, []string{"DHK"})
	}
	return nil, fmt.Errorf("unknown mode: %s", mkt.Mode)
}

func fetchCandles(db *sql.DB, mkt marketCfg, code string, days int) ([]*box.Candle, error) {
	switch mkt.Mode {
	case "hannam":
		return box.FetchCandlesHannam(db, code, days)
	case "foreign-cn", "foreign-hk":
		return box.FetchCandlesForeign(db, code, days)
	}
	return nil, fmt.Errorf("unknown mode: %s", mkt.Mode)
}

func paperWDMarketConfig(label string) (marketCfg, bool) {
	for _, mkt := range paperWDMarkets {
		if mkt.Label == label {
			return mkt, true
		}
	}
	return marketCfg{}, false
}

func fillMissingOpenSnapshots(l *PaperWDLedger, snapshots map[string]paperWDSnapshot, asOfDate string) {
	attempted := make(map[string]struct{})
	for _, p := range l.Positions {
		if p.Status == "closed" {
			continue
		}
		key := paperWDStockKey(p.Market, p.Shcode)
		if _, ok := snapshots[key]; ok {
			continue
		}
		if _, ok := attempted[key]; ok {
			continue
		}
		attempted[key] = struct{}{}
		mkt, ok := paperWDMarketConfig(p.Market)
		if !ok {
			fmt.Fprintf(os.Stderr, "[paper_wd] %s 스냅샷 조회 실패: 알 수 없는 시장\n", p.ID)
			continue
		}
		db, err := console.MsConn.GetDB(mkt.DBLabel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[paper_wd] %s 스냅샷 DB 실패: %v\n", p.ID, err)
			continue
		}
		candles, err := fetchCandles(db, mkt, p.Shcode, paperWDCandleDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[paper_wd] %s 스냅샷 조회 실패: %v\n", p.ID, err)
			continue
		}
		candles = candlesThroughDate(candles, asOfDate)
		if len(candles) == 0 {
			fmt.Fprintf(os.Stderr, "[paper_wd] %s 스냅샷 비어 있음 (%s)\n", p.ID, asOfDate)
			continue
		}
		snapshots[key] = paperWDSnapshot{candles: candles}
	}
}

// ── 원장 IO ─────────────────────────────────────────────────────────────────

func loadPaperWDLedger() (*PaperWDLedger, error) {
	data, err := os.ReadFile(paperWDLedgerFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &PaperWDLedger{Version: 1, Capital: paperWDCapital, Positions: []*PaperWDPosition{}}, nil
		}
		return nil, err
	}
	var l PaperWDLedger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("원장 JSON 파싱 실패 (자동 초기화 금지): %w", err)
	}
	if l.Capital == 0 {
		l.Capital = paperWDCapital
	}
	if l.Positions == nil {
		l.Positions = []*PaperWDPosition{}
	}
	if err := validatePaperWDLedger(&l); err != nil {
		return nil, err
	}
	return &l, nil
}

func savePaperWDLedger(l *PaperWDLedger) error {
	return savePaperWDLedgerAt(paperWDLedgerFile, l)
}

func savePaperWDLedgerAt(path string, l *PaperWDLedger) error {
	if err := validatePaperWDLedger(l); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("원장 JSON 생성 실패: %w", err)
	}
	return atomicWriteFile(path, data, 0644)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("디렉토리 생성 실패 (%s): %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("임시 파일 권한 설정 실패: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("임시 파일 쓰기 실패: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("임시 파일 sync 실패: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("임시 파일 닫기 실패: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("원자적 rename 실패: %w", err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// ── 진입/청산 로직 ──────────────────────────────────────────────────────────

// createPaperPosition 은 W신호 캔들의 실제 종가로 stage1 포지션을 만든다.
// 가격/인덱스가 유효하지 않으면 가짜 가격으로 대체하지 않고 진입을 거부한다.
func createPaperPosition(market, shcode, hname string, sig stg.WDefBoxSignal, candles []*box.Candle, capital float64) (*PaperWDPosition, error) {
	if err := validatePaperWDDate(sig.Date); err != nil {
		return nil, fmt.Errorf("신호일 오류: %w", err)
	}
	if sig.Pos < 0 || sig.Pos >= len(candles) {
		return nil, fmt.Errorf("신호 인덱스 범위 오류: %d/%d", sig.Pos, len(candles))
	}
	entryCandle := candles[sig.Pos]
	if entryCandle.Date != sig.Date {
		return nil, fmt.Errorf("신호 날짜/캔들 불일치: %s != %s", sig.Date, entryCandle.Date)
	}
	entryPrice := entryCandle.CloseOrigin
	if entryPrice <= 0 || math.IsNaN(entryPrice) || math.IsInf(entryPrice, 0) {
		return nil, fmt.Errorf("유효하지 않은 진입 종가: %v", entryPrice)
	}
	fx, ok := paperWDFX[market]
	if !ok || fx <= 0 {
		return nil, fmt.Errorf("시장 %s의 FX 설정 없음", market)
	}
	costBP, ok := paperWDCostBP[market]
	if !ok || costBP < 0 {
		return nil, fmt.Errorf("시장 %s의 비용 설정 없음", market)
	}
	if capital <= 0 {
		return nil, fmt.Errorf("유효하지 않은 자본: %.0f", capital)
	}

	stage1KRW := capital * paperWDPerPositionPct * paperWDStage1Pct
	qty := int(stage1KRW / fx / entryPrice)
	if qty < 1 {
		return nil, fmt.Errorf("배정 금액으로 1주 미만: price=%.4f fx=%.4f", entryPrice, fx)
	}
	stage1ActualKRW := float64(qty) * entryPrice * fx
	stage1Cost := stage1ActualKRW * costBP / 10000.0

	return &PaperWDPosition{
		ID:              paperWDPositionID(market, shcode, sig.Date),
		Market:          market,
		Shcode:          shcode,
		Hname:           hname,
		SignalDate:      sig.Date,
		HasDefBox:       sig.HasDefBox,
		DefBoxDate:      sig.DefBoxDate,
		DefBoxPrice:     sig.DefBoxPrice,
		DefBoxPriceRaw:  sig.DefBoxPriceOrigin,
		DefBoxBreakDate: "", // 미래값을 저장하지 않고 일별 캔들로 재평가한다.
		Stage1Date:      sig.Date,
		Stage1Price:     entryPrice,
		Stage1Qty:       qty,
		Stage1Amount:    stage1ActualKRW,
		Stage1Cost:      math.Round(stage1Cost),
		Status:          "stage1",
	}, nil
}

// processOpenPaperWDPositions 는 신호일부터 asOfDate까지의 실제 거래 캔들을 순서대로 재생한다.
// 누락 실행이 있어도 최초 DefBox 돌파일과 정확한 만기 거래일의 종가로 체결한다.
func processOpenPaperWDPositions(l *PaperWDLedger, snapshots map[string]paperWDSnapshot, asOfDate string) (stage2Entered, closed []*PaperWDPosition, errs []error) {
	for _, pos := range l.Positions {
		if pos.Status == "closed" {
			continue
		}
		snap, ok := snapshots[paperWDStockKey(pos.Market, pos.Shcode)]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: %s 시점 캔들 없음", pos.ID, asOfDate))
			continue
		}
		signalIdx := candleIndexByDate(snap.candles, pos.SignalDate)
		asOfIdx := candleIndexByDate(snap.candles, asOfDate)
		if signalIdx < 0 || asOfIdx < 0 || asOfIdx < signalIdx {
			errs = append(errs, fmt.Errorf("%s: 신호일(%s) 또는 기준일(%s) 거래 캔들 없음", pos.ID, pos.SignalDate, asOfDate))
			continue
		}

		if pos.Status == "stage1" && pos.HasDefBox {
			if pos.DefBoxPriceRaw <= 0 {
				errs = append(errs, fmt.Errorf("%s: DefBox 원본가격이 유효하지 않음", pos.ID))
				continue
			}
			lastBreakIdx := signalIdx + paperWDDefBoxTimeout
			if lastBreakIdx > asOfIdx {
				lastBreakIdx = asOfIdx
			}
			for i := signalIdx + 1; i <= lastBreakIdx; i++ {
				c := snap.candles[i]
				if c.CloseOrigin > pos.DefBoxPriceRaw {
					if err := enterStage2(pos, c.Date, c.CloseOrigin, l.Capital); err != nil {
						errs = append(errs, fmt.Errorf("%s: %w", pos.ID, err))
					} else {
						stage2Entered = append(stage2Entered, pos)
					}
					break
				}
			}
		}

		holdBars := paperWDDefaultHolding
		reason := "expiry"
		if pos.Status == "stage2" {
			holdBars = paperWDMaxHolding
			reason = "extended_expiry"
		}
		exitIdx := signalIdx + holdBars
		if exitIdx > asOfIdx {
			continue
		}
		exitCandle := snap.candles[exitIdx]
		if err := closePaperPosition(pos, exitCandle.Date, exitCandle.CloseOrigin, reason, l); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pos.ID, err))
			continue
		}
		closed = append(closed, pos)
	}
	return stage2Entered, closed, errs
}

func enterStage2(pos *PaperWDPosition, date string, entryPrice, capital float64) error {
	if err := validatePaperWDDate(date); err != nil {
		return fmt.Errorf("stage2 날짜 오류: %w", err)
	}
	if entryPrice <= 0 || math.IsNaN(entryPrice) || math.IsInf(entryPrice, 0) {
		return fmt.Errorf("유효하지 않은 stage2 체결가: %v", entryPrice)
	}
	fx := paperWDFX[pos.Market]
	costBP := paperWDCostBP[pos.Market]
	if fx <= 0 || capital <= 0 {
		return fmt.Errorf("stage2 자본/FX 설정 오류")
	}
	stage2KRW := capital * paperWDPerPositionPct * paperWDStage2Pct
	qty := int(stage2KRW / fx / entryPrice)
	if qty < 1 {
		return fmt.Errorf("stage2 배정 금액으로 1주 미만")
	}
	stage2ActualKRW := float64(qty) * entryPrice * fx

	pos.DefBoxBreakDate = date
	pos.Stage2Date = date
	pos.Stage2Price = entryPrice
	pos.Stage2Qty = qty
	pos.Stage2Amount = stage2ActualKRW
	pos.Stage2Cost = math.Round(stage2ActualKRW * costBP / 10000.0)
	pos.Status = "stage2"
	return nil
}

func closePaperPosition(pos *PaperWDPosition, exitDate string, exitPrice float64, reason string, l *PaperWDLedger) error {
	if err := validatePaperWDDate(exitDate); err != nil {
		return fmt.Errorf("청산일 오류: %w", err)
	}
	if exitPrice <= 0 || math.IsNaN(exitPrice) || math.IsInf(exitPrice, 0) {
		return fmt.Errorf("유효하지 않은 청산 종가: %v", exitPrice)
	}
	fx := paperWDFX[pos.Market]
	costBP := paperWDCostBP[pos.Market]
	totalAmount := pos.Stage1Amount + pos.Stage2Amount
	totalQty := pos.Stage1Qty + pos.Stage2Qty
	if fx <= 0 || totalAmount <= 0 || totalQty <= 0 {
		return fmt.Errorf("청산 계산 입력 오류: fx=%.4f amount=%.0f qty=%d", fx, totalAmount, totalQty)
	}

	exitKRW := float64(totalQty) * exitPrice * fx
	exitCost := math.Round(exitKRW * costBP / 10000.0)
	totalCost := pos.Stage1Cost + pos.Stage2Cost + exitCost
	realizedPL := exitKRW - totalAmount - totalCost
	realizedPLPct := realizedPL / totalAmount * 100

	pos.ExitDate = exitDate
	pos.ExitPrice = exitPrice
	pos.ExitReason = reason
	pos.ExitCost = exitCost
	pos.RealizedPL = math.Round(realizedPL)
	pos.RealizedPLPct = math.Round(realizedPLPct*100) / 100
	pos.Status = "closed"
	l.ClosedPL += pos.RealizedPL
	l.TradeCount++
	return nil
}

// ── 유틸리티 ────────────────────────────────────────────────────────────────

func countOpenPositions(l *PaperWDLedger) int {
	n := 0
	for _, p := range l.Positions {
		if p.Status != "closed" {
			n++
		}
	}
	return n
}

func countByStatus(l *PaperWDLedger) (stage1, stage2 int) {
	for _, p := range l.Positions {
		switch p.Status {
		case "stage1":
			stage1++
		case "stage2":
			stage2++
		}
	}
	return
}

func validatePaperWDDate(date string) error {
	if len(date) != 8 {
		return fmt.Errorf("날짜 %q는 YYYYMMDD 8자리여야 함", date)
	}
	if _, err := time.Parse("20060102", date); err != nil {
		return fmt.Errorf("날짜 %q 파싱 실패: %w", date, err)
	}
	return nil
}

func validatePaperWDLedger(l *PaperWDLedger) error {
	if l == nil || l.Capital <= 0 {
		return fmt.Errorf("원장 자본이 유효하지 않음")
	}
	seen := make(map[string]struct{}, len(l.Positions))
	for i, p := range l.Positions {
		if p == nil {
			return fmt.Errorf("원장 positions[%d]가 nil", i)
		}
		if p.ID == "" {
			return fmt.Errorf("원장 positions[%d] ID 없음", i)
		}
		if _, ok := seen[p.ID]; ok {
			return fmt.Errorf("원장 중복 ID: %s", p.ID)
		}
		seen[p.ID] = struct{}{}
		if _, ok := paperWDFX[p.Market]; !ok {
			return fmt.Errorf("%s: 알 수 없는 시장 %q", p.ID, p.Market)
		}
		if err := validatePaperWDDate(p.SignalDate); err != nil {
			return fmt.Errorf("%s: %w", p.ID, err)
		}
		if p.Stage1Price <= 0 || p.Stage1Qty <= 0 || p.Stage1Amount <= 0 {
			return fmt.Errorf("%s: stage1 체결 정보가 유효하지 않음", p.ID)
		}
		switch p.Status {
		case "stage1":
		case "stage2":
			if err := validatePaperWDDate(p.Stage2Date); err != nil {
				return fmt.Errorf("%s: %w", p.ID, err)
			}
			if p.Stage2Price <= 0 || p.Stage2Qty <= 0 || p.Stage2Amount <= 0 {
				return fmt.Errorf("%s: stage2 체결 정보가 유효하지 않음", p.ID)
			}
		case "closed":
			if err := validatePaperWDDate(p.ExitDate); err != nil {
				return fmt.Errorf("%s: %w", p.ID, err)
			}
			if p.ExitPrice <= 0 {
				return fmt.Errorf("%s: 청산가가 유효하지 않음", p.ID)
			}
		default:
			return fmt.Errorf("%s: 알 수 없는 상태 %q", p.ID, p.Status)
		}
	}
	return nil
}

func paperWDPositionID(market, shcode, signalDate string) string {
	return fmt.Sprintf("%s-%s-%s", market, shcode, signalDate)
}

func paperWDStockKey(market, shcode string) string {
	return market + "|" + shcode
}

func ledgerHasPositionID(l *PaperWDLedger, id string) bool {
	for _, p := range l.Positions {
		if p.ID == id {
			return true
		}
	}
	return false
}

func candlesThroughDate(candles []*box.Candle, asOfDate string) []*box.Candle {
	end := 0
	for end < len(candles) && candles[end].Date <= asOfDate {
		end++
	}
	return candles[:end]
}

func candleIndexByDate(candles []*box.Candle, date string) int {
	idx := sort.Search(len(candles), func(i int) bool { return candles[i].Date >= date })
	if idx < len(candles) && candles[idx].Date == date {
		return idx
	}
	return -1
}

func tradingBarsBetween(candles []*box.Candle, from, to string) (int, error) {
	fromIdx := candleIndexByDate(candles, from)
	toIdx := candleIndexByDate(candles, to)
	if fromIdx < 0 || toIdx < 0 || toIdx < fromIdx {
		return 0, fmt.Errorf("거래일 인덱스 조회 실패: %s~%s", from, to)
	}
	return toIdx - fromIdx, nil
}

// ── KR 종목명 캐시 ──────────────────────────────────────────────────────────

var krNameCache struct {
	sync.Once
	names map[string]string
}

func lookupKRName(code string) string {
	krNameCache.Do(func() {
		krNameCache.names = fetchStockNames()
	})
	if name, ok := krNameCache.names[code]; ok {
		return name
	}
	return code
}

// ── Telegram ────────────────────────────────────────────────────────────────

func tgPaperWDAlert(title string, pos *PaperWDPosition, stage string) {
	costBP := paperWDCostBP[pos.Market]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *B슬리브 Paper %s*\n", title))
	sb.WriteString(fmt.Sprintf("시장: *%s*  종목: `%s` (%s)\n", pos.Market, pos.Shcode, pos.Hname))
	sb.WriteString(fmt.Sprintf("신호일: %s\n", pos.SignalDate))

	switch stage {
	case "stage1":
		sb.WriteString(fmt.Sprintf("진입: W신호 50%% — %s\n", formatKRW(pos.Stage1Amount)))
		sb.WriteString(fmt.Sprintf("진입가: %.0f (수량: %d주)\n", pos.Stage1Price, pos.Stage1Qty))
		sb.WriteString(fmt.Sprintf("비용: %s (%dbp)\n", formatKRW(pos.Stage1Cost), int(costBP)))
		if pos.HasDefBox {
			sb.WriteString(fmt.Sprintf("DefBox: %s 가격: %.0f\n", pos.DefBoxDate, pos.DefBoxPriceRaw))
			sb.WriteString(fmt.Sprintf("▸ DefBox 돌파 대기 (최대 %d일)\n", paperWDDefBoxTimeout))
		}
	case "stage2":
		sb.WriteString(fmt.Sprintf("Stage2 진입: DefBox 돌파 50%% — %s\n", formatKRW(pos.Stage2Amount)))
		sb.WriteString(fmt.Sprintf("돌파가: %.0f (수량: %d주)\n", pos.Stage2Price, pos.Stage2Qty))
		sb.WriteString(fmt.Sprintf("비용: %s (%dbp)\n", formatKRW(pos.Stage2Cost), int(costBP)))
		totalAmt := pos.Stage1Amount + pos.Stage2Amount
		sb.WriteString(fmt.Sprintf("총투입: %s (100%%)\n", formatKRW(totalAmt)))
	case "exit":
		sb.WriteString(fmt.Sprintf("청산: %s\n", pos.ExitReason))
		sb.WriteString(fmt.Sprintf("청산가: %.0f\n", pos.ExitPrice))
		plSign := "🔴"
		if pos.RealizedPL > 0 {
			plSign = "🟢"
		}
		sb.WriteString(fmt.Sprintf("손익: %s *%+.0f원* (%.2f%%)\n", plSign, pos.RealizedPL, pos.RealizedPLPct))
		sb.WriteString(fmt.Sprintf("총비용: %s\n", formatKRW(pos.Stage1Cost+pos.Stage2Cost+pos.ExitCost)))
	}

	console.Tele("%s", sb.String())
}

func formatKRW(amount float64) string {
	abs := math.Abs(amount)
	if abs >= 1e8 {
		return fmt.Sprintf("%.1f억원", amount/1e8)
	}
	if abs >= 1e4 {
		return fmt.Sprintf("%.0f만원", amount/1e4)
	}
	return fmt.Sprintf("%.0f원", amount)
}

// ── 리포트 통계 ─────────────────────────────────────────────────────────────

func filterClosed(positions []*PaperWDPosition) []*PaperWDPosition {
	var out []*PaperWDPosition
	for _, p := range positions {
		if p.Status == "closed" {
			out = append(out, p)
		}
	}
	return out
}

func filterByMonth(positions []*PaperWDPosition, month string) []*PaperWDPosition {
	var out []*PaperWDPosition
	for _, p := range positions {
		if strings.HasPrefix(p.ExitDate, month) {
			out = append(out, p)
		}
	}
	return out
}

func paperWDMonthlyGate(monthlyClosed []*PaperWDPosition, stats map[string]interface{}) bool {
	return len(monthlyClosed) > 0 &&
		toFloat(stats["gross_realized_pl"]) > 0 &&
		toFloat(stats["net_realized_pl"]) > 0
}

func buildStats(positions []*PaperWDPosition, capital float64) map[string]interface{} {
	n := len(positions)
	s := map[string]interface{}{
		"total_trades":      n,
		"win_trades":        0,
		"win_rate_pct":      0.0,
		"gross_realized_pl": 0.0,
		"net_realized_pl":   0.0,
		"total_return":      0.0, // 하위 호환: net_realized_pl과 동일
		"total_return_pct":  0.0, // 고정 Paper 자본 대비 비용 후 수익률
		"avg_return":        0.0,
		"avg_return_pct":    0.0,
		"best_trade_pct":    0.0,
		"worst_trade_pct":   0.0,
		"total_cost":        0.0,
		"cost_ratio_pct":    0.0,
		"sharpe":            0.0,
		"sharpe_basis":      "monthly_net_return_sqrt12_sample_std",
		"sharpe_months":     0,
		"mdd_pct":           0.0,
	}
	if n == 0 || capital <= 0 {
		return s
	}

	wins := 0
	netPL := 0.0
	totalTradePct := 0.0
	totalCost := 0.0
	best := -math.MaxFloat64
	worst := math.MaxFloat64
	monthlyPL := make(map[string]float64)
	for _, p := range positions {
		netPL += p.RealizedPL
		totalTradePct += p.RealizedPLPct
		cost := p.Stage1Cost + p.Stage2Cost + p.ExitCost
		totalCost += cost
		if p.RealizedPL > 0 {
			wins++
		}
		if p.RealizedPLPct > best {
			best = p.RealizedPLPct
		}
		if p.RealizedPLPct < worst {
			worst = p.RealizedPLPct
		}
		if len(p.ExitDate) >= 6 {
			monthlyPL[p.ExitDate[:6]] += p.RealizedPL
		}
	}
	grossPL := netPL + totalCost
	avgPct := totalTradePct / float64(n)

	s["win_trades"] = wins
	s["win_rate_pct"] = round2(float64(wins) / float64(n) * 100)
	s["gross_realized_pl"] = math.Round(grossPL)
	s["net_realized_pl"] = math.Round(netPL)
	s["total_return"] = math.Round(netPL)
	s["total_return_pct"] = round2(netPL / capital * 100)
	s["avg_return"] = math.Round(netPL / float64(n))
	s["avg_return_pct"] = round2(avgPct)
	s["best_trade_pct"] = round2(best)
	s["worst_trade_pct"] = round2(worst)
	s["total_cost"] = math.Round(totalCost)
	if math.Abs(netPL) > 0 {
		s["cost_ratio_pct"] = round2(totalCost / math.Abs(netPL) * 100)
	}

	months := make([]string, 0, len(monthlyPL))
	for month := range monthlyPL {
		months = append(months, month)
	}
	sort.Strings(months)
	monthlyReturns := make([]float64, 0, len(months))
	for _, month := range months {
		monthlyReturns = append(monthlyReturns, monthlyPL[month]/capital)
	}
	s["sharpe_months"] = len(monthlyReturns)
	if len(monthlyReturns) >= 2 {
		mean := meanFloat64s(monthlyReturns)
		std := sampleStdFloat64s(monthlyReturns, mean)
		if std > 0 {
			s["sharpe"] = round2(mean / std * math.Sqrt(12))
		}
	}
	s["mdd_pct"] = round2(realizedEquityMDDPct(positions, capital))
	return s
}

func meanFloat64s(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func sampleStdFloat64s(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sumSq float64
	for _, v := range values {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(values)-1))
}

func realizedEquityMDDPct(positions []*PaperWDPosition, capital float64) float64 {
	dailyPL := make(map[string]float64)
	for _, p := range positions {
		dailyPL[p.ExitDate] += p.RealizedPL
	}
	dates := make([]string, 0, len(dailyPL))
	for date := range dailyPL {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	equity, peak, mdd := capital, capital, 0.0
	for _, date := range dates {
		equity += dailyPL[date]
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := (equity - peak) / peak * 100
			if dd < mdd {
				mdd = dd
			}
		}
	}
	return mdd
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func toFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

func printStats(s map[string]interface{}) {
	fmt.Printf("  트레이드: %v건  승률: %v%% (%v승)\n", s["total_trades"], s["win_rate_pct"], s["win_trades"])
	fmt.Printf("  총실현: %+.0f원  비용후: %+.0f원 (자본대비 %.2f%%)\n",
		toFloat(s["gross_realized_pl"]), toFloat(s["net_realized_pl"]), toFloat(s["total_return_pct"]))
	fmt.Printf("  평균수익: %+.0f원 (%.2f%%)  최고: %+.2f%%  최저: %+.2f%%\n",
		toFloat(s["avg_return"]), toFloat(s["avg_return_pct"]), toFloat(s["best_trade_pct"]), toFloat(s["worst_trade_pct"]))
	fmt.Printf("  총비용: %.0f원 (순손익대비 %.1f%%)\n", toFloat(s["total_cost"]), toFloat(s["cost_ratio_pct"]))
	fmt.Printf("  월수익률 Sharpe: %.2f (%v개월, √12)  실현 MDD: %.2f%%\n",
		toFloat(s["sharpe"]), s["sharpe_months"], toFloat(s["mdd_pct"]))
}
