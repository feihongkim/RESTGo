package stg

import (
	"RESTGo/box"
	"RESTGo/cond"
	"fmt"
	"testing"
)

func volumeWaveFixture(wave2Volume float64) ([]*box.Candle, cond.VolumeWaveConfig) {
	candles := make([]*box.Candle, 45)
	for i := range candles {
		close := 100.0
		volume := 90.0 + float64(i%3)*10
		candles[i] = &box.Candle{
			Shcode: "TEST", Date: fmt.Sprintf("202601%02d", i+1),
			Open: close - 0.5, High: close + 1, Low: close - 1, Close: close,
			OpenOrigin: close - 0.5, HighOrigin: close + 1,
			LowOrigin: close - 1, CloseOrigin: close,
			Volume: volume, Ma20: 100, Ma20Origin: 100, ATR: 5,
		}
	}

	// 2~3개월 전 역할의 선행 거래량봉(테스트에서는 lead 창을 단축).
	candles[10].Volume = 500

	// 1파동: 거래량 급증 + 직전 고점 돌파.
	candles[20].Open, candles[20].OpenOrigin = 100, 100
	candles[20].Close, candles[20].CloseOrigin = 110, 110
	candles[20].High, candles[20].HighOrigin = 111, 111
	candles[20].Low, candles[20].LowOrigin = 99, 99
	candles[20].Volume = 600

	// 5봉 고가 횡보: 가격 유지 + 거래량 수축.
	for i := 21; i <= 25; i++ {
		close := 108.0 + float64(i%2)
		candles[i].Open, candles[i].OpenOrigin = close-0.5, close-0.5
		candles[i].Close, candles[i].CloseOrigin = close, close
		candles[i].High, candles[i].HighOrigin = 110, 110
		candles[i].Low, candles[i].LowOrigin = 106, 106
		candles[i].Volume = 90 + float64(i%3)*10
	}

	// 2파동: 고가 횡보 상단 돌파, 1파동보다 큰 거래량.
	candles[26].Open, candles[26].OpenOrigin = 109, 109
	candles[26].Close, candles[26].CloseOrigin = 120, 120
	candles[26].High, candles[26].HighOrigin = 121, 121
	candles[26].Low, candles[26].LowOrigin = 108, 108
	candles[26].Volume = wave2Volume

	// MA20 눌림. 29번 봉이 MA20을 저가/종가로 테스트한다.
	pullbackClose := []float64{115, 110, 106}
	for k, i := range []int{27, 28, 29} {
		close := pullbackClose[k]
		candles[i].Open, candles[i].OpenOrigin = close+1, close+1
		candles[i].Close, candles[i].CloseOrigin = close, close
		candles[i].High, candles[i].HighOrigin = close+1, close+1
		candles[i].Low, candles[i].LowOrigin = close-1, close-1
		candles[i].Volume = 90 + float64(i%3)*10
	}
	candles[29].Ma20, candles[29].Ma20Origin = 107, 107
	candles[29].Low, candles[29].LowOrigin = 105, 105

	// 3파동: 양봉으로 MA20 재돌파.
	candles[30].Open, candles[30].OpenOrigin = 106, 106
	candles[30].Close, candles[30].CloseOrigin = 110, 110
	candles[30].High, candles[30].HighOrigin = 111, 111
	candles[30].Low, candles[30].LowOrigin = 105, 105
	candles[30].Ma20, candles[30].Ma20Origin = 108, 108
	candles[30].Volume = 120

	// 3파동 이후 과열 장대봉.
	candles[35].Open, candles[35].OpenOrigin = 120, 120
	candles[35].Close, candles[35].CloseOrigin = 135, 135
	candles[35].High, candles[35].HighOrigin = 145, 145
	candles[35].Low, candles[35].LowOrigin = 118, 118
	candles[35].Volume = 900
	candles[35].ATR = 5

	cfg := cond.DefaultVolumeWaveConfig()
	cfg.VolumeWindow = 5
	cfg.AccumulationMinLead = 5
	cfg.AccumulationMaxLead = 20
	cfg.BreakoutLookback = 5
	cfg.BaseMinBars = 5
	cfg.BaseMaxBars = 10
	cfg.PullbackMinBars = 2
	cfg.PullbackMaxBars = 10
	cfg.MA20RisingStreak = 0
	cfg.PullbackMaxVolumeRatio = 0.8
	cfg.ClimaxMaxBars = 10
	cfg.ClimaxUpperWickBody = 0.5
	return candles, cfg
}

func TestVolumeWaveAnalyzeFullCycle(t *testing.T) {
	candles, cfg := volumeWaveFixture(700)
	signals := VolumeWaveAnalyze(candles, cfg)
	wantKinds := []string{VolumeWaveStage1, VolumeWaveStage2, VolumeWaveStage3, VolumeWaveExit}
	wantPos := []int{20, 26, 30, 35}
	if len(signals) != len(wantKinds) {
		t.Fatalf("signals=%d want=%d: %+v", len(signals), len(wantKinds), signals)
	}
	for i := range wantKinds {
		if signals[i].Kind != wantKinds[i] || signals[i].Pos != wantPos[i] {
			t.Errorf("signal[%d]=%s@%d want %s@%d", i, signals[i].Kind, signals[i].Pos, wantKinds[i], wantPos[i])
		}
	}
	if signals[1].Cycle.Base.Bars != 5 || signals[2].Cycle.Pullback.TouchCount == 0 {
		t.Fatalf("pattern metrics missing: base=%+v pullback=%+v", signals[1].Cycle.Base, signals[2].Cycle.Pullback)
	}
}

func TestVolumeWaveAnalyzeRejectsSmallerSecondWaveVolume(t *testing.T) {
	candles, cfg := volumeWaveFixture(500) // wave1=600, required >=630
	signals := VolumeWaveAnalyze(candles, cfg)
	if len(signals) == 0 || signals[0].Kind != VolumeWaveStage1 {
		t.Fatalf("first wave missing: %+v", signals)
	}
	for _, signal := range signals {
		if signal.Kind == VolumeWaveStage2 || signal.Kind == VolumeWaveStage3 || signal.Kind == VolumeWaveExit {
			t.Fatalf("smaller wave2 volume advanced the original cycle: %+v", signals)
		}
	}
}

func TestVolumeWaveAnalyzeHasNoFutureDependency(t *testing.T) {
	candles, cfg := volumeWaveFixture(700)
	prefix := VolumeWaveAnalyze(candles[:27], cfg)
	full := VolumeWaveAnalyze(candles, cfg)
	if len(prefix) != 2 || len(full) < 2 {
		t.Fatalf("unexpected signal counts prefix=%d full=%d", len(prefix), len(full))
	}
	for i := 0; i < 2; i++ {
		if prefix[i].Kind != full[i].Kind || prefix[i].Pos != full[i].Pos || prefix[i].Date != full[i].Date {
			t.Fatalf("past signal changed after future append: prefix=%+v full=%+v", prefix[i], full[i])
		}
	}
}
