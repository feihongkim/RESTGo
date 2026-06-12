package stg

import (
	"fmt"

	"RESTGo/box"
	"RESTGo/cond"
)

// activeRules 는 LoadRules()로 초기화되는 활성 전략 목록
var activeRules []RuleConfig

// activeSellSettings 는 LoadSellStrategy()로 초기화되는 매도 설정 (없으면 비활성)
var activeSellSettings *SellSettings

// LoadStrategy 는 YAML 파일에서 전략 룰을 로드
func LoadStrategy(path string) error {
	rules, err := LoadRules(path)
	if err != nil {
		return fmt.Errorf("전략 로드 실패 (%s): %w", path, err)
	}
	activeRules = rules
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

// Analyze 는 캔들 리스트에 대해 Box/DefBox 분석을 수행하고 매수 신호를 반환
func Analyze(candles []*box.Candle, settings Settings) AnalysisResult {
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
		for _, signal := range evaluateBuySignals(ctx, settings) {
			result.BuySignals = append(result.BuySignals, signal)
			if activeSellSettings != nil {
				ctx.AddActivePosition(buildTradePositionFromSignal(ctx, signal))
			}
		}

		// 4. Curvekey 변경 시 Exposition 업데이트
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		// 5. 매도 평가 (활성 포지션 순회)
		if activeSellSettings != nil {
			for _, p := range ctx.ActivePositions {
				if !p.IsActive {
					continue
				}
				decision := EvaluateSellSignals(ctx, p, *activeSellSettings)
				if decision.ShouldSell {
					ExecutePartialSell(ctx, p, decision.PrimaryReason, decision.SellWeight, *activeSellSettings)
				} else if decision.RequiresHoldingExtensionUpdate {
					// Path 4: Period Expiry 후 홀딩 연장 평가
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

// buildTradePositionFromSignal 은 매수 신호로부터 TradePosition을 생성한다.
func buildTradePositionFromSignal(ctx *box.TradingContext, signal BuySignal) *box.TradePosition {
	cur := ctx.GetCurrentCandle()
	buyPrice := 0.0
	buyPriceOrigin := 0.0
	buyDate := ""
	if cur != nil {
		buyPrice = cur.Close
		buyPriceOrigin = cur.CloseOrigin
		buyDate = cur.Date
	}
	tradeId := fmt.Sprintf("%s_%s_%s_%d", ctx.Shcode, buyDate, signal.Reason, signal.Position)
	p := box.NewTradePosition(tradeId, signal.Reason, signal.Position, buyPrice, buyPriceOrigin, buyDate)
	p.DefBoxIndex = ctx.DefboxIndex
	p.DefBoxPrice = ctx.DefboxPrice
	p.MainboxPosition = ctx.MainboxPosition
	if mainBox := ctx.GetMainBox(); mainBox != nil {
		p.MainBoxPrice = mainBox.Price
		p.MainBoxIndex = -1 // BoxList 인덱스 별도 추적 필요 시 채움
	}
	// C# VirtualTradingBuy: 전략 발화 시점의 컨텍스트 값을 그대로 복사 (VirtualTrading.cs:238-239)
	// 주의: PenPosition은 DefBox 형성 위치가 아니라 "전략 발화 캔들" — 매도 측
	// IsTrendEntryFailure2WithPosition의 오실로 스캔 시작점으로 사용되므로 의미가 중요
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
func evaluateBuySignals(ctx *box.TradingContext, s Settings) []BuySignal {
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
					if sig := checkBuyConditions(ctx, s); sig.Triggered {
						out = append(out, sig)
					}
					// REST2 DetermineBuySignal (S13~S16)
					if sig := determineBuySignal(ctx, s); sig != nil {
						out = append(out, *sig)
					}
				}
			} else if ctx.DamChecker == 2 {
				// 돌파 이후 캔들: ShortRange 사후 평가만 (C# ProcessPostBreakoutConditions)
				if sig := processPostBreakoutSignals(ctx); sig != nil {
					out = append(out, *sig)
				}
			}
		}
	}

	// 후보군1 상태에서 추가 매수 기회 (C# BLogic2 통합 — Bsig는 캔들 간 유지됨)
	if !ctx.BuyOn && ctx.Bsig == "후보군1" {
		if sig := processAdditionalBuySignals(ctx); sig != nil {
			out = append(out, *sig)
		}
	}

	// FollowUp 재진입 처리 (C#: BLogic 끝에서 항상 호출)
	out = append(out, processFollowUpBuyDecisions(ctx)...)

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

func checkBuyConditions(ctx *box.TradingContext, s Settings) BuySignal {
	// 룰 엔진 전략이 로드된 경우 우선 적용
	if len(activeRules) > 0 {
		signal, stratName := EvaluateRules(activeRules, ctx, s)
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
