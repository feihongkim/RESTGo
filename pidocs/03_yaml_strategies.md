# RESTGo YAML 전략 명세서

> 작성자: Claude Code AI (pi coding agent)
> 날짜: 2026-06-25
> 대상: `rules/` 디렉토리 전체 YAML 전략 파일 분석

---

## 1. 개요

RESTGo는 매수·매도 전략을 **YAML 파일**로 정의한다. YAML 기반 룰 엔진은 `stg/buy_rule_engine.go`(매수)와 `stg/sell_rule_engine.go`(매도)에 의해 실행된다.

### 실행 흐름 요약

```
1. stg.LoadStrategy(path)             → YAML 파싱 → activeRules 저장
2. box.FetchCandles()                 → DB에서 캔들 조회
3. indicator.PrepareCandles()         → 13종 지표 일괄 계산
4. stg.Analyze() / analyzeInternal()  → 메인 루프 진입
   ├─ box.CheckAndCreateDefBox()      → DefBox 생성 조건 평가
   ├─ box.AnalyzeCurvature()          → 곡률 분석, Box 생성
   ├─ evaluateBuySignals()            → DefBox·돌파·매수 평가 통합
   │  ├─ checkDefBoxBreakout()        → 가격+거래대금+ATR 게이트
   │  │  ├─ cond.IsDefBoxBreakout()
   │  │  ├─ cond.IsVolumeBreakout()
   │  │  └─ ATR 변동성 필터
   │  ├─ checkBuyConditions() → EvaluateRules()
   │  │  └─ evaluateSingleRule() × N개 룰
   │  │     ├─ when: conditionRegistry[name]() → AND
   │  │     ├─ when_not: !conditionRegistry[name]() → NOT
   │  │     └─ any_of: conditionRegistry[name]() → OR
   │  ├─ determineBuySignal()         → REST2 S13~S16
   │  ├─ processPostBreakoutSignals() → ShortRange 사후평가
   │  └─ processFollowUpBuyDecisions()→ S17~S20 재진입
   ├─ evaluatePerCandleSignals()      → per_candle 룰 평가
   └─ EvaluateSellSignals()           → 매도 5-Path 결정
      ├─ evaluateRuleConditions() × 21 → 조건 평가
      ├─ TrackAndCheck()              → count_min/ratio_min
      ├─ evaluateRecovery()           → 회복 가능성
      └─ makeSellDecision()           → 5-Path 우선순위 결정
         ├─ Path 1 Critical
         ├─ Path 2 Composite
         ├─ Path 3 Extension
         ├─ Path 4 Expiry
         └─ Path 5 Individual
```

### 룰 평가 문법

```yaml
strategies:
  - name: "전략명"
    def_count: 1          # DefCount 정확 일치 (0=무관)
    def_count_min: 2       # DefCount 최솟값 (0=무관)
    evaluation: per_candle # 기본: on_breakout (돌파 캔들에서만)
    when: [...]            # AND 조건: 모두 true
    when_not: [...]        # NOT 조건: 모두 false
    any_of: [...]          # OR 조건: 하나 이상 true
    signal: "신호명"
```

---

## 2. 매수 전략

### 2.1 strategy1.yaml — Box 구조 기반 (기본)

**파일**: `rules/strategy1.yaml`
**평가 모드**: `on_breakout` (DefBox 돌파 캔들에서만)
**사용**: `./RESTGo stock analyze <code>` (기본)

#### 설정 오버라이드
```yaml
settings:
  DefBoxUpperWickToBodyRatioThreshold: 2.0  # 윗꼬리/몸통 비율 임계
```

#### 공통 게이트 (Core-3)
모든 SingleDef/MultiDef 전략은 아래 3개 조건을 공통으로 통과해야 함 (C# `HasCoreCommonConditions`):
1. `IsBullishCandle` — 양봉 여부
2. `HasPullbackOrCorrection` — 풀백/조정 패턴
3. `IsMa20NearMa60Complex` — MA20-MA60 근접 (복합 검증)

---

#### SingleDef 전략 (DefCount == 1)

##### ① S03_SingleDefStrictBuy_Option2 — 가장 엄격
| 구분 | 조건 |
|------|------|
| when (14개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsMainboxPriceAboveMa60OrMa120, IsMainboxConditionValid, IsBoxDensityValidByDistribution, MainBoxPositionBasedTiming, MainBoxPositionBasedTimingLess, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

> S01 + `MainBoxPositionBasedTimingLess` 추가 → 가장 보수적 진입

##### ② S01_SingleDefBuy — 표준 SingleDef
| 구분 | 조건 |
|------|------|
| when (13개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsMainboxPriceAboveMa60OrMa120, IsMainboxConditionValid, IsBoxDensityValidByDistribution, MainBoxPositionBasedTiming, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

> 가장 일반적인 DefBox 돌파 매수 전략

##### ③ S23_Intersection_4n8 — 4∩8 교집합 (완화)
| 구분 | 조건 |
|------|------|
| when (9개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

> S01에서 MA/밀도/타이밍 조건 제거 → 더 많은 진입 허용

##### ④ S08_SingleDefRelaxedDistanceBuy_Option3 — 거리 완화
| 구분 | 조건 |
|------|------|
| when (9개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMainboxDistanceTwiceOrMore, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween5, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `즉시매수` |

> MainboxDistance×2 + BoxCount≤5 → 거리·개수 완화

##### ⑤ S04_SingleDefWeakFoundationBuy_Option2 — 연약지반
| 구분 | 조건 |
|------|------|
| when (7개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsBoxCountBetween2, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `연약지반매수` |

> 최소 조건만 남긴 최완화 전략 — 신호명 `연약지반매수`

---

#### MultiDef 전략 (def_count_min: 2)

##### ⑥ S05_MultiDefStandardBuy_Option2 — 표준 MultiDef
| 구분 | 조건 |
|------|------|
| when (8개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMa60StrongerThanMa120By2Percent, IsBoxDensityValidByDistribution, MultiDefDamCountMax2, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `MD즉시매수` |

> 2개 이상 MainBox + MA60>MA120×1.02 + DamCount≤2

##### ⑦ S06_MultiDefRelaxedBuy_Option2 — 완화 MultiDef
| 구분 | 조건 |
|------|------|
| when (6개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMultiDefRelaxedDamCondition, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `multidef매수대기` |

> 대기 신호 (`multidef매수대기`) — 가장 완화된 MultiDef

##### ⑧ S10_MultiDefStandardBuy_Option3 — 관통 없는 MultiDef
| 구분 | 조건 |
|------|------|
| when (7개) | IsBullishCandle, HasPullbackOrCorrection, IsMa20NearMa60Complex, IsCloseNearDefboxPrice, IsMa60StrongerThanMa120By2Percent, IsBoxDensityValidByDistribution, MultiDefDamCountMax2 |
| when_not | HasExcessiveUpperWick |
| signal | `MD즉시매수` |

> S05에서 `IsPenetrationOptionValid` 제거

---

### 2.2 strategy2.yaml — 지표 기반 매수 (I01~I06)

**파일**: `rules/strategy2.yaml`
**평가 모드**: `on_breakout` (DefBox 돌파 캔들에서만 — 돌파+지표 확증 조합)
**사용**: `RESTGO_BUY_RULES=rules/strategy2.yaml ./RESTGo stock analyze <code>`

#### I01_TrendConfluenceBuy — 추세 확증 (최엄격)
| when | IsPriceAboveAllMa, IsMaProperArrangement, IsAllMaRising, IsAboveBBMiddle, IsRSIInBullZone |
| when_not | HasExcessiveUpperWick |
| signal | `추세확증매수` |

> 모든 MA 위 + 정배열 + 전 MA 상승 + BB중심선 상 + RSI 강세구간(45~65)

#### I02_SqueezeBreakoutBuy — 스퀴즈 돌파
| when | IsBBSqueezeBreakout, IsPriceAboveAllMa |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `스퀴즈돌파매수` |

> BBWidth<4% 수축 후 %B>0.8 상방 이탈 → 변동성 수축→분출

#### I03_ConvergenceBreakoutBuy — MA 수렴 후 발산
| when | IsMaConvergence, IsPriceAboveAllMa, IsRSIRising |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `수렴돌파매수` |

> MA5/20/60 수렴 + 가격 이탈 + RSI 상승 중

#### I04_GoldenCrossBuy — 골든크로스 확증
| when | IsMaGoldenCross5x20, IsRSIRising |
| any_of | IsAboveBBMiddle, IsBBUpperBreakout |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `골든크로스매수` |

> MA5×MA20 골든크로스 + RSI 상승 + (중심선 or 상단돌파)

#### I05_OversoldReboundBuy — 과매도 반등
| when | IsRSIRecoveringFromOversold, IsBBReboundFromLower |
| when_not | HasExcessiveUpperWick |
| signal | `과매도반등매수` |

> RSI 과매도 탈출 + BB 하단 터치 후 회복

#### I06_BandBreakoutBuy — 밴드 상단 돌파 (최완화)
| when | IsBBUpperBreakout, IsAllMaRising |
| when_not | IsRSIOverbought, HasExcessiveUpperWick |
| signal | `밴드돌파매수` |

> 종가 상단밴드 돌파 + 전 MA 상승 → 가장 단순 조건

---

### 2.3 strategy3.yaml — per_candle 15분봉 전략 (T11D, T12D)

**파일**: `rules/strategy3.yaml`
**평가 모드**: `evaluation: per_candle` (모든 캔들에서 평가, 쿨다운 4봉)
**사용**: `RESTGO_BUY_RULES=rules/strategy3.yaml ./RESTGo stock analyze <code>`

#### 설정
```yaml
FeeRate: 0.0005, SlippageRate: 0.0005   # 편도 0.05%씩
PerCandleCooldownBars: 4                  # 4봉 재진입 금지
ADXTrendThreshold: 20, RSIOversoldThreshold: 30
VWAPDeviationK: 1.5
ATRStopMultiplier: 3.0, ATRTargetMultiplier: 1.5
TimeExitBars: 32, TargetSellWeight: 0.5, TrailingEMAPeriod: 21
```

#### T11D_DonchianRebound
| when | IsDonchianBreakdown, IsEMA9Above21 |
| any_of | IsRSIOversold, IsVWAPDeviationBelow |
| when_not | IsRSIRecoveringFromOversold, IsDonchianBreakout |
| signal | `T11D_DonchianRebound` |

> Donchian 하방 회귀 + 단기 상승 추세(EMA9>21) + RSI 과매도/VWAP 이탈 확증

#### T12D_RSI_EMA50
| when | IsRSIOversold, IsPriceAboveEMA50 |
| any_of | IsVWAPDeviationBelow, IsMaProperArrangement |
| when_not | IsRSIRecoveringFromOversold, IsBBReboundFromLower, IsADXTrending |
| signal | `T12D_RSI_EMA50` |

> RSI 과매도 + 장기 추세 양호(종가>EMA50) + 확증

#### T12D_RSI_NoGate
| when | IsRSIOversold |
| any_of | IsVWAPDeviationBelow, IsMaProperArrangement |
| when_not | IsRSIRecoveringFromOversold, IsBBReboundFromLower, IsADXTrending |
| signal | `T12D_RSI_NoGate` |

> EMA 게이트 없이 RSI 과매도 단독 — EMA 게이트의 엣지 보호 여부 검증용

---

### 2.4 strategy_bb_hybrid.yaml — Box + Bollinger 복합

**파일**: `rules/strategy_bb_hybrid.yaml`
**평가 모드**: `on_breakout`

#### SH1_BBReboundDefBox — BB 하단 반등 + DefBox
| when | IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsBBReboundFromLower, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `BB하단반등매수` |

> 과매도 회복 중 DefBox 돌파 — 구조적 지지 + 통계적 과매도 이중 확증

#### SH2_BBSqueezeMultiDef — BB 스퀴즈 + MultiDef
| when | IsCloseNearDefboxPrice, IsBBSqueezeBreakout, IsMa60StrongerThanMa120By2Percent |
| when_not | HasExcessiveUpperWick |
| signal | `BB스퀴즈매수` |

> 밴드 수축 + 복수 방어선 응집 후 방향성 돌파

#### SH3_BBMiddleS01 — BB 중심선 + S01 보강
| when (11개) | Core-3 게이트 없이 S01 조건 + IsAboveBBMiddle |
| when_not | HasExcessiveUpperWick |
| any_of | HasLowTouchedMa20, IsMa20NearMa60Complex |
| signal | `BB중심선S01매수` |

> 기존 S01 + "최근 3봉 연속 BB중심선(%B≥0.5) 위" → 페이크 브레이크아웃 배제

#### SH4_BBReboundMultiDef — BB 하단 반등 + MultiDef 완화
| when | IsCloseNearDefboxPrice, IsBBReboundFromLower, IsMultiDefRelaxedDamCondition, IsPenetrationOptionValid |
| when_not | HasExcessiveUpperWick |
| signal | `BB하단반등MD매수` |

> SH1의 MultiDef 변형

---

### 2.5 strategy_bb_pure.yaml — Bollinger 3대 방법 순수구현

**파일**: `rules/strategy_bb_pure.yaml`
**평가 모드**: `on_breakout`
**출처**: John Bollinger "Bollinger on Bollinger Bands"

4개 룰, Method III(가장 선택적) → Method II → Method I 순서로 평가.

#### MIIIb_WBottomBox — W바텀 (Box 시퀀스 기반)
| when | IsBBWBottomBoxPattern |
| when_not | HasExcessiveUpperWick |
| signal | `MIIIb_W바텀Box` |

> 5이평 기울기 전환점 Box로 W 형태 감지 (하단Box→상단Box→하단Box)

#### MIII_WBottomReversal — W바텀 반전 (Method III)
| when | IsCloseNearDefboxPrice, IsBBWBottomPattern |
| when_not | HasExcessiveUpperWick |
| signal | `MIII_W바텀` |

> 과거 50봉 내 %B 기반 W패턴 탐지 + DefBox 돌파

#### MII_BandWalkTrend — Band Walk 추세 추종 (Method II)
| when | IsCloseNearDefboxPrice, IsBBWalkingUp |
| when_not | HasExcessiveUpperWick |
| signal | `MII_밴드워크` |

> 최근 5봉 중 과반(3봉+)이 %B ≥ 0.4 유지 → 강한 추세

#### MI_HistoricalSqueezeBreakout — 역사적 스퀴즈 (Method I)
| when | IsCloseNearDefboxPrice, IsBBSqueezeHistorical |
| when_not | HasExcessiveUpperWick |
| signal | `MI_역사적스퀴즈` |

> BBWidth ≤ 과거 120봉 최솟값 × 2.0 → 6개월 최저 수준 밴드 수축

---

## 3. 매도 전략 (5-Path)

### 3.1 sell_strategy1.yaml — 기본 매도 전략

**파일**: `rules/sell_strategy1.yaml`
**총 룰**: 21개
**5-Path 평가 순서**: Critical → Composite → Extension → Expiry → Individual(Priority 순)

#### 설정
```yaml
max_holding_period: 20, auto_liquidate_on_expiry: true
default_sell_weight: 0.5, small_remaining_threshold: 0.125, minimum_execution_size: 0.01
composite.threshold_high: 1.0, medium: 0.6, low: 0.3
composite.weight_strong: 1.0, medium: 0.5, weak: 0.25
```

#### Path 1: Critical (1개)
| 룰 | 조건함수 | Weight | 설명 |
|----|----------|--------|------|
| CriticalFailure | IsCriticalFailure | 1.0 | 급락+거래량폭증+누적하락+MA반전 → **100% 즉시 전량 청산** |

#### Path 3: Extension (1개)
| 룰 | 조건함수 | Weight | 설명 |
|----|----------|--------|------|
| MA5BreakdownDuringExtension | IsExtensionActive + IsMA5BreakdownDuringExtension | 1.0 | 연장 중 MA5 붕괴 → 100% 청산 |

#### Path 4: Expiry (1개)
| 룰 | 조건함수 | Weight | 설명 |
|----|----------|--------|------|
| PeriodExpiry | IsPeriodExpired | 1.0 (can_extend: true) | 20일 경과 → 연장 평가 후 청산 |

#### Path 5: Individual (18개, Priority 오름차순)

| Pri | 룰 | 조건함수 | Weight | Cat | Comp | 설명 |
|-----|-----|----------|--------|-----|------|------|
| 1 | EarlyDrop | IsEarlyDrop | 0.30 | Loss | - | 매수 직후 급락 (3일 내 -3%) |
| 2 | EarlyMainBoxBreak | IsEarlyMainBoxBreak | 0.50 | Loss | - | 매수 초기 MainBox 붕괴 |
| 2 | BBSqueezeExpansion | IsBBSqueezeExpansionWarning | 0.25 | Loss | - | BB 수축 후 확장 실패 |
| 3 | GapUpProfit | IsGapUpTakeProfit | 0.50 | Profit | - | 갭상승 10%+ 익절 |
| 3 | BBUpperBreakoutProfit | IsBBUpperBreakoutProfit | 0.50 | Profit | - | BB 상단 %B>0.95 + 수익>8% 익절 |
| 4 | MainBoxBreakdown | IsMainBoxBreakdownFailure | 0.50 | Loss | ✓ | MainBox 가격 하향 이탈 (count≥1, ratio≥0.05) |
| 4 | MainBoxPersistentBreakdown | IsMainBoxPersistentBreakdown | 0.50 | Loss | ✓ | MainBox 지속적 하향 이탈 |
| 4 | MainBoxBBBreakdown | IsMainBoxBBBreakdown | 0.50 | Loss | - | MainBox + BB 하방 돌파 |
| 5 | MainBoxRecoveryFail | IsMainBoxRecoveryFailure | 0.50 | Loss | - | MainBox 회복 실패 (count≥2, ratio≥0.05) |
| 6 | WeakFoundationFail | IsWeakFoundationFailure | 0.50 | Loss | ✓ | 연약지반 실패 (count≥3, ratio≥0.10) |
| 7 | TrendEntryFail1 | IsTrendEntryFailure1 + IsWithin10Days | 0.50 | Loss | ✓ | 추세 진입 실패 유형1 (count≥3) |
| 7 | TrendEntryFail2 | IsTrendEntryFailure2 + IsWithin10Days | 0.50 | Loss | ✓ | 추세 진입 실패 유형2 (count≥3) |
| 8 | MAReversalBoxPattern | IsMAReversalBoxPattern | 0.50 | Technical | - | MA 반전 + Box 패턴 (count≥2) |
| 8 | ConsecutiveNegativeCandles | IsConsecutiveNegativeCandles | 0.50 | Technical | - | 연속 음봉 패턴 (count≥2) |
| 9 | AdaptiveStopLoss | IsAdaptiveStopLoss | 0.50 | Loss | ✓ | 적응형 손절 (count≥1) |
| 10 | TimeDelayedStopLoss | IsTimeDelayedStopLossEnabled + IsTimeDelayedStopLoss | 0.50 | Loss | - | 시간 지연 손절 (count≥1) |
| 11 | StopLoss | IsStopLoss | 0.50 | Loss | - | 고정 % 손절 (count≥2) |
| 12 | MA5MA20DeadCross | IsMA5MA20DeadCross | 0.50 | Technical | ✓ | MA5/MA20 데드크로스 |

#### Composite Path 요약
Composite Eligible 7개 조건 (합산 ≥ threshold 시 청산):
- MainBoxBreakdown, MainBoxPersistentBreakdown, WeakFoundationFail
- TrendEntryFail1, TrendEntryFail2
- MA5MA20DeadCross, AdaptiveStopLoss

---

### 3.2 sell_strategy1_positive_only.yaml — 익절 전용

**파일**: `rules/sell_strategy1_positive_only.yaml`
**변경점**: sell_strategy1.yaml에서 **익절(BBUpperBreakoutProfit)만 남기고 나머지 모든 룰 제거**

| Pri | 룰 | 조건함수 | Weight | Cat |
|-----|-----|----------|--------|-----|
| 3 | BBUpperBreakoutProfit | IsBBUpperBreakoutProfit | 0.50 | Profit |

> Critical/Extension/Expiry/Path 1~2 룰 모두 제거. Expiry만 PeriodExpiry 남김.

---

### 3.3 sell_strategy1_posOnly_mh25.yaml — 익절 전용 + 보유기간 25일

**파일**: `rules/sell_strategy1_posOnly_mh25.yaml`
**변경점**: `max_holding_period: 25` (기본 20 → 25), 나머지는 positive_only와 동일

---

## 4. 그리드 서치 전략 (Grid)

### 4.1 grid_example.yaml
```yaml
base_strategy: rules/strategy3.yaml
markets: [KRW-BTC, KRW-ETH]
days: 1000, workers: 8
params:
  ADXTrendThreshold: [15, 20, 25]
  VolumeZScoreThreshold: [1.5, 2.0, 2.5]
  PerCandleCooldownBars: [4, 8, 16]
```
> 3³ = 27 조합 × 2 마켓 = 54 시나리오

### 4.2 grid_stage2.yaml
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

### 4.3 grid_w10b.yaml
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

---

## 5. Ablation 실험 (rules/ablation/)

전략 구성요소의 기여도를 검증하기 위한 27개 변형 YAML.

### 5.1 Sell 조건 제거 실험 (2개)

| 파일 | 제거 대상 | 목적 |
|------|-----------|------|
| `sell_strategy1_no_mbbwff.yaml` | MainBoxBreakdown, MainBoxBBBreakdown, WeakFoundationFail | 3개 조건 제거 시 성능 변화 |
| `sell_strategy1_no_technical.yaml` | Technical 카테고리 룰 (MAReversalBoxPattern, ConsecutiveNegativeCandles, MA5MA20DeadCross) | 기술적 매도 제거 효과 |

### 5.2 Position-only 변형 (8개)

| 파일 | max_holding_period | 기타 |
|------|-------------------|------|
| `sell_strategy1_posOnly_mh10.yaml` | 10 | 익절만, CriticalFailure 임계값 기본 |
| `sell_strategy1_posOnly_mh15.yaml` | 15 | 익절만 |
| `sell_strategy1_posOnly_mh30.yaml` | 30 | 익절만 |
| `sell_strategy1_posOnly_mh40.yaml` | 40 | 익절만 |
| `sell_strategy1_posOnly_mh50.yaml` | 50 | 익절만 |
| `sell_strategy1_posOnly_mh60.yaml` | 60 | 익절만 |
| `sell_strategy1_posOnly_cf_t02.yaml` | 20 | CriticalFailure daily_drop: -0.02 |
| `sell_strategy1_posOnly_cf_t03.yaml` | 20 | CriticalFailure daily_drop: -0.03 |
| `sell_strategy1_posOnly_cf_t05.yaml` | 20 | CriticalFailure daily_drop: -0.05 |
| `sell_strategy1_posOnly_cf_t005.yaml` | 20 | CriticalFailure daily_drop: -0.005 |

### 5.3 S06 전용 분석 (13개)

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

## 6. 아카이브 (rules/archive/)

14개 보관된 과거 실험 YAML.

| 파일 | 내용 |
|------|------|
| `strategy3.yaml.t11d` | Stage 1~2 T11D EMABull 버전 |
| `strategy3.yaml.stage_d` | Stage D EMA9>EMA21 단일 룰 |
| `strategy3.yaml.stage_a3` | Stage A-3 다중 트리거 |
| `strategy3.yaml.w8` | W8 실험 버전 |
| `strategy3_stage1.yaml` | Stage 1 파라미터 |
| `strategy3_t11d_only.yaml` | T11D 단독 (grid_stage2 참조) |
| `strategy3_t11only.yaml` | T11만 (grid_w10b 참조) |
| `strategy_s03_only.yaml` | S03 단독 |
| `strategy_s04_only.yaml` | S04 단독 |
| `strategy_s05_only.yaml` | S05 단독 |
| `strategy_s08_only.yaml` | S08 단독 |
| `strategy_t01_only.yaml` | T01 단독 |
| `strategy_t03_only.yaml` | T03 단독 |
| `strategy_time_exit_6bar.yaml` | TimeExit=6봉 변형 |

---

## 7. 등록된 조건 함수 (conditionRegistry)

`stg/buy_conditions_registry.go`의 `init()`에서 등록. 모든 YAML 룰의 `when`/`when_not`/`any_of`는 이 레지스트리에 등록된 이름만 사용 가능.

### 7.1 Box 구조 조건 (13개)
IsDefBoxBreakout, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsMainboxDistanceTwiceOrMore, IsSingleBreakout, IsBoxConditionValid, IsBoxConditionValid2, IsBoxCountBetween2, IsBoxCountBetween5, IsBoxDensityValidByCount, IsBoxDensityValidByDistribution, HasExcessiveUpperWick, MultiDefDamCountMax2

### 7.2 캔들 패턴 (2개)
IsBullishCandle, HasPullbackOrCorrection

### 7.3 이동평균선 (8개)
IsMa20NearMa60Complex, IsMa20NearMa60Simple, IsMa60StrongerThanMa120By2Percent, IsMainboxPriceAboveMa60OrMa120, HasLowTouchedMa20, IsMainboxConditionValid, MainBoxPositionBasedTiming, MainBoxPositionBasedTimingLess

### 7.4 RSI 조건 (5개)
IsRSIOversold, IsRSIOverbought, IsRSIRecoveringFromOversold, IsRSIRising, IsRSIInBullZone

### 7.5 Bollinger 조건 (9개)
IsBBLowerTouch, IsBBReboundFromLower, IsBBSqueezeBreakout, IsBBUpperBreakout, IsAboveBBMiddle, IsBBSqueezeHistorical, IsBBWalkingUp, IsBBWBottomPattern, IsBBWBottomBoxPattern

### 7.6 MA 조건 (6개)
IsMaGoldenCross5x20, IsMaGoldenCross20x60, IsMaProperArrangement, IsAllMaRising, IsMaConvergence, IsPriceAboveAllMa

### 7.7 15분봉 P0 조건 (20개)
IsMACDGoldenCross, IsMACDHistogramRising, IsStochGoldenCross, IsADXTrending, IsDIBullish, IsDIBearish, IsAboveVWAP, IsBelowVWAP, IsVWAPDeviation, IsVWAPReclaim, IsVolumeZScoreSpike, IsOBVRising, IsSuperTrendBullish, IsSuperTrendBearish, IsDonchianBreakout, IsDonchianBreakdown, IsKeltnerBreakout, IsNarrowRange, IsRSIFallingFromOverbought, IsBBUpperReject

### 7.8 숏 미러 (거부 필터, 3개)
IsMaDeadCross5x20, IsMaInverseArrangement, IsPriceBelowAllMa

### 7.9 EMA 조건 (5개)
IsEMABullArrangement, IsEMA9Above21, IsEMA21PullbackBounce, IsPriceAboveEMA50, IsVWAPDeviationBelow

### 7.10 관통/기타 (2개)
IsPenetrationOptionValid, IsMultiDefRelaxedDamCondition, IsATREntryValid

> **총 70+개 조건 함수 등록**

---

## 8. 등록된 매도 조건 함수 (sellConditionRegistry)

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
