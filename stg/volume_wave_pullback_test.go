package stg

import "testing"

func TestVolumeWavePullbackAnalyzeStage2NextOpenEntry(t *testing.T) {
	candles, waveCfg := volumeWaveFixture(700)
	candles[31].OpenOrigin = 112
	waves := VolumeWaveAnalyze(candles, waveCfg)
	cfg := DefaultVolumeWavePullbackConfig(2)
	cfg.StructureTolerance = 0.04
	entries := VolumeWavePullbackAnalyze(candles, waves, cfg)
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.SourcePos != 26 || e.FirePos != 30 || e.EntryPos != 31 {
		t.Fatalf("positions source/fire/entry=%d/%d/%d want 26/30/31", e.SourcePos, e.FirePos, e.EntryPos)
	}
	if e.EntryPriceOrigin != 112 {
		t.Fatalf("entry price=%.2f, want next open 112", e.EntryPriceOrigin)
	}
	if e.EntryPos <= e.SourcePos {
		t.Fatal("breakout-day chase must be impossible")
	}
}

func TestVolumeWavePullbackVolumeContractionGate(t *testing.T) {
	candles, waveCfg := volumeWaveFixture(700)
	waves := VolumeWaveAnalyze(candles, waveCfg)
	for _, i := range []int{27, 28, 29} {
		candles[i].Volume = 600
	}
	cfg := DefaultVolumeWavePullbackConfig(2)
	cfg.StructureTolerance = 0.04
	cfg.MaxPullbackAverageVolumeRatio = 0.50
	if got := VolumeWavePullbackAnalyze(candles, waves, cfg); len(got) != 0 {
		t.Fatalf("high-volume pullback must be rejected: %+v", got)
	}
}

func TestVolumeWavePullbackAblationCanDisableFilters(t *testing.T) {
	candles, waveCfg := volumeWaveFixture(700)
	waves := VolumeWaveAnalyze(candles, waveCfg)
	cfg := VolumeWavePullbackConfig{
		SourceStage: 1, MinWaitBars: 1, MaxWaitBars: 10,
		MinDepth: 0, MaxDepth: 1,
		Confirmation: VolumeWaveConfirmCloseUp,
		Structure:    VolumeWaveStructureNone,
	}
	entries := VolumeWavePullbackAnalyze(candles, waves, cfg)
	if len(entries) == 0 {
		t.Fatal("unfiltered first-pullback ablation should produce a VW1 entry")
	}
	if entries[0].SourcePos != 20 || entries[0].EntryPos <= entries[0].FirePos {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}
