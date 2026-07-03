# RESTGo YAML 전략 명세서

> 작성자: Claude Code AI (pi coding agent)
> 최종 수정: 2026-07-03
> 대상: `rules/` 디렉토리 전체 YAML 전략 파일 분석

---

## 1. 개요

RESTGo는 매수·매도 전략을 **YAML 파일**로 정의한다. 매수 측은 3가지 평가 경로가 있고, 매도 측은 5-Path 의사결정 엔진으로 동작한다.

### 평가 경로 (매수)

| 경로 | 필드 | 설명 | 사용 전략 |
|------|------|------|-----------|
| **on_breakout** | (trigger 미지정, evaluation 미지정) | C# DamChecker 상태머신 — DefBox 돌파 캔들 1회만 룰 평가 | `strategy1.yaml` |
| **trigger** | `trigger: <등록명>` | triggerRegistry edge 이벤트 발생 캔들에서만 평가, `once_per`로 중복 제어 | `buy_indicator.yaml`, `buy_bb_*.yaml`, `buy_trigger_example.yaml` |
| **per_candle** | `evaluation: per_candle` | 매 캔들 평가 + 쿨다운 (가상자산 15분봉 전용) | `buy_crypto_15m.yaml` |

> **on_breakout vs trigger**: on_breakout은 C# 정합이므로 strategy1.yaml에서 계속 유지한다. trigger는 stateless 설계로, 박스 아래로 되밀린 후 재돌파하면 edge가 다시 발생해 아직 발화 안 한 룰이 재평가된다.

### 등록 트리거 목록 (`stg/trigger_registry.go`)

| 트리거명 | 설명 |
|----------|------|
| `DefBoxBreakout` | 가격+거래대금+ATR 3중 게이트 통과 (on_breakout과 동일 게이트) |
| `PriceBreakout` | 순수 가격 돌파만 (거래대금·ATR 필터 분리) |
| `BBLowerBreakdown` | 종가가 BB 하단 밴드 하향 이탈 |
| `BBLowerReentry` | BB 하단 밴드 아래에서 안으로 복귀 |
| `BBSqueezeBreakout` | %B 상향 돌파 + 최근 BBWidth<4% 스퀴즈 존재 |

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
   │  │  ├─ cond.IsDefBoxBreakout()
   │  │  ├─ cond.IsVolumeBreakout()
   │  │  └─ ATR 변동성 필터
   │  ├─ checkBuyConditions() → EvaluateRules()
   │  │  └─ evaluateSingleRule() × N개 룰
   │  │     ├─ when: conditionRegistry[name]() → AND
   │  │     ├─ when_not: !conditionRegistry[name]() → NOT
   │  │     └─ any_of: conditionRegistry[name]() → OR
   │  ├─ determineBuySignal()          → REST2 S13~S16
   │  ├─ processPostBreakoutSignals()  → ShortRange (S19)
   │  └─ processFollowUpBuyDecisions() → S17~S20 재진입
   ├─ evaluateTriggerSignals()         → 트리거 경로 (buy_indicator, buy_bb_*, buy_trigger_example)
   │  ├─ triggerFn(ctx, s)             → edge 확인 (triggerRegistry)
   │  ├─ once_per 제어                → defbox/cooldown/none
   │  ├─ 같은 트리거 그룹 내 첫 매칭 승리
   │  └─ evaluateSingleRule()          → when/when_not/any_of 평가
   ├─ evaluatePerCandleSignals()       → per_candle 경로 (buy_crypto_15m)
   └─ EvaluateSellSignals()            → 매도 5-Path 결정
      ├─ evaluateRuleConditions() × N → 조건 평가
      ├─ TrackAndCheck()              → count_min/ratio_min 임계
      ├─ evaluateRecovery()           → 회복 가능성
      └─ makeSellDecision()           → 5-Path 우선순위 결정
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
    def_count: 1           # DefCount 정확 일치 (0=무관, on_breakout/trigger 모두 사용 가능)
    def_count_min: 2       # DefCount 최솟값 (0=무관)

    # ── 평가 경로 (택1) ──
    # ① 생략 → on_breakout (C# DamChecker 상태머신, strategy1 전용)
    # ② trigger: <등록명> → edge 발화 캔들에서 평가
    # ③ evaluation: per_candle → 매 캔들 평가 + 쿨다운

    trigger: DefBoxBreakout         # 트리거 경로 (edge 기반)
    once_per: defbox                # (기본) defbox | cooldown | none

    # ── 조건 ──
    when: [...]                     # AND 조건: 모두 true
    when_not: [...]                 # NOT 조건: 모두 false
    any_of: [...]                   # OR 조건: 하나 이상 true

    # ── 결과 ──
    signal: "신호명"                # 미지정 시 전략명 사용
```

---

## 4. 파일 인덱스

### 4.1 매수 전략

| 파일 | 평가 경로 | 내용 |
|------|-----------|------|
| `strategy1.yaml` | on_breakout | **기본 전략.** C# Stock1 REST1 포팅 — Box 구조 8룰 + Core-3 게이트 |
| `buy_indicator.yaml` | trigger | (구 strategy2) DefBox 돌파 순간 지표(RSI/BB/MA) 확증 6룰 (I01~I06) |
| `buy_bb_pure.yaml` | trigger | (구 strategy_bb_pure) Bollinger 3대 방법 — MIIIb/MIII → MII → MI |
| `buy_bb_hybrid.yaml` | trigger | (구 strategy_bb_hybrid) Box 구조 + BB 복합 4룰 (SH1~SH4) |
| `buy_trigger_example.yaml` | trigger | 트리거 문법 예시 3룰 (BBLowerReentry / BBSqueezeBreakout / DefBoxBreakout) |
| `buy_crypto_15m.yaml` | per_candle | (구 strategy3) 가상자산 15분봉 다중 트리거 OR (보류 영역) |

### 4.2 매도 전략

| 파일 | 내용 |
|------|------|
| `sell_default.yaml` | (구 sell_strategy1) **기본 매도.** 21룰 + 5-Path + tracking 임계 + composite |
| `sell_positive_only.yaml` | (구 sell_strategy1_positive_only) 양수 수익 구간만 매도 |
| `sell_positive_only_mh25.yaml` | (구 sell_strategy1_posOnly_mh25) 위 + max_holding=25 |

### 4.3 그리드 서치

| 파일 | 내용 |
|------|------|
| `grid_crypto_example.yaml` | (구 grid_example) 그리드 정의 예시 |
| `grid_crypto_stage2.yaml` | (구 grid_stage2) Stage 2 플래토 그리드 |
| `grid_crypto_w10b.yaml` | (구 grid_w10b) W10-B 청산 파라미터 그리드 |

---

## 5. 매수 전략 상세

### 5.1 strategy1.yaml — Box 구조 기반 (on_breakout)

**파일**: `rules/strategy1.yaml`
**평가 경로**: `on_breakout` (trigger 미지정) — C# DamChecker 상태머신
**사용**: `./RESTGo stock analyze <종목코드>` (기본)

#### 설정 오버라이드
```yaml
settings:
  DefBoxUpperWickToBodyRatioThreshold: 2.0  # 윗꼬리/몸통 비율 임계 (운영값)
```

#### Core-3 공통 게이트
모든 SingleDef/MultiDef 전략은 아래 3개 조건을 공통으로 통과해야 함 (C# `HasCoreCommonConditions`):
1. `IsBullishCandle` — 양봉 여부
2. `HasPullbackOrCorrection` — 풀백/조정 패턴
3. `IsMa20NearMa60Complex` — MA20-MA60 근접 (복합 검증)

---

#### SingleDef 전략 (def_count: 1, 평가 순서: 엄격 → 완화)

**① S03_SingleDefStrictBuy_Option2 — 가장 엄격**
| 구분 | 조건 |
|------|------|
| when (14개) | Core-3 + IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsMainboxPriceAboveMa60OrMa120, IsMainboxConditionValid, IsBoxDensityValidByDistribution, MainBoxPositionBasedTiming, MainBoxPositionBasedTimingLess, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

**② S01_SingleDefBuy — 표준 SingleDef**
| 구분 | 조건 |
|------|------|
| when (13개) | S03에서 MainBoxPositionBasedTimingLess 제거 |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

**③ S23_Intersection_4n8 — 4∩8 교집합 (완화)**
| 구분 | 조건 |
|------|------|
| when (9개) | Core-3 + IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

**④ S08_SingleDefRelaxedDistanceBuy_Option3 — 거리 완화**
| 구분 | 조건 |
|------|------|
| when (9개) | Core-3 + IsCloseNearDefboxPrice, IsMainboxDistanceTwiceOrMore, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween5, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

**⑤ S04_SingleDefWeakFoundationBuy_Option2 — 연약지반 (최완화)**
| 구분 | 조건 |
|------|------|
| when (7개) | Core-3 + IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsBoxCountBetween2, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `연약지반매수` |

---

#### MultiDef 전략 (def_count_min: 2)

**⑥ S05_MultiDefStandardBuy_Option2 — 표준 MultiDef**
| 구분 | 조건 |
|------|------|
| when (8개) | Core-3 + IsCloseNearDefboxPrice, IsMa60StrongerThanMa120By2Percent, IsBoxDensityValidByDistribution, MultiDefDamCountMax2, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `MD즉시매수` |

**⑦ S06_MultiDefRelaxedBuy_Option2 — 완화 MultiDef**
| 구분 | 조건 |
|------|------|
| when (6개) | Core-3 + IsCloseNearDefboxPrice, IsMultiDefRelaxedDamCondition, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `multidef매수대기` |

**⑧ S10_MultiDefStandardBuy_Option3 — 관통 없는 MultiDef**
| 구분 | 조건 |
|------|------|
| when (7개) | S05에서 IsPenetrationOptionValid 제거 |
| when_not | HasExcessiveUpperWick |
| signal | `MD즉시매수` |

---

### 5.2 buy_indicator.yaml — 지표 기반 (trigger)

**파일**: `rules/buy_indicator.yaml` (구 strategy2.yaml)
**평가 경로**: `trigger: DefBoxBreakout` + `once_per: defbox`
**사용**: `RESTGO_BUY_RULES=rules/buy_indicator.yaml ./RESTGo stock analyze <종목코드> 250`

#### I01_TrendConfluenceBuy — 추세 확증 (최엄격)
| when | IsPriceAboveAllMa, IsMaProperArrangement, IsAllMaRising, IsAboveBBMiddle, IsRSIInBullZone |
| when_not | HasExcessiveUpperWick |
| signal | `추세확증매수` |

#### I02_SqueezeBreakoutBuy — 스퀴즈 돌파
| when | IsBBSqueezeBreakout, IsPriceAboveAllMa |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `스퀴즈돌파매수` |

#### I03_ConvergenceBreakoutBuy — MA 수렴 후 발산
| when | IsMaConvergence, IsPriceAboveAllMa, IsRSIRising |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `수렴돌파매수` |

#### I04_GoldenCrossBuy — 골든크로스 확증
| when | IsMaGoldenCross5x20, IsRSIRising |
| any_of | IsAboveBBMiddle, IsBBUpperBreakout |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `골든크로스매수` |

#### I05_OversoldReboundBuy — 과매도 반등
| when | IsRSIRecoveringFromOversold, IsBBReboundFromLower |
| when_not | HasExcessiveUpperWick |
| signal | `과매도반등매수` |

#### I06_BandBreakoutBuy — 밴드 상단 돌파 (최완화)
| when | IsBBUpperBreakout, IsAllMaRising |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `밴드돌파매수` |

---

### 5.3 buy_bb_pure.yaml — Bollinger 3대 방법 (trigger)

**파일**: `rules/buy_bb_pure.yaml` (구 strategy_bb_pure.yaml)
**평가 경로**: `trigger: DefBoxBreakout` + `once_per: defbox`
**출처**: John Bollinger "Bollinger on Bollinger Bands"
**평가 순서**: Method III(가장 선택적) → Method II → Method I (첫 매칭 승리)

#### MIIIb_WBottomBox — W바텀 (Box 시퀀스 기반)
| when | IsBBWBottomBoxPattern |
| when_not | HasExcessiveUpperWick |
| signal | `MIIIb_W바텀Box` |

#### MIII_WBottomReversal — W바텀 반전 (Method III, %B 기반)
| when | IsCloseNearDefboxPrice, IsBBWBottomPattern |
| when_not | HasExcessiveUpperWick |
| signal | `MIII_W바텀` |

#### MII_BandWalkTrend — Band Walk 추세 추종 (Method II)
| when | IsCloseNearDefboxPrice, IsBBWalkingUp |
| when_not | HasExcessiveUpperWick |
| signal | `MII_밴드워크` |

#### MI_HistoricalSqueezeBreakout — 역사적 스퀴즈 (Method I)
| when | IsCloseNearDefboxPrice, IsBBSqueezeHistorical |
| when_not | HasExcessiveUpperWick |
| signal | `MI_역사적스퀴즈` |

---

### 5.4 buy_bb_hybrid.yaml — Box + Bollinger 복합 (trigger)

**파일**: `rules/buy_bb_hybrid.yaml` (구 strategy_bb_hybrid.yaml)
**평가 경로**: `trigger: DefBoxBreakout` + `once_per: defbox`

#### SH1_BBReboundDefBox — BB 하단 반등 + DefBox
| when | IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsBBReboundFromLower, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| def_count | 1 |
| signal | `BB하단반등매수` |

#### SH2_BBSqueezeMultiDef — BB 스퀴즈 + MultiDef
| when | IsCloseNearDefboxPrice, IsBBSqueezeBreakout, IsMa60StrongerThanMa120By2Percent |
| when_not | HasExcessiveUpperWick |
| def_count_min | 2 |
| signal | `BB스퀴즈매수` |

#### SH3_BBMiddleS01 — BB 중심선 + S01 보강
| when (11개) | IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsMainboxPriceAboveMa60OrMa120, IsMainboxConditionValid, IsBoxDensityValidByDistribution, MainBoxPositionBasedTiming, IsPenetrationOptionValid, IsAboveBBMiddle |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| def_count | 1 |
| signal | `BB중심선S01매수` |

#### SH4_BBReboundMultiDef — BB 하단 반등 + MultiDef 완화
| when | IsCloseNearDefboxPrice, IsBBReboundFromLower, IsMultiDefRelaxedDamCondition, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| def_count_min | 2 |
| signal | `BB하단반등MD매수` |

---

### 5.5 buy_crypto_15m.yaml — 15분봉 다중 트리거 (per_candle)

**파일**: `rules/buy_crypto_15m.yaml` (구 strategy3.yaml)
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

#### T11D_DonchianRebound
| when | IsDonchianBreakdown, IsEMA9Above21 |
| any_of | IsRSIOversold, IsVWAPDeviationBelow |
| when_not | IsRSIRecoveringFromOversold, IsDonchianBreakout |
| signal | `T11D_DonchianRebound` |

#### T12D_RSI_EMA50
| when | IsRSIOversold, IsPriceAboveEMA50 |
| any_of | IsVWAPDeviationBelow, IsMaProperArrangement |
| when_not | IsRSIRecoveringFromOversold, IsBBReboundFromLower, IsADXTrending |
| signal | `T12D_RSI_EMA50` |

#### T12D_RSI_NoGate
| when | IsRSIOversold |
| any_of | IsVWAPDeviationBelow, IsMaProperArrangement |
| when_not | IsRSIRecoveringFromOversold, IsBBReboundFromLower, IsADXTrending |
| signal | `T12D_RSI_NoGate` |

---

### 5.6 buy_trigger_example.yaml — 트리거 문법 예시

**파일**: `rules/buy_trigger_example.yaml`
**평가 경로**: `trigger` + `once_per` (3가지 패턴 예시)

#### T_BBLowerReentry_RSI — BB 하단 복귀 매수
| trigger | BBLowerReentry |
| when | IsRSIOversold |
| once_per | cooldown |
| signal | `즉시매수` |

#### T_SqueezeBreakout_Volume — 스퀴즈 상방 돌파 + 거래량
| trigger | BBSqueezeBreakout |
| when | IsVolumeZScoreSpike |
| once_per | cooldown |
| signal | `즉시매수` |

#### T_DefBoxBreakout_Basic — DefBox 재돌파 기본
| trigger | DefBoxBreakout |
| def_count_min | 1 |
| when | IsCloseNearDefboxPrice |
| when_not | HasExcessiveUpperWick |
| once_per | defbox |
| signal | `즉시매수` |

---

## 6. 매도 전략 (5-Path)

### 6.1 sell_default.yaml — 기본 매도 전략

**파일**: `rules/sell_default.yaml` (구 sell_strategy1.yaml)
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
    composite_weight: 0.5     # Composite 합산 시 가중치
    category: Loss            # Critical/Profit/Loss/Technical/Extension/Expiry
```

**tracking 필드 설명**:
- `count_min`: 발화 횟수가 N회 이상일 때만 실제 매도 신호 인정 (OR 조건)
- `ratio_min`: `발화횟수 / 보유기간 ≥ N`일 때만 인정 (OR 조건)
- `immediate: true`: TrackAndCheck 우회, 첫 발화 즉시 실행 (Extension 룰 전용)
- 누락 시 기본값: count_min=3, ratio_min=0.05

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

---

#### Path 1: Critical (1개)

| 룰 | 조건함수 | tracking | Weight | 설명 |
|----|----------|----------|--------|------|
| CriticalFailure | IsCriticalFailure | count_min=1, ratio_min=0.01 | 1.0 | 급락+거래량폭증+누적하락+MA반전 → **100% 즉시 전량 청산** |

#### Path 3: Extension (1개)

| 룰 | 조건함수 | tracking | Weight | 설명 |
|----|----------|----------|--------|------|
| MA5BreakdownDuringExtension | IsExtensionActive + IsMA5BreakdownDuringExtension | immediate=true | 1.0 | 연장 중 MA5 붕괴 → 100% 청산 |

#### Path 4: Expiry (1개)

| 룰 | 조건함수 | tracking | Weight | 설명 |
|----|----------|----------|--------|------|
| PeriodExpiry | IsPeriodExpired | count_min=1, ratio_min=0.01 | 1.0 (can_extend: true) | 20일 경과 → 연장 평가 후 청산 |

#### Path 5: Individual (18개)

| Pri | 룰 | 조건함수 | tracking | Weight | Cat | Comp | 설명 |
|-----|-----|----------|----------|--------|-----|------|------|
| 1 | EarlyDrop | IsEarlyDrop | 1/0.01 | 0.30 | Loss | - | 매수 직후 급락 (3일 내 -3%) |
| 2 | EarlyMainBoxBreak | IsEarlyMainBoxBreak | 1/0.01 | 0.50 | Loss | - | 매수 초기 MainBox 붕괴 |
| 2 | BBSqueezeExpansion | IsBBSqueezeExpansionWarning | 1/0.02 | 0.25 | Loss | - | BB 수축 후 확장 실패 |
| 3 | GapUpProfit | IsGapUpTakeProfit | 1/0.01 | 0.50 | Profit | - | 갭상승 10%+ 익절 |
| 3 | BBUpperBreakoutProfit | IsBBUpperBreakoutProfit | 1/0.01 | 0.50 | Profit | - | BB 상단 %B>0.95 + 수익>8% 익절 |
| 4 | MainBoxBreakdown | IsMainBoxBreakdownFailure | 1/0.05 | 0.50 | Loss | ✓ | MainBox 가격 하향 이탈 |
| 4 | MainBoxPersistentBreakdown | IsMainBoxPersistentBreakdown | 1/0.01 | 0.50 | Loss | ✓ | MainBox 지속적 하향 이탈 |
| 4 | MainBoxBBBreakdown | IsMainBoxBBBreakdown | 1/0.05 | 0.50 | Loss | - | MainBox + BB 하방 돌파 |
| 5 | MainBoxRecoveryFail | IsMainBoxRecoveryFailure | 2/0.05 | 0.50 | Loss | - | MainBox 회복 실패 |
| 6 | WeakFoundationFail | IsWeakFoundationFailure | 3/0.10 | 0.50 | Loss | ✓ | 연약지반 실패 |
| 7 | TrendEntryFail1 | IsTrendEntryFailure1 + IsWithin10Days | 3/0.10 | 0.50 | Loss | ✓ | 추세 진입 실패 유형1 |
| 7 | TrendEntryFail2 | IsTrendEntryFailure2 + IsWithin10Days | 3/0.10 | 0.50 | Loss | ✓ | 추세 진입 실패 유형2 |
| 8 | MAReversalBoxPattern | IsMAReversalBoxPattern | 2/0.05 | 0.50 | Technical | - | MA 반전 + Box 패턴 |
| 8 | ConsecutiveNegativeCandles | IsConsecutiveNegativeCandles | 2/0.05 | 0.50 | Technical | - | 연속 음봉 패턴 |
| 9 | AdaptiveStopLoss | IsAdaptiveStopLoss | 1/0.02 | 0.50 | Loss | ✓ | 적응형 손절 |
| 10 | TimeDelayedStopLoss | IsTimeDelayedStopLossEnabled + IsTimeDelayedStopLoss | 1/0.02 | 0.50 | Loss | - | 시간 지연 손절 |
| 11 | StopLoss | IsStopLoss | 2/0.02 | 0.50 | Loss | - | 고정 % 손절 |
| 12 | MA5MA20DeadCross | IsMA5MA20DeadCross | 1/0.05 | 0.50 | Technical | ✓ | MA5/MA20 데드크로스 |

#### Composite Path 요약
Composite Eligible 7개 (합산 ≥ threshold 시 청산):
- MainBoxBreakdown, MainBoxPersistentBreakdown, WeakFoundationFail
- TrendEntryFail1, TrendEntryFail2
- MA5MA20DeadCross, AdaptiveStopLoss

---

### 6.2 sell_positive_only.yaml — 익절 전용

**파일**: `rules/sell_positive_only.yaml`
**변경점**: sell_default.yaml에서 **익절(BBUpperBreakoutProfit)만 남기고 나머지 룰 제거**

| Pri | 룰 | 조건함수 | Weight | Cat |
|-----|-----|----------|--------|-----|
| 3 | BBUpperBreakoutProfit | IsBBUpperBreakoutProfit | 0.50 | Profit |

> Critical/Composite/Extension/Path 1~4 룰 모두 제거. Expiry만 PeriodExpiry 남김.

---

### 6.3 sell_positive_only_mh25.yaml — 익절 전용 + 보유기간 25일

**파일**: `rules/sell_positive_only_mh25.yaml`
**변경점**: `max_holding_period: 25` (기본 20 → 25), 나머지는 positive_only와 동일

---

## 7. Ablation 실험 (rules/ablation/)

전략 구성요소 기여도 검증용 변형 YAML.

### 7.1 Sell 조건 제거 실험 (2개)

| 파일 | 제거 대상 | 목적 |
|------|-----------|------|
| `sell_strategy1_no_mbbwff.yaml` | MainBoxBreakdown, MainBoxBBBreakdown, WeakFoundationFail | 3개 조건 제거 시 성능 변화 |
| `sell_strategy1_no_technical.yaml` | Technical 카테고리 룰 (MAReversalBoxPattern, ConsecutiveNegativeCandles, MA5MA20DeadCross) | 기술적 매도 제거 효과 |

### 7.2 Position-only 변형 (10개)

| 파일 | max_holding_period | 기타 |
|------|-------------------|------|
| `sell_strategy1_posOnly_mh10.yaml` | 10 | 익절만 |
| `sell_strategy1_posOnly_mh15.yaml` | 15 | 익절만 |
| `sell_strategy1_posOnly_mh30.yaml` | 30 | 익절만 |
| `sell_strategy1_posOnly_mh40.yaml` | 40 | 익절만 |
| `sell_strategy1_posOnly_mh50.yaml` | 50 | 익절만 |
| `sell_strategy1_posOnly_mh60.yaml` | 60 | 익절만 |
| `sell_strategy1_posOnly_cf_t02.yaml` | 20 | CriticalFailure daily_drop: -0.02 |
| `sell_strategy1_posOnly_cf_t03.yaml` | 20 | CriticalFailure daily_drop: -0.03 |
| `sell_strategy1_posOnly_cf_t05.yaml` | 20 | CriticalFailure daily_drop: -0.05 |
| `sell_strategy1_posOnly_cf_t005.yaml` | 20 | CriticalFailure daily_drop: -0.005 |

### 7.3 S06 전용 분석 (13개)

| 파일 | 변경 내용 | 목적 |
|------|-----------|------|
| `strategy_s06_only.yaml` | S06만 단독 | 베이스라인 |
| `strategy_s06_no_defcount.yaml` | def_count_min 제거 | DefCount 필터 기여도 |
| `strategy_s06_no_multidef.yaml` | IsMultiDefRelaxedDamCondition 제거 | DamCondition 기여도 |
| `strategy_s06_no_closenear.yaml` | IsCloseNearDefboxPrice 제거 | 근접 조건 기여도 |
| `strategy_s06_a.yaml` ~ `strategy_s06_d.yaml` | 조건 조합 변형 A~D | 부분 조건셋 평가 |
| `strategy_s06_star.yaml` | 모든 조건 (★ 표기) | 풀셋 |
| `strategy_s06_combined.yaml` | 모든 변형 통합 | 통합 비교 |
| `strategy_s06_p_*.yaml` (5개) | pair-wise 순열 | 2개 조건 조합별 기여도 |

---

## 8. 아카이브 (rules/archive/)

14개 보관된 과거 실험 YAML (수정 금지).

| 파일 | 내용 |
|------|------|
| `strategy3.yaml.t11d` | Stage 1~2 T11D EMABull 버전 |
| `strategy3.yaml.stage_d` | Stage D EMA9>EMA21 단일 룰 |
| `strategy3.yaml.stage_a3` | Stage A-3 다중 트리거 |
| `strategy3.yaml.w8` | W8 실험 버전 |
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

---

## 9. 등록된 트리거 함수 (triggerRegistry)

`stg/trigger_registry.go`의 `init()`에서 등록. 트리거는 반드시 edge 형태(돌파/이탈 발생 순간에만 true)여야 하며, 상태가 지속되는 level 조건과 분리되어 있다.

| 트리거명 | 설명 |
|----------|------|
| `DefBoxBreakout` | 가격+거래대금+ATR 3중 게이트 통과 (stateless) |
| `PriceBreakout` | 순수 가격 돌파만 (거래대금·ATR when으로 분리) |
| `BBLowerBreakdown` | 종가가 BB 하단 밴드 하향 이탈 |
| `BBLowerReentry` | BB 하단 밴드 아래에서 안으로 복귀 |
| `BBSqueezeBreakout` | %B 상향 돌파 + 최근 스퀴즈 존재 |

---

## 10. 등록된 조건 함수 (conditionRegistry)

`stg/buy_conditions_registry.go`의 `init()`에서 등록. 총 70+개. 모든 YAML 룰의 `when`/`when_not`/`any_of`는 이 레지스트리에 등록된 이름만 사용 가능.

### 10.1 Box 구조 (13개)
IsDefBoxBreakout, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsMainboxDistanceTwiceOrMore, IsSingleBreakout, IsBoxConditionValid, IsBoxConditionValid2, IsBoxCountBetween2, IsBoxCountBetween5, IsBoxDensityValidByCount, IsBoxDensityValidByDistribution, HasExcessiveUpperWick, MultiDefDamCountMax2

### 10.2 캔들 패턴 (2개)
IsBullishCandle, HasPullbackOrCorrection

### 10.3 이동평균선 (8개)
IsMa20NearMa60Complex, IsMa20NearMa60Simple, IsMa60StrongerThanMa120By2Percent, IsMainboxPriceAboveMa60OrMa120, HasLowTouchedMa20, IsMainboxConditionValid, MainBoxPositionBasedTiming, MainBoxPositionBasedTimingLess

### 10.4 RSI 조건 (5개)
IsRSIOversold, IsRSIOverbought, IsRSIRecoveringFromOversold, IsRSIRising, IsRSIInBullZone

### 10.5 Bollinger 조건 (9개)
IsBBLowerTouch, IsBBReboundFromLower, IsBBSqueezeBreakout, IsBBUpperBreakout, IsAboveBBMiddle, IsBBSqueezeHistorical, IsBBWalkingUp, IsBBWBottomPattern, IsBBWBottomBoxPattern

### 10.6 MA 조건 (6개)
IsMaGoldenCross5x20, IsMaGoldenCross20x60, IsMaProperArrangement, IsAllMaRising, IsMaConvergence, IsPriceAboveAllMa

### 10.7 15분봉 P0 조건 (20개)
IsMACDGoldenCross, IsMACDHistogramRising, IsStochGoldenCross, IsADXTrending, IsDIBullish, IsDIBearish, IsAboveVWAP, IsBelowVWAP, IsVWAPDeviation, IsVWAPReclaim, IsVolumeZScoreSpike, IsOBVRising, IsSuperTrendBullish, IsSuperTrendBearish, IsDonchianBreakout, IsDonchianBreakdown, IsKeltnerBreakout, IsNarrowRange, IsRSIFallingFromOverbought, IsBBUpperReject

### 10.8 숏 미러 (거부 필터, 3개)
IsMaDeadCross5x20, IsMaInverseArrangement, IsPriceBelowAllMa

### 10.9 EMA 조건 (5개)
IsEMABullArrangement, IsEMA9Above21, IsEMA21PullbackBounce, IsPriceAboveEMA50, IsVWAPDeviationBelow

### 10.10 관통/기타 (2개)
IsPenetrationOptionValid, IsMultiDefRelaxedDamCondition, IsATREntryValid

---

## 11. 등록된 매도 조건 함수 (sellConditionRegistry)

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

### Extension/Expiry (4개)
IsExtensionActive, IsMA5BreakdownDuringExtension, IsPeriodExpired, CanExtendHoldingOnExpiry

> **총 25개 매도 조건 함수 등록**

---

## 12. 그리드 서치 전략

### 12.1 grid_crypto_example.yaml
```yaml
base_strategy: rules/buy_crypto_15m.yaml
markets: [KRW-BTC, KRW-ETH]
days: 1000, workers: 8
params:
  ADXTrendThreshold: [15, 20, 25]
  VolumeZScoreThreshold: [1.5, 2.0, 2.5]
  PerCandleCooldownBars: [4, 8, 16]
```
> 3³ = 27 조합 × 2 마켓 = 54 시나리오

### 12.2 grid_crypto_stage2.yaml
```yaml
base_strategy: rules/strategy3_t11d_only.yaml
markets: [KRW-BTC, KRW-ETH, KRW-XRP, KRW-SOL]
days: 400000, workers: 8
params:
  RSIOversoldThreshold: [25, 28, 30, 32, 35]
  ATRStopMultiplier: [2.0, 2.5, 3.0, 3.5]
  ATRTargetMultiplier: [1.5, 2.0, 2.5]
```
> 5 × 4 × 3 = 60 조합 × 4 마켓 = 240 시나리오

### 12.3 grid_crypto_w10b.yaml
```yaml
base_strategy: rules/strategy3_t11only.yaml
markets: [KRW-BTC, KRW-ETH, KRW-XRP, KRW-SOL]
days: 400000, workers: 8
params:
  ATRStopMultiplier: [1.5, 2.0, 2.5, 3.0]
  ATRTargetMultiplier: [2.0, 3.0, 4.0]
  TimeExitBars: [16, 32, 64, 96]
```
> 4 × 3 × 4 = 48 조합 × 4 마켓 = 192 시나리오
