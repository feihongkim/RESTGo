package cond

import (
	"RESTGo/box"
	"testing"
)

func volumeWaveBox(pos, curve, boxType int, price float64) *box.Box {
	return &box.Box{
		BoxPosition: pos, CurvePosition: curve, BoxType: boxType,
		KindOfBox: box.KindBox, PriceOrigin: price,
	}
}

func TestEvaluateVolumeWaveHighBaseBoxesSRS(t *testing.T) {
	cfg := DefaultVolumeWaveConfig()
	cfg.BaseBoxPattern = VolumeWaveBaseBoxSRS
	boxes := []*box.Box{
		volumeWaveBox(8, 9, box.BoxTypeResistance, 90), // VW1 이전: 제외
		volumeWaveBox(22, 23, box.BoxTypeSupport, 105),
		volumeWaveBox(25, 26, box.BoxTypeResistance, 112),
		volumeWaveBox(28, 29, box.BoxTypeSupport, 107),
	}
	m, ok := EvaluateVolumeWaveHighBaseBoxes(boxes, 20, 32, cfg)
	if !ok || !m.PatternFound {
		t.Fatalf("S-R-S not found: %+v", m)
	}
	if m.FirstSupportPos != 22 || m.ResistancePos != 25 || m.SecondSupportPos != 28 {
		t.Fatalf("wrong pivots: %+v", m)
	}
}

func TestEvaluateVolumeWaveHighBaseBoxesRejectsFutureConfirmedBox(t *testing.T) {
	cfg := DefaultVolumeWaveConfig()
	cfg.BaseBoxPattern = VolumeWaveBaseBoxSRS
	boxes := []*box.Box{
		volumeWaveBox(22, 23, box.BoxTypeSupport, 105),
		volumeWaveBox(25, 26, box.BoxTypeResistance, 112),
		volumeWaveBox(28, 33, box.BoxTypeSupport, 107), // VW2=32 뒤에야 확인
	}
	if m, ok := EvaluateVolumeWaveHighBaseBoxes(boxes, 20, 32, cfg); ok {
		t.Fatalf("future-confirmed support accepted: %+v", m)
	}
}

func TestVolumeWaveCumulativeOBVAccumulation(t *testing.T) {
	candles := make([]*box.Candle, 20)
	for i := range candles {
		candles[i] = &box.Candle{Volume: 100, OBV: float64(i * 100)}
	}
	for i := 15; i < 20; i++ {
		candles[i].Volume = 150
	}
	ctx := box.NewTradingContext(candles, nil)
	ctx.Position = 19
	cfg := DefaultVolumeWaveConfig()
	cfg.AccumulationMode = VolumeWaveAccumulationCumulativeOBV
	cfg.AccumulationWindow = 5
	cfg.AccumulationVolumeRatio = 1.4
	if !IsVolumeWaveAccumulation(ctx, cfg) {
		t.Fatal("increased cumulative volume with rising OBV rejected")
	}
	candles[19].OBV = candles[14].OBV - 1
	if IsVolumeWaveAccumulation(ctx, cfg) {
		t.Fatal("falling OBV accepted")
	}
}

func TestEvaluateVolumeWaveStrictHighBase(t *testing.T) {
	candles := make([]*box.Candle, 8)
	candles[0] = &box.Candle{Close: 100, High: 102, Low: 99, Volume: 1000}
	for i := 1; i <= 6; i++ {
		candles[i] = &box.Candle{Close: 100 + float64(i%2), High: 103, Low: 98, Volume: float64(700 - i*50)}
	}
	candles[7] = &box.Candle{Close: 110, High: 111, Low: 100, Volume: 1100}
	cfg := DefaultVolumeWaveConfig()
	cfg.BaseMinBars, cfg.BaseMaxBars = 5, 10
	cfg.BaseMinCloseRetention = 0.8
	cfg.BaseMaxHighLowRange = 0.12
	cfg.BaseLateEarlyVolRatio = 0.9
	m, ok := EvaluateVolumeWaveHighBase(candles, 0, 7, cfg)
	if !ok {
		t.Fatalf("strict high base rejected: %+v", m)
	}
	candles[3].Low = 85
	if m, ok = EvaluateVolumeWaveHighBase(candles, 0, 7, cfg); ok {
		t.Fatalf("wide high-low base accepted: %+v", m)
	}
}

func TestEvaluateVolumeWaveHighBaseBoxesSupportHeld(t *testing.T) {
	cfg := DefaultVolumeWaveConfig()
	cfg.BaseBoxPattern = VolumeWaveBaseBoxSRSSupportHeld
	cfg.BaseSecondSupportRatio = 0.97
	boxes := []*box.Box{
		volumeWaveBox(22, 23, box.BoxTypeSupport, 105),
		volumeWaveBox(25, 26, box.BoxTypeResistance, 112),
		volumeWaveBox(28, 29, box.BoxTypeSupport, 100), // 95.2%, 기준 미달
	}
	if m, ok := EvaluateVolumeWaveHighBaseBoxes(boxes, 20, 32, cfg); ok {
		t.Fatalf("lower second support accepted: %+v", m)
	}
	cfg.BaseBoxPattern = VolumeWaveBaseBoxSRS
	if _, ok := EvaluateVolumeWaveHighBaseBoxes(boxes, 20, 32, cfg); !ok {
		t.Fatal("plain S-R-S should not enforce support ratio")
	}
}
