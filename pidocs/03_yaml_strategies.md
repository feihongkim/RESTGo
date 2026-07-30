# RESTGo YAML 전략 명세서

> 작성자: Claude Code AI (pi coding agent)
> 최종 수정: 2026-07-17
> 대상: `rules/` 디렉토리 전체 YAML 전략 파일 분석

---

## 1. 개요

RESTGo는 매수·매도 전략을 **YAML 파일**로 정의한다. 매수 측은 3가지 평가 경로(on_breakout / trigger / per_candle) + Armed Trigger(2단계 장전→발화)가 있고, 매도 측은 5-Path 의사결정 엔진으로 동작한다.

### 평가 경로 (매수)

| 경로 | 필드 | 설명 | 사용 전략 |
|------|------|------|-----------|
| **on_breakout** | (trigger 미지정, evaluation 미지정) | C# DamChecker 상태머신 — DefBox 돌파 캔들 1회만 룰 평가 | `strategy1.yaml` |
| **trigger** | `trigger: <등록명>` | triggerRegistry edge 이벤트 발생 캔들에서만 평가, `once_per`로 중복 제어 | `buy_indicator.yaml`, `buy_bb_*.yaml`, `buy_trigger_example.yaml` 등 |
| **per_candle** | `evaluation: per_candle` | 매 캔들 평가 + 쿨다운 (가상자산 15분봉 전용) | `buy_crypto_15m.yaml` |
| **armed trigger** | `trigger: <Armed등록명>` | armedTriggerRegistry — 패턴 완성(장전)→확인 이벤트(발화) 2단계 | YAML `trigger:`에서 일반 트리거와 동일하게 사용 |

> **on_breakout vs trigger**: on_breakout은 C# 정합이므로 strategy1.yaml에서 계속 유지한다. trigger는 stateless 설계로, 박스 아래로 되밀린 후 재돌파하면 edge가 다시 발생해 아직 발화 안 한 룰이 재평가된다.

### once_per 중복 제어

| 값 | 설명 |
|----|------|
| `defbox` (기본) | 같은 DefBox 구간에서 1회 발화 |
| `cooldown` | `PerCandleCooldownBars` 봉 쿨다운 |
| `none` | 제한 없음 (edge마다 발화) |

---

## 2. 실행 흐름 요약

```
1. stg.LoadStrategy(path)              → YAML 파싱 → activeRules + activeSettings
2. stg.LoadSellStrategyFile(path)      → YAML 파싱 → activeSellRules + SellSettings
3. box.FetchCandles()                  → DB에서 캔들 조회
4. indicator.PrepareCandles()          → 전 지표 일괄 계산 (MA/ATR/BB/RSI/ADX/...)
5. stg.Analyze() / analyzeInternal()   → 메인 루프 진입
   ├─ box.CheckAndCreateDefBox()       → DefBox 생성 조건 평가
   ├─ box.AnalyzeCurvature()           → 곡률 분석, Box 생성
   ├─ evaluateBuySignals()             → on_breakout 경로 (strategy1 전용)
   │  ├─ checkDefBoxBreakout()         → 가격+거래대금+ATR 게이트
   │  ├─ EvaluateRules()               → 첫 매칭 승리
   │  ├─ determineBuySignal()          → REST2 S13~S16
   │  ├─ processPostBreakoutSignals()  → ShortRange (S19)
   │  └─ processFollowUpBuyDecisions() → S17~S20 재진입
   ├─ evaluateTriggerSignals()         → trigger 경로
   │  ├─ triggerFn(ctx, s)             → edge 확인 (triggerRegistry 15종)
   │  ├─ once_per 제어                 → defbox/cooldown/none
   │  └─ evaluateSingleRule()          → when/when_not/any_of 평가
   ├─ evaluatePerCandleSignals()       → per_candle 경로 (buy_crypto_15m)
   ├─ evaluateArmedTriggers()          → Armed Trigger 틱 (armedTriggerRegistry 8종)
   │  ├─ 장전 조건 확인 → ArmedTriggerState 저장
   │  └─ 유효기간 내 발화 조건 → trigger처럼 룰 평가
   └─ EvaluateSellSignals()            → 매도 5-Path 결정
      ├─ evaluateRuleConditions() × N  → 조건 평가
      ├─ TrackAndCheck()               → count_min/ratio_min 임계
      ├─ evaluateRecovery()            → 회복 가능성
      └─ makeSellDecision()            → 5-Path 우선순위 결정
         ├─ Path 1 Critical
         ├─ Path 2 Composite
         ├─ Path 3 Extension
         ├─ Path 4 Expiry
         └─ Path 5 Individual (Priority 순)
```

---

## 3. 룰 평가 문법

```yaml
strategies:
  - name: "전략명"
    # ── 범위 필터 ──
    def_count: 1           # DefCount 정확 일치 (0=무관)
    def_count_min: 2       # DefCount 최솟값 (0=무관)

    # ── 평가 경로 (택1) ──
    # ① 생략 → on_breakout (C# DamChecker 상태머신, strategy1 전용)
    # ② trigger: <등록명> → edge 발화 캔들에서 평가 (triggerRegistry 15종)
    # ③ trigger: <Armed등록명> → 2단계 장전→발화 (armedTriggerRegistry 8종)
    # ④ evaluation: per_candle → 매 캔들 평가 + 쿨다운

    trigger: DefBoxBreakout         # 트리거 경로 (edge 또는 armed)
    once_per: defbox                # (기본) defbox | cooldown | none

    # ── 조건 ──
    when: [...]                     # AND 조건: 모두 true
    when_not: [...]                 # NOT 조건: 모두 false
    any_of: [...]                   # OR 조건: 하나 이상 true

    # ── 결과 ──
    signal: "신호명"                # 미지정 시 전략명 사용
```

---

## 4. 파일 인덱스 (65개)

### 4.1 매수 전략 (15개)

| 파일 | 평가 경로 | 내용 |
|------|-----------|------|
| `strategy1.yaml` | on_breakout | **기본 전략.** C# Stock1 REST1 포팅 — Box 구조 8룰 + Core-3 게이트 |
| `strategy1_gc.yaml` | on_breakout | 기본 전략 + Golden Cross 확증 변형 |
| `strategy1_s03s23.yaml` | on_breakout | S03+S23 교집합 조합 전략 |
| `buy_indicator.yaml` | trigger | (구 strategy2) DefBox 돌파 순간 지표(RSI/BB/MA) 확증 6룰 (I01~I06) |
| `buy_bb_pure.yaml` | trigger | (구 strategy_bb_pure) Bollinger 3대 방법 — MIIIb/MIII → MII → MI |
| `buy_bb_hybrid.yaml` | trigger | (구 strategy_bb_hybrid) Box 구조 + BB 복합 4룰 (SH1~SH4) |
| `buy_trigger_example.yaml` | trigger | 트리거 문법 예시 3룰 |
| `buy_crypto_15m.yaml` | per_candle | (구 strategy3) 가상자산 15분봉 다중 트리거 OR (보류 영역) |
| `buy_wdefbox.yaml` | trigger | W-DefBox 결합 매수 전략 |
| `buy_wdefbox_gc.yaml` | trigger | W-DefBox + Golden Cross 확증 |
| `buy_stg11_15m.yaml` | trigger | STG11 돌파후행 15분봉 전략 |

### 4.2 매도 전략 (5개)

| 파일 | 내용 |
|------|------|
| `sell_default.yaml` | (구 sell_strategy1) **기본 매도.** 21룰 + 5-Path + tracking 임계 + composite |
| `sell_positive_only.yaml` | (구 sell_strategy1_positive_only) 양수 수익 구간만 매도 |
| `sell_positive_only_mh25.yaml` | (구 sell_strategy1_posOnly_mh25) 위 + max_holding=25 |
| `sell_s03s23.yaml` | S03+S23 전략 전용 매도 |
| `sell_wdefbox.yaml` | W-DefBox 전략 전용 매도 |

### 4.3 그리드 서치 (4개)

| 파일 | 내용 |
|------|------|
| `grid_crypto_example.yaml` | (구 grid_example) 그리드 정의 예시 |
| `grid_crypto_stage2.yaml` | (구 grid_stage2) Stage 2 플래토 그리드 |
| `grid_crypto_w10b.yaml` | (구 grid_w10b) W10-B 청산 파라미터 그리드 |
| `grid_stg11.yaml` | STG11 돌파후행 그리드 서치 |

### 4.4 오버레이 (1개)

| 파일 | 내용 |
|------|------|
| `overlay_wdefbox.yaml` | W중력 오버레이 밀도 게이트 (han DB StrategySignalDaily) |

---

## 5. 등록된 트리거 (triggerRegistry — 15종)

`stg/trigger_registry.go`의 `init()`에서 등록. 트리거는 반드시 edge 형태(돌파/이탈 발생 순간에만 true)여야 하며, 상태가 지속되는 level 조건과 분리되어 있다.

| 트리거명 | 설명 |
|----------|------|
| `DefBoxBreakout` | 가격+거래대금+ATR 3중 게이트 통과 (stateless, on_breakout과 동일 게이트) |
| `PriceBreakout` | 순수 가격 돌파만 (거래대금·ATR when으로 분리) |
| `WBottomBox` | W패턴 S-R-S 완성 순간 |
| `BBLowerBreakdown` | 종가가 BB 하단 밴드 하향 이탈 |
| `BBLowerReentry` | BB 하단 밴드 아래에서 안으로 복귀 |
| `BBSqueezeBreakout` | %B 상향 돌파 + 최근 BBWidth<4% 스퀴즈 존재 |
| `Stg6PullbackTouch` | STG6 Pullback 터치 시점 |
| `Stg11MA60Breakdown` | STG11 MA60 하향 이탈 |
| `Stg11MA60FirstTouch` | STG11 MA60 최초 터치 |
| `Stg9ApexPerch` | STG9 Apex Perch 패턴 |
| `Stg7GCAccel` | STG7 Golden Cross 가속 시점 |
| `Stg14Oversold` | STG14 과매도 조건 |
| `DefBoxApproach` | DefBox 가격 접근 감지 |

---

## 6. 등록된 Armed 트리거 (armedTriggerRegistry — 8종)

`stg/armed_trigger_registry.go`의 `init()`에서 등록. 상태를 갖는 2단계 패턴(패턴 완성=장전 → 유효기간 내 확인 이벤트=발화)을 정의한다. YAML `trigger:`에서 일반 트리거와 동일하게 사용 가능.

| 트리거명 | 장전 조건 | 발화 조건 | 설명 |
|----------|-----------|-----------|------|
| `MTopCollapse` | MTop 패턴 완성 | 붕괴 확인 | MTop 패턴 + 붕괴 |
| `HNSNecklineBreak` | HNS 패턴 완성 | Neckline 돌파 | 헤드앤숄더 + 목선 |
| `DefBoxBreakoutFailure` | DefBox 돌파 실패 | 하락 확인 | 돌파 실패 패턴 |
| `DoubleBumpRetest` | DoubleBump 패턴 | 재시험 확인 | 이중 충돌 재시험 |
| `DoubleBumpBreakout2` | DoubleBump 패턴 | 2차 돌파 | 이중 충돌 2차 |
| `MA20PullbackBreakout` | MA20 Pullback | 돌파 확인 | MA20 풀백 돌파 |
| `Stg2Inverted120Retreat` | Stg2 역120 후퇴 패턴 | 확인 | 역120 후퇴 |
| `DefBoxBreakoutSurvival` | DefBox 돌파 | 생존 확인 | 돌파 생존 |

> Armed Trigger의 장전 상태는 `ctx.ArmedTriggerState`에 저장되며, 룰 필터와 무관하게 매 캔들 틱된다.

---

## 7. 등록된 조건 함수 (conditionRegistry — 81종)

`stg/buy_conditions_registry.go`의 `init()`에서 등록. 모든 YAML 룰의 `when`/`when_not`/`any_of`는 이 레지스트리에 등록된 이름만 사용 가능.

### 7.1 Box 구조 (13개)
IsDefBoxBreakout, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsMainboxDistanceTwiceOrMore, IsSingleBreakout, IsBoxConditionValid, IsBoxConditionValid2, IsBoxCountBetween2, IsBoxCountBetween5, IsBoxDensityValidByCount, IsBoxDensityValidByDistribution, HasExcessiveUpperWick, MultiDefDamCountMax2

### 7.2 캔들 패턴 (2개)
IsBullishCandle, HasPullbackOrCorrection

### 7.3 이동평균선 (8개)
IsMa20NearMa60Complex, IsMa20NearMa60Simple, IsMa60StrongerThanMa120By2Percent, IsMainboxPriceAboveMa60OrMa120, HasLowTouchedMa20, IsMainboxConditionValid, MainBoxPositionBasedTiming, MainBoxPositionBasedTimingLess

### 7.4 RSI 조건 (5개)
IsRSIOversold, IsRSIOverbought, IsRSIRecoveringFromOversold, IsRSIRising, IsRSIInBullZone

### 7.5 Bollinger 조건 (10개)
IsBBLowerTouch, IsBBReboundFromLower, IsBBSqueezeBreakout, IsBBUpperBreakout, IsAboveBBMiddle, IsBBSqueezeHistorical, IsBBWalkingUp, IsBBWBottomPattern, IsBBWBottomBoxPattern, HasDefBoxBeforeWPattern

### 7.6 MA 교차 조건 (9개)
IsMaGoldenCross5x20, IsMaGoldenCross20x60, IsMaProperArrangement, IsAllMaRising, IsMaConvergence, IsPriceAboveAllMa, IsMaDeadCross5x20, IsMaInverseArrangement, IsPriceBelowAllMa

### 7.7 15분봉 P0 조건 (20개)
IsMACDGoldenCross, IsMACDHistogramRising, IsStochGoldenCross, IsADXTrending, IsDIBullish, IsDIBearish, IsAboveVWAP, IsBelowVWAP, IsVWAPDeviation, IsVWAPReclaim, IsVolumeZScoreSpike, IsOBVRising, IsSuperTrendBullish, IsSuperTrendBearish, IsDonchianBreakout, IsDonchianBreakdown, IsKeltnerBreakout, IsNarrowRange, IsRSIFallingFromOverbought, IsBBUpperReject

### 7.8 EMA 조건 (5개)
IsEMABullArrangement, IsEMA9Above21, IsEMA21PullbackBounce, IsPriceAboveEMA50, IsVWAPDeviationBelow

### 7.9 관통/기타 (3개)
IsPenetrationOptionValid, IsMultiDefRelaxedDamCondition, IsATREntryValid

### 7.10 신규 전략 조건 (6개)
IsGoldenCrossPending, HasMTopStructure, IsStg11MA60Breakdown, IsS1SetupTerrain, IsS1EntryQuality, HasDefBoxOverhead

> **총 81종의 조건 함수**가 등록되어 있다.

---

## 8. 등록된 매도 조건 함수 (sellConditionRegistry — 24종)

`stg/sell_conditions_registry.go`의 `init()`에서 등록.

### Critical (1개)
IsCriticalFailure

### Profit Taking (2개)
IsGapUpTakeProfit, IsBBUpperBreakoutProfit

### Loss Cutting (10개)
IsMainBoxBreakdownFailure, IsMainBoxPersistentBreakdown, IsMainBoxRecoveryFailure, IsMainBoxBBBreakdown, IsWeakFoundationFailure, IsTrendEntryFailure1, IsTrendEntryFailure2, IsWithin10Days, IsStopLoss, IsAdaptiveStopLoss

### Time-Delayed (2개)
IsTimeDelayedStopLoss, IsTimeDelayedStopLossEnabled

### Early Warning (3개)
IsEarlyDrop, IsEarlyMainBoxBreak, IsBBSqueezeExpansionWarning

### Technical (3개)
IsMA5MA20DeadCross, IsConsecutiveNegativeCandles, IsMAReversalBoxPattern

### Extension/Expiry (3개)
IsExtensionActive, IsMA5BreakdownDuringExtension, IsPeriodExpired

> **총 24종 매도 조건 함수 등록**

---

## 9. 매수 전략 상세

### 9.1 strategy1.yaml — Box 구조 기반 (on_breakout)

**평가 경로**: `on_breakout` (trigger 미지정) — C# DamChecker 상태머신
**사용**: `./RESTGo stock analyze <종목코드>` (기본)

#### 설정 오버라이드
```yaml
settings:
  DefBoxUpperWickToBodyRatioThreshold: 2.0
```

#### Core-3 공통 게이트
모든 SingleDef/MultiDef 전략은 아래 3개 조건을 공통으로 통과해야 함 (C# `HasCoreCommonConditions`):
1. `IsBullishCandle` — 양봉 여부
2. `HasPullbackOrCorrection` — 풀백/조정 패턴
3. `IsMa20NearMa60Complex` — MA20-MA60 근접 (복합 검증)

#### SingleDef 전략 (def_count: 1, 평가 순서: 엄격 → 완화)

**① S03_SingleDefStrictBuy_Option2 — 가장 엄격**
| 구분 | 조건 |
|------|------|
| when (14개) | Core-3 + IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsMainboxPriceAboveMa60OrMa120, IsMainboxConditionValid, IsBoxDensityValidByDistribution, MainBoxPositionBasedTiming, MainBoxPositionBasedTimingLess, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

**② S01_SingleDefBuy — 표준 SingleDef**
- S03에서 MainBoxPositionBasedTimingLess 제거
- signal: `즉시매수`

**③ S23_Intersection_4n8 — 4∩8 교집합 (완화)**
- when (9개): Core-3 + IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsPenetrationOptionValid
- signal: `즉시매수`

**④ S08_SingleDefRelaxedDistanceBuy_Option3 — 거리 완화**
- when (9개): Core-3 + IsCloseNearDefboxPrice, IsMainboxDistanceTwiceOrMore, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween5, IsPenetrationOptionValid
- signal: `즉시매수`

**⑤ S04_SingleDefWeakFoundationBuy_Option2 — 연약지반 (최완화)**
- when (7개): Core-3 + IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsBoxCountBetween2, IsPenetrationOptionValid
- signal: `연약지반매수`

#### MultiDef 전략 (def_count_min: 2)

| 룰 | signal | 설명 |
|-----|--------|------|
| S05_MultiDefStandardBuy_Option2 | `MD즉시매수` | when (8개): Core-3 + CloseNear, MA60>MA120×2%, BoxDensity, DamCountMax2, Penetration |
| S06_MultiDefRelaxedBuy_Option2 | `multidef매수대기` | when (6개): Core-3 + CloseNear, MultiDefRelaxedDam, Penetration |
| S10_MultiDefStandardBuy_Option3 | `MD즉시매수` | S05에서 Penetration 제거 |

---

### 9.2 buy_indicator.yaml — 지표 기반 (trigger)

**평가 경로**: `trigger: DefBoxBreakout` + `once_per: defbox`
**사용**: `RESTGO_BUY_RULES=rules/buy_indicator.yaml ./RESTGo stock analyze <종목코드> 250`

| 룰 | signal | 핵심 조건 |
|----|--------|-----------|
| I01_TrendConfluenceBuy | `추세확증매수` | PriceAboveAllMa + MaProperArrangement + AllMaRising + AboveBBMiddle + RSIInBullZone |
| I02_SqueezeBreakoutBuy | `스퀴즈돌파매수` | IsBBSqueezeBreakout + IsPriceAboveAllMa |
| I03_ConvergenceBreakoutBuy | `수렴돌파매수` | IsMaConvergence + IsPriceAboveAllMa + IsRSIRising |
| I04_GoldenCrossBuy | `골든크로스매수` | IsMaGoldenCross5x20 + IsRSIRising |
| I05_OversoldReboundBuy | `과매도반등매수` | IsRSIRecoveringFromOversold + IsBBReboundFromLower |
| I06_BandBreakoutBuy | `밴드돌파매수` | IsBBUpperBreakout + IsAllMaRising |

---

### 9.3 buy_bb_pure.yaml — Bollinger 3대 방법 (trigger)

**평가 경로**: `trigger: DefBoxBreakout` + `once_per: defbox`
**평가 순서**: Method III(가장 선택적) → Method II → Method I

| 룰 | signal | 설명 |
|----|--------|------|
| MIIIb_WBottomBox | `MIIIb_W바텀Box` | W바텀 (Box 시퀀스 기반) |
| MIII_WBottomReversal | `MIII_W바텀` | W바텀 반전 (Method III, %B 기반) |
| MII_BandWalkTrend | `MII_밴드워크` | Band Walk 추세 추종 (Method II) |
| MI_HistoricalSqueezeBreakout | `MI_역사적스퀴즈` | 역사적 스퀴즈 (Method I) |

---

### 9.4 buy_bb_hybrid.yaml — Box + Bollinger 복합 (trigger)

**평가 경로**: `trigger: DefBoxBreakout` + `once_per: defbox`

| 룰 | signal | def_count | 설명 |
|----|--------|-----------|------|
| SH1_BBReboundDefBox | `BB하단반등매수` | 1 | BB 하단 반등 + DefBox |
| SH2_BBSqueezeMultiDef | `BB스퀴즈매수` | min 2 | BB 스퀴즈 + MultiDef |
| SH3_BBMiddleS01 | `BB중심선S01매수` | 1 | BB 중심선 + S01 보강 |
| SH4_BBReboundMultiDef | `BB하단반등MD매수` | min 2 | BB 하단 반등 + MultiDef 완화 |

---

### 9.5 buy_crypto_15m.yaml — 15분봉 다중 트리거 (per_candle)

**평가 경로**: `evaluation: per_candle` (매 캔들, 쿨다운 4봉)
**사용**: `RESTGO_BUY_RULES=rules/buy_crypto_15m.yaml ./RESTGo stock analyze <종목코드> 250`

#### 설정
```yaml
FeeRate: 0.0005, SlippageRate: 0.0005
PerCandleCooldownBars: 4
RSIOversoldThreshold: 30, VWAPDeviationK: 1.5
ATRStopMultiplier: 3.0, ATRTargetMultiplier: 1.5
TimeExitBars: 32, TargetSellWeight: 0.5, TrailingEMAPeriod: 21
```

| 룰 | signal | 핵심 조건 |
|----|--------|-----------|
| T11D_DonchianRebound | `T11D_DonchianRebound` | IsDonchianBreakdown + IsEMA9Above21 |
| T12D_RSI_EMA50 | `T12D_RSI_EMA50` | IsRSIOversold + IsPriceAboveEMA50 |
| T12D_RSI_NoGate | `T12D_RSI_NoGate` | IsRSIOversold (게이트 없음) |

---

### 9.6 buy_trigger_example.yaml — 트리거 문법 예시

**평가 경로**: `trigger` + `once_per` (3가지 패턴 예시)

| 룰 | trigger | once_per | 조건 |
|----|---------|----------|------|
| T_BBLowerReentry_RSI | BBLowerReentry | cooldown | IsRSIOversold |
| T_SqueezeBreakout_Volume | BBSqueezeBreakout | cooldown | IsVolumeZScoreSpike |
| T_DefBoxBreakout_Basic | DefBoxBreakout | defbox | CloseNear + !ExcessiveWick |

---

### 9.7 buy_wdefbox.yaml / buy_wdefbox_gc.yaml — W-DefBox 전략

**평가 경로**: trigger (W-DefBox 결합 신호)
- `buy_wdefbox.yaml`: W패턴 + DefBox 결합 매수
- `buy_wdefbox_gc.yaml`: W-DefBox + Golden Cross 확증 변형

### 9.8 buy_stg11_15m.yaml — STG11 15분봉

**평가 경로**: trigger — STG11 돌파후행 전략을 15분봉에 적용

---

## 10. 매도 전략 (5-Path)

### 10.1 sell_default.yaml — 기본 매도 전략

**총 룰**: 21개
**5-Path 평가 순서**: Path 1 Critical → Path 2 Composite → Path 3 Extension → Path 4 Expiry → Path 5 Individual (Priority 오름차순)

#### 룰 구조
```yaml
sell_rules:
  - name: RuleName
    path: individual          # critical/composite/extension/expiry/individual
    priority: 4               # Path 5 개별 룰 우선순위 (작을수록 우선)
    when: [ConditionA, ...]   # AND 조건
    tracking:                 # 발화 제어 (TrackAndCheck)
      count_min: 1            # 누적 횟수 임계
      ratio_min: 0.05         # 보유기간 대비 비율 임계
    weight: 0.5               # 부분 매도 비율
    composite_eligible: true  # Composite Path 합산 대상 여부
    category: Loss            # Critical/Profit/Loss/Technical/Extension/Expiry
```

#### 전역 설정
```yaml
settings:
  max_holding_period: 20
  auto_liquidate_on_expiry: true
  default_sell_weight: 0.5
  small_remaining_threshold: 0.125
  minimum_execution_size: 0.01
  critical_failure:
    daily_drop_threshold: -0.10
    panic_volume_multiplier: 2.0
    panic_min_drop_rate: -0.05
    cumulative_drop_threshold: -0.15
    cumulative_drop_days: 5
    ma_reversal_days: 3
```

#### Composite 설정
```yaml
composite:
  threshold_high_recovery: 1.0
  threshold_medium_recovery: 0.6
  threshold_low_recovery: 0.3
  weight_strong: 1.0
  weight_medium: 0.5
  weight_weak: 0.25
```

#### Path 1: Critical (1개)
| 룰 | Weight | 설명 |
|----|--------|------|
| CriticalFailure | 1.0 | 급락+거래량폭증+누적하락+MA반전 → **100% 즉시 전량 청산** |

#### Path 3: Extension (1개)
| 룰 | Weight | 설명 |
|----|--------|------|
| MA5BreakdownDuringExtension | 1.0 | 연장 중 MA5 붕괴 → 100% 청산 |

#### Path 4: Expiry (1개)
| 룰 | Weight | 설명 |
|----|--------|------|
| PeriodExpiry | 1.0 (can_extend) | 20일 경과 → 연장 평가 후 청산 |

#### Path 5: Individual (18개)

| Pri | 룰 | Weight | Cat | Composite |
|-----|-----|--------|-----|-----------|
| 1 | EarlyDrop | 0.30 | Loss | - |
| 2 | EarlyMainBoxBreak | 0.50 | Loss | - |
| 2 | BBSqueezeExpansion | 0.25 | Loss | - |
| 3 | GapUpProfit | 0.50 | Profit | - |
| 3 | BBUpperBreakoutProfit | 0.50 | Profit | - |
| 4 | MainBoxBreakdown | 0.50 | Loss | ✓ |
| 4 | MainBoxPersistentBreakdown | 0.50 | Loss | ✓ |
| 4 | MainBoxBBBreakdown | 0.50 | Loss | - |
| 5 | MainBoxRecoveryFail | 0.50 | Loss | - |
| 6 | WeakFoundationFail | 0.50 | Loss | ✓ |
| 7 | TrendEntryFail1 | 0.50 | Loss | ✓ |
| 7 | TrendEntryFail2 | 0.50 | Loss | ✓ |
| 8 | MAReversalBoxPattern | 0.50 | Technical | - |
| 8 | ConsecutiveNegativeCandles | 0.50 | Technical | - |
| 9 | AdaptiveStopLoss | 0.50 | Loss | ✓ |
| 10 | TimeDelayedStopLoss | 0.50 | Loss | - |
| 11 | StopLoss | 0.50 | Loss | - |
| 12 | MA5MA20DeadCross | 0.50 | Technical | ✓ |

#### Composite Path 요약
Composite Eligible 7개 (합산 ≥ threshold 시 청산):
MainBoxBreakdown, MainBoxPersistentBreakdown, WeakFoundationFail, TrendEntryFail1, TrendEntryFail2, MA5MA20DeadCross, AdaptiveStopLoss

---

### 10.2 sell_positive_only.yaml — 익절 전용

**변경점**: sell_default.yaml에서 익절만 남기고 나머지 룰 제거
| Pri | 룰 | Weight | Cat |
|-----|-----|--------|-----|
| 3 | BBUpperBreakoutProfit | 0.50 | Profit |

### 10.3 sell_positive_only_mh25.yaml — 익절 + 보유 25일

**변경점**: `max_holding_period: 25` (기본 20 → 25)

### 10.4 sell_s03s23.yaml / sell_wdefbox.yaml

- **sell_s03s23.yaml**: S03+S23 조합 전략 전용 매도
- **sell_wdefbox.yaml**: W-DefBox 전략 전용 매도

---

## 11. Ablation 실험 (rules/ablation/ — 34개)

### 11.1 S06 전용 분석 (13개)
| 파일 | 설명 |
|------|------|
| `strategy_s06_only.yaml` | S06만 단독 베이스라인 |
| `strategy_s06_no_defcount.yaml` | DefCount 필터 제거 |
| `strategy_s06_no_multidef.yaml` | DamCondition 제거 |
| `strategy_s06_no_closenear.yaml` | 근접 조건 제거 |
| `strategy_s06_a.yaml` ~ `_d.yaml` | 조건 조합 변형 A~D |
| `strategy_s06_star.yaml` | 풀셋 (★) |
| `strategy_s06_combined.yaml` | 모든 변형 통합 |
| `strategy_s06_p_*.yaml` (5개) | pair-wise 순열 |

### 11.2 Sell 조건 제거 (4개)
| 파일 | 제거 대상 |
|------|-----------|
| `sell_strategy1_no_mbbwff.yaml` | MainBoxBreakdown, MainBoxBBBreakdown, WeakFoundationFail |
| `sell_strategy1_no_technical.yaml` | Technical 카테고리 전체 |
| `sell_notechnical.yaml` | Technical 룰 제거 (신규) |
| `sell_mh20_notechnical.yaml` | Technical 제거 + max_holding 20 |

### 11.3 MinHold 변형 (4개)
| 파일 | min_hold |
|------|----------|
| `sell_minhold3.yaml` | 3일 |
| `sell_minhold5.yaml` | 5일 |
| `sell_minhold10.yaml` | 10일 |
| `sell_minhold20.yaml` | 20일 |

### 11.4 Position-only 변형 (10개)
| 파일 | max_holding | 기타 |
|------|-------------|------|
| `sell_strategy1_posOnly_mh10~60.yaml` (6개) | 10~60 | 익절만 |
| `sell_strategy1_posOnly_cf_t005~05.yaml` (4개) | 20 | CriticalFailure 임계 변형 |

### 11.5 WDefBox 전용 (1개)
| 파일 | 설명 |
|------|------|
| `sell_wdefbox_dstop20.yaml` | W-DefBox + 동적손절 20% |

---

## 12. 아카이브 (rules/archive/ — 10개)

보관된 과거 실험 YAML (수정 금지).

| 파일 | 내용 |
|------|------|
| `strategy3_stage1.yaml` | Stage 1 파라미터 |
| `strategy3_t11d_only.yaml` | T11D 단독 |
| `strategy3_t11only.yaml` | T11 단독 |
| `strategy_s03_only.yaml` | S03 단독 |
| `strategy_s04_only.yaml` | S04 단독 |
| `strategy_s05_only.yaml` | S05 단독 |
| `strategy_s08_only.yaml` | S08 단독 |
| `strategy_t01_only.yaml` | T01 단독 |
| `strategy_t03_only.yaml` | T03 단독 |
| `strategy_time_exit_6bar.yaml` | TimeExit=6봉 변형 |

> 참고: `strategy3.yaml.*` (stage_a3, stage_d, t11d, w8) 확장자 파일들은 `.yaml`이 아니므로 YAML 카운트에서 제외된다.

---

## 13. 그리드 서치 전략

### 13.1 grid_crypto_example.yaml
```yaml
base_strategy: rules/buy_crypto_15m.yaml
markets: [KRW-BTC, KRW-ETH]
days: 1000, workers: 8
params:
  ADXTrendThreshold: [15, 20, 25]
  VolumeZScoreThreshold: [1.5, 2.0, 2.5]
  PerCandleCooldownBars: [4, 8, 16]
```

### 13.2 grid_crypto_stage2.yaml
```yaml
base_strategy: rules/strategy3_t11d_only.yaml
markets: [KRW-BTC, KRW-ETH, KRW-XRP, KRW-SOL]
days: 400000, workers: 8
params:
  RSIOversoldThreshold: [25, 28, 30, 32, 35]
  ATRStopMultiplier: [2.0, 2.5, 3.0, 3.5]
  ATRTargetMultiplier: [1.5, 2.0, 2.5]
```

### 13.3 grid_crypto_w10b.yaml
```yaml
base_strategy: rules/strategy3_t11only.yaml
markets: [KRW-BTC, KRW-ETH, KRW-XRP, KRW-SOL]
days: 400000, workers: 8
params:
  ATRStopMultiplier: [1.5, 2.0, 2.5, 3.0]
  ATRTargetMultiplier: [2.0, 3.0, 4.0]
  TimeExitBars: [16, 32, 64, 96]
```

### 13.4 grid_stg11.yaml
STG11 돌파후행 전략 파라미터 그리드 서치.
