# RESTGo 금융 분석 로직 상세

> 작성자: Claude Code AI (pi coding agent)
> 날짜: 2026-06-25
> 대상: RESTGo 프로젝트의 매수·매도 분석 로직 전체

---

## 1. 분석 파이프라인 개요

```
KIS2 DB 캔들 조회
  → indicator 패키지: 기술적 지표 계산 (MA, Bollinger, RSI 등 O(N))
  → box 패키지: 곡률 분석 → Box 생성 → DefBox 생성
  → stg.Analyze(): 메인 분석 루프
       ├── checkDefBoxBreakout(): 돌파 게이트 (가격 + 거래대금 + ATR)
       ├── buy_rule_engine.go: YAML 매수 룰 평가 (돌파 캔들 1회)
       ├── buy_followup.go: S13~S20 후속 매수 처리
       └── sell_rule_engine.go: 5-Path 매도 평가 (매 캔들)
  → AnalysisResult 반환 (신호, 포지션, 수익률)
```

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
| **MA5/20/60/120** | candle_processor.go | 5,20,60,120 | 추세 방향, 지지/저항 |
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

### 4.1 YAML 룰 엔진 (`stg/buy_rule_engine.go`)

```yaml
# rules/strategy1.yaml 구조
strategies:
  - name: "S01_SingleDefBuy"
    def_count: 1           # DefBox 연결 MainBox 정확 개수
    when: [...]            # AND 조건 목록
    when_not: [...]        # NOT 조건 목록
    any_of: [...]          # OR 조건 목록
    signal: "즉시매수"
```

**평가 순서**: 룰은 **정의된 순서대로 평가, 첫 매칭이 승리** → 엄격한 전략을 위에 배치

### 4.2 조건 카테고리

| 카테고리 | 파일 | 조건 수 | 내용 |
|----------|------|---------|------|
| **Box 구조** | `buy_conditions.go` | 12개 | DefBox 돌파, MainBox 관계, Box 밀도 |
| **MA/패턴** | `buy_conditions_extra.go` | +α | MA20/MA60 관계, 캔들패턴, 관통, MultiDef |
| **지표 기반** | `buy_indicator.go` | 16개 | RSI 5종, Bollinger 5종, MA 6종 |
| **오실레이터** | `buy_oscillator.go` | +α | 관통 옵션, 공용 오실로 헬퍼 |
| **FollowUp** | `buy_followup.go` | +α | ShortRange, 거래대금 게이트, 재진입 |

### 4.3 돌파 게이트 (`stg/analyzer.go`)

```
checkDefBoxBreakout():
  1. 가격 돌파 확인 (DefboxPrice 상향 돌파)
  2. 거래대금 돌파 (IsVolumeBreakout)
  3. ATR 변동성 조건
  → 3가지 모두 충족 시에만 돌파 인정
```

룰 평가는 **돌파 캔들에서만 1회** 수행. 이후 캔들은 ShortRange 사후 평가만 진행.

### 4.4 후속 매수 (FollowUp / REST2)

| 범위 | 유형 | 설명 |
|------|------|------|
| **S13~S16** | 후보군1 상태머신 | `DetermineBuySignal`: DefBox 이후 조건 관찰 → 진입 결정 |
| **S17~S20** | 재진입 | 완전 청산 후 재진입 조건 평가 |
| **게이트** | `BuyOn` / `SellHelper` | 실제 매수 실행 여부 및 청산 후 재진입 방지 |

### 4.5 중복 신호 방지

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

### 5.2 매도 조건 21종

#### Path 1: Critical (1개)
| 조건 | 파일 | 설명 |
|------|------|------|
| `IsCriticalFailure` | sell_helpers.go | 급락 + 거래량 폭증 + 누적 하락 + MA 반전 |

#### Path 2: Composite (합산)
Composite Eligible 조건 7개: MainBoxBreakdown, MainBoxPersistentBreakdown, WeakFoundationFail, TrendEntryFail1/2, MA5MA20DeadCross, AdaptiveStopLoss

#### Path 3: Extension (1개)
| 조건 | 파일 | 설명 |
|------|------|------|
| `IsMA5BreakdownDuringExtension` | sell_holding_extension.go | 연장 중 MA5 붕괴 |

#### Path 4: Expiry (1개)
| 조건 | 파일 | 설명 |
|------|------|------|
| `IsPeriodExpired` | sell_helpers.go | 최대 보유기간 도달 (기본 20일) |

#### Path 5: Individual (Priority 순, 18개)

| Priority | 조건 | Weight | 카테고리 |
|----------|------|--------|----------|
| 1 | EarlyDrop | 0.30 | Loss |
| 2 | EarlyMainBoxBreak | 0.50 | Loss |
| 2 | BBSqueezeExpansion | 0.25 | Loss |
| 3 | TrendEntryFail1 | 0.50 | Technical |
| 4 | MainBoxBreakdown | 0.50 | Technical |
| 5 | MainBoxPersistentBreakdown | 0.50 | Technical |
| 6 | TrendEntryFail2 | 0.50 | Technical |
| 7 | WeakFoundationFail | 0.50 | Technical |
| 8 | MA5MA20DeadCross | 0.50 | Technical |
| 9 | AdaptiveStopLoss | 0.50 | Technical |
| 9 | ConsecutiveNegative | 0.30 | Technical |
| 9 | GapUpTakeProfit | 0.50 | Profit |
| 10 | BBUpperBreakoutProfit | 0.50 | Profit |
| 10 | MA5Breakdown | 0.50 | Technical |
| 11 | MainBoxWaveFailure | 0.30 | Technical |
| 12 | BBSqueezeFailure | 0.25 | Technical |
| 12 | MBBWidthFailure | 0.25 | Technical |
| 14 | CloseBelowMA20 | 0.25 | Technical |

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

## 6. 연구 도구 (study/)

### 6.1 Walk-Forward 분석 (`walk_forward.go`)
- 시간 순차적 학습/테스트 분할
- 전략 파라미터 최적화 검증

### 6.2 Event Study (`event_study.go`)
- DefBox 돌파/하향돌파 이후 가격 분포 측정
- IsDefBoxBreakdown: 2026-06-17 신규 추가 (돌파의 거울 함수)

### 6.3 Grid 전략 (`grid.go`)
- YAML 정의 매개변수 기반 그리드 매매
- `grid_stage2.yaml`, `grid_w10b.yaml` 설정

### 6.4 패턴 스캔
- `wbottom_scan.go`: W-bottom 패턴 탐지
- `wdefbox_scan.go`: W-DefBox 조합 탐지
- `miiib_scan.go`: MIIIB 패턴 탐지
- `combined_scan.go`: 복합 시간봉 스캔

### 6.5 Pair 트레이딩 (`pair.go`)
- 두 종목 간 스프레드 기반 트레이딩
- `box/candle_loader_pair.go`에서 캔들 쌍 로드

---

## 7. Ablation 실험 체계 (`rules/ablation/`)

Ablation은 전략의 어떤 구성요소가 실제 성능에 기여하는지 검증하는 실험 설계:

| 유형 | 파일 수 | 예시 |
|------|---------|------|
| S06 전용 분석 | 13개 | `strategy_s06_only.yaml`, `strategy_s06_no_multidef.yaml` 등 |
| Sell 조건 제거 | 2개 | `sell_strategy1_no_mbbwff.yaml`, `sell_strategy1_no_technical.yaml` |
| Position-only 변형 | 8개 | `sell_strategy1_posOnly_mh10.yaml` ~ `mh60.yaml` |
| 복합 실험 | 5개 | `strategy_s06_combined.yaml`, `strategy_s06_p_*.yaml` |

---

## 8. 복합 시간봉 분석 (`combined_analyze.go`)

일봉과 15분봉을 결합한 분석:
1. 일봉 분석 결과 (주요 추세/DefBox)
2. 15분봉 분석 결과 (세부 진입 타이밍)
3. 두 시간 프레임 신호 결합 → 최종 매매 결정

- `cond/buy_indicator_15m.go`: 15분봉 전용 지표 조건
- `stg/sell_15m.go`: 15분봉 매도 평가

---

## 9. 설계 평가

### 강점
1. **YAML 룰 엔진**: 전략 추가/수정이 코드 변경 없이 가능 → 연구 속도 향상
2. **5-Path 매도**: Critical → Composite → Extension → Expiry → Individual 계층적 의사결정
3. **부분 매도**: weight 기반 점진적 청산 → 리스크 관리 정교화
4. **Ablation 체계**: 28개 변형 YAML로 전략 컴포넌트 기여도 정량 평가
5. **C# 충실 포팅**: 원본 Stock1 로직 그대로 Go로 이식 → 검증된 로직 신뢰성
6. **Rolling Sum O(N)**: 대량 캔들 분석에도 효율적인 지표 계산

### 약점 / 개선 포인트
1. **Same-Candle Fill 가정**: 매수 신호와 체결이 동일 캔들에서 발생 (실제로는 다음 캔들 시가 체결이 현실적)
2. **고정 파라미터**: RSI(14), BB(20,2σ) 등이 코드에 하드코딩 → YAML에서 조정 불가
3. **15분봉 결합도**: `combined_analyze.go`가 일봉-15분봉을 단단히 결합 → 다른 시간봉 추가 어려움
4. **DefBox 단일 가정**: 한 종목에 한 번에 하나의 DefBox만 존재 → 다중 진입 시나리오 제한
5. **백테스트 분리 부재**: `study/`가 CLI에서만 실행 가능 → 라이브러리화 미흡

### C# 미포팅 항목
- VirtualTrading / 백테스트 헬퍼 클래스
- CandlePatternEvaluator 일부 미사용 패턴 함수
- 상세: `/home/feihong/code/Jarvis/project/RESTGo/csharp-porting-gap.md`

---

## 10. 발전 방향 제안

| 영역 | 제안 | 우선순위 |
|------|------|----------|
| **Next-Candle Fill** | 매수/매도 신호를 다음 캔들 시가로 체결 → 현실적 슬리피지 반영 | HIGH |
| **지표 파라미터 YAML화** | RSI period, BB multiplier 등을 YAML에서 조정 가능하게 | MEDIUM |
| **백테스트 라이브러리화** | study 패키지를 CLI 독립적 라이브러리로 분리 → 자동화 가능 | MEDIUM |
| **Multi-DefBox** | 동시 다중 DefBox 지원 → 더 많은 진입 기회 포착 | LOW |
| **멀티 타임프레임 추상화** | combined_analyze를 일반화된 멀티 TF 프레임워크로 확장 | LOW |
