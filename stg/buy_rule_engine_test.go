package stg

import (
	"os"
	"path/filepath"
	"testing"

	"RESTGo/box"
)

// 테스트 전용 조건 등록 (전역 레지스트리 공유 → 충돌 방지를 위해 t_ 접두사 사용)
func init() {
	RegisterCondition("t_true", func(ctx *box.TradingContext, s Settings) bool { return true })
	RegisterCondition("t_false", func(ctx *box.TradingContext, s Settings) bool { return false })
}

func testCtx(defCount int) *box.TradingContext {
	ctx := box.NewTradingContext(nil, nil)
	ctx.DefCount = defCount
	return ctx
}

// ─────────────────────────────────────────────
// EvaluateRules — when / when_not / any_of
// ─────────────────────────────────────────────

func TestEvaluateRules(t *testing.T) {
	tests := []struct {
		name       string
		rule       RuleConfig
		defCount   int
		wantSignal string
	}{
		{
			name:       "when 모두 통과 → 매칭",
			rule:       RuleConfig{Name: "R1", When: []string{"t_true", "t_true"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "buy",
		},
		{
			name:       "when 하나라도 실패 → 미매칭",
			rule:       RuleConfig{Name: "R1", When: []string{"t_true", "t_false"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "",
		},
		{
			name:       "when_not 조건이 true → 차단",
			rule:       RuleConfig{Name: "R1", When: []string{"t_true"}, WhenNot: []string{"t_true"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "",
		},
		{
			name:       "when_not 조건이 false → 통과",
			rule:       RuleConfig{Name: "R1", When: []string{"t_true"}, WhenNot: []string{"t_false"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "buy",
		},
		{
			name:       "any_of 하나라도 true → 통과",
			rule:       RuleConfig{Name: "R1", AnyOf: []string{"t_false", "t_true"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "buy",
		},
		{
			name:       "any_of 모두 false → 미매칭",
			rule:       RuleConfig{Name: "R1", AnyOf: []string{"t_false", "t_false"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "",
		},
		{
			name:       "def_count 일치 → 통과",
			rule:       RuleConfig{Name: "R1", DefCount: 1, When: []string{"t_true"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "buy",
		},
		{
			name:       "def_count 불일치 → 미매칭",
			rule:       RuleConfig{Name: "R1", DefCount: 1, When: []string{"t_true"}, Signal: "buy"},
			defCount:   2,
			wantSignal: "",
		},
		{
			name:       "def_count_min 이상 → 통과",
			rule:       RuleConfig{Name: "R1", DefCountMin: 2, When: []string{"t_true"}, Signal: "buy"},
			defCount:   3,
			wantSignal: "buy",
		},
		{
			name:       "def_count_min 미만 → 미매칭",
			rule:       RuleConfig{Name: "R1", DefCountMin: 2, When: []string{"t_true"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "",
		},
		{
			name:       "미등록 조건명 → 미매칭 (조용한 통과 금지)",
			rule:       RuleConfig{Name: "R1", When: []string{"t_does_not_exist"}, Signal: "buy"},
			defCount:   1,
			wantSignal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signal, _ := EvaluateRules([]RuleConfig{tt.rule}, testCtx(tt.defCount), Settings{})
			if signal != tt.wantSignal {
				t.Errorf("EvaluateRules() signal = %q, want %q", signal, tt.wantSignal)
			}
		})
	}
}

// ─────────────────────────────────────────────
// 전략별 중복 신호 방지 (C# LastBuySignalPosition_StrategyN 포팅)
// ─────────────────────────────────────────────

func TestEvaluateRules_DuplicatePrevention(t *testing.T) {
	rules := []RuleConfig{
		{Name: "R1", When: []string{"t_true"}, Signal: "buy1"},
		{Name: "R2", When: []string{"t_true"}, Signal: "buy2"},
	}
	ctx := testCtx(1)

	// 1회차: R1 발화, 위치 기록
	ctx.Position = 10
	if signal, name := EvaluateRules(rules, ctx, Settings{}); signal != "buy1" || name != "R1" {
		t.Fatalf("1회차 = (%q, %q), want (buy1, R1)", signal, name)
	}
	if pos, ok := ctx.LastBuySignalPosition["R1"]; !ok || pos != 10 {
		t.Errorf("R1 발화 위치 기록 안 됨: %v, %v", pos, ok)
	}

	// 2회차: R1은 차단되고 다음 전략 R2가 발화 (C#: 다른 전략은 계속 평가)
	ctx.Position = 11
	if signal, name := EvaluateRules(rules, ctx, Settings{}); signal != "buy2" || name != "R2" {
		t.Fatalf("2회차 = (%q, %q), want (buy2, R2)", signal, name)
	}

	// 3회차: 둘 다 발화 완료 → 무신호
	ctx.Position = 12
	if signal, _ := EvaluateRules(rules, ctx, Settings{}); signal != "" {
		t.Fatalf("3회차 = %q, want 무신호", signal)
	}

	// DefBox 변경 시 리셋 → 다시 발화 가능
	ctx.ResetBuySignalPositions()
	ctx.Position = 20
	if signal, name := EvaluateRules(rules, ctx, Settings{}); signal != "buy1" || name != "R1" {
		t.Fatalf("리셋 후 = (%q, %q), want (buy1, R1)", signal, name)
	}
}

func TestEvaluateRules_FirstMatchWins(t *testing.T) {
	rules := []RuleConfig{
		{Name: "Strict", When: []string{"t_false"}, Signal: "strict_buy"},
		{Name: "Relaxed", When: []string{"t_true"}, Signal: "relaxed_buy"},
		{Name: "MostRelaxed", When: []string{"t_true"}, Signal: "most_relaxed_buy"},
	}

	signal, name := EvaluateRules(rules, testCtx(1), Settings{})
	if signal != "relaxed_buy" || name != "Relaxed" {
		t.Errorf("첫 매칭 룰이 승리해야 함: got (%q, %q), want (relaxed_buy, Relaxed)", signal, name)
	}
}

// ─────────────────────────────────────────────
// LoadRules — YAML 파싱
// ─────────────────────────────────────────────

func TestLoadRules(t *testing.T) {
	yamlContent := `
strategies:
  - name: "TestStrategy"
    def_count: 1
    when:
      - t_true
    when_not:
      - t_false
    any_of:
      - t_true
    signal: "테스트매수"
  - name: "MultiStrategy"
    def_count_min: 2
    signal: "MD매수"
`
	path := filepath.Join(t.TempDir(), "test_rules.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules() error = %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("룰 개수 = %d, want 2", len(rules))
	}

	r := rules[0]
	if r.Name != "TestStrategy" || r.DefCount != 1 || r.Signal != "테스트매수" {
		t.Errorf("첫 룰 파싱 오류: %+v", r)
	}
	if len(r.When) != 1 || len(r.WhenNot) != 1 || len(r.AnyOf) != 1 {
		t.Errorf("when/when_not/any_of 파싱 오류: %+v", r)
	}
	if rules[1].DefCountMin != 2 {
		t.Errorf("def_count_min 파싱 오류: %+v", rules[1])
	}
}

func TestLoadRules_FileNotFound(t *testing.T) {
	if _, err := LoadRules("does_not_exist.yaml"); err == nil {
		t.Error("없는 파일이면 에러를 반환해야 함")
	}
}

// ─────────────────────────────────────────────
// 실전 룰 파일 검증 — strategy1.yaml의 모든 조건명이 레지스트리에 등록됐는지
// (YAML 오타로 전략이 조용히 무력화되는 사고 방지)
// ─────────────────────────────────────────────

func TestStrategy1YAML_AllConditionsRegistered(t *testing.T) {
	rules, err := LoadRules("../rules/strategy1.yaml")
	if err != nil {
		t.Fatalf("rules/strategy1.yaml 로드 실패: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("전략이 하나도 없음")
	}

	for _, rule := range rules {
		if rule.Signal == "" {
			t.Errorf("전략 %q: signal이 비어 있음", rule.Name)
		}
		for _, group := range [][]string{rule.When, rule.WhenNot, rule.AnyOf} {
			for _, condName := range group {
				if _, ok := ConditionRegistryGet(condName); !ok {
					t.Errorf("전략 %q: 미등록 조건 %q", rule.Name, condName)
				}
			}
		}
	}
}

// ─────────────────────────────────────────────
// analyzer.go 내부 헬퍼
// ─────────────────────────────────────────────

func TestFindLastDefBoxIndex(t *testing.T) {
	boxList := []*box.Box{
		{KindOfBox: box.KindBox},
		{KindOfBox: box.KindDefBox},
		{KindOfBox: box.KindMainBox},
		{KindOfBox: box.KindDefBox},
		{KindOfBox: box.KindBox},
	}
	if got := findLastDefBoxIndex(boxList); got != 3 {
		t.Errorf("findLastDefBoxIndex() = %d, want 3", got)
	}
	if got := findLastDefBoxIndex([]*box.Box{{KindOfBox: box.KindBox}}); got != -1 {
		t.Errorf("DefBox 없으면 -1이어야 함, got %d", got)
	}
}

// ─────────────────────────────────────────────
// strategy2.yaml — 지표 기반 전략 세트 검증
// ─────────────────────────────────────────────

func TestStrategy2YAML_AllConditionsRegistered(t *testing.T) {
	rules, err := LoadRules("../rules/strategy2.yaml")
	if err != nil {
		t.Fatalf("rules/strategy2.yaml 로드 실패: %v", err)
	}
	if len(rules) != 6 {
		t.Fatalf("전략 개수 = %d, want 6 (I01~I06)", len(rules))
	}

	for _, rule := range rules {
		if rule.Signal == "" {
			t.Errorf("전략 %q: signal이 비어 있음", rule.Name)
		}
		for _, group := range [][]string{rule.When, rule.WhenNot, rule.AnyOf} {
			for _, condName := range group {
				if _, ok := ConditionRegistryGet(condName); !ok {
					t.Errorf("전략 %q: 미등록 조건 %q", rule.Name, condName)
				}
			}
		}
	}
}

// TestStrategy2YAML_TrendConfluenceFires 는 추세 확증 상황(정배열·전 MA 상승·
// 중심선 위·건전 RSI)에서 I01이 첫 매칭으로 발화하는지 실제 레지스트리로 검증
func TestStrategy2YAML_TrendConfluenceFires(t *testing.T) {
	rules, err := LoadRules("../rules/strategy2.yaml")
	if err != nil {
		t.Fatalf("rules/strategy2.yaml 로드 실패: %v", err)
	}

	candles := make([]*box.Candle, 20)
	for i := range candles {
		candles[i] = &box.Candle{
			Open: 109, Close: 110, High: 111, Low: 108,
			Ma5: 105, Ma20: 102, Ma60: 100,
			Gradient: 1, Gradient20: 0.5, Gradient60: 0.2,
			RSI:            60,
			BollingerLower: 90, BollingerUpper: 120,
			BBPercent: (110.0 - 90.0) / 30.0, // ≈ 0.67
		}
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 19

	signal, name := EvaluateRules(rules, ctx, DefaultSettings())
	if name != "I01_TrendConfluenceBuy" || signal != "추세확증매수" {
		t.Errorf("got (%q, %q), want (추세확증매수, I01_TrendConfluenceBuy)", signal, name)
	}

	// RSI 과열(75)이면 I01의 IsRSIInBullZone 탈락 → I06(밴드돌파)도 상단 미돌파라 무신호
	for _, c := range candles {
		c.RSI = 75
	}
	ctx2 := box.NewTradingContext(candles, nil)
	ctx2.Position = 19
	if signal, name := EvaluateRules(rules, ctx2, DefaultSettings()); name != "" {
		t.Errorf("RSI 과열 시 무신호여야 함: got (%q, %q)", signal, name)
	}
}

// ─────────────────────────────────────────────
// TODO: stg.Analyze 통합 테스트
// 실제 종목 캔들 스냅샷(JSON)을 testdata/에 저장해두고
// C# Stock1의 분석 결과(Box 목록·신호)와 일치하는지 골든 테스트로 검증할 것.
// ─────────────────────────────────────────────

func TestAnalyze_TODO(t *testing.T) {
	t.Skip("TODO: testdata/<종목코드>.json 캔들 스냅샷 기반 골든 테스트 — " +
		"C# Stock1과 Box 개수/위치/매수신호 동일성 검증")
}
