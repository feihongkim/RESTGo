package stg

// mainbox_retest_analyze.go — strategy1과 분리된 독립 연구 분석기.
// DefBox 돌파 뒤 상승·저항Box·MainBox 수평선 재시험·반등을 순차적으로 재생한다.

import (
	"RESTGo/box"
)

const (
	MainBoxRetestGroupA  = "A_TOUCH_RECLAIM"
	MainBoxRetestGroupB  = "B_RESIST_TOUCH"
	MainBoxRetestGroupC  = "C1_SUPPORT_PREVHIGH"
	MainBoxRetestGroupC0 = "C0_SUPPORT_CONFIRM"
	MainBoxRetestGroupC2 = "C2_SUPPORT_MA5"

	MainBoxRetestModeTouch           = "touch"
	MainBoxRetestModeUndercutReclaim = "undercut_reclaim"
	MainBoxRetestModeLongWickReclaim = "long_wick_reclaim"
)

type MainBoxRetestConfig struct {
	MinRunupPct         float64 `json:"min_runup_pct"`
	MinResistanceGapPct float64 `json:"min_resistance_gap_pct"`
	MaxRetestBars       int     `json:"max_retest_bars"`
	TouchTolerance      float64 `json:"touch_tolerance"`
	MaxIntradayUndercut float64 `json:"max_intraday_undercut"`
	CloseRecoveryTol    float64 `json:"close_recovery_tolerance"`
	MaxCloseBreakdown   float64 `json:"max_close_breakdown"`
	ReboundWindowBars   int     `json:"rebound_window_bars"`
	RetestMode          string  `json:"retest_mode"`
	MinLowerWickBody    float64 `json:"min_lower_wick_body"`
}

func DefaultMainBoxRetestConfig() MainBoxRetestConfig {
	return MainBoxRetestConfig{
		MinRunupPct: .10, MinResistanceGapPct: .08, MaxRetestBars: 80,
		TouchTolerance: .03, MaxIntradayUndercut: .06, CloseRecoveryTol: .01,
		MaxCloseBreakdown: .05, ReboundWindowBars: 10,
		RetestMode: MainBoxRetestModeTouch, MinLowerWickBody: 1.5,
	}
}

type MainBoxRetestSignal struct {
	Group              string  `json:"group"`
	Shcode             string  `json:"shcode"`
	BreakoutPos        int     `json:"breakout_pos"`
	BreakoutDate       string  `json:"breakout_date"`
	ResistancePos      int     `json:"resistance_pos"`
	ResistanceDate     string  `json:"resistance_date,omitempty"`
	SupportPos         int     `json:"support_pos"`
	SupportDate        string  `json:"support_date,omitempty"`
	RetestPos          int     `json:"retest_pos"`
	RetestDate         string  `json:"retest_date"`
	FirePos            int     `json:"fire_pos"`
	FireDate           string  `json:"fire_date"`
	EntryPos           int     `json:"entry_pos"`
	EntryDate          string  `json:"entry_date"`
	EntryPriceOrigin   float64 `json:"entry_price_origin"`
	DefBoxIndex        int     `json:"defbox_index"`
	MainBoxIndex       int     `json:"mainbox_index"`
	MainBoxPriceOrigin float64 `json:"mainbox_price_origin"`
	MaxRunupPct        float64 `json:"max_runup_pct"`
	RetestDepthPct     float64 `json:"retest_depth_pct"`
	OriginalSignal     string  `json:"original_signal,omitempty"`
	OriginalStrategy   string  `json:"original_strategy,omitempty"`
}

type mainBoxRetestCycle struct {
	breakoutPos, defIdx, mainIdx     int
	line, maxHigh                    float64
	resistancePos, resistanceCurve   int
	supportPos, supportCurve         int
	retestA, retestB, retestC        int
	firedA, firedB, firedC           bool
	firedC0, firedC2                 bool
	originalSignal, originalStrategy string
	invalid                          bool
}

// MainBoxRetestAnalyze는 engine과 같은 CheckAndCreateDefBox→AnalyzeCurvature 순서로 실행한다.
// A/B는 가격 retest 즉시 장전하고, C는 R 뒤 MainBox 근처 support Box 확인 후 장전한다.
// 모든 군의 체결은 반등 확인봉 다음 봉 시가다.
func MainBoxRetestAnalyze(candles []*box.Candle, cfg MainBoxRetestConfig) []MainBoxRetestSignal {
	return MainBoxRetestAnalyzeWithRules(candles, cfg, nil, DefaultSettings())
}

// MainBoxRetestAnalyzeWithRules는 rules가 있으면 최초 DefBox 돌파가 해당 매수 룰에도
// 매칭된 cycle만 등록한다. 기존 strategy1 신호군과 전체 복합 돌파군을 분리 측정할 때 사용한다.
func MainBoxRetestAnalyzeWithRules(candles []*box.Candle, cfg MainBoxRetestConfig, rules []RuleConfig, settings Settings) []MainBoxRetestSignal {
	if len(candles) < 60 {
		return nil
	}
	if cfg.MaxRetestBars <= 0 {
		cfg = DefaultMainBoxRetestConfig()
	}
	for _, c := range candles {
		c.Curvekey = 0
	}
	ctx := box.NewTradingContext(candles, []*box.Box{})
	ctx.Shcode = candles[0].Shcode
	seen := map[int]bool{}
	var cycles []*mainBoxRetestCycle
	var out []MainBoxRetestSignal
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
		box.CheckAndCreateDefBox(ctx, settings.DamOption)
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)
		if ctx.DefChecker != 0 {
			if idx := findLastDefBoxIndex(ctx.BoxList); idx >= 0 && ctx.DefboxIndex != idx {
				ctx.DefboxIndex = idx
				ctx.UpdateBoxInfo()
			}
		}
		newBoxes := ctx.BoxList[before:]

		// 기존 cycle을 먼저 진행한다. 새 저항/지지 Box는 CurvePosition=i에서만 사용된다.
		for _, cy := range cycles {
			if cy.invalid || i <= cy.breakoutPos {
				continue
			}
			if i-cy.breakoutPos > cfg.MaxRetestBars || candles[i].CloseOrigin < cy.line*(1-cfg.MaxCloseBreakdown) {
				cy.invalid = true
				continue
			}
			if candles[i].HighOrigin > cy.maxHigh {
				cy.maxHigh = candles[i].HighOrigin
			}
			runup := cy.maxHigh/cy.line - 1
			for _, b := range newBoxes {
				if b.KindOfBox == box.KindDefBox || b.CurvePosition != i || b.BoxPosition <= cy.breakoutPos {
					continue
				}
				if b.BoxType == box.BoxTypeResistance && b.PriceOrigin >= cy.line*(1+cfg.MinResistanceGapPct) {
					cy.resistancePos = b.BoxPosition
					cy.resistanceCurve = i
				}
				if b.BoxType == box.BoxTypeSupport && cy.resistancePos >= 0 && b.BoxPosition > cy.resistancePos && b.PriceOrigin >= cy.line*(1-cfg.MaxIntradayUndercut) && b.PriceOrigin <= cy.line*(1+cfg.TouchTolerance) {
					cy.supportPos = b.BoxPosition
					cy.supportCurve = i
				}
			}
			if runup >= cfg.MinRunupPct && mainBoxRetestTouched(candles[i], cy.line, cfg) {
				if cy.retestA < 0 {
					cy.retestA = i
				}
				if cy.resistancePos >= 0 && cy.retestB < 0 {
					cy.retestB = i
				}
			}
			if runup >= cfg.MinRunupPct && cy.supportCurve > 0 && cy.retestC < 0 &&
				mainBoxRetestTouched(candles[cy.supportPos], cy.line, cfg) {
				cy.retestC = cy.supportCurve
				// C0: support Box가 현재 CurvePosition에서 확인되는 즉시 다음 봉 시가 체결.
				if !cy.firedC0 && i == cy.supportCurve && i+1 < len(candles) && candles[i+1].OpenOrigin > 0 {
					out = append(out, makeMainBoxRetestSignal(MainBoxRetestGroupC0, ctx.Shcode, candles, cy, cy.supportPos, i))
					cy.firedC0 = true
				}
			}
			for _, g := range []struct {
				name  string
				arm   int
				fired *bool
			}{{MainBoxRetestGroupA, cy.retestA, &cy.firedA}, {MainBoxRetestGroupB, cy.retestB, &cy.firedB}, {MainBoxRetestGroupC, cy.retestC, &cy.firedC}} {
				if *g.fired || g.arm < 0 || i <= g.arm || i-g.arm > cfg.ReboundWindowBars {
					continue
				}
				if mainBoxRetestRebound(candles, i, cy.line, cfg) && i+1 < len(candles) && candles[i+1].OpenOrigin > 0 {
					retest := g.arm
					if g.name == MainBoxRetestGroupC {
						retest = cy.supportPos
					}
					out = append(out, makeMainBoxRetestSignal(g.name, ctx.Shcode, candles, cy, retest, i))
					*g.fired = true
				}
			}
			// C2: support 확인 뒤 MA5를 아래→위로 양봉 재돌파.
			if !cy.firedC2 && cy.retestC >= 0 && i > cy.retestC && i-cy.retestC <= cfg.ReboundWindowBars &&
				mainBoxRetestMA5Rebound(candles, i, cy.line, cfg) && i+1 < len(candles) && candles[i+1].OpenOrigin > 0 {
				out = append(out, makeMainBoxRetestSignal(MainBoxRetestGroupC2, ctx.Shcode, candles, cy, cy.supportPos, i))
				cy.firedC2 = true
			}
		}

		// 최신 DefBox의 첫 복합 돌파를 새 cycle로 등록. Main link/가격은 이 순간 고정한다.
		if ctx.DefboxIndex >= 0 && !seen[ctx.DefboxIndex] && checkDefBoxBreakout(ctx, settings) {
			qualified := true
			originalSignal, originalStrategy := "", ""
			if len(rules) > 0 {
				originalSignal, originalStrategy = EvaluateRules(rules, ctx, settings)
				qualified = originalSignal != ""
			}
			def := ctx.GetDefBox()
			if qualified && def != nil && len(def.MainDefLink) > 0 {
				mi := def.MainDefLink[0]
				if mi >= 0 && mi < len(ctx.BoxList) {
					line := ctx.BoxList[mi].PriceOrigin
					if line <= 0 {
						line = def.PriceOrigin
					}
					if line > 0 {
						cycles = append(cycles, &mainBoxRetestCycle{breakoutPos: i, defIdx: ctx.DefboxIndex, mainIdx: mi, line: line, maxHigh: candles[i].HighOrigin, resistancePos: -1, supportPos: -1, retestA: -1, retestB: -1, retestC: -1, originalSignal: originalSignal, originalStrategy: originalStrategy})
						seen[ctx.DefboxIndex] = true
					}
				}
			}
		}
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) > 0 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}
	}
	return out
}

func mainBoxRetestTouched(c *box.Candle, line float64, cfg MainBoxRetestConfig) bool {
	base := c.LowOrigin <= line*(1+cfg.TouchTolerance) && c.LowOrigin >= line*(1-cfg.MaxIntradayUndercut) && c.CloseOrigin >= line*(1-cfg.CloseRecoveryTol)
	if !base {
		return false
	}
	if cfg.RetestMode == MainBoxRetestModeUndercutReclaim || cfg.RetestMode == MainBoxRetestModeLongWickReclaim {
		if c.LowOrigin >= line || c.CloseOrigin < line {
			return false
		}
	}
	if cfg.RetestMode == MainBoxRetestModeLongWickReclaim {
		body := c.CloseOrigin - c.OpenOrigin
		if body < 0 {
			body = -body
		}
		lowerWick := c.OpenOrigin
		if c.CloseOrigin < lowerWick {
			lowerWick = c.CloseOrigin
		}
		lowerWick -= c.LowOrigin
		return body > 0 && lowerWick/body >= cfg.MinLowerWickBody
	}
	return true
}
func mainBoxRetestRebound(c []*box.Candle, i int, line float64, cfg MainBoxRetestConfig) bool {
	if i < 1 {
		return false
	}
	return c[i].CloseOrigin > c[i].OpenOrigin && c[i].CloseOrigin > c[i-1].HighOrigin && c[i].CloseOrigin >= line*(1-cfg.CloseRecoveryTol)
}
func mainBoxRetestMA5Rebound(c []*box.Candle, i int, line float64, cfg MainBoxRetestConfig) bool {
	if i < 1 || c[i].Ma5Origin <= 0 || c[i-1].Ma5Origin <= 0 {
		return false
	}
	return c[i].CloseOrigin > c[i].OpenOrigin && c[i-1].CloseOrigin <= c[i-1].Ma5Origin &&
		c[i].CloseOrigin > c[i].Ma5Origin && c[i].CloseOrigin >= line*(1-cfg.CloseRecoveryTol)
}
func makeMainBoxRetestSignal(group, shcode string, c []*box.Candle, cy *mainBoxRetestCycle, retest, fire int) MainBoxRetestSignal {
	s := MainBoxRetestSignal{Group: group, Shcode: shcode, BreakoutPos: cy.breakoutPos, BreakoutDate: c[cy.breakoutPos].Date, ResistancePos: cy.resistancePos, SupportPos: cy.supportPos, RetestPos: retest, RetestDate: c[retest].Date, FirePos: fire, FireDate: c[fire].Date, EntryPos: fire + 1, EntryDate: c[fire+1].Date, EntryPriceOrigin: c[fire+1].OpenOrigin, DefBoxIndex: cy.defIdx, MainBoxIndex: cy.mainIdx, MainBoxPriceOrigin: cy.line, MaxRunupPct: (cy.maxHigh/cy.line - 1) * 100, RetestDepthPct: (c[retest].LowOrigin/cy.line - 1) * 100, OriginalSignal: cy.originalSignal, OriginalStrategy: cy.originalStrategy}
	if cy.resistancePos >= 0 {
		s.ResistanceDate = c[cy.resistancePos].Date
	}
	if cy.supportPos >= 0 {
		s.SupportDate = c[cy.supportPos].Date
	}
	return s
}
