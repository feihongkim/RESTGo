package stg

import (
	"testing"

	"RESTGo/box"
)

// TestLoadSellStrategy 는 rules/sell_strategy1.yaml이 정상 로드되고 21개 룰을 가지는지 확인.
func TestLoadSellStrategy(t *testing.T) {
	settings, err := LoadSellStrategy("../rules/sell_strategy1.yaml")
	if err != nil {
		t.Fatalf("LoadSellStrategy 실패: %v", err)
	}
	if len(activeSellRules) != 21 {
		t.Fatalf("매도 룰 개수: 기대 21, 실제 %d", len(activeSellRules))
	}
	if settings.MaxHoldingPeriod != 20 {
		t.Errorf("MaxHoldingPeriod: 기대 20, 실제 %d", settings.MaxHoldingPeriod)
	}

	// 각 룰에 등록된 모든 조건명이 sellConditionRegistry에 등록되어 있는지 확인
	// (오타로 인한 조용한 무력화 방지)
	for _, rule := range activeSellRules {
		for _, name := range rule.When {
			if _, ok := sellConditionRegistry[name]; !ok {
				t.Errorf("룰 %q의 when 조건 %q이 레지스트리에 미등록", rule.Name, name)
			}
		}
		for _, name := range rule.AnyOf {
			if _, ok := sellConditionRegistry[name]; !ok {
				t.Errorf("룰 %q의 any_of 조건 %q이 레지스트리에 미등록", rule.Name, name)
			}
		}
		for _, name := range rule.WhenNot {
			if _, ok := sellConditionRegistry[name]; !ok {
				t.Errorf("룰 %q의 when_not 조건 %q이 레지스트리에 미등록", rule.Name, name)
			}
		}
	}
}

// TestExecutePartialSell 은 weight 비율 기반 부분 매도 동작과 잔량 추적을 확인.
func TestExecutePartialSell(t *testing.T) {
	candles := []*box.Candle{
		{Close: 100, CloseOrigin: 100, Date: "2026-01-01"},
		{Close: 110, CloseOrigin: 110, Date: "2026-01-02"},
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 1

	pos := box.NewTradePosition("T1", "TestStg", 0, 100, 100, "2026-01-01")
	s := DefaultSellSettings()

	// 50% 매도 → 잔량 0.5
	if !ExecutePartialSell(ctx, pos, "TestReason", 0.5, s) {
		t.Fatal("첫 부분 매도 실패")
	}
	if pos.RemainingQuantity != 0.5 {
		t.Errorf("잔량: 기대 0.5, 실제 %f", pos.RemainingQuantity)
	}
	if pos.IsActive == false {
		t.Error("아직 활성이어야 함")
	}

	// 다시 50% → 잔량 0.25 → SmallRemainingThreshold(0.125) 초과이므로 정상 진행
	if !ExecutePartialSell(ctx, pos, "TestReason2", 0.5, s) {
		t.Fatal("두번째 부분 매도 실패")
	}
	if pos.RemainingQuantity != 0.25 {
		t.Errorf("잔량: 기대 0.25, 실제 %f", pos.RemainingQuantity)
	}

	// 50% → 잔량 0.125 (가드 미발동: 진입 시 0.25 > 임계 0.125)
	if !ExecutePartialSell(ctx, pos, "TestReason3", 0.5, s) {
		t.Fatal("세번째 부분 매도 실패")
	}
	if pos.RemainingQuantity != 0.125 {
		t.Errorf("잔량: 기대 0.125, 실제 %f", pos.RemainingQuantity)
	}
	if !pos.IsActive {
		t.Errorf("잔량 0.125는 IsFullyLiquidated 미달, 아직 활성이어야 함")
	}

	// 다음 매도 진입: 잔량 0.125 = 임계값 → SmallRemaining 가드 발동, weight=1.0 강제 → 전량 청산
	if !ExecutePartialSell(ctx, pos, "TestReason4", 0.5, s) {
		t.Fatal("네번째 부분 매도 실패")
	}
	if pos.IsActive {
		t.Errorf("SmallRemaining 가드로 완전 청산되어야 함, 실제 잔량=%f, IsActive=%v", pos.RemainingQuantity, pos.IsActive)
	}
}

// TestTrackAndCheck 은 듀얼 임계값(count OR ratio) 동작을 확인.
func TestTrackAndCheck(t *testing.T) {
	candles := make([]*box.Candle, 20)
	for i := range candles {
		candles[i] = &box.Candle{}
	}
	ctx := box.NewTradingContext(candles, nil)
	pos := box.NewTradePosition("T1", "Stg", 0, 100, 100, "2026-01-01")
	s := DefaultSellSettings()
	tr := SellTracking{CountMin: 3, RatioMin: 5.0} // ratio 임계를 사실상 비활성화 (max=1.0)

	// 1회: count=1 < 3 → false
	ctx.Position = 1
	if TrackAndCheck(ctx, pos, "X", true, tr, s) {
		t.Fatal("1회 발생 시 false 기대")
	}
	// 2회: count=2 < 3 → false
	ctx.Position = 2
	if TrackAndCheck(ctx, pos, "X", true, tr, s) {
		t.Fatal("2회 발생 시 false 기대")
	}
	// 3회: count=3 >= 3 → true
	ctx.Position = 3
	if !TrackAndCheck(ctx, pos, "X", true, tr, s) {
		t.Fatal("3회 발생 시 true 기대")
	}

	// immediate 모드: triggered면 즉시 true
	trImmediate := SellTracking{Immediate: true}
	pos2 := box.NewTradePosition("T2", "Stg", 0, 100, 100, "2026-01-01")
	ctx.Position = 1
	if !TrackAndCheck(ctx, pos2, "Y", true, trImmediate, s) {
		t.Fatal("immediate 모드에서 true 기대")
	}
	if TrackAndCheck(ctx, pos2, "Y", false, trImmediate, s) {
		t.Fatal("immediate 모드에서도 triggered=false면 false 기대")
	}
}
