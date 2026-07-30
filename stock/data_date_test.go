package stock

import "testing"

// TestResolveDataDate 는 기준일 선택 규칙을 고정한다.
// 이 함수가 최신 날짜를 잘못 고르면 소수 종목 기준 신호 수가 StrategySignalDaily에
// 박히고, 기존 행 보존 규약 때문에 나중에 데이터가 차도 덮어써지지 않는다.
func TestResolveDataDate(t *testing.T) {
	tests := []struct {
		name      string
		lastDates map[string]int
		threshold float64
		wantDate  string
		wantMax   string
	}{
		{
			// 2026-07-30 실제 상황: 적재가 종목코드 순으로 진행 중 (23/4298만 최신)
			name:      "적재 진행 중 — 커버리지 미달 최신일을 건너뛴다",
			lastDates: map[string]int{"20260730": 23, "20260720": 4275},
			threshold: 0.8,
			wantDate:  "20260720",
			wantMax:   "20260730",
		},
		{
			name:      "적재 완료 — 최신일을 그대로 쓴다",
			lastDates: map[string]int{"20260730": 4298},
			threshold: 0.8,
			wantDate:  "20260730",
			wantMax:   "20260730",
		},
		{
			// 상장폐지·거래정지로 뒤처진 종목이 소수 섞이는 정상 상태
			name:      "소수 종목만 뒤처짐 — 최신일 유지",
			lastDates: map[string]int{"20260730": 4000, "20260720": 200, "20260601": 98},
			threshold: 0.8,
			wantDate:  "20260730",
			wantMax:   "20260730",
		},
		{
			name:      "누적으로 임계 충족 — 경계에서 멈춘다",
			lastDates: map[string]int{"20260730": 50, "20260729": 35, "20260728": 15},
			threshold: 0.8,
			wantDate:  "20260729", // 누적 85% ≥ 80%, 20260730 단독은 50%라 미달
			wantMax:   "20260730",
		},
		{
			name:      "임계 1.0 — 전 종목이 커버하는 날짜만 허용",
			lastDates: map[string]int{"20260730": 4000, "20260720": 298},
			threshold: 1.0,
			wantDate:  "20260720",
			wantMax:   "20260730",
		},
		{
			name:      "단일 종목",
			lastDates: map[string]int{"20260730": 1},
			threshold: 0.8,
			wantDate:  "20260730",
			wantMax:   "20260730",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, cov, max := resolveDataDate(tt.lastDates, tt.threshold)
			if got != tt.wantDate {
				t.Errorf("기준일 = %q, want %q (커버리지 %.3f)", got, tt.wantDate, cov)
			}
			if max != tt.wantMax {
				t.Errorf("최댓값 = %q, want %q", max, tt.wantMax)
			}
			if cov < tt.threshold && cov != 1.0 {
				t.Errorf("커버리지 %.3f 가 임계 %.3f 미만인데 선택됨", cov, tt.threshold)
			}
		})
	}
}

func TestResolveDataDateEmpty(t *testing.T) {
	date, cov, max := resolveDataDate(map[string]int{}, 0.8)
	if date != "" || cov != 0 || max != "" {
		t.Errorf("빈 입력 = (%q, %v, %q), want (\"\", 0, \"\")", date, cov, max)
	}
}
