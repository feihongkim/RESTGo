package stg

import (
	"testing"

	"RESTGo/box"
)

// 합성 스펙으로 armed 상태머신 검증: 장전 → 유효기간 내 발화 → 장전당 1회 → 만료 → 재장전
func TestArmedTriggerMechanics(t *testing.T) {
	armAt := map[int]bool{10: true, 30: true, 60: true}
	fireAt := map[int]bool{13: true, 14: true, 55: true, 62: true}
	RegisterArmedTrigger("_test_armed", ArmedTriggerSpec{
		WindowBars: 5,
		CheckArm: func(ctx *box.TradingContext, s Settings, newBox bool) (interface{}, bool) {
			return ctx.Position, armAt[ctx.Position]
		},
		CheckFire: func(ctx *box.TradingContext, s Settings, state interface{}, armPos int) bool {
			return fireAt[ctx.Position]
		},
	})
	defer delete(armedTriggerRegistry, "_test_armed")

	candles := make([]*box.Candle, 80)
	for i := range candles {
		candles[i] = &box.Candle{}
	}
	ctx := box.NewTradingContext(candles, nil)
	s := DefaultSettings()

	var fires []int
	for i := 6; i < 80; i++ {
		ctx.Position = i
		if tickArmedTrigger("_test_armed", ctx, s) {
			fires = append(fires, i)
		}
	}
	// 기대: 장전10 → 발화13 (14는 장전당 1회로 불발) / 장전30 → 35까지 유효, 발화 없음(55는 만료 후) /
	//       장전60 → 발화62
	want := []int{13, 62}
	if len(fires) != len(want) {
		t.Fatalf("발화 위치 %v, want %v", fires, want)
	}
	for k := range want {
		if fires[k] != want[k] {
			t.Fatalf("발화 위치 %v, want %v", fires, want)
		}
	}
}

// 등록된 armed 트리거 3종이 조회되는지 + IsKnownTrigger 동작
func TestArmedTriggersRegistered(t *testing.T) {
	for _, name := range []string{"MTopCollapse", "HNSNecklineBreak", "MA20PullbackBreakout"} {
		if _, ok := ArmedTriggerRegistryGet(name); !ok {
			t.Errorf("armed 트리거 미등록: %s", name)
		}
		if !IsKnownTrigger(name) {
			t.Errorf("IsKnownTrigger(%s) = false", name)
		}
	}
	if !IsKnownTrigger("WBottomBox") {
		t.Error("일반 트리거가 IsKnownTrigger에서 누락")
	}
	if IsKnownTrigger("NoSuchTrigger") {
		t.Error("미등록 이름이 true")
	}
}
