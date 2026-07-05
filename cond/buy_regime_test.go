package cond

import (
	"testing"

	"RESTGo/box"
)

func gcCandles(gapAt func(i int) float64) []*box.Candle {
	n := 60
	out := make([]*box.Candle, n)
	for i := range out {
		ma120 := 100.0
		gap := gapAt(i) // %
		out[i] = &box.Candle{Ma120: ma120, Ma60: ma120 * (1 - gap/100)}
	}
	return out
}

func TestGCPending_Basic(t *testing.T) {
	// 간격이 10%에서 선형 축소 → pos=55에서 2% 부근, 20봉 연속 축소 → pending
	c := gcCandles(func(i int) float64 { return 10.0 - float64(i)*0.145 })
	pending, inv, gap, shrink := GoldenCrossPendingInfo(c, 55)
	if !inv || !pending {
		t.Errorf("축소 중 역배열인데 pending=false (gap=%.2f shrink=%d)", gap, shrink)
	}
	if shrink < 4 {
		t.Errorf("축소 체크포인트 부족: %d", shrink)
	}
}

func TestGCPending_GapTooWide(t *testing.T) {
	// 축소 중이지만 간격 8% → 임박 아님
	c := gcCandles(func(i int) float64 { return 12.0 - float64(i)*0.07 })
	pending, inv, gap, _ := GoldenCrossPendingInfo(c, 55)
	if !inv || gap < 3.0 {
		t.Fatalf("테스트 셋업 오류: gap=%.2f", gap)
	}
	if pending {
		t.Error("간격 8%인데 pending=true")
	}
}

func TestGCPending_NotShrinking(t *testing.T) {
	// 간격 고정 2% → 축소 아님
	c := gcCandles(func(i int) float64 { return 2.0 })
	if pending, _, _, shrink := GoldenCrossPendingInfo(c, 55); pending || shrink != 0 {
		t.Errorf("간격 고정인데 pending=%v shrink=%d", pending, shrink)
	}
}

func TestGCPending_AlreadyCrossed(t *testing.T) {
	// MA60 > MA120 (이미 교차) → pending 아님
	c := gcCandles(func(i int) float64 { return 5.0 - float64(i)*0.145 }) // pos 55에서 음수
	pending, inv, gap, _ := GoldenCrossPendingInfo(c, 55)
	if gap > 0 || inv {
		t.Fatalf("셋업 오류: gap=%.2f inv=%v", gap, inv)
	}
	if pending {
		t.Error("이미 교차했는데 pending=true")
	}
}
