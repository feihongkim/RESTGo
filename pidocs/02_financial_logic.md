# RESTGo 금융 분석 로직 상세

> 작성자: Claude Code AI (pi coding agent)
> 최종 수정: 2026-07-17
> 대상: RESTGo 프로젝트의 매수·매도 분석 로직 전체

---

## 1. 분석 파이프라인 개요

```
KIS2 / hannam / 해외 DB 캔들 조회
  → indicator 패키지: 기술적 지표 계산 (MA, Bollinger, RSI 등 O(N))
  → box 패키지: 곡률 분석 → Box 생성 → DefBox 생성
  → stg.Analyze(): 메인 분석 루프
       ├── checkDefBoxBreakout(): 돌파 게이트 (가격 + 거래대금 + ATR)
       ├── evaluateBuySignals(): on_breakout 경로 (strategy1 전용)
       ├── evaluateTriggerSignals(): trigger 경로 (triggerRegistry edge + once_per)
       ├── evaluatePerCandleSignals(): per_candle 경로 (매 캔들)
       ├── evaluateArmedTriggers(): Armed Trigger 상태 틱 (장전→발화)
       └── EvaluateSellSignals(): 5-Path 매도 평가 (매 캔들)
  → AnalysisResult 반환 (신호, 포지션, 수익률)
```

### 평가 경로 요약

| 경로 | 엔진 | 설명 |
|------|------|------|
| **on_breakout** | DamChecker 상태머신 | C# 정합. DefBox 돌파 캔들 1회만 룰 평가 (`strategy1.yaml`) |
| **trigger** | triggerRegistry edge | 등록된 edge(발생 순간) 이벤트에서 평가, `once_per`로 중복 제어 |
| **per_candle** | 매 캔들 평가 | `evaluation: per_candle` — 쿨다운 기반 (가상자산 15분봉 전용) |
| **armed trigger** | 2단계 상태머신 | 패턴 완성(장전) → 확인 이벤트(발화), YAML `trigger:`로 사용 |

---

## 2. Box 분석 시스템 (C# Stock1 포팅)

### 2.1 핵심 개념

| 개념 | 설명 | C# 대응 |
|------|------|---------|
| **Candle** | OHLCV 캔들 + 모든 지표값 내장 | `vo/Candle.cs` |
| **Box** | 가격 구간 (HighBox: 저항, LowBox: 지지) | `vo/Box.cs` |
| **CurveKey** | 현재 추세 방향 (1=상승, -1=하락) | `biz/Contexts/TradingContext.cs` |
| **DefBox** | 최종 저항선 돌파 지점 (매수 트리거) | `vo/Box.cs` |
| **MainBox** | DefBox 이전의 주요 지지/저항 Box | `vo/Box.cs` |

### 2.2 곡률 분석 (`box/curvature.go`)

```
AnalyzeCurvature(ctx):
  1. 직전 CurveKey 확인 (+1 상승 / -1 하락)
  2. 추세 반전 조건 평가:
     - 상승→하락: ShouldReverseToBearish() → HighBox 생성
     - 하락→상승: ShouldReverseToBullish() → LowBox 생성
  3. CurveKey 반전 시 Exposition 갱신 (새 추세 시작점)
  4. 반환: 새 CurveKey (반전 안 됐으면 이전값 유지)
```

### 2.3 DefBox 생성 조건 (`box/defbox.go`)

DefBox는 Box들이 특정 조건을 만족할 때 생성:
- Box 수, 가격 관계, 시간 간격, 침투 깊이 등을 평가
- 생성 시 `ctx.DefChecker` 증가, `ctx.DefboxPrice` 설정
- 매수 신호는 DefBox가 가격을 돌파할 때 발생

---

## 3. 기술적 지표 (indicator/)

### 3.1 계산 방식

모든 지표는 **Rolling Sum O(N)** 방식으로 효율적 계산:

```
Rolling Sum 기법:
  sum(t) = sum(t-1) + new - old (period 이전 값)
  → O(N) 전체 계산 (중첩 루프 없음)
```

### 3.2 지표 목록

| 지표 | 파일 | 파라미터 | 용도 |
|------|------|----------|------|
| **MA5/20/60/120/200** | candle_processor.go | 5,20,60,120,200 | 추세 방향, 지지/저항 |
| **ATR** | candle_processor.go | 14 | 변동성, 돌파 게이트 |
| **Bollinger Bands** | bollinger.go | 20, 2σ | 변동성, 과매수/과매도, %B |
| **RSI** | rsi.go | 14 (Wilder) | 과매수/과매도 (30/70) |
| **MACD** | macd.go | 12,26,9 | 추세 전환 |
| **EMA** | ema.go | - | 지수이동평균 |
| **Stochastic** | stochastic.go | 14,3,3 | 단기 과매수/과매도 |
| **Donchian** | donchian.go | 20 | 돌파 채널 |
| **Keltner** | keltner.go | 20, 2×ATR | 변동성 채널 |
| **OBV** | obv.go | - | 거래량 추세 |
| **ADX** | adx.go | 14 | 추세 강도 |
| **SuperTrend** | supertrend.go | 10, 3 | 추세 추종 |
| **VWAP** | vwap.go | - | 거래량 가중 평균가 |

---

## 4. 매수 시스템

### 4.1 평가 경로별 흐름

#### on_breakout (C# DamChecker, `strategy1.yaml`)
```
evaluateBuySignals():
  ├─ checkDefBoxBreakout() → 가격+거래대금+ATR 게이트
  ├─ EvaluateRules() → 첫 매칭 승리 (엄격→완화 순)
  ├─ determineBuySignal() → REST2 S13~S16
  ├─ processPostBreakoutSignals() → ShortRange (S19)
  └─ processFollowUpBuyDecisions() → S17~S20 재진입
```

#### trigger (`buy_indicator.yaml`, `buy_bb_*.yaml` 등)
```
evaluateTriggerSignals():
  ├─ triggerFn(ctx, s) → edge 확인 (triggerRegistry 15종)
  ├─ once_per 제어 → defbox/cooldown/none
  ├─ 같은 트리거 그룹 내 첫 매칭 승리
  └─ evaluateSingleRule() → when/when_not/any_of 평가
```

#### armed trigger (2단계: 장전→발화)
```
evaluateArmedTriggers():
  ├─ 매 캔들 armed 상태 틱
  ├─ 패턴 완성 → 장전 (ArmedTriggerState 저장, 유효기간 설정)
  └─ 유효기간 내 확인 이벤트 → 발화 → 일반 trigger처럼 룰 평가
```

#### per_candle (`buy_crypto_15m.yaml`)
```
evaluatePerCandleSignals():
  └─ 매 캔들 평가 + 쿨다운(PerCandleCooldownBars)
```

### 4.2 YAML 룰 엔진 (`stg/buy_rule_engine.go`)

```yaml
# 규칙 구조
strategies:
  - name: "전략명"
    def_count: 1           # DefCount 정확 일치 (0=무관)
    def_count_min: 2       # DefCount 최솟값 (0=무관)
    trigger: DefBoxBreakout # trigger 경로 (on_breakout 시 생략)
    evaluation: per_candle # per_candle 경로
    once_per: defbox       # defbox | cooldown | none
    when: [...]            # AND 조건 목록
    when_not: [...]        # NOT 조건 목록
    any_of: [...]          # OR 조건 목록
    signal: "신호명"        # 미지정 시 전략명 사용
```

### 4.3 등록된 트리거 (triggerRegistry — 15종)

| 트리거명 | 경로 | 설명 |
|----------|------|------|
| `DefBoxBreakout` | 일반 | 가격+거래대금+ATR 3중 게이트 (stateless) |
| `PriceBreakout` | 일반 | 순수 가격 돌파만 |
| `WBottomBox` | 일반 | W패턴 S-R-S 완성 순간 |
| `BBLowerBreakdown` | 일반 | BB 하단 밴드 하향 이탈 |
| `BBLowerReentry` | 일반 | BB 하단 아래→안 복귀 |
| `BBSqueezeBreakout` | 일반 | %B 상향 돌파 + 스퀴즈 존재 |
| `Stg6PullbackTouch` | 일반 | STG6 Pullback 터치 |
| `Stg11MA60Breakdown` | 일반 | STG11 MA60 하향 이탈 |
| `Stg11MA60FirstTouch` | 일반 | STG11 MA60 최초 터치 |
| `Stg9ApexPerch` | 일반 | STG9 Apex Perch |
| `Stg7GCAccel` | 일반 | STG7 Golden Cross 가속 |
| `Stg14Oversold` | 일반 | STG14 과매도 |
| `DefBoxApproach` | 일반 | DefBox 가격 접근 감지 |

### 4.4 등록된 Armed 트리거 (armedTriggerRegistry — 8종)

| 트리거명 | 설명 |
|----------|------|
| `MTopCollapse` | MTop 패턴 완성(장전) → 붕괴 확인(발화) |
| `HNSNecklineBreak` | HNS 패턴 완성(장전) → Neckline 돌파(발화) |
| `DefBoxBreakoutFailure` | DefBox 돌파 실패(장전) → 하락 확인(발화) |
| `DoubleBumpRetest` | DoubleBump 재시험(장전) → 확인(발화) |
| `DoubleBumpBreakout2` | DoubleBump 2차 돌파 |
| `MA20PullbackBreakout` | MA20 Pullback(장전) → 돌파 확인(발화) |
| `Stg2Inverted120Retreat` | STG2 역120 후퇴 |
| `DefBoxBreakoutSurvival` | DefBox 돌파 생존 |

### 4.5 조건 카테고리 (conditionRegistry — 81종)

| 카테고리 | 파일 | 조건 수 | 대표 조건 |
|----------|------|---------|-----------|
| **Box 구조** | buy_conditions.go | 13개 | IsDefBoxBreakout, IsCloseNearDefboxPrice, IsMainboxCloserThanCurrentPosition, IsSingleBreakout, IsBoxConditionValid, IsBoxCountBetween2/5, IsBoxDensityValidByDistribution, HasExcessiveUpperWick, MultiDefDamCountMax2 등 |
| **캔들 패턴** | buy_conditions_extra.go | 2개 | IsBullishCandle, HasPullbackOrCorrection |
| **MA 조건** | buy_conditions_extra.go | 8개 | IsMa20NearMa60Complex, IsMa60StrongerThanMa120By2Percent, HasLowTouchedMa20, MainBoxPositionBasedTiming 등 |
| **관통/기타** | buy_oscillator.go | 3개 | IsPenetrationOptionValid, IsMultiDefRelaxedDamCondition, IsATREntryValid |
| **RSI** | buy_indicator.go | 5개 | IsRSIOversold, IsRSIOverbought, IsRSIRecoveringFromOversold, IsRSIRising, IsRSIInBullZone |
| **Bollinger** | buy_indicator.go | 10개 | IsBBLowerTouch, IsBBReboundFromLower, IsBBSqueezeBreakout, IsBBUpperBreakout, IsAboveBBMiddle, IsBBSqueezeHistorical, IsBBWalkingUp, IsBBWBottomPattern, IsBBWBottomBoxPattern, HasDefBoxBeforeWPattern |
| **MA Gold/Dead** | buy_indicator.go | 9개 | IsMaGoldenCross5x20, IsMaGoldenCross20x60, IsMaProperArrangement, IsAllMaRising, IsMaConvergence, IsPriceAboveAllMa, IsMaDeadCross5x20, IsMaInverseArrangement, IsPriceBelowAllMa |
| **15분봉 P0** | buy_indicator_15m.go | 20개 | IsMACDGoldenCross, IsStochGoldenCross, IsADXTrending, IsAboveVWAP, IsVolumeZScoreSpike, IsSuperTrendBullish, IsDonchianBreakout 등 |
| **EMA** | buy_indicator_15m.go | 5개 | IsEMABullArrangement, IsEMA9Above21, IsPriceAboveEMA50, IsVWAPDeviationBelow 등 |
| **신규 전략** | buy_volume_wave.go 등 | 6개 | IsGoldenCrossPending, HasMTopStructure, IsStg11MA60Breakdown, IsS1SetupTerrain, IsS1EntryQuality, HasDefBoxOverhead |

> **81종의 조건 함수**가 conditionRegistry에 등록되어 있으며, 모든 YAML 룰의 `when`/`when_not`/`any_of`에서 참조 가능하다.

### 4.6 후속 매수 (FollowUp / REST2)

| 범위 | 유형 | 설명 |
|------|------|------|
| **S13~S16** | 후보군1 상태머신 | `DetermineBuySignal`: DefBox 이후 조건 관찰 → 진입 결정 |
| **S17~S20** | 재진입 | 완전 청산 후 재진입 조건 평가 |
| **게이트** | `BuyOn` / `SellHelper` | 실제 매수 실행 여부 및 청산 후 재진입 방지 |

### 4.7 중복 신호 방지

```
ctx.LastBuySignalPosition[전략명] = 현재 Position
→ 동일 DefBox 구간에서 한 전략은 1회만 발화
→ DefBox 변경 시 리셋
```

---

## 5. 매도 시스템 (5-Path)

### 5.1 5-Path 결정 흐름

```
makeSellDecision(signals, recoveryStrength, pos):
  Path 1: Critical     → IsCriticalFailure? → 100% 즉시 청산
  Path 2: Composite    → composite_eligible 신호 합산 ≥ threshold? → 가중 청산
  Path 3: Extension    → 연장 활성 + 개별 신호? → 100% 청산
  Path 4: Expiry       → 최대 보유기간 초과? → 100% 청산 (can_extend: 연장 평가)
  Path 5: Individual   → Priority 순으로 개별 신호 평가 → weight 비율 부분 매도
```

### 5.2 매도 조건 24종 (sellConditionRegistry)

#### Path 1: Critical (1개)
| 조건 | 설명 |
|------|------|
| `IsCriticalFailure` | 급락 + 거래량 폭증 + 누적 하락 + MA 반전 |

#### Path 2: Composite (7개 eligible)
MainBoxBreakdown, MainBoxPersistentBreakdown, WeakFoundationFail, TrendEntryFail1/2, MA5MA20DeadCross, AdaptiveStopLoss

#### Path 3: Extension (2개)
| 조건 | 설명 |
|------|------|
| `IsExtensionActive` | 연장 상태 확인 |
| `IsMA5BreakdownDuringExtension` | 연장 중 MA5 붕괴 |

#### Path 4: Expiry (1개)
| 조건 | 설명 |
|------|------|
| `IsPeriodExpired` | 최대 보유기간 도달 (기본 20일) |

#### Path 5: Individual (이하 18개, Priority 순)

| Pri | 조건 | Weight | 카테고리 |
|-----|------|--------|----------|
| 1 | IsEarlyDrop | 0.30 | Loss |
| 2 | IsEarlyMainBoxBreak | 0.50 | Loss |
| 2 | IsBBSqueezeExpansionWarning | 0.25 | Loss |
| 3 | IsGapUpTakeProfit | 0.50 | Profit |
| 3 | IsBBUpperBreakoutProfit | 0.50 | Profit |
| 4 | IsMainBoxBreakdownFailure | 0.50 | Loss (composite) |
| 4 | IsMainBoxPersistentBreakdown | 0.50 | Loss (composite) |
| 4 | IsMainBoxBBBreakdown | 0.50 | Loss |
| 5 | IsMainBoxRecoveryFailure | 0.50 | Loss |
| 6 | IsWeakFoundationFailure | 0.50 | Loss (composite) |
| 7 | IsTrendEntryFailure1 | 0.50 | Loss (composite) |
| 7 | IsTrendEntryFailure2 | 0.50 | Loss (composite) |
| 8 | IsMAReversalBoxPattern | 0.50 | Technical |
| 8 | IsConsecutiveNegativeCandles | 0.50 | Technical |
| 9 | IsAdaptiveStopLoss | 0.50 | Loss (composite) |
| 10 | IsTimeDelayedStopLoss | 0.50 | Loss |
| 11 | IsStopLoss | 0.50 | Loss |
| 12 | IsMA5MA20DeadCross | 0.50 | Technical (composite) |

### 5.3 부분 매도 (`stg/sell_executor.go`)

```
ExecutePartialSell(pos, reason, weight, settings):
  1. 잔량 ≤ SmallRemainingThreshold(0.125)? → 100% 청산
  2. sellQty × weight < MinimumExecutionSize(0.01)? → 스킵
  3. SellExecution 기록 + RemainingQuantity 감소
  4. 완전 청산 시:
     - IsActive = false
     - 가중평균 수익률 계산 (CalculateWeightedAverageReturn)
     - SellHelper 설정 (재진입 게이트)
```

---

## 6. 신규 분석 시스템 (문서화 이전 추가)

### 6.1 Volume Wave 분석
- **cond/**: `buy_volume_wave.go` — 거래량 파동 기반 매수 조건
- **stg/**: `volume_wave_analyze.go` + `volume_wave_pullback.go` — 신호 분석기
- **study/**: `volume_wave_scan.go`, `volume_wave_matrix.go`, `volume_wave_chart.go`, `volume_wave_box_study.go`, `volume_wave_strict_study.go` — 5종 연구 러너

### 6.2 Descending Trendline (하락추세선)
- **stg/**: `descending_trendline_analyze.go` — 하락추세선 돌파 분석기
- **study/**: `descending_trendline_study.go`, `descending_trendline_ma_study.go`, `descending_trendline_sideways_study.go`, `descending_trendline_chart.go` — 4종 연구 러너

### 6.3 MainBox Retest (재시험)
- **stg/**: `mainbox_retest_analyze.go` — MainBox 재돌파 분석기
- **study/**: `mainbox_retest_study.go`, `mainbox_retest_s1_study.go`, `mainbox_retest_refine_study.go`, `mainbox_retest_temporal.go` — 4종 연구 러너

### 6.4 HNS (헤드앤숄더) / MTop
- **cond/**: `sell_hns.go`, `sell_mtop.go` — HNS/MTop 매도 조건
- **stg/**: `hns_analyze.go`, `mpattern_analyze.go` — 패턴 분석기
- **study/**: `hns_scan.go`, `mtop_scan.go` — 패턴 스캔

### 6.5 Pullback / Regime / DoubleBump
- **cond/**: `buy_pullback.go`, `buy_regime.go`, `buy_doublebump.go`
- **stg/**: `pullback_analyze.go`
- **study/**: `pullback_scan.go`

### 6.6 STG11 (돌파후행) / STG6
- **cond/**: `buy_stg11.go`, `buy_stg6.go`
- **rules/**: `buy_stg11_15m.yaml`, `grid_stg11.yaml`

### 6.7 WGC (W바텀 GoldenCross)
- **stg/**: `wgc_analyze.go`
- **study/**: `wgc_scan.go`

### 6.8 Overlay Density (W중력)
- **stg/**: `overlay_density.go` — W중력 밀도 게이트
- **rules/**: `overlay_wdefbox.yaml`

### 6.9 Trigger Scan (범용 트리거 측정)
- **stg/**: `trigger_scan.go` — Trigger×Condition 조합 측정 엔진
- **study/**: `trigger_scan.go` — 연구 러너

---

## 7. 연구 도구 (study/) — 전체 22종

### 7.1 핵심 백테스트 (6종)
| 파일 | CLI 명령 | 설명 |
|------|----------|------|
| `grid.go` | `gridtest` | 파라미터 그리드 서치 |
| `edge.go` | `edgetest`, `baseline` | 조건별 전방 수익률 + Welch t-검정 |
| `walk_forward.go` | `walkforward` | IS/OOS 슬라이딩 워크포워드 |
| `baseline_30m.go` | `baseline30m` | 30분봉 베이스라인 |
| `breakdown.go` | `breakdown_study` | 돌파/이탈/회복 이벤트 분석 |
| `event_study.go` | `strategy_study` | YAML 전략 매수/매도 이벤트 분석 |

### 7.2 패턴 스캔 (8종)
| 파일 | CLI 명령 | 설명 |
|------|----------|------|
| `wbottom_scan.go` | `wbottom_scan` | W-bottom (%B) 패턴 |
| `miiib_scan.go` | `miiib_scan` | MIIIb_WBottomBox 신호 |
| `wdefbox_scan.go` | `wdefbox_scan` | W+DefBox 결합 신호 |
| `combined_scan.go` | `combined_scan` | WD+S1 합성 전략 |
| `mtop_scan.go` | `mtop_scan` | MTop 패턴 |
| `hns_scan.go` | `hns_scan` | HNS 패턴 |
| `pullback_scan.go` | `pullback_scan` | Pullback |
| `wgc_scan.go` | `wgc_scan` | WGC 패턴 |

### 7.3 Volume Wave (5종)
| 파일 | CLI 명령 | 설명 |
|------|----------|------|
| `volume_wave_scan.go` | `volume_wave_scan` | 신호 스캔 |
| `volume_wave_matrix.go` | `volume_wave_matrix` | 매트릭스 분석 |
| `volume_wave_chart.go` | `volume_wave_chart` | 차트 샘플 생성 |
| `volume_wave_box_study.go` | `volume_wave_box_study` | Box 결합 스터디 |
| `volume_wave_strict_study.go` | `volume_wave_strict_study` | 엄격 조건 스터디 |

### 7.4 기타 연구 (3종)
| 파일 | CLI 명령 | 설명 |
|------|----------|------|
| `pair.go` | `pairtest` | 페어 트레이딩 검증 |
| `trigger_scan.go` | `trigger_scan` | 범용 Trigger Scan |
| `stats.go` | (내부) | 승률/PF/MDD 통계 헬퍼 |

---

## 8. Ablation 실험 체계 (`rules/ablation/` — 34개)

Ablation은 전략의 어떤 구성요소가 실제 성능에 기여하는지 검증하는 실험 설계:

| 유형 | 파일 수 | 예시 |
|------|---------|------|
| S06 전용 분석 | 13개 | `strategy_s06_only.yaml`, `strategy_s06_no_defcount.yaml`, `strategy_s06_p_*.yaml` (5개 pair-wise) |
| Sell 조건 제거 | 4개 | `sell_strategy1_no_mbbwff.yaml`, `sell_strategy1_no_technical.yaml`, `sell_notechnical.yaml`, `sell_mh20_notechnical.yaml` |
| MinHold 변형 | 4개 | `sell_minhold3.yaml`, `sell_minhold5.yaml`, `sell_minhold10.yaml`, `sell_minhold20.yaml` |
| Position-only 변형 | 10개 | `sell_strategy1_posOnly_mh10.yaml` ~ `mh60.yaml`, `cf_t005` ~ `cf_t05` |
| WDefBox 전용 | 1개 | `sell_wdefbox_dstop20.yaml` |
| 복합 실험 | 2개 | `strategy_s06_combined.yaml`, `strategy_s06_star.yaml` |

---

## 9. 복합 시간봉 분석 (`combined_analyze.go`)

일봉과 15분봉을 결합한 분석:
1. 일봉 분석 결과 (주요 추세/DefBox)
2. 15분봉 분석 결과 (세부 진입 타이밍)
3. 두 시간 프레임 신호 결합 → 최종 매매 결정

- `cond/buy_indicator_15m.go`: 15분봉 전용 지표 조건
- `stg/sell_15m.go`: 15분봉 매도 평가

---

## 10. 설계 평가

### 강점
1. **YAML 룰 엔진**: 전략 추가/수정이 코드 변경 없이 가능 → 연구 속도 향상
2. **5-Path 매도**: Critical → Composite → Extension → Expiry → Individual 계층적 의사결정
3. **부분 매도**: weight 기반 점진적 청산 → 리스크 관리 정교화
4. **Ablation 체계**: 34개 변형 YAML로 전략 컴포넌트 기여도 정량 평가
5. **C# 충실 포팅**: 원본 Stock1 로직 그대로 Go로 이식 → 검증된 로직 신뢰성
6. **Rolling Sum O(N)**: 대량 캔들 분석에도 효율적인 지표 계산
7. **Trigger/Armed Trigger**: 코드 변경 없이 YAML에서 무한 전략 조합 가능
8. **연구 러너 22종**: 그리드→엣지→워크포워드→패턴스캔 파이프라인 완비

### 약점 / 개선 포인트
1. **Same-Candle Fill 가정**: 매수 신호와 체결이 동일 캔들에서 발생 (실제로는 다음 캔들 시가 체결이 현실적)
2. **고정 파라미터**: RSI(14), BB(20,2σ) 등이 코드에 하드코딩 → YAML에서 조정 불가
3. **15분봉 결합도**: `combined_analyze.go`가 일봉-15분봉을 단단히 결합 → 다른 시간봉 추가 어려움
4. **DefBox 단일 가정**: 한 종목에 한 번에 하나의 DefBox만 존재 → 다중 진입 시나리오 제한
5. **백테스트 분리 부재**: `study/`가 CLI에서만 실행 가능 → 라이브러리화 미흡
6. **연구간 중복**: Volume Wave, MainBox Retest, Descending Trendline 등 유사한 분석기 구조 반복

### C# 미포팅 항목
- VirtualTrading / 백테스트 헬퍼 클래스
- CandlePatternEvaluator 일부 미사용 패턴 함수
- 상세: `/home/feihong/code/Jarvis/project/RESTGo/csharp-porting-gap.md`

---

## 11. 발전 방향 제안

| 영역 | 제안 | 우선순위 |
|------|------|----------|
| **Next-Candle Fill** | 매수/매도 신호를 다음 캔들 시가로 체결 → 현실적 슬리피지 반영 | HIGH |
| **지표 파라미터 YAML화** | RSI period, BB multiplier 등을 YAML에서 조정 가능하게 | MEDIUM |
| **백테스트 라이브러리화** | study 패키지를 CLI 독립적 라이브러리로 분리 → 자동화 가능 | MEDIUM |
| **Multi-DefBox** | 동시 다중 DefBox 지원 → 더 많은 진입 기회 포착 | LOW |
| **멀티 타임프레임 추상화** | combined_analyze를 일반화된 멀티 TF 프레임워크로 확장 | LOW |
| **분석기 공통화** | 유사한 분석기(Volume Wave, MainBox Retest 등) 구조 통합 | LOW |
