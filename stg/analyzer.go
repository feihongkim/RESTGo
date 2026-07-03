package stg

import (
	"fmt"

	"RESTGo/box"
	"RESTGo/cond"
)

// activeRules 는 LoadRulesWithSettings()로 초기화되는 활성 전략 목록
var activeRules []RuleConfig

// activeSettings 는 LoadRulesWithSettings()로 초기화되는 활성 설정값
// YAML의 settings 블록이 DefaultSettings()를 덮어쓴다.
var activeSettings Settings = DefaultSettings()

// activeSellSettings 는 LoadSellStrategy()로 초기화되는 매도 설정 (없으면 비활성)
var activeSellSettings *SellSettings

// GetActiveSettings 는 LoadStrategy()로 로드된 현재 활성 Settings를 반환한다.
func GetActiveSettings() Settings {
	return activeSettings
}

// LoadStrategy 는 YAML 파일에서 전략 룰과 settings 오버라이드를 함께 로드
func LoadStrategy(path string) error {
	rules, settings, err := LoadRulesWithSettings(path)
	if err != nil {
		return fmt.Errorf("전략 로드 실패 (%s): %w", path, err)
	}
	activeRules = rules
	activeSettings = settings
	fmt.Printf("[stg] 전략 %d개 로드: %s\n", len(rules), path)
	return nil
}

// LoadSellStrategyFile 은 매도 룰 YAML을 로드하고 activeSellSettings를 설정한다.
// 호출 시점에 매도 평가가 활성화된다.
func LoadSellStrategyFile(path string) error {
	settings, err := LoadSellStrategy(path)
	if err != nil {
		return err
	}
	activeSellSettings = &settings
	return nil
}

// AnalyzeWithRules 는 명시적 rules/settings로 분석한다 (그리드 러너용).
// 전역 activeRules를 수정하지 않으므로 병렬 호출 안전.
func AnalyzeWithRules(candles []*box.Candle, rules []RuleConfig, settings Settings) AnalysisResult {
	return analyzeInternal(candles, settings, rules)
}

// Analyze 는 캔들 리스트에 대해 Box/DefBox 분석을 수행하고 매수 신호를 반환
func Analyze(candles []*box.Candle, settings Settings) AnalysisResult {
	return analyzeInternal(candles, settings, activeRules)
}

// analyzeInternal 은 Analyze/AnalyzeWithRules 공용 구현체.
func analyzeInternal(candles []*box.Candle, settings Settings, rules []RuleConfig) AnalysisResult {
	if len(candles) < 6 {
		return AnalysisResult{}
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
		ctx.Hname = candles[0].Hname
	}
	result := AnalysisResult{}

	for i := 5; i < len(candles); i++ {
		ctx.Position = i

		if i == 5 {
			if candles[i].Gradient >= 0.0 {
				candles[i].Curvekey = 1
			} else {
				candles[i].Curvekey = -1
			}
			continue
		}

		// 1. DefBox 생성 조건 체크
		box.CheckAndCreateDefBox(ctx, settings.DamOption)

		// 2. 곡률 분석 및 CurveKey 업데이트
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)

		// 3. DefBox 돌파 및 매수 신호 평가 (REST1 + REST2 + FollowUp — 한 캔들 다중 신호 가능)
		//    + 트리거 룰 평가 (trigger: 필드가 있는 룰 — 메인이벤트 발화 캔들에서만 조건 평가)
		candleSignals := evaluateBuySignals(ctx, settings, rules)
		candleSignals = append(candleSignals, evaluateTriggerSignals(ctx, settings, rules)...)
		for _, signal := range candleSignals {
			result.BuySignals = append(result.BuySignals, signal)
			if activeSellSettings != nil {
				pos := buildTradePositionFromSignal(ctx, signal)
				// 비용 모델 적용 (same-candle fill 기준)
				pos.FeeRate = settings.FeeRate
				pos.SlippageRate = settings.SlippageRate
				pos.BuyCost = pos.BuyPriceOrigin * (settings.FeeRate + settings.SlippageRate)
				ctx.AddActivePosition(pos)
			}
		}

		// per-candle 룰 평가 (evaluation: per_candle 룰은 매 캔들에서 평가)
		for _, signal := range evaluatePerCandleSignals(ctx, settings, rules) {
			result.BuySignals = append(result.BuySignals, signal)
			// per_candle 포지션은 항상 생성 (E1~E4 적용, activeSellSettings 불필요)
			pos := buildTradePositionFromSignalNextOpen(ctx, signal, candles)
			pos.FeeRate = settings.FeeRate
			pos.SlippageRate = settings.SlippageRate
			pos.BuyCost = pos.BuyPriceOrigin * (settings.FeeRate + settings.SlippageRate)
			pos.IsPerCandle = true
			ctx.AddActivePosition(pos)
		}

		// 4. Curvekey 변경 시 Exposition 업데이트
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// 5a. per_candle 포지션: E1~E4 청산 패스 (항상 활성)
		{
			var nextCandle *box.Candle
			if i+1 < len(candles) {
				nextCandle = candles[i+1]
			}
			for _, p := range ctx.ActivePositions {
				if !p.IsActive || !p.IsPerCandle {
					continue
				}
				cur := ctx.GetCurrentCandle()
				if shouldExit, reason, fillPrice, weight := Evaluate15mExit(p, cur, nextCandle, ctx.Position, settings); shouldExit {
					execute15mExit(ctx, p, reason, fillPrice, weight)
				}
			}
		}

		// 5b. 일봉 포지션: 기존 매도 룰 엔진 (IsPerCandle 포지션 제외)
		if activeSellSettings != nil {
			for _, p := range ctx.ActivePositions {
				if !p.IsActive || p.IsPerCandle {
					continue
				}
				decision := EvaluateSellSignals(ctx, p, *activeSellSettings)
				if decision.ShouldSell {
					ExecutePartialSell(ctx, p, decision.PrimaryReason, decision.SellWeight, *activeSellSettings)
				} else if decision.RequiresHoldingExtensionUpdate {
					if cond.CanExtendHoldingOnExpiry(ctx, p) {
						p.IsWaitingForSellSignalAfterExpiry = true
						p.PeriodExpiredAtPosition = ctx.Position
					} else {
						ExecutePartialSell(ctx, p, "PeriodExpiry", decision.SellWeight, *activeSellSettings)
					}
				}
			}
		}
	}

	result.BoxList = ctx.BoxList
	result.Positions = ctx.ActivePositions
	return result
}

// buildTradePositionFromSignal 은 매수 신호로부터 TradePosition을 생성한다 (same-candle fill, on_breakout용).
func buildTradePositionFromSignal(ctx *box.TradingContext, signal BuySignal) *box.TradePosition {
	cur := ctx.GetCurrentCandle()
	buyPrice, buyPriceOrigin, buyDate, buyTime := 0.0, 0.0, "", ""
	if cur != nil {
		buyPrice = cur.Close
		buyPriceOrigin = cur.CloseOrigin
		buyDate = cur.Date
		buyTime = cur.Time
	}
	p := buildPosition(ctx, signal, buyPrice, buyPriceOrigin, buyDate)
	p.BuyTime = buyTime
	return p
}

// buildTradePositionFromSignalNextOpen 은 per_candle 신호 체결을 다음 봉 시가로 설정한다 (look-ahead 방지).
func buildTradePositionFromSignalNextOpen(ctx *box.TradingContext, signal BuySignal, candles []*box.Candle) *box.TradePosition {
	pos := ctx.Position
	buyPrice, buyPriceOrigin, buyDate, buyTime := 0.0, 0.0, "", ""
	if cur := ctx.GetCurrentCandle(); cur != nil {
		buyPrice = cur.Close
		buyPriceOrigin = cur.CloseOrigin
		buyDate = cur.Date
		buyTime = cur.Time
	}
	if pos+1 < len(candles) {
		next := candles[pos+1]
		buyPrice = next.Open
		buyPriceOrigin = next.OpenOrigin
		buyDate = next.Date
		buyTime = next.Time
	}
	p := buildPosition(ctx, signal, buyPrice, buyPriceOrigin, buyDate)
	p.BuyTime = buyTime
	return p
}

func buildPosition(ctx *box.TradingContext, signal BuySignal, buyPrice, buyPriceOrigin float64, buyDate string) *box.TradePosition {
	tradeId := fmt.Sprintf("%s_%s_%s_%d", ctx.Shcode, buyDate, signal.Reason, signal.Position)
	p := box.NewTradePosition(tradeId, signal.Reason, signal.Position, buyPrice, buyPriceOrigin, buyDate)
	p.DefBoxIndex = ctx.DefboxIndex
	p.DefBoxPrice = ctx.DefboxPrice
	p.MainboxPosition = ctx.MainboxPosition
	if mainBox := ctx.GetMainBox(); mainBox != nil {
		p.MainBoxPrice = mainBox.Price
		p.MainBoxIndex = -1
	}
	// C# VirtualTradingBuy: 전략 발화 시점의 컨텍스트 값을 그대로 복사 (VirtualTrading.cs:238-239)
	p.PenPosition = ctx.PenPosition
	p.MomentumPosition = ctx.MomentumPosition
	return p
}

// evaluateBuySignals 는 한 캔들의 매수 평가 전체 (C# BFunction.BLogic 구조 정렬):
//  1. DefBox 변경 감지 → 상태 리셋
//  2. DamChecker==1 + 돌파(가격+거래량+ATR) → REST1 룰 + REST2 DetermineBuySignal (돌파 캔들에서만)
//  3. DamChecker==2 (돌파 이후 캔들) → ShortRange 사후 평가만
//  4. 후보군1 상태 → 추가 매수 기회
//  5. FollowUp 재진입 처리 (매 캔들)
func evaluateBuySignals(ctx *box.TradingContext, s Settings, rules []RuleConfig) []BuySignal {
	var out []BuySignal

	if ctx.DefChecker != 0 {
		if tempDefboxIdx := findLastDefBoxIndex(ctx.BoxList); tempDefboxIdx != -1 {
			if ctx.DefboxIndex != tempDefboxIdx {
				ctx.DefboxIndex = tempDefboxIdx
				ctx.DamChecker = 1
				ctx.UpdateBoxInfo()
				ctx.ResetBuySignalPositions()
				ctx.BuyHelper = "" // C# BFunction: DefBox 변경 시 BuyHelper 초기화
			}

			if ctx.DamChecker == 1 {
				if checkDefBoxBreakout(ctx, s) {
					ctx.DamChecker = 2
					// REST1 룰 평가 — C#과 동일하게 돌파 캔들에서만 수행
					if sig := checkBuyConditions(ctx, s, rules); sig.Triggered {
						out = append(out, sig)
					}
					// REST2 DetermineBuySignal (S13~S16) — EnableREST2로 제어
					if s.EnableREST2 {
						if sig := determineBuySignal(ctx, s); sig != nil {
							out = append(out, *sig)
						}
					}
				}
			} else if ctx.DamChecker == 2 && s.EnableREST2 {
				// 돌파 이후 캔들: ShortRange 사후 평가만 (C# ProcessPostBreakoutConditions)
				if sig := processPostBreakoutSignals(ctx); sig != nil {
					out = append(out, *sig)
				}
			}
		}
	}

	// 후보군1 상태에서 추가 매수 기회 (C# BLogic2 통합 — Bsig는 캔들 간 유지됨)
	if s.EnableREST2 && !ctx.BuyOn && ctx.Bsig == "후보군1" {
		if sig := processAdditionalBuySignals(ctx); sig != nil {
			out = append(out, *sig)
		}
	}

	// FollowUp 재진입 처리 (C#: BLogic 끝에서 항상 호출) — EnableREST2로 제어
	if s.EnableREST2 {
		out = append(out, processFollowUpBuyDecisions(ctx)...)
	}

	return out
}

// checkDefBoxBreakout 는 DefBox 돌파 인정 게이트.
// C# BFunction.CheckDefBoxBreakout 포팅: 가격 돌파 + 거래대금 + ATR 세 조건 모두 충족해야
// DamChecker가 2로 전환된다 (실패 시 1 유지 → 다음 캔들 재시도 가능).
func checkDefBoxBreakout(ctx *box.TradingContext, s Settings) bool {
	if !cond.IsDefBoxBreakout(ctx) {
		return false
	}
	cur := ctx.GetCurrentCandle()
	if cur == nil {
		return false
	}
	if !cond.IsVolumeBreakout(cur, s.VolumeLimit) {
		return false
	}
	if s.ATREntryFilterEnabled && cur.ATRPercentage > s.ATREntryMaxThreshold {
		return false
	}
	return true
}

// buildTriggeredSignal 은 매수 신호가 확정됐을 때 BuySignal 필드를 채워서 반환
func buildTriggeredSignal(ctx *box.TradingContext, reason string) BuySignal {
	sig := BuySignal{
		Triggered: true,
		Reason:    reason,
		Position:  ctx.Position,
	}

	if cur := ctx.GetCurrentCandle(); cur != nil {
		sig.Date = cur.Date
	}

	if defBox := ctx.GetDefBox(); defBox != nil {
		sig.DefboxPrice = defBox.PriceOrigin
	}

	if mainBox := ctx.GetMainBox(); mainBox != nil {
		sig.MainboxPrice = mainBox.PriceOrigin
		if mainBox.BoxPosition >= 0 && mainBox.BoxPosition < len(ctx.CandleList) {
			sig.MainboxDate = ctx.CandleList[mainBox.BoxPosition].Date
		}
	}

	return sig
}

func checkBuyConditions(ctx *box.TradingContext, s Settings, rules []RuleConfig) BuySignal {
	// 룰 엔진 전략이 로드된 경우 우선 적용
	if len(rules) > 0 {
		signal, stratName := EvaluateRules(rules, ctx, s)
		if signal != "" {
			// C# 전략 발화 시 상태 기록: PenPosition + SetBuySignal(즉시매수, 전략명, 신호명)
			ctx.PenPosition = ctx.Position
			setBuySignalState(ctx, "즉시매수", stratName, signal)
			return finalizeBuySignal(ctx, stratName)
		}
		return BuySignal{}
	}

	// 룰 파일 없을 때 기존 하드코딩 로직 fallback
	// fallback도 룰 엔진과 동일하게 DefBox당 1회만 발화 (중복 신호 방지)
	const fallbackName = "DefBoxBreakout"
	if _, fired := ctx.LastBuySignalPosition[fallbackName]; fired {
		return BuySignal{}
	}

	if !cond.IsCloseNearDefboxPrice(ctx, s.DefBoxNearPriceThreshold, s.MainBoxNearPriceThreshold) {
		return BuySignal{}
	}
	if !cond.IsMainboxDistanceTwiceOrMore(ctx) {
		return BuySignal{}
	}
	if !cond.IsBoxDensityValidByCount(ctx) {
		return BuySignal{}
	}
	if !cond.IsSingleBreakout(ctx) {
		return BuySignal{}
	}
	if !cond.IsBoxConditionValid2(ctx) {
		return BuySignal{}
	}
	if cond.HasExcessiveUpperWick(ctx, s.DefBoxUpperWickToBodyRatioThreshold) {
		return BuySignal{}
	}
	ctx.LastBuySignalPosition[fallbackName] = ctx.Position
	ctx.PenPosition = ctx.Position
	setBuySignalState(ctx, "즉시매수", fallbackName, "즉시매수")
	return finalizeBuySignal(ctx, fallbackName)
}

func findLastDefBoxIndex(boxList []*box.Box) int {
	for i := len(boxList) - 1; i >= 0; i-- {
		if boxList[i].KindOfBox == box.KindDefBox {
			return i
		}
	}
	return -1
}

// evaluateTriggerSignals 는 trigger: 필드가 있는 룰을 평가한다.
// 매 캔들에서 각 룰의 트리거(메인이벤트, edge)를 먼저 확인하고,
// 트리거가 발화한 캔들에서만 when/when_not/any_of 조건을 평가한다.
// 같은 트리거를 쓰는 룰이 여러 개면 트리거는 캔들당 1회만 계산한다 (메모이즈).
// 발화 신호의 체결은 on_breakout과 동일한 same-candle fill.
//
// 기존 on_breakout(DamChecker 상태머신)과의 차이: DefBox당 1회 게이트가 아니라
// 트리거 edge마다 조건을 평가한다. 중복 발화는 once_per로 제어한다 (기본 defbox).
// REST2/FollowUp 상태(BuyHelper, Bsig 등)는 건드리지 않는다.
func evaluateTriggerSignals(ctx *box.TradingContext, s Settings, rules []RuleConfig) []BuySignal {
	if len(rules) == 0 {
		return nil
	}
	var signals []BuySignal
	triggerFired := map[string]bool{}   // 캔들 내 트리거 메모이즈
	triggerMatched := map[string]bool{} // 같은 트리거 그룹 내 첫 매칭 승리 (YAML 순서 = 우선순위)

	for _, rule := range rules {
		if rule.Trigger == "" {
			continue
		}
		// 같은 트리거를 쓰는 룰 중 이번 캔들에서 이미 매칭된 것이 있으면 스킵
		// (on_breakout 경로의 "첫 매칭 승리"와 동일 철학 — 엄격한 룰을 위에 배치)
		if triggerMatched[rule.Trigger] {
			continue
		}

		// 중복 발화 방지 (once_per)
		switch rule.OncePer {
		case "", "defbox":
			// DefBox 구간당 1회 — DefBox 변경 시 evaluateBuySignals의 ResetBuySignalPositions()로 리셋
			if _, fired := ctx.LastBuySignalPosition[rule.Name]; fired {
				continue
			}
		case "cooldown":
			if lastPos, fired := ctx.LastPerCandleSignalPosition[rule.Name]; fired {
				if ctx.Position-lastPos < s.PerCandleCooldownBars {
					continue
				}
			}
		case "none":
			// 제한 없음
		}

		// DefCount 필터 (on_breakout과 동일)
		if rule.DefCount > 0 && ctx.DefCount != rule.DefCount {
			continue
		}
		if rule.DefCountMin > 0 && ctx.DefCount < rule.DefCountMin {
			continue
		}

		// 트리거(메인이벤트) 확인 — 캔들당 1회 계산
		fired, seen := triggerFired[rule.Trigger]
		if !seen {
			fn, ok := triggerRegistry[rule.Trigger]
			if !ok {
				fmt.Printf("[rule] 미등록 트리거: %s\n", rule.Trigger)
				triggerFired[rule.Trigger] = false
				continue
			}
			fired = fn(ctx, s)
			triggerFired[rule.Trigger] = fired
		}
		if !fired {
			continue
		}

		// 트리거 발화 캔들에서만 세부 조건 평가
		sig, stratName := evaluateSingleRule(rule, ctx, s)
		if sig == "" {
			continue
		}

		switch rule.OncePer {
		case "", "defbox":
			ctx.LastBuySignalPosition[stratName] = ctx.Position
		case "cooldown":
			ctx.LastPerCandleSignalPosition[stratName] = ctx.Position
		}
		triggerMatched[rule.Trigger] = true

		out := buildTriggeredSignal(ctx, stratName)
		out.Helper = "trigger:" + rule.Trigger + ":" + sig
		signals = append(signals, out)
	}
	return signals
}

// evaluatePerCandleSignals 는 evaluation: per_candle 로 표시된 룰을 매 캔들에서 평가한다.
// 쿨다운: 동일 룰은 PerCandleCooldownBars 봉 내 재발화 불가.
// 포지션 중복: 같은 전략명의 활성 포지션이 있으면 스킵.
func evaluatePerCandleSignals(ctx *box.TradingContext, s Settings, rules []RuleConfig) []BuySignal {
	if len(rules) == 0 {
		return nil
	}
	var signals []BuySignal
	for _, rule := range rules {
		if rule.Evaluation != "per_candle" || rule.Trigger != "" {
			continue
		}
		// 쿨다운 체크
		if lastPos, fired := ctx.LastPerCandleSignalPosition[rule.Name]; fired {
			if ctx.Position-lastPos < s.PerCandleCooldownBars {
				continue
			}
		}
		// 같은 전략명의 활성 포지션이 있으면 스킵
		hasActivePos := false
		for _, p := range ctx.ActivePositions {
			if p.IsActive && p.StrategyName == rule.Name {
				hasActivePos = true
				break
			}
		}
		if hasActivePos {
			continue
		}

		sig, stratName := evaluateSingleRule(rule, ctx, s)
		if sig != "" {
			ctx.LastPerCandleSignalPosition[stratName] = ctx.Position
			cur := ctx.GetCurrentCandle()
			date := ""
			if cur != nil {
				date = cur.Date
			}
			// Reason = 전략명 (TradePosition.StrategyName으로 저장되어 활성 포지션 체크에 사용)
			// Helper = 실제 신호값 + "per_candle" 마커
			signals = append(signals, BuySignal{
				Triggered: true,
				Reason:    stratName,
				Position:  ctx.Position,
				Date:      date,
				Helper:    "per_candle:" + sig,
			})
		}
	}
	return signals
}
