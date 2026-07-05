package stg

import (
	"testing"

	"RESTGo/box"
)

// ScanTrigger: 조건 필터·쿨다운·미등록 감지
func TestScanTriggerMechanics(t *testing.T) {
	fireAt := map[int]bool{10: true, 12: true, 30: true}
	condAt := map[int]bool{10: true, 30: true}
	RegisterTrigger("_test_scan_trig", func(ctx *box.TradingContext, s Settings) bool {
		return fireAt[ctx.Position]
	})
	RegisterCondition("_test_scan_cond", func(ctx *box.TradingContext, s Settings) bool {
		return condAt[ctx.Position]
	})
	defer delete(triggerRegistry, "_test_scan_trig")
	defer delete(conditionRegistry, "_test_scan_cond")

	candles := make([]*box.Candle, 60)
	for i := range candles {
		candles[i] = &box.Candle{}
	}
	s := DefaultSettings()

	// 조건 없음: 3회 전부
	fires, unknown := ScanTrigger(candles, TriggerScanConfig{Trigger: "_test_scan_trig"}, s)
	if unknown != "" || len(fires) != 3 {
		t.Fatalf("무조건 발화 %v (unknown=%q), want 3회", fires, unknown)
	}
	// when 조건: 10, 30만
	fires, _ = ScanTrigger(candles, TriggerScanConfig{Trigger: "_test_scan_trig", When: []string{"_test_scan_cond"}}, s)
	if len(fires) != 2 || fires[0] != 10 || fires[1] != 30 {
		t.Fatalf("조건 필터 발화 %v, want [10 30]", fires)
	}
	// when_not: 12만
	fires, _ = ScanTrigger(candles, TriggerScanConfig{Trigger: "_test_scan_trig", WhenNot: []string{"_test_scan_cond"}}, s)
	if len(fires) != 1 || fires[0] != 12 {
		t.Fatalf("when_not 발화 %v, want [12]", fires)
	}
	// 쿨다운 5: 10 발화 후 12 억제
	fires, _ = ScanTrigger(candles, TriggerScanConfig{Trigger: "_test_scan_trig", CooldownBars: 5}, s)
	if len(fires) != 2 || fires[0] != 10 || fires[1] != 30 {
		t.Fatalf("쿨다운 발화 %v, want [10 30]", fires)
	}
	// 미등록 감지
	if _, unknown := ScanTrigger(candles, TriggerScanConfig{Trigger: "NoSuch"}, s); unknown == "" {
		t.Error("미등록 트리거인데 unknown 빈 문자열")
	}
	if _, unknown := ScanTrigger(candles, TriggerScanConfig{Trigger: "_test_scan_trig", When: []string{"NoSuchCond"}}, s); unknown == "" {
		t.Error("미등록 조건인데 unknown 빈 문자열")
	}
}
