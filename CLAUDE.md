# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 프로젝트 개요 및 목적

**RESTGo**는 Go로 작성된 CLI 기반 운영 도구이자 퀀트 연구 플랫폼입니다. 크게 네 가지 역할을 합니다:
1. **다중 MSSQL 쿼리** — 4개 DB(key/han/var/KIS2)에 직접 쿼리 실행
2. **주식 Box/매수·매도 분석** — C# `Stock1` 프로젝트의 핵심 분석 로직을 Go로 포팅한 `box/` + `indicator/` + `cond/` + `stg/` 패키지. 매수·매도 전략은 `rules/*.yaml`의 YAML 룰 엔진으로 정의
3. **백테스트·전략 연구** — `study/` 패키지의 러너 12종 (그리드 서치, 엣지 검증, walk-forward, 페어 트레이딩, W패턴 스캔 등). 대상 데이터는 국내 일봉(KIS2 1.5년 / hannam 16년), 해외 일봉, Upbit 암호화폐 15분봉
4. **Python 분석 스크립트 실행** — `py/` 디렉토리의 차트·백테스트·테마 전략 스크립트를 Go CLI에서 호출

모든 DB 접속 정보는 AES-256-GCM으로 암호화되어 `config.yaml`에 저장됩니다.

패키지 의존 방향은 단방향 계층입니다: `box`/`cond`/`indicator`/`console`(인프라) → `stg`(전략 엔진, box·cond만 의존) → `study`(연구 러너) → `stock`(CLI 라우터) → `main.go`

## 주요 명령어

```bash
# 빌드
go build

# SQL 쿼리
./RESTGo sqlquery "SELECT TOP 10 * FROM SomeTable"
./RESTGo sqlquery -db han "SELECT * FROM Users"
./RESTGo sqlquery -db KIS2 "EXEC sp_GetData @param='value'"

# 주식 Box/매수·매도 분석 (KIS2 DB에서 캔들 조회)
./RESTGo stock analyze <종목코드> [일수=250]
# 예) ./RESTGo stock analyze 005930 250

# 전 종목 배치 분석 → zpicture/batch_signals.json 저장 (고루틴 20개 병렬)
./RESTGo stock batch [일수=250]

# 매수/매도 전략 YAML 교체 (기본: rules/strategy1.yaml / rules/sell_default.yaml)
RESTGO_BUY_RULES=rules/buy_indicator.yaml RESTGO_SELL_RULES=rules/sell_positive_only_mh25.yaml ./RESTGo stock analyze 005930 250

# ── 연구/백테스트 명령 (study/ 패키지, DB 필요) ──
./RESTGo stock gridtest <grid_yaml> [output_json]          # 파라미터 그리드 서치 (rules/grid_*.yaml)
./RESTGo stock edgetest [markets_csv] [output_json]        # 조건별 돌파 엣지 검증 (Welch t-검정)
./RESTGo stock baseline [markets_csv] [output_json] [strategy_yaml]   # 베이스라인 백테스트 (결정성 검증 포함)
./RESTGo stock walkforward <market> [output_json] [strategy_yaml]     # IS/OOS 슬라이딩 워크포워드 검증
./RESTGo stock pairtest                                    # 페어 트레이딩 검증 (상관·ADF)
./RESTGo stock baseline30m [markets_csv] [output_json] [strategy_yaml]  # 30분봉 베이스라인
./RESTGo stock breakdown_study                             # 돌파/이탈/회복 이벤트 사후 분석
./RESTGo stock strategy_study                              # YAML 전략 매수/매도 이벤트 스터디
./RESTGo stock wbottom_scan [--foreign-jp|--foreign-cn|--foreign-hk] [n]   # W바텀(%B) 패턴 예시 수집
./RESTGo stock miiib_scan [--foreign-*] [--max N] [--out path]             # MIIIb_WBottomBox 신호 수집
./RESTGo stock wdefbox_scan [--foreign-*|--hannam] [--max N] [--candles N] [--out path] [--defbox-only]  # W+DefBox 결합 신호 스캔
./RESTGo stock combined_scan [--foreign-*] [--max N] [--out path]          # WD+S1 합성 전략 스캔
./RESTGo stock densitygate [일자=오늘] [overlay_yaml]                        # W중력 밀도 게이트 판정 (han DB StrategySignalDaily, 기본 rules/overlay_wdefbox.yaml, RESTGO_OVERLAY_RULES로 교체)
./RESTGo stock mtop_scan|hns_scan|pullback_scan [--max N] [--candles N] [--out path]  # 패턴 신호 스캔 + 전방수익률 엣지 측정 (3종 모두 기각된 실험 — zpicture/*_report.md 참조, 러너는 재사용용 보존)
./RESTGo stock trigger_scan --trigger <이름> [--when C1,C2] [--when-not ...] [--cooldown N] [--set K=V]  # 범용 트리거×조건 조합 측정 (일반·armed 트리거, engine-parity 검증됨 — 새 조합 실험의 기본 도구)

# Python 분석 스크립트 실행 (host: /home/feihong/code/REST/RESTGo/venv)
./RESTGo py box_chart <종목코드>
./RESTGo py box_batch
./RESTGo py batch_chart
./RESTGo py tg_send
./RESTGo py <스크립트경로> [인수...]   # 임의 스크립트 직접 실행

# 테스트 (DB 불필요 — cond/indicator/stg는 순수 함수. box/study/stock은 테스트 없음)
go test ./...

# 의존성 정리
go mod tidy
```

지원 DB: `key` (기본), `han`, `var`, `KIS2`

## 핵심 아키텍처

### 초기화 흐름 (`console/init.go`)

`console.Init()`이 호출 순서를 보장합니다:
1. `config.yaml` 로드 및 복호화 → DB 접속 정보 획득
2. 4개 MSSQL 연결 풀 생성 (각 max 100 open / 20 idle)
3. `key` DB의 `KeyValueStore` 테이블에서 `han`·`var`·`KIS2` DB 접속 정보를 동적으로 읽어 연결
4. RabbitMQ 세션 초기화 (큐: LOG, FEILOGIC, slice2DB, KISData)
5. 로그 배치 프로세서 goroutine 시작 (100건 또는 100ms 단위로 RabbitMQ 전송)

`main.go`에서 `defer console.Cleanup()`으로 종료 시 모든 리소스를 안전하게 해제합니다.

### 암호화 계층 (`console/crypto.go`)

`config.yaml`의 크리덴셜은 2단계로 암호화됩니다:
1. 원본 문자열에 랜덤 패딩(앞 5자 + 뒤 7자) 삽입
2. AES-256-GCM 암호화 → Base64 인코딩

복호화는 역순: Base64 → AES-GCM 복호화 → 패딩 제거. 크리덴셜 추가·수정 시 반드시 `Encrypt()`를 통해 암호화해야 합니다.

### 이중 로깅 시스템 (`console/logger.go`, `console/structured_logger.go`)

| 함수 | 내부 방식 | 용도 |
|------|-----------|------|
| `Log()`, `LogError()` 등 | 커스텀 배치 큐 | 레거시 텍스트 로그 |
| `LogInfo()`, `LogErr()` 등 | Zap JSON | 구조화 로그 |
| `Tele()` | RabbitMQ Sender="KIS-tele" | Telegram 알림 |

로그 레벨은 `config.yaml`의 `loglevel` 값으로 제어됩니다: `DEBUG` > `INFO` > `ERROR` > `TEST`.

### 싱글턴 전역 리소스

모든 핵심 리소스는 패키지 레벨 싱글턴입니다:
- `console.MsConn` — MSSQL 연결 풀 (`database.go`)
- `console.RabbitMQSession` — RabbitMQ 세션 (`rabbitmq.go`)
- `console.ZapLogger` — Zap 로거 (`structured_logger.go`)

`console/init.go`에서 일괄 초기화하므로, `console` 패키지 함수 호출 전에 반드시 `console.Init()`이 실행된 상태여야 합니다.

### SQL 쿼리 실행 (`console/sqlquery.go`)

- `SELECT` / `WITH` / `EXEC` / `SP_*` → 컬럼 너비 자동 조정된 테이블 형식 출력
- `INSERT` / `UPDATE` / `DELETE` → 영향받은 행 수 출력
- NULL 값은 `<NULL>` 문자열로 대체

### 주식 분석 패키지 구조 (C# `Stock1` 기준)

C# 참조 프로젝트: `ssh feihong@192.168.3.120:/home/feihong/code/REST/RESTG/`

| Go 패키지 | C# 대응 | 역할 |
|-----------|---------|------|
| `box/types.go` | `vo/Box.cs`, `vo/Candle.cs` | Box·Candle 데이터 구조체 |
| `box/context.go` | `biz/Contexts/TradingContext.cs` | 분석 루프 공유 상태 |
| `box/curvature.go` | `biz/BoxEvaluators/CurvatureAnalyzer.cs` | 곡률 분석 + DefBox 생성 |
| `box/curve.go` | `biz/BoxEvaluators/CurveConditionEvaluator.cs` | 추세 전환 조건 |
| `box/defbox.go` | `biz/BoxEvaluators/DefBoxConditionEvaluator.cs` | DefBox 생성 조건 |
| `box/box_price.go` | `biz/BoxEvaluators/BoxPriceCalculator.cs` | 구간 최고/최저가 계산 |
| `box/box_creation.go` | `biz/BoxEvaluators/BoxCreationService.cs` | Box 생성·추가 |
| `box/sell_types.go` | — | 매도 측 타입 (SellDecision 등) |
| `box/candle_loader.go` | — (DB 커스텀) | 일봉 로더: KIS2(`FetchCandles`, 1.5년) + hannam(`FetchCandlesHannam`, 16년) + 해외(`FetchCandlesForeign`) + 종목 리스트 조회 |
| `box/candle_loader_upbit.go` | — (Go 신규) | Upbit 암호화폐 15분봉 로더 (`FetchUpbitCandles15m`, TUF DB `candles_15m`) |
| `box/candle_loader_pair.go` | — (Go 신규) | Upbit 페어 15분봉 로더 (두 마켓 시각 정렬 → `PairedCandle`) |
| `indicator/candle_processor.go` | `common/CandleProcessor.cs` | 스케일링·MA(5/20/60/120/200)·기울기·ATR 단일 패스 계산 (rolling sum O(N)) |
| `indicator/bollinger.go` | `common/CandleProcessor.cs` (Bollinger 영역) | Bollinger Bands (rolling sum of squares) |
| `indicator/rsi.go` | — (Go 신규) | RSI (Wilder, period=14) |
| `indicator/` 기타 10종 | — (Go 신규) | EMA·MACD·ADX·Donchian·Keltner·Stochastic·OBV·SuperTrend·VWAP + `pair.go`(페어 스프레드·Pearson 상관·ADF 정상성 검정) |
| `cond/buy_conditions.go` | `biz/Evaluators/BoxConditionEvaluator.cs` | Box 구조 매수 조건 함수 |
| `cond/buy_conditions_extra.go` | `biz/Evaluators/MovingAverageConditionEvaluator.cs`, `CandlePatternEvaluator.cs`, `PenetrationEvaluator.cs` 등 | MA·캔들패턴·관통·MultiDef 조건 |
| `cond/buy_oscillator.go` | `biz/Evaluators/OscillatorEvaluator.cs` | 오실레이터·관통 옵션 + 공용 오실로 헬퍼 |
| `cond/buy_followup.go` | `BuyDecisionProcessor.FollowUp.cs` 의존 조건 + `VolumeConditionEvaluator.cs` | ShortRange·거래대금 게이트·재진입 조건 |
| `cond/buy_indicator.go` | — (Go 신규) | 지표 기반 매수 조건 16종 (RSI 5·Bollinger 5·MA 6) |
| `cond/buy_indicator_15m.go` | — (Go 신규) | 15분봉(암호화폐)용 매수 조건 (Donchian·EMA·볼린저 Method I~III·W바텀 Box 등) |
| `cond/sell_*.go` (10개) | `biz/SellEvaluators/` 전체 | 매도 조건 함수 (`sell_` prefix로 분류) |
| `stg/analyzer.go` | `biz/StockAnalyzer.cs` + `biz/BFunction.cs` (BLogic) | 분석 메인 루프·돌파 게이트 — 모든 백테스트의 공통 엔진 |
| `stg/buy_rule_engine.go` | `biz/Processors/BuyDecisionProcessor.cs` (개념적 대응) | YAML 룰 평가 엔진 |
| `stg/buy_followup.go` | `BuyDecisionProcessor.Options.cs`(REST2) + `FollowUp.cs` | S13~S20 후속 매수 처리 |
| `stg/buy_conditions_registry.go` | — | 조건명 → `cond` 함수 매핑 등록 |
| `stg/sell_*.go` (6개) | `SellDecisionEngine.cs` + `SellSignalCollector.cs` + `SFunction.*` | 매도 룰 엔진·5-Path 결정·부분 매도·지속성 추적·15분봉 청산(`sell_15m.go`) |
| `stg/buy_settings.go` | `vo/Settings.cs` | 분석 설정값 (`ApplySettingsOverrides`로 그리드 서치 오버라이드) |
| `stg/types.go` | `vo/BuySignalCondition.cs`, `vo/AnalysisResult.cs` | 신호·결과 타입 (Positions 포함) |
| `stg/wpattern_analyze.go` | — (Go 신규) | 순수 W바텀(MIIIb) 패턴 분석기 `WPatternAnalyze` (DefBox 게이트 없음) |
| `stg/wpattern_defbox.go` | — (Go 신규) | W바텀+DefBox 결합 분석기 `WDefBoxAnalyze` (W신호 50% 진입 + 20일 내 DefBox 돌파 시 추가 50%) |
| `stg/combined_analyze.go` | — (Go 신규) | WD(분할진입) + S1(DefBox 단독 100% 진입) 합성 신호 단일 패스 탐지 `CombinedAnalyze` |
| `study/` | — | 연구 러너 12종 (아래 "연구 인프라" 절 참조) |
| `stock/handler.go` | — | CLI 라우터 — `analyze`/`batch`는 직접 처리, 연구 명령은 `study.*`로 위임 |
| `py/` | — | Python 차트·백테스트·테마 전략 스크립트 |
| `console/py_runner.go` | — | Go→Python 실행 래퍼 |

**cond/·stg/ 파일명 규칙**: `buy_*` = 매수 측, `sell_*` = 매도 측, prefix 없음 = 공용(`stg/analyzer.go` 통합 루프, `stg/types.go` 결과 타입), `*_test.go` = 해당 파일 테스트(Go 규칙상 같은 폴더).

**C# 미포팅 항목**: `CandlePatternEvaluator`의 미사용 패턴 함수들과 `VirtualTrading`/백테스트 헬퍼만 남음 (모두 현행 파이프라인 미사용 — 상세는 `/home/feihong/code/Jarvis/project/RESTGo/csharp-porting-gap.md`)

### rules/ 디렉토리 구성 (2026-07-03 네이밍 정리 — 상세는 `rules/README.md`)

파일명 규칙: `buy_*` 매수 / `sell_*` 매도 / `grid_*` 그리드 서치 / `ablation/` 소거 실험 / `archive/` 스냅샷(수정 금지)

| 파일/디렉토리 | 평가 방식 | 내용 |
|--------------|-----------|------|
| `strategy1.yaml` | on_breakout | **기본 매수 전략** — C# REST1 포팅 (Box 구조 8룰 + Core-3 게이트). C# 매수 정합 기준이라 이름·형식 유지 |
| `buy_indicator.yaml` | trigger | (구 strategy2) DefBox 돌파 순간 지표(RSI/BB/MA) 확증 6종 (I01~I06) |
| `buy_bb_pure.yaml` | trigger | (구 strategy_bb_pure) John Bollinger 3대 방법 (MIIIb/MIII W바텀 → MII 밴드워크 → MI 스퀴즈) |
| `buy_bb_hybrid.yaml` | trigger | (구 strategy_bb_hybrid) Box+BB 복합 (SH1~SH4) |
| `buy_trigger_example.yaml` | trigger | 트리거 문법 예시 3종 (문법 설명 주석 포함) |
| `buy_crypto_15m.yaml` | per_candle | (구 strategy3) 암호화폐 15분봉 — 보류 영역, 추후 보완 예정 |
| `sell_default.yaml` | — | (구 sell_strategy1) **기본 매도** — 21룰 + 5-Path 결정 + 부분매도. 매도 로직은 재설계 예정 |
| `sell_positive_only.yaml`, `sell_positive_only_mh25.yaml` | — | 매도 변형 (양수 수익 구간만 / max_holding 25) |
| `grid_crypto_*.yaml` | — | `gridtest`용 파라미터 스윕 (암호화폐, 보류 영역) |
| `ablation/` (27개) | — | 소거 실험 — 매도 룰 제거·보유기간·회복임계 스윕 + s06 조건별 기여도 (실험 재현성 위해 구형식 유지) |
| `archive/` (14개) | — | 과거 전략 스냅샷 보관 — 수정 금지, 참고용 |

### 연구 인프라 (`study/` 패키지)

각 파일이 `stock` 서브커맨드 하나에 대응하는 `Handle*` 함수를 노출합니다. 백테스트 코어는 전부 `stg.AnalyzeWithRules`를 공유합니다.

| 파일 | 명령 | 역할 |
|------|------|------|
| `grid.go` | `gridtest` | 파라미터 조합 전개 → `stg.ApplySettingsOverrides`로 스윕, 재현성 검증(`verifyGridDeterminism`) 포함 |
| `edge.go` | `edgetest`, `baseline` | 조건 레지스트리 기반 조건별 전방 수익률 수집 + Welch t-검정, 베이스라인은 결정성 검증(동일 입력 2회 비교) 포함 |
| `walk_forward.go` | `walkforward` | 15분봉 기준 IS 6개월/OOS 2개월 슬라이딩 윈도우, OOS/IS 성과비(RatioPF·RatioAvgNet) 중앙값으로 과최적화 판정 |
| `pair.go` | `pairtest` | 페어 트레이딩 검증 (상관·ADF, stg 미의존 순수 지표 스터디) |
| `baseline_30m.go` | `baseline30m` | 30분봉 타임프레임 베이스라인 |
| `breakdown.go` | `breakdown_study` | 돌파/이탈/회복 이벤트 사후 분석 (stg 미의존) |
| `event_study.go` | `strategy_study` | YAML 전략 매수/매도 이벤트 스터디 |
| `wbottom_scan.go` / `miiib_scan.go` / `wdefbox_scan.go` / `combined_scan.go` | `*_scan` | W패턴 분석기 3종(`WPatternAnalyze`/`WDefBoxAnalyze`/`CombinedAnalyze`) 호출 → 전 종목 예시 JSON 덤프 |
| `stats.go` | (내부) | 승률/PF/MDD 등 공용 통계 헬퍼 |

연구 결과 JSON(베이스라인 등)은 `zpicture/`에 저장됩니다. 커밋 메시지가 연구 노트 역할을 하며, 실패한 실험도 "실패"로 명시해 커밋으로 보존하는 규율을 따릅니다.

### YAML 룰 엔진 (`stg/buy_rule_engine.go` + `rules/strategy1.yaml`)

매수 조건은 코드가 아닌 YAML로 정의합니다. 전략 추가·수정 시 코드 변경 없이 `rules/strategy1.yaml`만 편집하면 됩니다.

- 룰 필드: `def_count`(정확 일치) / `def_count_min`(이상) / `when`(AND) / `any_of`(OR) / `when_not`(NOT) / `signal`
- 조건명은 `stg/buy_conditions_registry.go`의 `init()`에서 `RegisterCondition()`으로 등록된 이름만 사용 가능. 새 조건 추가 시 ① `cond/`에 함수 구현 → ② 레지스트리에 등록 → ③ YAML에서 참조
- 룰은 **순서대로 평가되어 첫 매칭이 승리**하므로 엄격한 전략을 위에 배치
- **전략별 중복 신호 방지**: 같은 DefBox 구간에서 한 전략은 1회만 발화 (`ctx.LastBuySignalPosition`에 기록, DefBox 변경 시 리셋 — C# `LastBuySignalPosition_StrategyN` 포팅)
- **돌파 게이트** (`stg/analyzer.go` `checkDefBoxBreakout`): 가격 돌파 + 거래대금(`IsVolumeBreakout`) + ATR 모두 충족해야 돌파 인정. 룰 평가는 **돌파 캔들에서만** 1회 수행되고, 이후 캔들은 ShortRange 사후 평가만 한다 (C# BLogic 정렬)
- **FollowUp/REST2** (`stg/buy_followup.go`): REST2 S13~S16(`DetermineBuySignal`, 후보군1 상태머신) + S17~S20 재진입 처리. S15/S17/S18은 C#과 동일하게 사문(도달 불가) 게이트 보존
- **트리거(메인이벤트) 아키텍처** (`stg/trigger_registry.go`, 2026-07-03): 룰에 `trigger:` 필드를 지정하면 on_breakout/per_candle 대신 트리거 경로에서 평가된다 — 매 캔들 트리거(edge)를 확인하고 발화한 캔들에서만 when/when_not/any_of 평가. 중복 발화는 `once_per:` (defbox 기본 / cooldown / none)로 제어. 트리거는 반드시 **edge**(발생 순간에만 true)여야 하며 `RegisterTrigger()`로 등록 — level(상태) 조건과 레지스트리가 분리되어 있음. 등록된 트리거: `DefBoxBreakout`(stateless 복합 게이트), `PriceBreakout`(가격만), `WBottomBox`(W패턴 S-R-S 완성 순간), `BBLowerBreakdown`, `BBLowerReentry`, `BBSqueezeBreakout` (edge 함수는 `cond/buy_triggers.go`). 기존 on_breakout 룰(trigger 미지정)은 DamChecker 상태머신 경로 그대로 — batch A/B로 신호 동일성 검증 완료
- **armed(장전→발화) 트리거** (`stg/armed_trigger.go`·`armed_trigger_registry.go`, 2026-07-05): 상태를 갖는 2단계 패턴(패턴 완성=장전 → 유효기간 내 확인 이벤트=발화)을 `RegisterArmedTrigger()`로 등록하면 일반 트리거와 동일하게 YAML `trigger:`에서 사용 가능. 상태는 `ctx.ArmedTriggerState`에 저장, 룰 필터와 무관하게 매 캔들 틱. 등록: `MTopCollapse`/`HNSNecklineBreak`/`MA20PullbackBreakout`(3종 모두 단독 엣지 기각 — 조합 실험용). `RunArmedTrigger()`는 연구·검증용 단독 실행기 — 전용 분석기(stg/{mpattern,hns,pullback}_analyze.go)와 발화 동일성 검증 완료. 이로써 "패턴(트리거) × 상황 조건(when)"이 코드 없이 YAML로 자유 조합된다
- `stock/handler.go`가 `stg.LoadStrategy(buyRulesPath())` / `stg.LoadSellStrategyFile(sellRulesPath())`로 로드 — 기본 `rules/strategy1.yaml` / `rules/sell_default.yaml`, 환경변수 `RESTGO_BUY_RULES` / `RESTGO_SELL_RULES`로 교체 가능 (예: `RESTGO_BUY_RULES=rules/buy_indicator.yaml ./RESTGo stock analyze 005930 250`). **매수 룰 로드 실패 시** `stg/analyzer.go`의 하드코딩 fallback 로직이 조용히 사용되므로 주의 (매도 룰 실패는 `[warn]` 출력 후 매도 평가 비활성)
- **CriticalFailure 임계값** 등 전역 매도 설정은 `sell_default.yaml`에서 YAML로 오버라이드 가능

### Python 패키지 구조 (`py/`)

| 디렉토리 | 역할 |
|----------|------|
| `py/analysis/` | Box 차트 생성 (`box_chart.py`는 `zpicture/batch_signals.json` 소비), MA5 변곡 분석, W패턴/WDefBox 연구 스크립트 다수 (`wd_*_study.py`, `wbottom_chart.py` 등) |
| `py/batch/` | 차트 일괄 생성, Telegram 발송 |
| `py/strategy/theme/` | Box 분석과 독립적인 외국인 수급 기반 테마 전략 4종 (momentum/rotation/surge/sector_surge) |
| `py/backtest/` | 테마 전략 공통 백테스트 엔진 |
| `py/common/` | DB 연결 공통 모듈 |

### Python 실행 환경

- Python: `/home/feihong/code/REST/RESTGo/venv/bin/python3` (host 서버, RESTGo 전용 venv)
- 스크립트 루트: `/home/feihong/code/REST/RESTGo` (host 서버)
- `console/py_runner.go`의 `PythonBin`, `ProjectRoot` 상수로 관리

## 원격 서버 접속 정보

- `ssh feihong@192.168.3.120` → 이 컨테이너의 호스트 서버 (hostname: `white`)에 접속됩니다.
- SSH 키: `~/.ssh/id_ed25519` (ED25519, 공개키 등록 완료)
- C# 참조 프로젝트: `/home/feihong/code/REST/RESTG/Stock1/` (branch: `feature/multi-position-sell-strategy`)

## 주의사항

- **크리덴셜 평문 금지**: `config.yaml`에 접속 정보를 추가할 때 반드시 `crypto.go`의 `Encrypt()` 함수로 암호화한 값을 사용해야 합니다. 평문 저장 시 보안 사고로 이어집니다.
- **DB 추가 절차**: `han`, `var`, `KIS2` DB의 접속 정보는 코드가 아닌 `key` DB의 `KeyValueStore` 테이블에서 읽습니다. 새 DB 환경을 추가하려면 해당 테이블을 먼저 업데이트해야 합니다.
- **RabbitMQ 의존성**: 로깅 시스템이 RabbitMQ와 결합되어 있어, RabbitMQ 연결 실패 시 로그가 유실될 수 있습니다. `rabbitmq.go`의 재연결 로직이 있으나, 프로덕션 환경에서는 큐 가용성을 모니터링해야 합니다.
- **컴파일된 바이너리**: 저장소에 `RESTGo` 바이너리가 포함되어 있습니다. 코드 변경 후 반드시 `go build`로 재빌드해야 변경사항이 반영됩니다.
- **Go 버전**: `go.mod`에 `go 1.25.0`이 명시되어 있습니다. 로컬 Go 버전이 맞지 않으면 빌드 오류가 발생할 수 있습니다.


## 문서 작성 위치
이 프로젝트의 모든 docs 문서는 `/home/feihong/code/Jarvis/project/RESTGo/` 에 작성한다.
