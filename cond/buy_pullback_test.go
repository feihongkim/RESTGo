package cond

import (
	"testing"

	"RESTGo/box"
)

// pbCtx 는 눌림 구조 테스트용 컨텍스트: 상승 MA20 + R(40)·S(50) 박스.
// customize로 캔들 필드 조정.
func pbCtx(customize func(candles []*box.Candle)) *box.TradingContext {
	n := 60
	candles := make([]*box.Candle, n)
	for i := range candles {
		// MA20 단조 상승 (+++ 충족), 가격은 이평 아래 눌림
		ma := 100.0 + float64(i)*0.5
		candles[i] = &box.Candle{
			Open: ma - 5, Close: ma - 4, High: ma - 2, Low: ma - 7, Ma20: ma,
		}
	}
	if customize != nil {
		customize(candles)
	}
	boxes := []*box.Box{
		{BoxPosition: 40, BoxType: box.BoxTypeResistance, Price: 122},
		{BoxPosition: 50, BoxType: box.BoxTypeSupport, Price: 112},
	}
	ctx := box.NewTradingContext(candles, boxes)
	ctx.Position = 52
	return ctx
}

// R box(40)에서 고가가 MA20을 터치하도록 설정하는 헬퍼
func withRTouch(candles []*box.Candle) {
	candles[40].High = candles[40].Ma20 + 1 // 고가로 넘음, 종가는 아래 유지
}

func TestPullback_Basic(t *testing.T) {
	ctx := pbCtx(withRTouch)
	p, ok := FindMA20PullbackPattern(ctx, 3)
	if !ok {
		t.Fatal("정상 눌림 구조인데 불성립")
	}
	if p.RPos != 40 || p.SPos != 50 || p.TouchCount != 1 || p.RToSBars != 10 {
		t.Errorf("패턴 필드 오류: %+v", p)
	}
	if p.DepthPct <= 0 {
		t.Errorf("눌림 깊이가 양수여야 함: %+v", p)
	}
}

func TestPullback_CloseAboveMA_Rejects(t *testing.T) {
	// R±3봉 중 한 봉이 종가로 MA20을 넘음 → 불성립
	ctx := pbCtx(func(c []*box.Candle) {
		withRTouch(c)
		c[42].Close = c[42].Ma20 + 1
	})
	if _, ok := FindMA20PullbackPattern(ctx, 3); ok {
		t.Error("종가로 이평을 넘었는데 성립")
	}
}

func TestPullback_NoHighTouch_Rejects(t *testing.T) {
	// 고가로도 못 건드림 (거부 증거 없음) → 불성립
	if _, ok := FindMA20PullbackPattern(pbCtx(nil), 3); ok {
		t.Error("고가 터치가 없는데 성립")
	}
}

func TestPullback_MA20NotRising_Rejects(t *testing.T) {
	// R 시점 MA20 하락 → 불성립
	ctx := pbCtx(func(c []*box.Candle) {
		withRTouch(c)
		c[39].Ma20 = c[40].Ma20 + 1 // 이전 > 지금 → + 아님
	})
	if _, ok := FindMA20PullbackPattern(ctx, 3); ok {
		t.Error("MA20 상승 3연속 미충족인데 성립")
	}
}

func TestPullback_SupportAboveMA_Rejects(t *testing.T) {
	// support box 캔들 종가가 이평 위 → 눌림 아님
	ctx := pbCtx(func(c []*box.Candle) {
		withRTouch(c)
		c[50].Close = c[50].Ma20 + 1
	})
	if _, ok := FindMA20PullbackPattern(ctx, 3); ok {
		t.Error("support가 이평 위인데 성립")
	}
}

func TestIsMA20RisingStreak(t *testing.T) {
	candles := make([]*box.Candle, 10)
	for i := range candles {
		candles[i] = &box.Candle{Ma20: 100 + float64(i)}
	}
	if !IsMA20RisingStreak(candles, 9, 3) {
		t.Error("단조 상승인데 false")
	}
	candles[8].Ma20 = 200 // 8→9 하락
	if IsMA20RisingStreak(candles, 9, 3) {
		t.Error("직전 하락인데 true")
	}
}

func TestIsMA20BullishBreakout(t *testing.T) {
	candles := make([]*box.Candle, 10)
	for i := range candles {
		ma := 100.0 + float64(i)
		candles[i] = &box.Candle{Open: ma - 3, Close: ma - 2, Ma20: ma} // 이평 아래
	}
	// 9번 캔들: 양봉 + 종가가 이평 위 (전일은 아래)
	candles[9].Open, candles[9].Close = 105, 112 // Ma20=109
	if !IsMA20BullishBreakout(candles, 9, 3) {
		t.Error("정상 돌파인데 false")
	}
	// 음봉이면 불발
	candles[9].Open, candles[9].Close = 113, 112
	if IsMA20BullishBreakout(candles, 9, 3) {
		t.Error("음봉인데 true")
	}
	// 전일 종가가 이미 이평 위면 edge 아님
	candles[9].Open, candles[9].Close = 105, 112
	candles[8].Close = candles[8].Ma20 + 1
	if IsMA20BullishBreakout(candles, 9, 3) {
		t.Error("전일 이미 이평 위인데 true")
	}
}

// streak=0 (+++ 폐지): MA20 비상승이어도 성립해야 함
func TestPullback_StreakZero_AcceptsFlatMA(t *testing.T) {
	ctx := pbCtx(func(c []*box.Candle) {
		withRTouch(c)
		c[39].Ma20 = c[40].Ma20 + 1 // R 시점 MA20 하락
	})
	if _, ok := FindMA20PullbackPattern(ctx, 0); !ok {
		t.Error("streak=0인데 MA20 조건으로 기각됨")
	}
	if _, ok := FindMA20PullbackPattern(ctx, 3); ok {
		t.Error("streak=3이면 기각돼야 함")
	}
}

// LastDefBoxAboveMA20: DefBox 유무·위치 판정
func TestLastDefBoxAboveMA20(t *testing.T) {
	n := 30
	candles := make([]*box.Candle, n)
	for i := range candles {
		candles[i] = &box.Candle{Ma20: 100}
	}
	// DefBox 없음
	ctx := box.NewTradingContext(candles, []*box.Box{
		{BoxPosition: 5, BoxType: box.BoxTypeResistance, KindOfBox: box.KindBox, Price: 120},
	})
	if _, _, exists := LastDefBoxAboveMA20(ctx, 20); exists {
		t.Error("DefBox 없는데 exists=true")
	}
	// MA20 위 DefBox
	ctx = box.NewTradingContext(candles, []*box.Box{
		{BoxPosition: 5, KindOfBox: box.KindDefBox, Price: 110},
	})
	above, dist, exists := LastDefBoxAboveMA20(ctx, 20)
	if !exists || !above || dist < 9.9 || dist > 10.1 {
		t.Errorf("MA20 위 DefBox 판정 실패: above=%v dist=%v exists=%v", above, dist, exists)
	}
	// 최근 DefBox가 MA20 아래면 above=false (더 오래된 위쪽 DefBox가 있어도 최근 것 기준)
	ctx = box.NewTradingContext(candles, []*box.Box{
		{BoxPosition: 5, KindOfBox: box.KindDefBox, Price: 110},
		{BoxPosition: 10, KindOfBox: box.KindDefBox, Price: 90},
	})
	if above, _, _ := LastDefBoxAboveMA20(ctx, 20); above {
		t.Error("최근 DefBox가 아래인데 above=true")
	}
	// pos 이후에 생긴 DefBox는 무시
	ctx = box.NewTradingContext(candles, []*box.Box{
		{BoxPosition: 25, KindOfBox: box.KindDefBox, Price: 110},
	})
	if _, _, exists := LastDefBoxAboveMA20(ctx, 20); exists {
		t.Error("pos 이후 DefBox를 참조함 (미래 정보)")
	}
}
