package stg

// descending_trendline_analyze.go — R1→S1→낮은 R2→유사 S2 장기 수렴 후
// R1-R2 하락추세선 종가 돌파를 다음 봉 시가로 포착하는 독립 연구 분석기.

import (
	"RESTGo/box"
	"math"
)

const (
	DescendingGoldenNone     = "none"
	DescendingGoldenRecent   = "recent"
	DescendingGoldenImminent = "imminent"
	DescendingGoldenEither   = "either"
)

type DescendingTrendlineConfig struct {
	SupportTolerance     float64 `json:"support_tolerance"`
	MinResistanceDrop    float64 `json:"min_resistance_drop"`
	MinPatternBars       int     `json:"min_pattern_bars"`
	MaxPatternBars       int     `json:"max_pattern_bars"`
	MaxBreakoutWait      int     `json:"max_breakout_wait"`
	BreakoutBuffer       float64 `json:"breakout_buffer"`
	MaxFloorBreakdown    float64 `json:"max_floor_breakdown"`
	RequireMA20Recovery  bool    `json:"require_ma20_recovery"`
	RequireMA60Recovery  bool    `json:"require_ma60_recovery"`
	RequireVolume        bool    `json:"require_volume"`
	BreakoutVolumeRatio  float64 `json:"breakout_volume_ratio"`
	RequireApexProximity bool    `json:"require_apex_proximity"`
	MaxApexDistanceBars  int     `json:"max_apex_distance_bars"`
	RequireMAConvergence bool    `json:"require_ma_convergence"`
	MAGapLookback        int     `json:"ma_gap_lookback"`
	MaxMA60120Gap        float64 `json:"max_ma60_120_gap"`
	GoldenCrossMode      string  `json:"golden_cross_mode"`
	GoldenCrossLookback  int     `json:"golden_cross_lookback"`
	RequireLateSideways  bool    `json:"require_late_sideways"`
	SidewaysLookback     int     `json:"sideways_lookback"`
	MaxSidewaysRange     float64 `json:"max_sideways_range"`
	MaxSidewaysNetChange float64 `json:"max_sideways_net_change"`
}

func DefaultDescendingTrendlineConfig() DescendingTrendlineConfig {
	return DescendingTrendlineConfig{SupportTolerance: .05, MinResistanceDrop: .05, MinPatternBars: 40, MaxPatternBars: 180, MaxBreakoutWait: 40, BreakoutBuffer: 0, MaxFloorBreakdown: .05, BreakoutVolumeRatio: 1.5, MaxApexDistanceBars: 10, MAGapLookback: 20, MaxMA60120Gap: .02, GoldenCrossMode: DescendingGoldenNone, GoldenCrossLookback: 20, SidewaysLookback: 20, MaxSidewaysRange: .10, MaxSidewaysNetChange: .03}
}

type DescendingTrendlinePattern struct {
	R1Pos, S1Pos, R2Pos, S2Pos                     int
	R1Curve, S1Curve, R2Curve, S2Curve             int
	R1Price, S1Price, R2Price, S2Price             float64
	FloorPrice, ResistanceDrop, SupportDiff, Slope float64
	ApexPosition                                   float64
	PatternBars                                    int
}

type DescendingTrendlineSignal struct {
	Shcode           string                     `json:"shcode"`
	Date             string                     `json:"date"`
	Pos              int                        `json:"pos"`
	EntryDate        string                     `json:"entry_date"`
	EntryPos         int                        `json:"entry_pos"`
	EntryPriceOrigin float64                    `json:"entry_price_origin"`
	TrendlinePrice   float64                    `json:"trendline_price"`
	BreakoutPct      float64                    `json:"breakout_pct"`
	VolumeRatio      float64                    `json:"volume_ratio"`
	Pattern          DescendingTrendlinePattern `json:"pattern"`
}

type descendingTrendlineArm struct {
	pattern DescendingTrendlinePattern
	armPos  int
}

func DescendingTrendlineAnalyze(candles []*box.Candle, cfg DescendingTrendlineConfig) []DescendingTrendlineSignal {
	if len(candles) < 80 {
		return nil
	}
	if cfg.MaxPatternBars <= 0 {
		cfg = DefaultDescendingTrendlineConfig()
	}
	for _, c := range candles {
		c.Curvekey = 0
	}
	ctx := box.NewTradingContext(candles, []*box.Box{})
	ctx.Shcode = candles[0].Shcode
	var arm *descendingTrendlineArm
	var out []DescendingTrendlineSignal
	for i := 5; i < len(candles); i++ {
		ctx.Position = i
		if i == 5 {
			if candles[i].Gradient >= 0 {
				candles[i].Curvekey = 1
			} else {
				candles[i].Curvekey = -1
			}
			continue
		}
		before := len(ctx.BoxList)
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)
		newBox := len(ctx.BoxList) > before
		if arm != nil {
			line := descendingLineAt(arm.pattern, i)
			floor := arm.pattern.FloorPrice
			if i-arm.armPos > cfg.MaxBreakoutWait || candles[i].CloseOrigin < floor*(1-cfg.MaxFloorBreakdown) || line <= floor {
				arm = nil
			} else if i > arm.armPos && i+1 < len(candles) {
				prevLine := descendingLineAt(arm.pattern, i-1)
				cur := candles[i]
				edge := candles[i-1].CloseOrigin <= prevLine*(1+cfg.BreakoutBuffer) && cur.CloseOrigin > line*(1+cfg.BreakoutBuffer) && cur.CloseOrigin > cur.OpenOrigin
				if edge && DescendingTrendlinePassesContext(candles, i, arm.pattern, cfg) {
					vr := 0.0
					if cur.VolMa20 > 0 {
						vr = cur.Volume / cur.VolMa20
					}
					out = append(out, DescendingTrendlineSignal{Shcode: ctx.Shcode, Date: cur.Date, Pos: i, EntryDate: candles[i+1].Date, EntryPos: i + 1, EntryPriceOrigin: candles[i+1].OpenOrigin, TrendlinePrice: line, BreakoutPct: (cur.CloseOrigin/line - 1) * 100, VolumeRatio: vr, Pattern: arm.pattern})
					arm = nil
				}
			}
		}
		if newBox && len(ctx.BoxList) >= 4 {
			n := len(ctx.BoxList)
			if p, ok := buildDescendingTrendlinePattern(ctx.BoxList[n-4:], cfg); ok && p.S2Curve == i {
				arm = &descendingTrendlineArm{p, i}
			}
		}
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) > 0 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}
	}
	return out
}

func buildDescendingTrendlinePattern(bs []*box.Box, cfg DescendingTrendlineConfig) (DescendingTrendlinePattern, bool) {
	var p DescendingTrendlinePattern
	if len(bs) != 4 {
		return p, false
	}
	r1, s1, r2, s2 := bs[0], bs[1], bs[2], bs[3]
	if r1.BoxType != box.BoxTypeResistance || s1.BoxType != box.BoxTypeSupport || r2.BoxType != box.BoxTypeResistance || s2.BoxType != box.BoxTypeSupport {
		return p, false
	}
	if !(r1.BoxPosition < s1.BoxPosition && s1.BoxPosition < r2.BoxPosition && r2.BoxPosition < s2.BoxPosition) || r1.PriceOrigin <= 0 || s1.PriceOrigin <= 0 || r2.PriceOrigin <= 0 || s2.PriceOrigin <= 0 {
		return p, false
	}
	bars := s2.BoxPosition - r1.BoxPosition
	if bars < cfg.MinPatternBars || bars > cfg.MaxPatternBars {
		return p, false
	}
	drop := 1 - r2.PriceOrigin/r1.PriceOrigin
	if drop < cfg.MinResistanceDrop {
		return p, false
	}
	sd := math.Abs(s2.PriceOrigin/s1.PriceOrigin - 1)
	if sd > cfg.SupportTolerance {
		return p, false
	}
	slope := (r2.PriceOrigin - r1.PriceOrigin) / float64(r2.BoxPosition-r1.BoxPosition)
	floor := (s1.PriceOrigin + s2.PriceOrigin) / 2
	lineS2 := r1.PriceOrigin + slope*float64(s2.BoxPosition-r1.BoxPosition)
	if slope >= 0 || lineS2 <= floor*1.02 {
		return p, false
	}
	apex := float64(r1.BoxPosition) + (floor-r1.PriceOrigin)/slope
	p = DescendingTrendlinePattern{R1Pos: r1.BoxPosition, S1Pos: s1.BoxPosition, R2Pos: r2.BoxPosition, S2Pos: s2.BoxPosition, R1Curve: r1.CurvePosition, S1Curve: s1.CurvePosition, R2Curve: r2.CurvePosition, S2Curve: s2.CurvePosition, R1Price: r1.PriceOrigin, S1Price: s1.PriceOrigin, R2Price: r2.PriceOrigin, S2Price: s2.PriceOrigin, FloorPrice: floor, ResistanceDrop: drop, SupportDiff: sd, Slope: slope, ApexPosition: apex, PatternBars: bars}
	return p, true
}
func descendingLineAt(p DescendingTrendlinePattern, pos int) float64 {
	return p.R1Price + p.Slope*float64(pos-p.R1Pos)
}

// DescendingTrendlinePassesContext는 돌파 시점까지 알려진 apex·MA60/120 수렴만 사용한다.
func DescendingTrendlinePassesContext(c []*box.Candle, i int, p DescendingTrendlinePattern, cfg DescendingTrendlineConfig) bool {
	if i < 1 || i >= len(c) {
		return false
	}
	cur := c[i]
	if cfg.RequireMA20Recovery && (cur.Ma20Origin <= 0 || cur.CloseOrigin <= cur.Ma20Origin) {
		return false
	}
	if cfg.RequireMA60Recovery && (cur.Ma60Origin <= 0 || cur.CloseOrigin <= cur.Ma60Origin) {
		return false
	}
	if cfg.RequireVolume && (cur.VolMa20 <= 0 || cur.Volume < cur.VolMa20*cfg.BreakoutVolumeRatio) {
		return false
	}
	if cfg.RequireApexProximity && math.Abs(p.ApexPosition-float64(i)) > float64(cfg.MaxApexDistanceBars) {
		return false
	}
	if cfg.RequireMAConvergence {
		lb := cfg.MAGapLookback
		if lb <= 0 || i-lb < 0 {
			return false
		}
		gap, old := ma60120Gap(c[i]), ma60120Gap(c[i-lb])
		if gap < 0 || old < 0 || gap >= old || gap > cfg.MaxMA60120Gap {
			return false
		}
	}
	if cfg.RequireLateSideways && !descendingLateSideways(c, i, cfg) {
		return false
	}
	switch cfg.GoldenCrossMode {
	case DescendingGoldenRecent:
		return descendingRecentGolden(c, i, cfg.GoldenCrossLookback)
	case DescendingGoldenImminent:
		return descendingImminentGolden(c, i, cfg.MaxMA60120Gap)
	case DescendingGoldenEither:
		return descendingRecentGolden(c, i, cfg.GoldenCrossLookback) || descendingImminentGolden(c, i, cfg.MaxMA60120Gap)
	}
	return true
}
func descendingLateSideways(c []*box.Candle, i int, cfg DescendingTrendlineConfig) bool {
	lb := cfg.SidewaysLookback
	if lb <= 1 || i-lb < 0 {
		return false
	}
	start, end := i-lb, i-1
	hi, lo := c[start].HighOrigin, c[start].LowOrigin
	for j := start + 1; j <= end; j++ {
		if c[j].HighOrigin > hi {
			hi = c[j].HighOrigin
		}
		if c[j].LowOrigin < lo {
			lo = c[j].LowOrigin
		}
	}
	if lo <= 0 || hi/lo-1 > cfg.MaxSidewaysRange {
		return false
	}
	return math.Abs(c[end].CloseOrigin/c[start].CloseOrigin-1) <= cfg.MaxSidewaysNetChange
}

func ma60120Gap(c *box.Candle) float64 {
	if c.Ma60Origin <= 0 || c.Ma120Origin <= 0 {
		return -1
	}
	return math.Abs(c.Ma60Origin-c.Ma120Origin) / c.Ma120Origin
}
func descendingRecentGolden(c []*box.Candle, i, lookback int) bool {
	if lookback <= 0 {
		lookback = 20
	}
	start := i - lookback
	if start < 1 {
		start = 1
	}
	for j := start; j <= i; j++ {
		if c[j-1].Ma60Origin > 0 && c[j-1].Ma120Origin > 0 && c[j-1].Ma60Origin <= c[j-1].Ma120Origin && c[j].Ma60Origin > c[j].Ma120Origin {
			return true
		}
	}
	return false
}
func descendingImminentGolden(c []*box.Candle, i int, maxGap float64) bool {
	if i < 5 || ma60120Gap(c[i]) < 0 || ma60120Gap(c[i]) > maxGap || c[i].Ma60Origin > c[i].Ma120Origin {
		return false
	}
	fast := c[i].Ma60Origin - c[i-5].Ma60Origin
	slow := c[i].Ma120Origin - c[i-5].Ma120Origin
	return fast > slow && fast > 0
}
