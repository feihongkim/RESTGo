package stock

import (
	"RESTGo/box"
	"RESTGo/stg"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func paperWDTestCandles(n int, start time.Time, close float64) []*box.Candle {
	out := make([]*box.Candle, 0, n)
	for d := start; len(out) < n; d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		out = append(out, &box.Candle{
			Date: d.Format("20060102"), OpenOrigin: close,
			HighOrigin: close, LowOrigin: close, CloseOrigin: close,
		})
	}
	return out
}

func newPaperWDTestPosition(t *testing.T, candles []*box.Candle, hasDefBox bool, defBoxPrice float64) *PaperWDPosition {
	t.Helper()
	sig := stg.WDefBoxSignal{
		Date: candles[0].Date, Pos: 0, HasDefBox: hasDefBox,
		DefBoxDate: candles[0].Date, DefBoxPriceOrigin: defBoxPrice,
	}
	pos, err := createPaperPosition("KR", "TEST", "테스트", sig, candles, paperWDCapital)
	if err != nil {
		t.Fatalf("createPaperPosition: %v", err)
	}
	return pos
}

func TestPaperWDStage2AndExitUseActualTradingCandles(t *testing.T) {
	candles := paperWDTestCandles(35, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100)
	candles[5].CloseOrigin = 111 // 최초 DefBox 종가 돌파
	candles[30].CloseOrigin = 130
	pos := newPaperWDTestPosition(t, candles, true, 110)
	ledger := &PaperWDLedger{Version: 1, Capital: paperWDCapital, Positions: []*PaperWDPosition{pos}}
	snapshots := map[string]paperWDSnapshot{
		paperWDStockKey("KR", "TEST"): {candles: candles},
	}

	stage2, closed, errs := processOpenPaperWDPositions(ledger, snapshots, candles[30].Date)
	if len(errs) != 0 {
		t.Fatalf("process errors: %v", errs)
	}
	if len(stage2) != 1 || len(closed) != 1 {
		t.Fatalf("stage2=%d closed=%d, want 1/1", len(stage2), len(closed))
	}
	if pos.Stage2Date != candles[5].Date || pos.Stage2Price != 111 {
		t.Fatalf("stage2 fill = %s %.2f, want %s 111", pos.Stage2Date, pos.Stage2Price, candles[5].Date)
	}
	if pos.ExitDate != candles[30].Date || pos.ExitPrice != 130 {
		t.Fatalf("exit fill = %s %.2f, want %s 130", pos.ExitDate, pos.ExitPrice, candles[30].Date)
	}
	if pos.ExitReason != "extended_expiry" || pos.Status != "closed" {
		t.Fatalf("exit state = %s/%s", pos.ExitReason, pos.Status)
	}
}

func TestPaperWDDefaultExpiryUses20thTradingBarClose(t *testing.T) {
	candles := paperWDTestCandles(25, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100)
	candles[20].CloseOrigin = 90
	pos := newPaperWDTestPosition(t, candles, false, 0)
	ledger := &PaperWDLedger{Version: 1, Capital: paperWDCapital, Positions: []*PaperWDPosition{pos}}
	snapshots := map[string]paperWDSnapshot{
		paperWDStockKey("KR", "TEST"): {candles: candles},
	}

	_, closed, errs := processOpenPaperWDPositions(ledger, snapshots, candles[20].Date)
	if len(errs) != 0 || len(closed) != 1 {
		t.Fatalf("closed=%d errs=%v", len(closed), errs)
	}
	if pos.ExitPrice != 90 || pos.ExitDate != candles[20].Date || pos.ExitReason != "expiry" {
		t.Fatalf("unexpected expiry: date=%s price=%.2f reason=%s", pos.ExitDate, pos.ExitPrice, pos.ExitReason)
	}
	calendarDays := int(mustParsePaperWDDate(t, candles[20].Date).Sub(mustParsePaperWDDate(t, candles[0].Date)).Hours() / 24)
	if calendarDays == 20 {
		t.Fatalf("fixture must prove trading bars differ from calendar days")
	}
	bars, err := tradingBarsBetween(candles, candles[0].Date, candles[20].Date)
	if err != nil || bars != 20 {
		t.Fatalf("trading bars=%d err=%v", bars, err)
	}
}

func TestPaperWDRejectsFakeOrMismatchedEntryPrice(t *testing.T) {
	candles := paperWDTestCandles(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100)
	sig := stg.WDefBoxSignal{Date: candles[0].Date, Pos: 0}
	candles[0].CloseOrigin = 0
	if pos, err := createPaperPosition("KR", "TEST", "테스트", sig, candles, paperWDCapital); err == nil || pos != nil {
		t.Fatalf("zero price must be rejected, got pos=%+v err=%v", pos, err)
	}

	candles[0].CloseOrigin = 100
	sig.Date = candles[1].Date
	if _, err := createPaperPosition("KR", "TEST", "테스트", sig, candles, paperWDCapital); err == nil {
		t.Fatal("signal/candle date mismatch must be rejected")
	}
}

func TestPaperWDLedgerValidationRejectsInvalidDateAndDuplicateID(t *testing.T) {
	candles := paperWDTestCandles(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100)
	pos := newPaperWDTestPosition(t, candles, false, 0)
	ledger := &PaperWDLedger{Version: 1, Capital: paperWDCapital, Positions: []*PaperWDPosition{pos}}

	pos.SignalDate = "bad-date"
	if err := validatePaperWDLedger(ledger); err == nil {
		t.Fatal("invalid date must fail instead of becoming a zombie position")
	}
	pos.SignalDate = candles[0].Date
	ledger.Positions = append(ledger.Positions, pos)
	if err := validatePaperWDLedger(ledger); err == nil {
		t.Fatal("duplicate position ID must fail validation")
	}
}

func TestPaperWDPositionIDIsIdempotencyKey(t *testing.T) {
	candles := paperWDTestCandles(2, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), 100)
	pos := newPaperWDTestPosition(t, candles, false, 0)
	ledger := &PaperWDLedger{Capital: paperWDCapital, Positions: []*PaperWDPosition{pos}}
	id := paperWDPositionID("KR", "TEST", candles[0].Date)
	if !ledgerHasPositionID(ledger, id) {
		t.Fatalf("existing ID %s was not detected", id)
	}
}

func TestPaperWDStatsUseMonthlyAnnualizedSharpeAndProfitGate(t *testing.T) {
	capital := 100_000.0
	positions := []*PaperWDPosition{
		{ExitDate: "20260130", RealizedPL: 1_000, RealizedPLPct: 1, Status: "closed"},
		{ExitDate: "20260227", RealizedPL: 2_000, RealizedPLPct: 2, Status: "closed"},
		{ExitDate: "20260331", RealizedPL: 3_000, RealizedPLPct: 3, Status: "closed"},
	}
	stats := buildStats(positions, capital)
	wantSharpe := 2 * math.Sqrt(12) // monthly returns 1%,2%,3%: mean=2%, sample std=1%
	if math.Abs(toFloat(stats["sharpe"])-round2(wantSharpe)) > 1e-9 {
		t.Fatalf("sharpe=%v want %.2f", stats["sharpe"], round2(wantSharpe))
	}
	if stats["sharpe_months"] != 3 {
		t.Fatalf("sharpe_months=%v want 3", stats["sharpe_months"])
	}
	if !paperWDMonthlyGate(positions, stats) {
		t.Fatal("positive gross and net monthly realized profit must pass")
	}
	stats["net_realized_pl"] = -1.0
	if paperWDMonthlyGate(positions, stats) {
		t.Fatal("negative after-cost profit must fail regardless of Sharpe")
	}
}

func TestAtomicWriteFileReplacesCompleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.json")
	if err := atomicWriteFile(path, []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(path, []byte("second"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "second" {
		t.Fatalf("content=%q err=%v", got, err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".ledger.json.tmp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %v err=%v", matches, err)
	}
}

func TestSavePaperWDLedgerPreservesScanDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	ledger := &PaperWDLedger{
		Version: 1, Capital: paperWDCapital, StartDate: "20260102",
		LastUpdated: "20260203", Positions: []*PaperWDPosition{},
	}
	if err := savePaperWDLedgerAt(path, ledger); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved PaperWDLedger
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.LastUpdated != "20260203" {
		t.Fatalf("LastUpdated=%q, want exact scan date", saved.LastUpdated)
	}
}

func mustParsePaperWDDate(t *testing.T, date string) time.Time {
	t.Helper()
	v, err := time.Parse("20060102", date)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
