package stg

import (
	"RESTGo/box"
	"testing"
)

func TestBuildDescendingTrendlinePattern(t *testing.T) {
	cfg := DefaultDescendingTrendlineConfig()
	cfg.MinPatternBars = 20
	bs := []*box.Box{{BoxType: box.BoxTypeResistance, BoxPosition: 10, CurvePosition: 12, PriceOrigin: 150}, {BoxType: box.BoxTypeSupport, BoxPosition: 25, CurvePosition: 28, PriceOrigin: 100}, {BoxType: box.BoxTypeResistance, BoxPosition: 45, CurvePosition: 48, PriceOrigin: 130}, {BoxType: box.BoxTypeSupport, BoxPosition: 70, CurvePosition: 73, PriceOrigin: 103}}
	p, ok := buildDescendingTrendlinePattern(bs, cfg)
	if !ok {
		t.Fatal("valid R-S-R-S rejected")
	}
	if p.Slope >= 0 || p.SupportDiff > .05 {
		t.Fatalf("bad metrics: %+v", p)
	}
	if descendingLineAt(p, p.R2Pos) != p.R2Price {
		t.Fatal("trendline does not pass R2")
	}
}
func TestBuildDescendingTrendlineRejectsUnevenFloor(t *testing.T) {
	cfg := DefaultDescendingTrendlineConfig()
	cfg.MinPatternBars = 10
	bs := []*box.Box{{BoxType: 1, BoxPosition: 10, PriceOrigin: 150}, {BoxType: 0, BoxPosition: 20, PriceOrigin: 100}, {BoxType: 1, BoxPosition: 30, PriceOrigin: 130}, {BoxType: 0, BoxPosition: 40, PriceOrigin: 112}}
	if _, ok := buildDescendingTrendlinePattern(bs, cfg); ok {
		t.Fatal("uneven supports accepted")
	}
}
func TestDescendingTrendlineMAConvergenceAndImminentGolden(t *testing.T) {
	candles := make([]*box.Candle, 21)
	for i := range candles {
		candles[i] = &box.Candle{Ma60Origin: 95 + float64(i)*.24, Ma120Origin: 100, CloseOrigin: 101}
	}
	cfg := DefaultDescendingTrendlineConfig()
	cfg.RequireMAConvergence = true
	cfg.MAGapLookback = 20
	cfg.MaxMA60120Gap = .01
	cfg.GoldenCrossMode = DescendingGoldenImminent
	if !DescendingTrendlinePassesContext(candles, 20, DescendingTrendlinePattern{}, cfg) {
		t.Fatal("converging imminent golden rejected")
	}
	cfg.RequireApexProximity = true
	cfg.MaxApexDistanceBars = 5
	if DescendingTrendlinePassesContext(candles, 20, DescendingTrendlinePattern{ApexPosition: 40}, cfg) {
		t.Fatal("far from apex accepted")
	}
}

func TestDescendingTrendlineLateSideways(t *testing.T) {
	candles := make([]*box.Candle, 11)
	for i := range candles {
		candles[i] = &box.Candle{OpenOrigin: 100, HighOrigin: 103, LowOrigin: 97, CloseOrigin: 100 + float64(i%2)}
	}
	cfg := DefaultDescendingTrendlineConfig()
	cfg.RequireLateSideways = true
	cfg.SidewaysLookback = 10
	cfg.MaxSidewaysRange = .08
	cfg.MaxSidewaysNetChange = .03
	if !DescendingTrendlinePassesContext(candles, 10, DescendingTrendlinePattern{}, cfg) {
		t.Fatal("valid late sideways rejected")
	}
	candles[5].HighOrigin = 115
	if DescendingTrendlinePassesContext(candles, 10, DescendingTrendlinePattern{}, cfg) {
		t.Fatal("wide late range accepted")
	}
}

func TestDescendingTrendlineFilters(t *testing.T) {
	cfg := DefaultDescendingTrendlineConfig()
	cfg.RequireMA20Recovery = true
	cfg.RequireVolume = true
	c := &box.Candle{CloseOrigin: 110, Ma20Origin: 100, Volume: 200, VolMa20: 100}
	if !DescendingTrendlinePassesContext([]*box.Candle{c, c}, 1, DescendingTrendlinePattern{}, cfg) {
		t.Fatal("valid filters rejected")
	}
	c.CloseOrigin = 90
	if DescendingTrendlinePassesContext([]*box.Candle{c, c}, 1, DescendingTrendlinePattern{}, cfg) {
		t.Fatal("below MA20 accepted")
	}
}
