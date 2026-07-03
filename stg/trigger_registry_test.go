package stg

import (
	"testing"

	"RESTGo/box"
)

// 테스트 전용 트리거 등록 (t_ 접두사)
func init() {
	// ctx.PenPosition을 발화 스위치로 쓰는 제어 가능한 edge 트리거
	RegisterTrigger("t_trig_on", func(ctx *box.TradingContext, s Settings) bool { return true })
	RegisterTrigger("t_trig_off", func(ctx *box.TradingContext, s Settings) bool { return false })
}

func triggerTestCtx() *box.TradingContext {
	ctx := box.NewTradingContext([]*box.Candle{
		{Date: "20260101", Close: 100, CloseOrigin: 100},
		{Date: "20260102", Close: 101, CloseOrigin: 101},
	}, nil)
	ctx.Position = 1
	ctx.DefCount = 1
	return ctx
}

func TestEvaluateTriggerSignals_FiresOnTrigger(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy"},
	}
	got := evaluateTriggerSignals(ctx, DefaultSettings(), rules)
	if len(got) != 1 {
		t.Fatalf("신호 수 = %d, want 1", len(got))
	}
	if got[0].Reason != "T1" || got[0].Helper != "trigger:t_trig_on:buy" {
		t.Errorf("신호 = %+v", got[0])
	}
}

func TestEvaluateTriggerSignals_TriggerNotFired(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_off", When: []string{"t_true"}, Signal: "buy"},
	}
	if got := evaluateTriggerSignals(ctx, DefaultSettings(), rules); len(got) != 0 {
		t.Errorf("트리거 미발화인데 신호 %d개", len(got))
	}
}

func TestEvaluateTriggerSignals_ConditionBlocks(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_false"}, Signal: "buy"},
	}
	if got := evaluateTriggerSignals(ctx, DefaultSettings(), rules); len(got) != 0 {
		t.Errorf("when 실패인데 신호 %d개", len(got))
	}
}

func TestEvaluateTriggerSignals_OncePerDefbox(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy"}, // once_per 기본 = defbox
	}
	s := DefaultSettings()

	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 1 {
		t.Fatalf("1차 발화 실패: %d", len(got))
	}
	// 같은 DefBox 구간 → 재발화 금지
	ctx.Position = 1
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 0 {
		t.Errorf("defbox 내 재발화됨: %d", len(got))
	}
	// DefBox 변경 시뮬레이션 → 리셋 후 재발화 가능
	ctx.ResetBuySignalPositions()
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 1 {
		t.Errorf("리셋 후 재발화 실패: %d", len(got))
	}
}

func TestEvaluateTriggerSignals_OncePerNone(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy", OncePer: "none"},
	}
	s := DefaultSettings()
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 1 {
		t.Fatalf("1차 발화 실패")
	}
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 1 {
		t.Errorf("once_per: none 인데 재발화 안 됨")
	}
}

func TestEvaluateTriggerSignals_OncePerCooldown(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy", OncePer: "cooldown"},
	}
	s := DefaultSettings()
	s.PerCandleCooldownBars = 4

	ctx.Position = 1
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 1 {
		t.Fatalf("1차 발화 실패")
	}
	ctx.Position = 3 // 쿨다운 내
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 0 {
		t.Errorf("쿨다운 내 재발화됨")
	}
	ctx.Position = 5 // 쿨다운 경과
	if got := evaluateTriggerSignals(ctx, s, rules); len(got) != 1 {
		t.Errorf("쿨다운 경과 후 재발화 실패")
	}
}

func TestEvaluateTriggerSignals_DefCountFilter(t *testing.T) {
	ctx := triggerTestCtx()
	ctx.DefCount = 1
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", DefCountMin: 2, When: []string{"t_true"}, Signal: "buy"},
	}
	if got := evaluateTriggerSignals(ctx, DefaultSettings(), rules); len(got) != 0 {
		t.Errorf("def_count_min 미달인데 발화")
	}
}

func TestEvaluateTriggerSignals_UnknownTrigger(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_no_such_trigger", When: []string{"t_true"}, Signal: "buy"},
	}
	if got := evaluateTriggerSignals(ctx, DefaultSettings(), rules); len(got) != 0 {
		t.Errorf("미등록 트리거인데 발화")
	}
}

// 트리거 룰은 on_breakout(EvaluateRules)·per_candle 경로에서 제외되어야 한다
func TestTriggerRulesExcludedFromOtherPaths(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy"},
	}
	if sig, _ := EvaluateRules(rules, ctx, DefaultSettings()); sig != "" {
		t.Errorf("트리거 룰이 on_breakout 경로에서 발화: %s", sig)
	}
	if got := evaluatePerCandleSignals(ctx, DefaultSettings(), rules); len(got) != 0 {
		t.Errorf("트리거 룰이 per_candle 경로에서 발화")
	}
}

func TestDefBoxBreakoutTriggerRegistered(t *testing.T) {
	for _, name := range []string{"DefBoxBreakout", "PriceBreakout"} {
		if _, ok := TriggerRegistryGet(name); !ok {
			t.Errorf("기본 트리거 미등록: %s", name)
		}
	}
}

// 같은 트리거 그룹 내 첫 매칭 승리 — YAML 순서가 우선순위
func TestEvaluateTriggerSignals_FirstMatchWinsPerTrigger(t *testing.T) {
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "T1", Trigger: "t_trig_on", When: []string{"t_false"}, Signal: "buy1"}, // 미매칭
		{Name: "T2", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy2"},  // 첫 매칭
		{Name: "T3", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "buy3"},  // 같은 트리거 → 스킵
	}
	got := evaluateTriggerSignals(ctx, DefaultSettings(), rules)
	if len(got) != 1 || got[0].Reason != "T2" {
		t.Fatalf("첫 매칭 승리 실패: %+v", got)
	}
	// T3는 매칭 기록도 없어야 함 (다음 캔들에서 발화 가능)
	if _, fired := ctx.LastBuySignalPosition["T3"]; fired {
		t.Errorf("스킵된 룰에 발화 기록이 남음")
	}
}

// 서로 다른 트리거는 같은 캔들에서 독립적으로 발화 가능
func TestEvaluateTriggerSignals_DifferentTriggersIndependent(t *testing.T) {
	RegisterTrigger("t_trig_on2", func(ctx *box.TradingContext, s Settings) bool { return true })
	ctx := triggerTestCtx()
	rules := []RuleConfig{
		{Name: "TA", Trigger: "t_trig_on", When: []string{"t_true"}, Signal: "a"},
		{Name: "TB", Trigger: "t_trig_on2", When: []string{"t_true"}, Signal: "b"},
	}
	got := evaluateTriggerSignals(ctx, DefaultSettings(), rules)
	if len(got) != 2 {
		t.Fatalf("독립 트리거 발화 수 = %d, want 2", len(got))
	}
}
