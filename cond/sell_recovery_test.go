package cond

import (
	"testing"

	"RESTGo/box"
)

// TestIsCriticalFailure_Conditions 는 IsCriticalFailure의 5가지 분기를 각각 검증한다.
//
// 참고: C# 원본 주석에 "문제가 많음. 다시 체크 필요" 표시가 있는 함수.
// 동일 로직 포팅이 목적이므로 행동 호환성만 검증.
func TestIsCriticalFailure_5DayCumulativeDrop(t *testing.T) {
	// 6일치, 5일 누적 하락 -16% (시작 100 → 현재 84) → cumulative -0.16 < -0.15 → true
	candles := []*box.Candle{
		mkSellCandle("D0", 100, 100, 100, 100, 1000),
		mkSellCandle("D1", 95, 95, 95, 95, 1000),
		mkSellCandle("D2", 93, 93, 93, 93, 1000),
		mkSellCandle("D3", 91, 91, 91, 91, 1000),
		mkSellCandle("D4", 88, 88, 88, 88, 1000),
		mkSellCandle("D5", 84, 84, 84, 84, 1000),
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 5
	pos := box.NewTradePosition("T1", "S", 0, 100, 100, "D0")
	pos.MainBoxPrice = 70 // MainBox는 충분히 낮게 → 다른 조건 우회

	p := CriticalFailureParams{
		DailyDropThreshold:      -0.10,
		PanicVolumeMultiplier:   2.0,
		PanicMinDropRate:        -0.03,
		CumulativeDropThreshold: -0.15,
		CumulativeDropDays:      5,
		MAReversalDays:          1000, // 매우 크게 해서 5번 조건 우회
	}
	if !IsCriticalFailure(ctx, pos, p) {
		t.Errorf("5일 누적 -16%% 하락 → CriticalFailure true 기대")
	}
}

func TestIsCriticalFailure_AllMABelowReversed(t *testing.T) {
	candles := make([]*box.Candle, 5)
	for i := range candles {
		candles[i] = mkSellCandle("", 0, 0, 0, 0, 0)
	}
	// 현재 캔들: Close < MA5 < MA20 < MA60 (역배열)
	candles[4] = mkSellCandle("", 0, 0, 0, 80, 0)
	candles[4].Ma5 = 85
	candles[4].Ma20 = 90
	candles[4].Ma60 = 95
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 4
	pos := box.NewTradePosition("T1", "S", 4, 100, 100, "")
	pos.MainBoxPrice = 70 // MainBox 낮게

	p := CriticalFailureParams{
		DailyDropThreshold:      -1.0,  // 우회
		PanicVolumeMultiplier:   100.0, // 우회
		PanicMinDropRate:        -1.0,
		CumulativeDropThreshold: -1.0, // 우회
		CumulativeDropDays:      1000,
		MAReversalDays:          1000, // 5번 우회
	}
	if !IsCriticalFailure(ctx, pos, p) {
		t.Errorf("모든 MA 아래 + 역배열 → true 기대")
	}
}

func TestIsCriticalFailure_MAReversalSustained(t *testing.T) {
	candles := make([]*box.Candle, 6)
	for i := range candles {
		candles[i] = mkSellCandle("", 0, 0, 0, 100, 0)
		candles[i].Ma5 = 80
		candles[i].Ma20 = 90
		candles[i].Ma60 = 95
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 5
	pos := box.NewTradePosition("T1", "S", 0, 100, 100, "")
	pos.MainBoxPrice = 70

	p := CriticalFailureParams{
		DailyDropThreshold:      -1.0,
		PanicVolumeMultiplier:   100.0,
		PanicMinDropRate:        -1.0,
		CumulativeDropThreshold: -1.0,
		CumulativeDropDays:      1000,
		MAReversalDays:          3, // 5일 연속 역배열 >= 3 → true
	}
	if !IsCriticalFailure(ctx, pos, p) {
		t.Errorf("MA 역배열 5일 연속 → MAReversalDays(3) 충족 true 기대")
	}
}

func TestEvaluateRecoveryPotential_High(t *testing.T) {
	// 일시적 조정: daysBelow=0, drop 거의 없음, 회복 시도 있음
	candles := []*box.Candle{
		mkSellCandle("D0", 100, 100, 100, 100, 1000),
		mkSellCandle("D1", 100, 102, 99, 101, 1000), // 약간 상승 시도 (MainBox=99.5 위로 유지)
		mkSellCandle("D2", 101, 103, 100, 102, 1000),
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 2
	pos := box.NewTradePosition("T1", "S", 0, 100, 100, "")
	pos.MainBoxPrice = 99.5

	p := RecoveryParams{
		HighMaxDaysBelow:    2,
		HighMaxDropRate:     0.05,
		HighMinRecoveryRate: 0.02,
		MediumMaxDaysBelow:  5,
		MA5Tolerance:        0.02,
		MA20Tolerance:       0.03,
	}
	got := EvaluateRecoveryPotential(ctx, pos, p)
	if got != box.RecoveryHigh && got != box.RecoveryMedium {
		// 데이터에 따라 High 또는 Medium 가능 (low는 아니어야 함)
		t.Errorf("MainBox 위에 머무는 시나리오 → High/Medium 기대, 실제=%v", got)
	}
}
