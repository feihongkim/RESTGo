package stg

import (
	"testing"

	"RESTGo/box"
)

func followupCtx() *box.TradingContext {
	// MainBox(idx0) ← DefBox(idx1) 구조, 캔들 4개
	candles := []*box.Candle{
		{Open: 99, Close: 101, High: 102, Low: 98},
		{Open: 100, Close: 103, High: 104, Low: 99}, // 양봉, DefBox(100) 근접
		{Open: 102, Close: 101, High: 103, Low: 100},
		{Open: 101, Close: 104, High: 105, Low: 100},
	}
	boxList := []*box.Box{
		{KindOfBox: box.KindMainBox, Price: 90, BoxPosition: 0},
		{KindOfBox: box.KindDefBox, Price: 100, BoxPosition: 0, MainDefLink: []int{0}},
	}
	ctx := box.NewTradingContext(candles, boxList)
	ctx.Position = 1
	ctx.DefboxIndex = 1
	ctx.DefboxPrice = 100
	ctx.DefboxPosition = 0
	ctx.MainboxPosition = 0
	return ctx
}

// ─────────────────────────────────────────────
// setBuySignalState — C# SetBuySignal의 BuyHelper 제외 목록
// ─────────────────────────────────────────────

func TestSetBuySignalState(t *testing.T) {
	ctx := followupCtx()
	ctx.BuyHelper = "이전값"

	// 제외 목록: BuyHelper 미갱신
	for _, h := range []string{"매수대기", "multidef매수대기", "!multidef매수대기"} {
		setBuySignalState(ctx, "후보군1", "S", h)
		if ctx.BuyHelper != "이전값" {
			t.Errorf("%q는 BuyHelper를 갱신하면 안 됨 (got %q)", h, ctx.BuyHelper)
		}
		if ctx.BuyHelperReport != h {
			t.Errorf("BuyHelperReport는 항상 갱신돼야 함")
		}
	}

	// 일반 헬퍼: BuyHelper 갱신
	setBuySignalState(ctx, "즉시매수", "S01", "MD즉시매수")
	if ctx.BuyHelper != "MD즉시매수" || ctx.Bsig != "즉시매수" || ctx.StgName != "S01" {
		t.Errorf("상태 갱신 오류: %+v", ctx)
	}
	if ctx.MomentumPosition != ctx.Position {
		t.Error("MomentumPosition은 현재 위치로 갱신돼야 함")
	}
}

// ─────────────────────────────────────────────
// determineBuySignal (REST2)
// ─────────────────────────────────────────────

func TestDetermineBuySignal_MultiAlt(t *testing.T) {
	// S16: DefCount>=2 + 근접 + 양봉 + 윗꼬리 정상 → 후보군1 (신호 없음)
	ctx := followupCtx()
	ctx.DefCount = 2
	sig := determineBuySignal(ctx, DefaultSettings())

	if sig != nil {
		t.Fatalf("후보군1은 실매수 신호를 반환하면 안 됨: %+v", sig)
	}
	if ctx.Bsig != "후보군1" {
		t.Errorf("Bsig = %q, want 후보군1", ctx.Bsig)
	}
	if ctx.StgName != "16_candi_MultiDefBoxAlternative_REST2" {
		t.Errorf("StgName = %q", ctx.StgName)
	}
	if ctx.BuyHelperReport != "multidef매수대기" {
		t.Errorf("BuyHelperReport = %q", ctx.BuyHelperReport)
	}
	if ctx.BuyHelper == "multidef매수대기" {
		t.Error("multidef매수대기는 BuyHelper 제외 대상 (C# SetBuySignal)")
	}
	if ctx.PenPosition != ctx.Position {
		t.Error("PenPosition이 설정돼야 함")
	}
}

func TestDetermineBuySignal_DuplicatePrevention(t *testing.T) {
	ctx := followupCtx()
	ctx.DefCount = 2
	ctx.LastBuySignalPosition["DetermineBuySignal"] = 1
	if sig := determineBuySignal(ctx, DefaultSettings()); sig != nil || ctx.Bsig != "" {
		t.Error("중복 방지 후에는 평가 자체가 스킵돼야 함")
	}
}

// ─────────────────────────────────────────────
// processPostBreakoutSignals (S19 ShortRange) — 게이트
// ─────────────────────────────────────────────

func TestProcessPostBreakoutSignals_Gates(t *testing.T) {
	t.Run("PenPosition 0 → nil", func(t *testing.T) {
		ctx := followupCtx()
		ctx.PenPosition = 0
		if processPostBreakoutSignals(ctx) != nil {
			t.Error("PenPosition 미설정이면 nil이어야 함")
		}
	})

	t.Run("중복 방지 → nil", func(t *testing.T) {
		ctx := followupCtx()
		ctx.PenPosition = 1
		ctx.Position = 3
		ctx.LastBuySignalPosition["ShortRange"] = 2
		if processPostBreakoutSignals(ctx) != nil {
			t.Error("이미 발화했으면 nil이어야 함")
		}
	})

	t.Run("Position이 Pen과 같음 → nil", func(t *testing.T) {
		ctx := followupCtx()
		ctx.PenPosition = 1
		ctx.Position = 1
		if processPostBreakoutSignals(ctx) != nil {
			t.Error("Pen 이후가 아니면 nil이어야 함")
		}
	})
}

// ─────────────────────────────────────────────
// processAdditionalBuySignals — 중복 방지
// ─────────────────────────────────────────────

func TestProcessAdditionalBuySignals_DuplicatePrevention(t *testing.T) {
	ctx := followupCtx()
	ctx.LastBuySignalPosition["AdditionalBuySignals"] = 1
	if processAdditionalBuySignals(ctx) != nil {
		t.Error("이미 발화했으면 nil이어야 함")
	}
}

// ─────────────────────────────────────────────
// processFollowUpBuyDecisions — 사문 게이트 보존 확인
// ─────────────────────────────────────────────

func TestProcessFollowUpBuyDecisions_DeadGates(t *testing.T) {
	// BuyHelper/SellHelper가 게이트 값이 아니면 어떤 신호도 발생하지 않음
	ctx := followupCtx()
	if out := processFollowUpBuyDecisions(ctx); len(out) != 0 {
		t.Errorf("게이트 미충족 시 신호 없음이어야 함: %+v", out)
	}
}

func TestProcessFollowUpBuyDecisions_MultiDefWaiting(t *testing.T) {
	// 게이트를 인위적으로 열었을 때 S17이 정상 발화하는지 (전환 로직 자체 검증)
	candles := []*box.Candle{
		{Open: 99, Close: 101, High: 102, Low: 98},   // 0
		{Open: 100, Close: 103, High: 104, Low: 99},  // 1 (Pen)
		{Open: 105, Close: 104, High: 106, Low: 103}, // 2 (pen+1 음봉)
		{Open: 95, Close: 106, High: 107, Low: 94},   // 3 재돌파 양봉
	}
	boxList := []*box.Box{
		{KindOfBox: box.KindMainBox, Price: 90, BoxPosition: 0},
		{KindOfBox: box.KindDefBox, Price: 100, BoxPosition: 0, MainDefLink: []int{0}},
	}
	ctx := box.NewTradingContext(candles, boxList)
	ctx.Position = 3
	ctx.PenPosition = 1
	ctx.DefboxIndex = 1
	ctx.DefboxPrice = 100
	ctx.BuyHelper = "multidef매수대기" // 게이트 인위 개방

	out := processFollowUpBuyDecisions(ctx)
	if len(out) != 1 {
		t.Fatalf("S17 신호 1건 기대, got %d", len(out))
	}
	if out[0].Reason != "17_MultiDefWaitingBuy_FollowUp" || out[0].Helper != "MD즉시매수" {
		t.Errorf("신호 내용 오류: %+v", out[0])
	}
	if !ctx.BuyOn {
		t.Error("실매수 후 BuyOn=true여야 함")
	}
}
