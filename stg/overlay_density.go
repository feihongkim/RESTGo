package stg

// overlay_density.go — W중력(밀도 기반 오버레이) 포트폴리오 게이트 (2026-07-04).
//
// 배경: 배분 정책 연구(zpicture/wdefbox_alloc_policy_report.md, wdefbox_walkforward_density_report.md)에서
// "직전 28달력일 신호 밀도 ≥ 롤링 q60 임계값" 필터가 walkforward로 검증됨 (신호 분리 11/14승,
// OOS 체인 CAGR 4.20%). 이 파일은 그 규칙의 운영용 Go 구현이다.
//
// 아키텍처: 밀도는 전 종목 신호 흐름의 횡단면 집계이므로 종목 단독 룰 엔진(cond/·YAML)이 아니라
// 신호 생산(1단계) 이후의 포트폴리오 게이트(2단계)로 분리한다. 매수 룰은 불변.
//
// 시맨틱 (py/analysis/portfolio_sim_policy.py · walkforward_density.py 규칙 A와 동일):
//   Density(D)   = [D-WindowDays, D) 달력일 구간의 신호 수 합 — 당일 제외
//   Threshold(D) = [D-LookbackYears, D) 구간에 발생한 "신호별 밀도" 분포의 Quantile 분위수.
//                  같은 날의 신호들은 동일 밀도를 가지므로 날짜별 밀도 × 신호 수만큼 가중된다.
//   Pass         = Density ≥ Threshold
// 분위수 공식은 Python quantiles()와 동일: sorted[round(q*(n-1))] (인덱스 최근접).
//
// 신호 이력 소스: hannam DB StrategySignalDaily(strategy, trade_date CHAR(8), signal_count).

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DailySignalCount 는 일별 신호 수 1행 (StrategySignalDaily 테이블 대응).
type DailySignalCount struct {
	Date  string // YYYYMMDD
	Count int
}

// DensityGateConfig 는 게이트 파라미터. 기본값은 walkforward 검증값.
type DensityGateConfig struct {
	WindowDays    int     // 밀도 윈도우 (달력일). 기본 28
	Quantile      float64 // 임계 분위수. 기본 0.60
	LookbackYears int     // 임계값 산출 기간 (년). 기본 4
	SizingK       int     // 통과 시 포지션 크기 = equity/SizingK. 기본 50
}

// DefaultDensityGateConfig 는 검증된 운영 기본값을 반환한다.
func DefaultDensityGateConfig() DensityGateConfig {
	return DensityGateConfig{WindowDays: 28, Quantile: 0.60, LookbackYears: 4, SizingK: 50}
}

// GateDecision 은 특정 일자에 대한 게이트 판정 결과.
type GateDecision struct {
	Date            string
	Density         int     // 직전 WindowDays일 신호 수 (당일 제외)
	Threshold       int     // 롤링 분위수 임계값
	Pass            bool    // Density ≥ Threshold
	SuggestedWeight float64 // 통과 시 제안 비중 = 1/SizingK (미통과 시 0)
	HistoryDays     int     // 임계값 산출에 실제 사용된 신호-일 수 (부족 시 경고 판단용)
}

// DensityGate 는 신호 이력 기반 밀도 게이트. 불변(immutable)으로 생성 후 조회만 한다.
type DensityGate struct {
	cfg    DensityGateConfig
	dates  []time.Time // 오름차순 신호 발생일
	counts []int       // dates와 병렬
	prefix []int       // prefix[i] = counts[0..i-1] 합 (구간 합 O(logN))
}

const dateLayout = "20060102"

// NewDensityGate 는 일별 신호 이력으로 게이트를 만든다. 이력은 날짜 중복 없이 정렬돼 있지 않아도 된다.
func NewDensityGate(cfg DensityGateConfig, history []DailySignalCount) (*DensityGate, error) {
	if cfg.WindowDays <= 0 || cfg.Quantile <= 0 || cfg.Quantile >= 1 || cfg.LookbackYears <= 0 || cfg.SizingK <= 0 {
		return nil, fmt.Errorf("DensityGate: 잘못된 설정 %+v", cfg)
	}
	merged := map[time.Time]int{}
	for _, h := range history {
		t, err := time.Parse(dateLayout, h.Date)
		if err != nil {
			return nil, fmt.Errorf("DensityGate: 날짜 파싱 실패 %q: %w", h.Date, err)
		}
		merged[t] += h.Count
	}
	g := &DensityGate{cfg: cfg}
	for t := range merged {
		g.dates = append(g.dates, t)
	}
	sort.Slice(g.dates, func(i, j int) bool { return g.dates[i].Before(g.dates[j]) })
	g.counts = make([]int, len(g.dates))
	g.prefix = make([]int, len(g.dates)+1)
	for i, t := range g.dates {
		g.counts[i] = merged[t]
		g.prefix[i+1] = g.prefix[i] + g.counts[i]
	}
	return g, nil
}

// sumRange 는 [from, to) 구간 신호 수 합.
func (g *DensityGate) sumRange(from, to time.Time) int {
	lo := sort.Search(len(g.dates), func(i int) bool { return !g.dates[i].Before(from) })
	hi := sort.Search(len(g.dates), func(i int) bool { return !g.dates[i].Before(to) })
	return g.prefix[hi] - g.prefix[lo]
}

// DensityOn 은 date 기준 직전 WindowDays 달력일(당일 제외) 신호 수.
func (g *DensityGate) DensityOn(date string) (int, error) {
	d, err := time.Parse(dateLayout, date)
	if err != nil {
		return 0, fmt.Errorf("DensityGate: 날짜 파싱 실패 %q: %w", date, err)
	}
	return g.sumRange(d.AddDate(0, 0, -g.cfg.WindowDays), d), nil
}

// ThresholdOn 은 date 기준 직전 LookbackYears년 내 신호별 밀도 분포의 Quantile 분위수와
// 표본 크기(신호 수)를 반환한다. 표본이 없으면 임계값 0 (게이트 무조건 통과 — 호출측에서 HistoryDays로 경고).
func (g *DensityGate) ThresholdOn(date string) (int, int, error) {
	d, err := time.Parse(dateLayout, date)
	if err != nil {
		return 0, 0, fmt.Errorf("DensityGate: 날짜 파싱 실패 %q: %w", date, err)
	}
	from := d.AddDate(-g.cfg.LookbackYears, 0, 0)
	// 구간 내 각 신호일의 밀도를 신호 수만큼 가중해 수집
	var densities []int
	total := 0
	lo := sort.Search(len(g.dates), func(i int) bool { return !g.dates[i].Before(from) })
	hi := sort.Search(len(g.dates), func(i int) bool { return !g.dates[i].Before(d) })
	for i := lo; i < hi; i++ {
		dens := g.sumRange(g.dates[i].AddDate(0, 0, -g.cfg.WindowDays), g.dates[i])
		for c := 0; c < g.counts[i]; c++ {
			densities = append(densities, dens)
		}
		total += g.counts[i]
	}
	if total == 0 {
		return 0, 0, nil
	}
	sort.Ints(densities)
	// Python quantiles()와 동일: idx = round(q*(n-1)) — Python round는 half-even이므로 동일 구현
	idx := roundHalfEven(g.cfg.Quantile * float64(total-1))
	if idx < 0 {
		idx = 0
	}
	if idx > total-1 {
		idx = total - 1
	}
	return densities[idx], total, nil
}

// ─────────────────────────── 설정 YAML · DB 로더 ───────────────────────────

// OverlayConfig 는 오버레이 게이트 설정 파일(rules/overlay_wdefbox.yaml) 구조.
type OverlayConfig struct {
	Strategies    []string `yaml:"strategies"`     // 신호 흐름을 합산할 전략명 (StrategySignalDaily.strategy)
	WindowDays    int      `yaml:"window_days"`    // 기본 28
	Quantile      float64  `yaml:"quantile"`       // 기본 0.60
	LookbackYears int      `yaml:"lookback_years"` // 기본 4
	SizingK       int      `yaml:"sizing_k"`       // 기본 50
}

// LoadOverlayConfig 는 오버레이 YAML을 읽는다. 누락 필드는 검증된 기본값으로 채운다.
func LoadOverlayConfig(path string) (OverlayConfig, error) {
	cfg := OverlayConfig{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("오버레이 설정 로드 실패 (%s): %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("오버레이 설정 파싱 실패 (%s): %w", path, err)
	}
	def := DefaultDensityGateConfig()
	if cfg.WindowDays == 0 {
		cfg.WindowDays = def.WindowDays
	}
	if cfg.Quantile == 0 {
		cfg.Quantile = def.Quantile
	}
	if cfg.LookbackYears == 0 {
		cfg.LookbackYears = def.LookbackYears
	}
	if cfg.SizingK == 0 {
		cfg.SizingK = def.SizingK
	}
	if len(cfg.Strategies) == 0 {
		return cfg, fmt.Errorf("오버레이 설정 (%s): strategies가 비어 있음", path)
	}
	return cfg, nil
}

// GateConfig 는 OverlayConfig에서 게이트 파라미터만 추출한다.
func (c OverlayConfig) GateConfig() DensityGateConfig {
	return DensityGateConfig{WindowDays: c.WindowDays, Quantile: c.Quantile,
		LookbackYears: c.LookbackYears, SizingK: c.SizingK}
}

// FetchSignalDailyCounts 는 hannam DB StrategySignalDaily에서 지정 전략들의
// 일별 신호 수를 날짜별 합산해 반환한다 (전략 집합 = 하나의 신호 흐름).
func FetchSignalDailyCounts(db *sql.DB, strategies []string) ([]DailySignalCount, error) {
	if len(strategies) == 0 {
		return nil, fmt.Errorf("FetchSignalDailyCounts: 전략 목록이 비어 있음")
	}
	placeholders := make([]string, len(strategies))
	args := make([]interface{}, len(strategies))
	for i, s := range strategies {
		placeholders[i] = fmt.Sprintf("@p%d", i+1)
		args[i] = s
	}
	query := fmt.Sprintf(`
		SELECT trade_date, SUM(signal_count)
		FROM StrategySignalDaily
		WHERE strategy IN (%s)
		GROUP BY trade_date
		ORDER BY trade_date`, strings.Join(placeholders, ","))
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("StrategySignalDaily 조회 실패: %w", err)
	}
	defer rows.Close()
	var out []DailySignalCount
	for rows.Next() {
		var r DailySignalCount
		if err := rows.Scan(&r.Date, &r.Count); err != nil {
			return nil, fmt.Errorf("row 스캔 실패: %w", err)
		}
		r.Date = strings.TrimSpace(r.Date) // CHAR(8) 패딩 방어
		out = append(out, r)
	}
	return out, rows.Err()
}

// roundHalfEven 은 Python round()와 동일한 은행가 반올림 (패리티 보장용).
func roundHalfEven(x float64) int {
	f := math.Floor(x)
	switch diff := x - f; {
	case diff > 0.5:
		return int(f) + 1
	case diff < 0.5:
		return int(f)
	case int(f)%2 == 0:
		return int(f)
	default:
		return int(f) + 1
	}
}

// Evaluate 는 date에 대한 게이트 판정을 반환한다.
func (g *DensityGate) Evaluate(date string) (GateDecision, error) {
	dens, err := g.DensityOn(date)
	if err != nil {
		return GateDecision{}, err
	}
	thr, n, err := g.ThresholdOn(date)
	if err != nil {
		return GateDecision{}, err
	}
	dec := GateDecision{Date: date, Density: dens, Threshold: thr, HistoryDays: n}
	dec.Pass = dens >= thr && n > 0
	if dec.Pass {
		dec.SuggestedWeight = 1.0 / float64(g.cfg.SizingK)
	}
	return dec, nil
}
