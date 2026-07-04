package cond

import (
	"testing"

	"RESTGo/box"
)

// mkCollapseCandles 는 (색, 저가, 고가, 종가, MA20)를 지정한 캔들 배열 생성.
// color: +1 양봉(종가>시가), -1 음봉, 0 도지.
func mkCollapseCandles(spec []struct {
	color    int
	low, high, close float64
	ma20     float64
}) []*box.Candle {
	out := make([]*box.Candle, len(spec))
	for i, s := range spec {
		c := &box.Candle{Low: s.low, High: s.high, Close: s.close, Ma20: s.ma20}
		switch s.color {
		case 1:
			c.Open = s.close - 1 // 양봉
		case -1:
			c.Open = s.close + 1 // 음봉
		default:
			c.Open = s.close // 도지
		}
		out[i] = c
	}
	return out
}

type cs = struct {
	color    int
	low, high, close float64
	ma20     float64
}

// 기본 음양음: 음(105→) 양(반등) 음(붕괴, 양봉 저가 아래 + MA20 걸침) → 발화
func TestMTopCollapse_BasicFire(t *testing.T) {
	candles := mkCollapseCandles([]cs{
		{-1, 104, 108, 105, 100}, // 음
		{1, 103, 107, 106, 100},  // 양 (저가 103)
		{-1, 98, 105, 99, 100},   // 음: 종가 99 < 양저가 103, 범위 98~108이 MA20(100) 걸침
	})
	start, ok := FindMTopCollapseRuns(candles, 2, 0)
	if !ok || start != 0 {
		t.Errorf("기본 음양음 발화 실패: start=%d ok=%v", start, ok)
	}
}

// 연속 동색 런 묶기: 음음-양양-음음 → 하나의 음양음으로 인식
func TestMTopCollapse_RunGrouping(t *testing.T) {
	candles := mkCollapseCandles([]cs{
		{-1, 106, 110, 107, 100},
		{-1, 104, 108, 105, 100}, // 음 런
		{1, 103, 107, 106, 100},
		{1, 104, 108, 107, 100},  // 양 런 (최저 저가 103)
		{-1, 100, 106, 101, 100},
		{-1, 97, 102, 98, 100},   // 음 런: 종가 98 < 103, MA20 걸침
	})
	start, ok := FindMTopCollapseRuns(candles, 5, 0)
	if !ok || start != 0 {
		t.Errorf("런 묶기 실패: start=%d ok=%v", start, ok)
	}
}

// 반등분 미반납: 마지막 음봉 종가가 양봉 저가 위 → 불발
func TestMTopCollapse_NoFullGiveback(t *testing.T) {
	candles := mkCollapseCandles([]cs{
		{-1, 104, 108, 105, 100},
		{1, 103, 107, 106, 100},
		{-1, 103, 106, 104, 100}, // 종가 104 >= 양저가 103
	})
	if _, ok := FindMTopCollapseRuns(candles, 2, 0); ok {
		t.Error("반등분 미반납인데 발화")
	}
}

// MA20에서 먼 붕괴 → 불발 (걸침도 없고 ±3% 밖)
func TestMTopCollapse_FarFromMA20(t *testing.T) {
	candles := mkCollapseCandles([]cs{
		{-1, 124, 128, 125, 100},
		{1, 123, 127, 126, 100},
		{-1, 118, 125, 119, 100}, // 범위 118~128, MA20=100 안 걸침, 종가 119 vs 100 = 19%
	})
	if _, ok := FindMTopCollapseRuns(candles, 2, 0); ok {
		t.Error("MA20에서 먼데 발화")
	}
}

// 도지가 런을 끊음 → 불발
func TestMTopCollapse_DojiBreaks(t *testing.T) {
	candles := mkCollapseCandles([]cs{
		{-1, 104, 108, 105, 100},
		{0, 103, 107, 106, 100}, // 도지 (양 런이어야 할 자리)
		{-1, 98, 105, 99, 100},
	})
	if _, ok := FindMTopCollapseRuns(candles, 2, 0); ok {
		t.Error("도지가 끼었는데 발화")
	}
}

// minStart 제한: 첫 음 런이 minStart 이전으로 넘어가면 불발
func TestMTopCollapse_MinStart(t *testing.T) {
	candles := mkCollapseCandles([]cs{
		{-1, 104, 108, 105, 100}, // idx0 — minStart=1이면 못 씀
		{1, 103, 107, 106, 100},
		{-1, 98, 105, 99, 100},
	})
	if _, ok := FindMTopCollapseRuns(candles, 2, 1); ok {
		t.Error("첫 음 런이 minStart 이전인데 발화")
	}
}

// FindBBMTopBoxPattern: R(상단이탈)-S(MA20위)-R(BB내부) 3박스 성립
func TestFindBBMTopBoxPattern(t *testing.T) {
	n := 80
	candles := make([]*box.Candle, n)
	for i := range candles {
		c := &box.Candle{Open: 99, Close: 100, High: 101, Low: 98, Ma20: 100}
		c.BollingerUpper, c.BollingerLower = 110, 90
		candles[i] = c
	}
	// P1(idx 50, resist): 직전 10봉(40~49) 중 6봉 고가 >= 상단 + BBW 팽창
	for i := 40; i < 46; i++ {
		candles[i].High = 111
	}
	// BBW 팽창: p1 시점 밴드폭 > 5봉 전, 그리고 최근 20봉 최솟값×1.2 초과
	for i := 25; i < 46; i++ {
		candles[i].BollingerUpper, candles[i].BollingerLower = 104, 96 // 좁은 밴드
	}
	for i := 46; i <= 50; i++ {
		candles[i].BollingerUpper, candles[i].BollingerLower = 112, 88 // 팽창
	}
	// 중간 support(idx 58): 종가가 MA20 위
	candles[58].Close, candles[58].Ma20 = 105, 100
	// P2(idx 66, resist): BB 내부 (고가 101 < 상단 110, 직전 10봉 이탈 없음)
	boxes := []*box.Box{
		{BoxPosition: 50, BoxType: box.BoxTypeResistance},
		{BoxPosition: 58, BoxType: box.BoxTypeSupport},
		{BoxPosition: 66, BoxType: box.BoxTypeResistance},
	}
	ctx := box.NewTradingContext(candles, boxes)
	ctx.Position = 70

	p1, p2, ok := FindBBMTopBoxPattern(ctx, 60)
	if !ok || p1 != 50 || p2 != 66 {
		t.Errorf("M 3박스 탐지 실패: p1=%d p2=%d ok=%v (want 50, 66, true)", p1, p2, ok)
	}

	// P1 상단 이탈 제거 → 불발
	for i := 40; i < 46; i++ {
		candles[i].High = 101
	}
	if _, _, ok := FindBBMTopBoxPattern(ctx, 60); ok {
		t.Error("P1 상단 이탈 없는데 성립")
	}
}
