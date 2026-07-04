package cond

import (
	"testing"

	"RESTGo/box"
)

// hnsCtx 는 R-S-R-S-R 5박스와 균일 캔들을 가진 컨텍스트 생성.
func hnsCtx(lsP, t1P, hP, t2P, rsP float64) *box.TradingContext {
	n := 120
	candles := make([]*box.Candle, n)
	for i := range candles {
		candles[i] = &box.Candle{Open: 99, Close: 100, High: 101, Low: 98,
			Ma20: 105, Ma60: 100, Volume: 1000}
	}
	boxes := []*box.Box{
		{BoxPosition: 30, BoxType: box.BoxTypeResistance, Price: lsP},
		{BoxPosition: 45, BoxType: box.BoxTypeSupport, Price: t1P},
		{BoxPosition: 60, BoxType: box.BoxTypeResistance, Price: hP},
		{BoxPosition: 75, BoxType: box.BoxTypeSupport, Price: t2P},
		{BoxPosition: 90, BoxType: box.BoxTypeResistance, Price: rsP},
	}
	ctx := box.NewTradingContext(candles, boxes)
	ctx.Position = 95
	return ctx
}

func TestHNS_Basic(t *testing.T) {
	// LS 110, T1 100, H 120, T2 101, RS 111 — 정상 H&S
	ctx := hnsCtx(110, 100, 120, 101, 111)
	p, ok := FindHNSPattern(ctx, HNSLookbackBars)
	if !ok {
		t.Fatal("정상 H&S인데 불성립")
	}
	if p.HPos != 60 || p.LSPos != 30 || p.RSPos != 90 {
		t.Errorf("박스 위치 오류: %+v", p)
	}
	// 넥라인: T1(45,100) → T2(75,101), 기울기 1/30 — pos 90에서 100 + 45/30 = 101.5
	if v := p.NecklineValue(90); v < 101.49 || v > 101.51 {
		t.Errorf("넥라인 외삽 오류: %v (want 101.5)", v)
	}
	if p.HeadPromPct <= 0 || p.PatternWidth != 60 {
		t.Errorf("속성 오류: %+v", p)
	}
}

func TestHNS_HeadNotHighest(t *testing.T) {
	// 머리가 왼어깨보다 낮음 → 불성립
	if _, ok := FindHNSPattern(hnsCtx(125, 100, 120, 101, 111), HNSLookbackBars); ok {
		t.Error("머리가 최고점이 아닌데 성립")
	}
}

func TestHNS_ShoulderAsymmetry(t *testing.T) {
	// 어깨 차이 |110-95|=15 > 120×10% → 불성립
	if _, ok := FindHNSPattern(hnsCtx(110, 90, 120, 91, 95), HNSLookbackBars); ok {
		t.Error("어깨 비대칭 10% 초과인데 성립")
	}
}

func TestHNS_TroughAboveShoulder(t *testing.T) {
	// 골(T1=112)이 어깨(110) 위 → 구조 불성립
	if _, ok := FindHNSPattern(hnsCtx(110, 112, 120, 101, 111), HNSLookbackBars); ok {
		t.Error("골이 어깨 위인데 성립")
	}
}

func TestHNS_NoUptrend(t *testing.T) {
	// 머리 캔들 MA20 < MA60 → 선행 상승 추세 없음 → 불성립
	ctx := hnsCtx(110, 100, 120, 101, 111)
	ctx.CandleList[60].Ma20, ctx.CandleList[60].Ma60 = 95, 100
	if _, ok := FindHNSPattern(ctx, HNSLookbackBars); ok {
		t.Error("MA20<MA60인데 성립")
	}
}

func TestHNS_VolumeRatioAttr(t *testing.T) {
	// RS 부근 거래량 절반 → VolumeRatio ≈ 0.5 (게이트 아님, 속성 기록)
	ctx := hnsCtx(110, 100, 120, 101, 111)
	for i := 88; i <= 92; i++ {
		ctx.CandleList[i].Volume = 500
	}
	p, ok := FindHNSPattern(ctx, HNSLookbackBars)
	if !ok {
		t.Fatal("성립해야 함")
	}
	if p.VolumeRatio < 0.49 || p.VolumeRatio > 0.51 {
		t.Errorf("VolumeRatio = %v, want ~0.5", p.VolumeRatio)
	}
}
