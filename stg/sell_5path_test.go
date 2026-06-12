package stg

import (
	"testing"

	"RESTGo/box"
)

// TestSellDecision_CriticalPath 는 Critical 신호 발생 시 100% 청산 결정을 검증한다.
func TestSellDecision_CriticalPath(t *testing.T) {
	signals := []box.SellSignal{
		{ConditionName: "CriticalFailure", Path: box.PathCritical, SignalWeight: 1.0, IsTriggered: true, ThresholdMet: true},
		{ConditionName: "MainBoxBreakdown", Path: box.PathIndividual, Priority: 4, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if !d.ShouldSell || d.SellWeight != 1.0 || d.PrimaryReason != "CriticalFailure" || d.DecisionPath != "Phase2-Critical" {
		t.Errorf("Critical Path 결정 오류: %+v", d)
	}
}

// TestSellDecision_CompositePath 는 composite_eligible 신호 합산 ≥ threshold일 때 동작 검증.
func TestSellDecision_CompositePath(t *testing.T) {
	// strength = 0.5 + 0.5 = 1.0 >= MediumRecovery threshold 0.6 → strong weight 1.0
	signals := []box.SellSignal{
		{ConditionName: "MainBoxBreakdown", Path: box.PathIndividual, Priority: 4, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true, CompositeEligible: true, CompositeWeight: 0.5},
		{ConditionName: "AdaptiveStopLoss", Path: box.PathIndividual, Priority: 9, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true, CompositeEligible: true, CompositeWeight: 0.5},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if !d.ShouldSell {
		t.Errorf("Composite Path가 트리거되어야 함")
	}
	if d.DecisionPath != "Phase2-Composite" {
		t.Errorf("DecisionPath: 기대 Phase2-Composite, 실제 %s", d.DecisionPath)
	}
	if d.SellWeight != 1.0 {
		t.Errorf("Strength 1.0 → weight 1.0 기대, 실제 %f", d.SellWeight)
	}
}

// TestSellDecision_IndividualPathPriority 는 여러 Individual 신호가 트리거됐을 때
// 최소 Priority 신호가 선택되는지 검증한다.
func TestSellDecision_IndividualPathPriority(t *testing.T) {
	signals := []box.SellSignal{
		{ConditionName: "MA5MA20DeadCross", Path: box.PathIndividual, Priority: 12, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true},
		{ConditionName: "GapUpProfit", Path: box.PathIndividual, Priority: 3, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true},
		{ConditionName: "MainBoxBreakdown", Path: box.PathIndividual, Priority: 4, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if !d.ShouldSell {
		t.Fatal("Individual Path 트리거 기대")
	}
	if d.PrimaryReason != "GapUpProfit" {
		t.Errorf("최저 Priority(3) 신호 GapUpProfit 선택 기대, 실제 %s", d.PrimaryReason)
	}
}

// TestSellDecision_ExpiryPath 는 Period Expiry 발생 시 RequiresHoldingExtensionUpdate 표시 동작.
func TestSellDecision_ExpiryPath(t *testing.T) {
	signals := []box.SellSignal{
		{ConditionName: "PeriodExpiry", Path: box.PathExpiry, SignalWeight: 1.0, IsTriggered: true, ThresholdMet: true},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if d.ShouldSell {
		t.Errorf("Expiry Path는 SFunction에서 연장 평가 후 결정 — 여기서는 ShouldSell=false")
	}
	if !d.RequiresHoldingExtensionUpdate {
		t.Errorf("RequiresHoldingExtensionUpdate=true 기대")
	}
}

// TestSellDecision_ExtensionPath 는 홀딩 연장 중 매도 신호 → 100% 청산 동작 검증.
func TestSellDecision_ExtensionPath(t *testing.T) {
	signals := []box.SellSignal{
		{ConditionName: "MA5BreakdownDuringExtension", Path: box.PathExtension, SignalWeight: 1.0, IsTriggered: true, ThresholdMet: true},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	pos.IsWaitingForSellSignalAfterExpiry = true

	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if !d.ShouldSell || d.SellWeight != 1.0 || d.DecisionPath != "Phase1-Extension" {
		t.Errorf("Extension Path 결정 오류: %+v", d)
	}
}

// TestSellDecision_NoSignals 는 어떤 신호도 트리거되지 않을 때 NoSell 반환 검증.
func TestSellDecision_NoSignals(t *testing.T) {
	signals := []box.SellSignal{
		{ConditionName: "Foo", Path: box.PathIndividual, Priority: 1, SignalWeight: 0.5, IsTriggered: false, ThresholdMet: false},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if d.ShouldSell {
		t.Errorf("아무 신호도 트리거되지 않으면 NoSell 기대, 실제 %+v", d)
	}
}

// TestSellDecision_CompositeBelowThreshold 는 strength < threshold일 때 Individual로 fallback 검증.
func TestSellDecision_CompositeBelowThreshold(t *testing.T) {
	signals := []box.SellSignal{
		{ConditionName: "MainBoxBreakdown", Path: box.PathIndividual, Priority: 4, SignalWeight: 0.5, IsTriggered: true, ThresholdMet: true, CompositeEligible: true, CompositeWeight: 0.3},
	}
	s := DefaultSellSettings()
	pos := box.NewTradePosition("T", "S", 0, 100, 100, "")
	d := makeSellDecision(signals, box.RecoveryMedium, nil, pos, s)
	if !d.ShouldSell {
		t.Errorf("Composite 임계 미달이지만 Individual Path로 트리거되어야 함")
	}
	if d.DecisionPath != "Phase1-Individual" {
		t.Errorf("DecisionPath: 기대 Phase1-Individual, 실제 %s", d.DecisionPath)
	}
}
