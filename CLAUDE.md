# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 프로젝트 개요 및 목적

**RESTGo**는 Go로 작성된 CLI 기반 운영 도구입니다. 크게 세 가지 역할을 합니다:
1. **다중 MSSQL 쿼리** — 4개 DB(key/han/var/KIS2)에 직접 쿼리 실행
2. **주식 Box/매수·매도 분석** — C# `Stock1` 프로젝트의 핵심 분석 로직을 Go로 포팅한 `box/` + `indicator/` + `cond/` + `stg/` 패키지. 매수·매도 전략은 `rules/*.yaml`의 YAML 룰 엔진으로 정의
3. **Python 분석 스크립트 실행** — `py/` 디렉토리의 차트·백테스트·테마 전략 스크립트를 Go CLI에서 호출

모든 DB 접속 정보는 AES-256-GCM으로 암호화되어 `config.yaml`에 저장됩니다.

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

# Python 분석 스크립트 실행 (host: /home/feihong/code/REST/RESTGo/venv)
./RESTGo py box_chart <종목코드>
./RESTGo py box_batch
./RESTGo py batch_chart
./RESTGo py tg_send
./RESTGo py <스크립트경로> [인수...]   # 임의 스크립트 직접 실행

# 테스트 (DB 불필요 — cond/stg는 순수 함수)
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
| `box/candle_loader.go` | — (DB 커스텀) | KIS2 DB 캔들 조회 |
| `indicator/candle_processor.go` | `common/CandleProcessor.cs` | 스케일링·MA·ATR 계산 (rolling sum O(N)) |
| `indicator/bollinger.go` | `common/CandleProcessor.cs` (Bollinger 영역) | Bollinger Bands (rolling sum of squares) |
| `indicator/rsi.go` | — (Go 신규) | RSI (Wilder, period=14) |
| `cond/buy_conditions.go` | `biz/Evaluators/BoxConditionEvaluator.cs` | Box 구조 매수 조건 함수 |
| `cond/buy_conditions_extra.go` | `biz/Evaluators/MovingAverageConditionEvaluator.cs`, `CandlePatternEvaluator.cs`, `PenetrationEvaluator.cs` 등 | MA·캔들패턴·관통·MultiDef 조건 |
| `cond/buy_oscillator.go` | `biz/Evaluators/OscillatorEvaluator.cs` | 오실레이터·관통 옵션 + 공용 오실로 헬퍼 |
| `cond/buy_followup.go` | `BuyDecisionProcessor.FollowUp.cs` 의존 조건 + `VolumeConditionEvaluator.cs` | ShortRange·거래대금 게이트·재진입 조건 |
| `cond/buy_indicator.go` | — (Go 신규) | 지표 기반 매수 조건 16종 (RSI 5·Bollinger 5·MA 6) |
| `cond/sell_*.go` (10개) | `biz/SellEvaluators/` 전체 | 매도 조건 함수 (`sell_` prefix로 분류) |
| `stg/analyzer.go` | `biz/StockAnalyzer.cs` + `biz/BFunction.cs` (BLogic) | 분석 메인 루프·돌파 게이트 |
| `stg/buy_rule_engine.go` | `biz/Processors/BuyDecisionProcessor.cs` (개념적 대응) | YAML 룰 평가 엔진 |
| `stg/buy_followup.go` | `BuyDecisionProcessor.Options.cs`(REST2) + `FollowUp.cs` | S13~S20 후속 매수 처리 |
| `stg/buy_conditions_registry.go` | — | 조건명 → `cond` 함수 매핑 등록 |
| `stg/sell_*.go` (5개) | `SellDecisionEngine.cs` + `SellSignalCollector.cs` + `SFunction.*` | 매도 룰 엔진·5-Path 결정·부분 매도 |
| `stg/buy_settings.go` | `vo/Settings.cs` | 분석 설정값 |
| `stg/types.go` | `vo/BuySignalCondition.cs`, `vo/AnalysisResult.cs` | 신호·결과 타입 (Positions 포함) |
| `rules/strategy1.yaml` | — | 매수 전략 정의 (SingleDef 5종 + MultiDef 3종) |
| `rules/strategy2.yaml` | — | 지표 기반 매수 전략 6종 (I01~I06) — `RESTGO_BUY_RULES`로 교체 실행 |
| `rules/sell_strategy1.yaml` | — | 매도 룰 21종 + Composite/전역 설정 |
| `py/` | — | Python 차트·백테스트·테마 전략 스크립트 |
| `console/py_runner.go` | — | Go→Python 실행 래퍼 |

**cond/·stg/ 파일명 규칙**: `buy_*` = 매수 측, `sell_*` = 매도 측, prefix 없음 = 공용(`stg/analyzer.go` 통합 루프, `stg/types.go` 결과 타입), `*_test.go` = 해당 파일 테스트(Go 규칙상 같은 폴더).

**C# 미포팅 항목**: `CandlePatternEvaluator`의 미사용 패턴 함수들과 `VirtualTrading`/백테스트 헬퍼만 남음 (모두 현행 파이프라인 미사용 — 상세는 `/home/feihong/code/Jarvis/project/RESTGo/csharp-porting-gap.md`)

### YAML 룰 엔진 (`stg/buy_rule_engine.go` + `rules/strategy1.yaml`)

매수 조건은 코드가 아닌 YAML로 정의합니다. 전략 추가·수정 시 코드 변경 없이 `rules/strategy1.yaml`만 편집하면 됩니다.

- 룰 필드: `def_count`(정확 일치) / `def_count_min`(이상) / `when`(AND) / `any_of`(OR) / `when_not`(NOT) / `signal`
- 조건명은 `stg/buy_conditions_registry.go`의 `init()`에서 `RegisterCondition()`으로 등록된 이름만 사용 가능. 새 조건 추가 시 ① `cond/`에 함수 구현 → ② 레지스트리에 등록 → ③ YAML에서 참조
- 룰은 **순서대로 평가되어 첫 매칭이 승리**하므로 엄격한 전략을 위에 배치
- **전략별 중복 신호 방지**: 같은 DefBox 구간에서 한 전략은 1회만 발화 (`ctx.LastBuySignalPosition`에 기록, DefBox 변경 시 리셋 — C# `LastBuySignalPosition_StrategyN` 포팅)
- **돌파 게이트** (`stg/analyzer.go` `checkDefBoxBreakout`): 가격 돌파 + 거래대금(`IsVolumeBreakout`) + ATR 모두 충족해야 돌파 인정. 룰 평가는 **돌파 캔들에서만** 1회 수행되고, 이후 캔들은 ShortRange 사후 평가만 한다 (C# BLogic 정렬)
- **FollowUp/REST2** (`stg/buy_followup.go`): REST2 S13~S16(`DetermineBuySignal`, 후보군1 상태머신) + S17~S20 재진입 처리. S15/S17/S18은 C#과 동일하게 사문(도달 불가) 게이트 보존
- `stock/handler.go`가 `stg.LoadStrategy(buyRulesPath())`로 로드 — 기본 `rules/strategy1.yaml`, `RESTGO_BUY_RULES` 환경변수로 교체 가능 (예: `RESTGO_BUY_RULES=rules/strategy2.yaml ./RESTGo stock analyze 005930 250`). 로드 실패 시 `stg/analyzer.go`의 하드코딩 fallback 로직 사용 (실패가 조용히 무시되므로 주의)

### Python 패키지 구조 (`py/`)

| 디렉토리 | 역할 |
|----------|------|
| `py/analysis/` | Box 차트 생성 (`box_chart.py`는 `zpicture/batch_signals.json` 소비), MA5 변곡 분석 |
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
- SSH 키: `~/.ssh/id_rsa` (RSA 4096-bit, 공개키 등록 완료)
- C# 참조 프로젝트: `/home/feihong/code/REST/RESTG/Stock1/` (branch: `feature/multi-position-sell-strategy`)

## 주의사항

- **크리덴셜 평문 금지**: `config.yaml`에 접속 정보를 추가할 때 반드시 `crypto.go`의 `Encrypt()` 함수로 암호화한 값을 사용해야 합니다. 평문 저장 시 보안 사고로 이어집니다.
- **DB 추가 절차**: `han`, `var`, `KIS2` DB의 접속 정보는 코드가 아닌 `key` DB의 `KeyValueStore` 테이블에서 읽습니다. 새 DB 환경을 추가하려면 해당 테이블을 먼저 업데이트해야 합니다.
- **RabbitMQ 의존성**: 로깅 시스템이 RabbitMQ와 결합되어 있어, RabbitMQ 연결 실패 시 로그가 유실될 수 있습니다. `rabbitmq.go`의 재연결 로직이 있으나, 프로덕션 환경에서는 큐 가용성을 모니터링해야 합니다.
- **컴파일된 바이너리**: 저장소에 `RESTGo` 바이너리가 포함되어 있습니다. 코드 변경 후 반드시 `go build`로 재빌드해야 변경사항이 반영됩니다.
- **Go 버전**: `go.mod`에 `go 1.25.0`이 명시되어 있습니다. 로컬 Go 버전이 맞지 않으면 빌드 오류가 발생할 수 있습니다.


## 문서 작성 위치
이 프로젝트의 모든 docs 문서는 `/home/feihong/code/Jarvis/project/RESTGo/` 에 작성한다.
