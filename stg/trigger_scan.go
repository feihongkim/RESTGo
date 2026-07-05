package stg

// trigger_scan.go — 범용 트리거 스캔 코어 (2026-07-05, 구조 개선 ②).
//
// 임의의 트리거(일반 edge 또는 armed 2단계) × when/when_not 조건 조합을 코드 없이 측정하기 위한
// 단일 종목 스캔 루프. YAML 엔진과 동일한 캔들 처리 순서(CheckAndCreateDefBox → AnalyzeCurvature →
// 평가)를 따르므로, 여기서 측정한 조합을 YAML로 전략화하면 같은 신호가 난다 (engine-parity).
// ※ RunArmedTrigger는 전용 분석기 재현용(analyzer-parity, DefBox 순서가 다름) — 목적이 다르다.

import "RESTGo/box"

// TriggerScanConfig 는 ScanTrigger 설정.
type TriggerScanConfig struct {
	Trigger      string   // 트리거명 (일반 또는 armed)
	When         []string // AND 조건 (conditionRegistry 등록명)
	WhenNot      []string // NOT 조건
	CooldownBars int      // 발화 후 재발화 금지 봉 수 (0 = 없음)
}

// ScanTrigger 는 캔들 배열에서 트리거×조건 조합의 발화 위치를 반환한다.
// 미등록 트리거/조건이 있으면 nil과 함께 이름을 반환한다 (조용한 실패 방지).
func ScanTrigger(candles []*box.Candle, cfg TriggerScanConfig, s Settings) (fires []int, unknown string) {
	_, isNormal := triggerRegistry[cfg.Trigger]
	_, isArmed := armedTriggerRegistry[cfg.Trigger]
	if !isNormal && !isArmed {
		return nil, "trigger:" + cfg.Trigger
	}
	for _, n := range append(append([]string{}, cfg.When...), cfg.WhenNot...) {
		if _, ok := conditionRegistry[n]; !ok {
			return nil, "condition:" + n
		}
	}
	if len(candles) < 60 {
		return nil, ""
	}

	ctx := box.NewTradingContext(candles, []*box.Box{})
	if len(candles) > 0 {
		ctx.Shcode = candles[0].Shcode
	}

	lastFire := -1 << 30
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

		// 엔진 순서: DefBox 생성 → 곡률 분석
		box.CheckAndCreateDefBox(ctx, s.DamOption)
		candles[i].Curvekey = box.AnalyzeCurvature(ctx)

		// 트리거 평가 (armed는 매 캔들 틱 필수 — 쿨다운 중에도 상태는 갱신)
		var fired bool
		if isArmed {
			fired = tickArmedTrigger(cfg.Trigger, ctx, s)
		} else {
			fired = triggerRegistry[cfg.Trigger](ctx, s)
		}

		// Exposition 갱신 (엔진과 동일 위치)
		if candles[i-1].Curvekey != candles[i].Curvekey && len(ctx.BoxList) >= 1 {
			ctx.Exposition = box.CalculateExposition(ctx.BoxList[len(ctx.BoxList)-1])
		}

		if !fired {
			continue
		}
		if cfg.CooldownBars > 0 && i-lastFire < cfg.CooldownBars {
			continue
		}
		ok := true
		for _, n := range cfg.When {
			if !conditionRegistry[n](ctx, s) {
				ok = false
				break
			}
		}
		if ok {
			for _, n := range cfg.WhenNot {
				if conditionRegistry[n](ctx, s) {
					ok = false
					break
				}
			}
		}
		if !ok {
			continue
		}
		fires = append(fires, i)
		lastFire = i
	}
	return fires, ""
}
