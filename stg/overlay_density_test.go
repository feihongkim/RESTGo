package stg

import "testing"

func mkGate(t *testing.T, cfg DensityGateConfig, hist []DailySignalCount) *DensityGate {
	t.Helper()
	g, err := NewDensityGate(cfg, hist)
	if err != nil {
		t.Fatalf("NewDensityGate: %v", err)
	}
	return g
}

// 윈도우 경계: [D-28, D) — D-28일은 포함, 당일과 D-29일은 제외 (Python bisect 시맨틱 동일)
func TestDensityWindowBoundary(t *testing.T) {
	cfg := DefaultDensityGateConfig()
	g := mkGate(t, cfg, []DailySignalCount{
		{"20250101", 5},  // D-29 → 제외
		{"20250102", 7},  // D-28 → 포함
		{"20250129", 3},  // D-1  → 포함
		{"20250130", 11}, // 당일  → 제외
	})
	dens, err := g.DensityOn("20250130")
	if err != nil {
		t.Fatal(err)
	}
	if dens != 10 {
		t.Errorf("DensityOn = %d, want 10 (7+3)", dens)
	}
}

// 같은 날짜 중복 행은 합산되어야 한다
func TestDuplicateDateMerge(t *testing.T) {
	g := mkGate(t, DefaultDensityGateConfig(), []DailySignalCount{
		{"20250110", 2}, {"20250110", 3},
	})
	dens, _ := g.DensityOn("20250120")
	if dens != 5 {
		t.Errorf("중복 날짜 합산 실패: %d, want 5", dens)
	}
}

// 임계값: 신호별 밀도 분포의 q60, Python round(q*(n-1)) half-even 공식과 일치해야 한다
func TestThresholdQuantileParity(t *testing.T) {
	cfg := DefaultDensityGateConfig()
	// 40일 간격(윈도우 겹침 없음)으로 신호 배치 → 각 날짜의 밀도가 독립적으로 결정됨
	// day1: count 1 (직전 밀도 0), day2: count 2 (밀도 0), day3: count 3 (밀도 0) ...
	// 대신 겹침을 만들기 위해 15일 간격 3회 배치:
	//   d0=20250101 c=4 (밀도 0)
	//   d1=20250116 c=2 (밀도 4)   — d0가 윈도우 안
	//   d2=20250131 c=1 (밀도 6)   — d0, d1 윈도우 안 (30일 전 = 20250101, [1/3, 1/31) → d0 제외? 1/31-28=1/3 → d0=1/1 제외, d1만 포함=2)
	g := mkGate(t, cfg, []DailySignalCount{
		{"20250101", 4}, {"20250116", 2}, {"20250131", 1},
	})
	// 각 신호일 밀도 검산
	d0, _ := g.DensityOn("20250101") // 0
	d1, _ := g.DensityOn("20250116") // 4
	d2, _ := g.DensityOn("20250131") // [1/3, 1/31) → 1/16만 → 2
	if d0 != 0 || d1 != 4 || d2 != 2 {
		t.Fatalf("밀도 검산 실패: %d %d %d, want 0 4 2", d0, d1, d2)
	}
	// 조회일 2025-02-10: lookback 내 신호별 밀도 멀티셋 = [0,0,0,0, 4,4, 2] → 정렬 [0,0,0,0,2,4,4]
	// n=7, idx = round(0.6*6) = round(3.6) = 4 → densities[4] = 2
	thr, n, err := g.ThresholdOn("20250210")
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Errorf("표본 수 = %d, want 7", n)
	}
	if thr != 2 {
		t.Errorf("Threshold = %d, want 2", thr)
	}
}

// half-even 반올림이 Python round와 일치하는지
func TestRoundHalfEven(t *testing.T) {
	cases := []struct {
		x    float64
		want int
	}{{2.5, 2}, {3.5, 4}, {2.4, 2}, {2.6, 3}, {0.0, 0}, {0.5, 0}, {1.5, 2}}
	for _, c := range cases {
		if got := roundHalfEven(c.x); got != c.want {
			t.Errorf("roundHalfEven(%v) = %d, want %d (Python round 동일해야 함)", c.x, got, c.want)
		}
	}
}

// Evaluate: 통과/미통과와 제안 비중
func TestEvaluate(t *testing.T) {
	cfg := DefaultDensityGateConfig()
	g := mkGate(t, cfg, []DailySignalCount{
		{"20250101", 4}, {"20250116", 2}, {"20250131", 1},
	})
	// 2025-02-10: 밀도 = [1/13, 2/10) → 1/16(2) + 1/31(1) = 3, 임계값 = 2 (위 테스트) → 통과
	dec, err := g.Evaluate("20250210")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Density != 3 || dec.Threshold != 2 || !dec.Pass {
		t.Errorf("Evaluate = %+v, want Density 3 / Threshold 2 / Pass", dec)
	}
	if dec.SuggestedWeight != 1.0/50.0 {
		t.Errorf("SuggestedWeight = %v, want 0.02", dec.SuggestedWeight)
	}
	// 먼 미래(이력이 lookback 밖) → 표본 0 → 미통과
	dec2, _ := g.Evaluate("20351231")
	if dec2.Pass || dec2.HistoryDays != 0 {
		t.Errorf("이력 없음인데 통과: %+v", dec2)
	}
}

// 잘못된 설정 거부
func TestInvalidConfig(t *testing.T) {
	_, err := NewDensityGate(DensityGateConfig{WindowDays: 0, Quantile: 0.6, LookbackYears: 4, SizingK: 50}, nil)
	if err == nil {
		t.Error("WindowDays=0 인데 에러 없음")
	}
	_, err = NewDensityGate(DensityGateConfig{WindowDays: 28, Quantile: 1.5, LookbackYears: 4, SizingK: 50}, nil)
	if err == nil {
		t.Error("Quantile=1.5 인데 에러 없음")
	}
}

// count=0 행은 밀도·임계 계산에 기여하지 않아야 한다.
// daily_batch가 "신호 0건인 날"과 "배치 미실행일"을 구분하려고 0행을 남기는데,
// 이게 게이트 판정을 바꾸면 안 된다 (2026-07-30 도입 시 전제).
func TestZeroCountRowsDoNotAffectDecision(t *testing.T) {
	cfg := DefaultDensityGateConfig()
	base := []DailySignalCount{
		{"20250102", 7}, {"20250115", 20}, {"20250129", 3},
	}
	withZeros := append([]DailySignalCount{}, base...)
	for _, d := range []string{"20250103", "20250106", "20250120", "20250128"} {
		withZeros = append(withZeros, DailySignalCount{d, 0})
	}

	a, err := mkGate(t, cfg, base).Evaluate("20250130")
	if err != nil {
		t.Fatal(err)
	}
	b, err := mkGate(t, cfg, withZeros).Evaluate("20250130")
	if err != nil {
		t.Fatal(err)
	}
	if a.Density != b.Density || a.Threshold != b.Threshold || a.Pass != b.Pass || a.HistoryDays != b.HistoryDays {
		t.Errorf("0행이 판정을 바꿨다:\n  없이: 밀도%d 임계%d pass=%v 표본%d\n  있이: 밀도%d 임계%d pass=%v 표본%d",
			a.Density, a.Threshold, a.Pass, a.HistoryDays,
			b.Density, b.Threshold, b.Pass, b.HistoryDays)
	}
	// 다만 커버리지는 늘어야 한다 — 0행의 존재 이유가 그것이다
	if b.RecordedInWindow <= a.RecordedInWindow {
		t.Errorf("0행이 커버리지에 반영되지 않음: %d → %d", a.RecordedInWindow, b.RecordedInWindow)
	}
}

// 이력이 판정일보다 뒤처지면 LastRecorded로 드러나야 한다 (2026-07-30 오판정 재발 방지).
func TestGateDetectsHistoryHole(t *testing.T) {
	cfg := DefaultDensityGateConfig()
	g := mkGate(t, cfg, []DailySignalCount{
		{"20260601", 50}, {"20260610", 60}, {"20260626", 40},
	})

	// 기록이 끊긴 뒤(20260730)를 판정하면 윈도우가 비어 밀도 0 — 가뭄이 아니라 미기록이다
	dec, err := g.Evaluate("20260730")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Density != 0 {
		t.Errorf("밀도 = %d, want 0", dec.Density)
	}
	if dec.RecordedInWindow != 0 {
		t.Errorf("RecordedInWindow = %d, want 0 (윈도우에 기록 없음)", dec.RecordedInWindow)
	}
	if dec.LastRecorded != "20260626" {
		t.Errorf("LastRecorded = %q, want 20260626", dec.LastRecorded)
	}
	if !(dec.LastRecorded < dec.Date) {
		t.Error("이력 마지막 기록일이 판정일보다 과거인데 탐지되지 않음")
	}

	// 기록이 있는 구간을 판정하면 커버리지가 잡혀야 한다
	ok, err := g.Evaluate("20260627")
	if err != nil {
		t.Fatal(err)
	}
	if ok.RecordedInWindow != 3 {
		t.Errorf("RecordedInWindow = %d, want 3", ok.RecordedInWindow)
	}
}
