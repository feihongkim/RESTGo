# RESTGo 시스템 아키텍처 분석

> 작성자: Claude Code AI (pi coding agent)
> 날짜: 2026-06-25
> 대상: RESTGo 프로젝트 전체 시스템 구조

---

## 1. 개요

RESTGo는 Go 1.25.0 기반 CLI 운영 도구로, 크게 세 가지 역할을 수행한다:

1. **다중 MSSQL 쿼리** — 4개 DB(key/han/var/KIS2)에 직접 SQL 실행
2. **주식 Box/매수·매도 분석** — C# Stock1 프로젝트의 핵심 분석 로직을 Go로 포팅
3. **Python 분석 스크립트 실행** — 차트·백테스트·테마 전략 스크립트 호출

DB 접속 정보는 AES-256-GCM 암호화되어 `config.yaml`에 저장된다.

---

## 2. 프로젝트 규모

| 지표 | 값 |
|------|-----|
| Go 파일 수 | 247개 |
| 총 LOC | ~41,400 |
| 테스트 파일 | 42개 |
| YAML 룰 파일 | 46개 (전략 + ablation + archive + grid) |
| Python 스크립트 | 34개 |
| Go 의존성 | 9개 |

### 주요 의존성
- `go-sql-driver/mysql` — MSSQL 연결 (TDS 프로토콜)
- `amqp091-go` — RabbitMQ 메시징
- `yaml.v3` — YAML 룰 엔진 파싱
- `zap` — 구조화 로깅

---

## 3. 디렉토리 구조

```
RESTGo/
├── main.go                    # 진입점: console.Init() → CLI 명령어 라우팅
├── go.mod / go.sum            # Go 모듈 정의 (go 1.25.0)
├── config.yaml                # 암호화된 DB 접속 정보
├── RESTGo                     # 컴파일된 바이너리
│
├── console/                   # [인프라 계층] 초기화·DB·로깅·암호화·Python 실행
│   ├── init.go                # 초기화 흐름 오케스트레이션
│   ├── config.go              # YAML 설정 파일 로드
│   ├── crypto.go              # AES-256-GCM 암호화/복호화
│   ├── database.go            # MSSQL 커넥션 풀 (싱글턴 MsConn)
│   ├── sqlquery.go            # SQL 쿼리 실행 및 테이블 출력
│   ├── rabbitmq.go            # RabbitMQ 세션 관리
│   ├── logger.go              # 레거시 배치 로깅
│   ├── structured_logger.go   # Zap 구조화 로깅
│   ├── py_runner.go           # Python 스크립트 실행 래퍼
│   └── cleanup.go             # 리소스 해제 (defer)
│
├── box/                       # [도메인 모델] Box·Candle 데이터 구조체
│   ├── types.go               # Box, Candle 기본 타입
│   ├── sell_types.go          # 매도 관련 타입 (TradePosition, SellExecution)
│   ├── context.go             # TradingContext: 분석 루프 공유 상태
│   ├── curvature.go           # 곡률 분석 + DefBox 생성
│   ├── curve.go               # 추세 전환 조건 (Curvekey)
│   ├── defbox.go              # DefBox 생성 조건 평가
│   ├── box_price.go           # 구간 최고/최저가 계산
│   ├── box_creation.go        # Box 생성·추가 서비스
│   ├── candle_loader.go       # KIS2 DB 캔들 조회
│   ├── candle_loader_pair.go  # Pair 종목 캔들 로더
│   └── candle_loader_upbit.go # Upbit 거래소 캔들 로더
│
├── indicator/                 # [기술적 지표] Rolling sum 기반 O(N) 계산
│   ├── candle_processor.go    # 스케일링·MA(5/20/60/120)·ATR
│   ├── bollinger.go           # Bollinger Bands (period=20, 2σ)
│   ├── rsi.go                 # RSI (Wilder, period=14)
│   ├── macd.go                # MACD
│   ├── ema.go                 # EMA
│   ├── stochastic.go          # Stochastic
│   ├── donchian.go            # Donchian Channel
│   ├── keltner.go             # Keltner Channel
│   ├── obv.go                 # On-Balance Volume
│   ├── adx.go                 # ADX
│   ├── supertrend.go          # SuperTrend
│   ├── vwap.go                # VWAP
│   └── pair.go                # Pair 트레이딩 지표
│
├── cond/                      # [조건 함수] 매수·매도 조건 평가 (순수 함수)
│   ├── buy_conditions.go      # Box 구조 기반 매수 조건 (12개)
│   ├── buy_conditions_extra.go# MA·캔들패턴·관통·MultiDef 조건
│   ├── buy_indicator.go       # 지표 기반 매수 조건 16종 (RSI 5·BB 5·MA 6)
│   ├── buy_indicator_15m.go   # 15분봉 지표 조건
│   ├── buy_oscillator.go      # 오실레이터·관통 옵션 + 공용 헬퍼
│   ├── buy_followup.go        # ShortRange·거래대금 게이트·재진입 조건
│   │
│   ├── sell_profit_taking.go  # 익절 조건 (GapUp, BBUpperBreakout)
│   ├── sell_loss_cutting.go   # 손절 조건 (EarlyDrop, MainBoxBreak 등)
│   ├── sell_technical.go      # 기술적 매도 (MA cross, AdaptiveStop)
│   ├── sell_early_warning.go  # 조기경보 (BBSqueeze, WeakFoundation)
│   ├── sell_ma_reversal.go    # MA 반전 매도
│   ├── sell_bb_volatility.go  # BB 변동성 기반 매도
│   ├── sell_adaptive_stop.go  # 적응형 손절
│   ├── sell_recovery.go       # 회복 감지 (Composite Path)
│   ├── sell_holding_extension.go # 보유 연장 평가
│   └── sell_helpers.go        # 매도 공용 헬퍼
│
├── stg/                       # [전략 엔진] YAML 룰 엔진·분석 메인루프
│   ├── analyzer.go            # 분석 메인루프 (Box/DefBox + 돌파 게이트)
│   ├── combined_analyze.go    # 복합 시간봉 분석 (일봉+15분봉)
│   ├── buy_rule_engine.go     # YAML 매수 룰 평가 엔진
│   ├── buy_conditions_registry.go # 조건명 → cond 함수 매핑
│   ├── buy_followup.go        # S13~S20 후속 매수 처리 (REST2)
│   ├── buy_settings.go        # 분석 설정값 (Settings struct)
│   │
│   ├── sell_rule_engine.go    # YAML 매도 룰·5-Path 결정 엔진
│   ├── sell_executor.go       # 부분 매도 실행·가중평균 수익률
│   ├── sell_conditions_registry.go # 매도 조건명 레지스트리
│   ├── sell_settings.go       # 매도 설정값 (SellSettings struct)
│   ├── sell_tracker.go        # 매도 트래킹 (count_min, ratio_min)
│   ├── sell_15m.go            # 15분봉 매도 평가
│   │
│   ├── types.go               # 신호·결과 타입 (AnalysisResult, Positions)
│   ├── wpattern_analyze.go    # W-패턴 분석
│   └── wpattern_defbox.go     # W-패턴 DefBox
│
├── stock/                     # [CLI 핸들러] 명령어 → 분석 파이프라인
│   └── handler.go             # CLI 명령어 라우팅 구현
│
├── study/                     # [연구 도구] 백테스트·통계·스캔
│   ├── walk_forward.go        # Walk-forward 분석
│   ├── baseline_30m.go        # 30분봉 베이스라인
│   ├── breakdown.go           # 브레이크다운 분석
│   ├── combined_scan.go       # 복합 스캔
│   ├── edge.go                # Edge 분석
│   ├── event_study.go         # 이벤트 스터디
│   ├── grid.go                # 그리드 전략
│   ├── miiib_scan.go          # MIIIB 패턴 스캔
│   ├── pair.go                # Pair 트레이딩 분석
│   ├── stats.go               # 통계 유틸리티
│   ├── wbottom_scan.go        # W-bottom 스캔
│   └── wdefbox_scan.go        # W-DefBox 스캔
│
├── rules/                     # [전략 정의] YAML 룰 파일
│   ├── strategy1.yaml         # 매수 전략 (SingleDef 5종 + MultiDef 3종)
│   ├── strategy2.yaml         # 지표 기반 매수 전략 6종 (I01~I06)
│   ├── strategy3.yaml         # 추가 전략 변형
│   ├── strategy_bb_*.yaml     # Bollinger 기반 전략
│   ├── sell_strategy1.yaml    # 매도 룰 21종 + 5-Path 설정
│   ├── sell_strategy1_positive_only.yaml
│   ├── sell_strategy1_posOnly_*.yaml
│   ├── grid_*.yaml            # 그리드 전략 설정
│   ├── ablation/              # 28개 ablation 실험 YAML
│   └── archive/               # 9개 보관된 실험 YAML
│
├── py/                        # [Python 분석] 차트·백테스트·테마 전략
│   ├── analysis/              # Box 차트 생성, MA5 변곡 분석
│   ├── batch/                 # 차트 일괄 생성, Telegram 발송
│   ├── backtest/              # 테마 전략 공통 백테스트 엔진
│   ├── strategy/theme/        # 외국인 수급 기반 테마 전략 4종
│   └── common/                # DB 연결 공통 모듈
│
└── zpicture/                  # 분석 결과 이미지 저장소
```

---

## 4. 초기화 흐름

`console.Init()`이 호출 순서를 보장한다:

```
console.Init()
  ├── 1. config.yaml 로드 및 복호화 (AES-256-GCM)
  ├── 2. 4개 MSSQL 연결 풀 생성 (max 100 open / 20 idle)
  ├── 3. key DB의 KeyValueStore에서 han/var/KIS2 동적 연결정보 읽기
  ├── 4. RabbitMQ 세션 초기화 (큐: LOG, FEILOGIC, slice2DB, KISData)
  └── 5. 로그 배치 프로세서 goroutine 시작 (100건/100ms)
```

`main.go`에서 `defer console.Cleanup()`으로 종료 시 모든 리소스 해제.

---

## 5. 보안 아키텍처

### 암호화 계층 (`console/crypto.go`)

2단계 암호화:
1. 원본 문자열에 **랜덤 패딩**(앞 5자 + 뒤 7자) 삽입
2. **AES-256-GCM** 암호화 → Base64 인코딩

복호화는 역순: Base64 → AES-GCM 복호화 → 패딩 제거.

### 주의사항
- config.yaml에 평문 크리덴셜 절대 금지
- `han`, `var`, `KIS2` DB 정보는 코드가 아닌 `key` DB의 `KeyValueStore`에서 동적 조회
- 암호화 키는 별도 환경변수/파일로 관리 (config.yaml 외부)

---

## 6. 싱글턴 전역 리소스

모든 핵심 리소스는 패키지 레벨 싱글턴:

| 변수 | 타입 | 용도 |
|------|------|------|
| `console.MsConn` | `*sql.DB` | MSSQL 연결 풀 |
| `console.RabbitMQSession` | RabbitMQ 세션 | 메시지 큐 |
| `console.ZapLogger` | `*zap.Logger` | 구조화 로거 |
| `stg.activeRules` | `[]RuleConfig` | 활성 매수 룰 |
| `stg.activeSellSettings` | `*SellSettings` | 활성 매도 설정 |

`console.Init()`에서 일괄 초기화 → 이후 모든 패키지에서 안전하게 사용 가능.

---

## 7. 이중 로깅 시스템

| 함수 | 방식 | 용도 |
|------|------|------|
| `Log()` / `LogError()` | 커스텀 배치 큐 → RabbitMQ | 레거시 텍스트 로그 |
| `LogInfo()` / `LogErr()` | Zap JSON | 구조화 로그 |
| `Tele()` | RabbitMQ (Sender="KIS-tele") | Telegram 알림 |

로그 레벨: `DEBUG` > `INFO` > `ERROR` > `TEST` (config.yaml `loglevel`)

---

## 8. SQL 쿼리 실행

- `SELECT` / `WITH` / `EXEC` / `SP_*` → 컬럼 너비 자동 조정 테이블 출력
- `INSERT` / `UPDATE` / `DELETE` → 영향받은 행 수 출력
- NULL 값 → `<NULL>` 문자열
- DB 선택: `-db key|han|var|KIS2` (기본: key)

---

## 9. Python 실행 환경

- Python: `/home/feihong/code/REST/RESTGo/venv/bin/python3` (host 전용 venv)
- 프로젝트 루트: `/home/feihong/code/REST/RESTGo`
- Go → Python 호출: `console/py_runner.go` (상수 `PythonBin`, `ProjectRoot`)

### Python 실행 명령어
```bash
./RESTGo py box_chart <종목코드>   # Box 차트
./RESTGo py box_batch              # 배치 차트
./RESTGo py batch_chart            # 일괄 차트
./RESTGo py tg_send                # Telegram 발송
./RESTGo py <스크립트경로> [인수]  # 임의 스크립트
```

---

## 10. 원격 서버 접속

- SSH: `ssh feihong@192.168.3.120` (hostname: `white`)
- SSH 키: `~/.ssh/id_rsa` (RSA 4096-bit)
- C# 참조 프로젝트: `/home/feihong/code/REST/RESTG/Stock1/` (branch: `feature/multi-position-sell-strategy`)

---

## 11. 설계 평가

### 강점
1. **관심사 분리 양호**: console(인프라) / box(도메인) / cond(조건함수) / stg(엔진) / study(연구) 경계 명확
2. **순수 함수 지향**: cond 패키지 함수들은 대부분 부작용 없는 순수 함수 → 테스트 용이
3. **YAML 기반 전략**: 전략 수정 시 재빌드 불필요, ablation 실험 체계 우수
4. **O(N) 지표 계산**: Rolling sum 기법으로 Bollinger, MA 등 효율적 계산
5. **C# 포팅 충실도**: 원본 로직을 정확히 포팅하면서 Go 이디엄 적용

### 약점 / 개선 포인트
1. **싱글턴 남용**: 전역 변수로 인해 테스트 격리 어려움, 의존성 주입 부재
2. **RabbitMQ 의존성**: 로깅 시스템이 RabbitMQ에 강결합 → 연결 실패 시 로그 유실 위험
3. **에러 처리 불일치**: 일부 함수는 에러를 반환하지 않고 내부에서 삼키는 패턴
4. **테스트 커버리지**: 247개 중 42개(17%)만 테스트 파일 존재, Study 패키지 테스트 전무
5. **15분봉·일봉 결합**: `combined_analyze.go`가 두 시간 프레임을 단단히 결합 → 확장성 제한

---

## 12. 위험 요소

| 위험 | 심각도 | 설명 |
|------|--------|------|
| RabbitMQ 장애 | **HIGH** | 로그 유실 + Telegram 알림 중단 |
| config.yaml 유출 | **CRITICAL** | DB 크리덴셜 노출 (AES 암호화는 방어선) |
| Go 버전 의존성 | LOW | 1.25.0은 정식 릴리즈, LTS 아님 → 마이그레이션 필요 가능 |
| C#-Go 불일치 | MEDIUM | 포팅 오류 시 분석 결과 차이 발생 (ablation으로 검증 중) |
