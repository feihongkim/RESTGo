# RESTGo 시스템 아키텍처 분석

> 작성자: Claude Code AI (pi coding agent)
> 최종 수정: 2026-07-17
> 대상: RESTGo 프로젝트 전체 시스템 구조

---

## 1. 개요

RESTGo는 Go 1.25.0 기반 CLI 운영 도구로, 크게 네 가지 역할을 수행한다:

1. **다중 MSSQL 쿼리** — 4개 DB(key/han/var/KIS2)에 직접 SQL 실행
2. **주식 Box/매수·매도 분석** — C# Stock1 프로젝트의 핵심 분석 로직을 Go로 포팅
3. **백테스트·전략 연구** — `study/` 패키지의 러너 22종 (그리드 서치, 엣지 검증, walk-forward, 패턴 스캔 등)
4. **Python 분석 스크립트 실행** — 차트·백테스트·테마 전략 스크립트 호출

DB 접속 정보는 AES-256-GCM 암호화되어 `config.yaml`에 저장된다.

---

## 2. 프로젝트 규모

| 지표 | 값 |
|------|-----|
| Go 파일 수 | 306개 |
| 총 LOC | ~50,000+ |
| 테스트 파일 | 58개 |
| YAML 룰 파일 | 65개 (전략 + ablation + archive + grid + overlay) |
| Python 스크립트 | 39개 |
| Go 의존성 | 9개 |

### 주요 의존성
- `github.com/denisenkom/go-mssqldb` — MSSQL 연결 (TDS 프로토콜)
- `github.com/rabbitmq/amqp091-go` — RabbitMQ 메시징
- `gopkg.in/yaml.v2` / `gopkg.in/yaml.v3` — YAML 룰 엔진 파싱
- `go.uber.org/zap` — 구조화 로깅
- `golang.org/x/crypto` — 암호화 유틸리티

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
│   ├── candle_loader.go       # KIS2·hannam·해외 DB 캔들 조회
│   ├── candle_loader_pair.go  # Pair 종목 캔들 로더
│   └── candle_loader_upbit.go # Upbit 거래소 캔들 로더
│
├── indicator/                 # [기술적 지표] Rolling sum 기반 O(N) 계산
│   ├── candle_processor.go    # 스케일링·MA(5/20/60/120/200)·ATR
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
│   ├── buy_conditions.go      # Box 구조 기반 매수 조건
│   ├── buy_conditions_extra.go# MA·캔들패턴·관통·MultiDef 조건
│   ├── buy_indicator.go       # 지표 기반 매수 조건 16종 (RSI/BB/MA)
│   ├── buy_indicator_15m.go   # 15분봉 지표 조건
│   ├── buy_oscillator.go      # 오실레이터·관통 옵션 + 공용 헬퍼
│   ├── buy_followup.go        # ShortRange·거래대금 게이트·재진입 조건
│   ├── buy_triggers.go        # 트리거(edge) 조건 함수
│   ├── buy_volume_wave.go     # 거래량 파동(Volume Wave) 조건
│   ├── buy_pullback.go        # Pullback 매수 조건
│   ├── buy_regime.go          # Regime(시장 국면) 판별 조건
│   ├── buy_doublebump.go      # DoubleBump(이중 충돌) 패턴 조건
│   ├── buy_stg11.go           # STG11 돌파후행 전략 조건
│   ├── buy_stg6.go            # STG6 전략 조건
│   ├── buy_rstg_more.go       # r_stg 추가 조건
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
│   ├── sell_helpers.go        # 매도 공용 헬퍼
│   ├── sell_hns.go            # HNS(헤드앤숄더) 매도 조건
│   └── sell_mtop.go           # MTop 매도 조건
│
├── stg/                       # [전략 엔진] YAML 룰 엔진·분석 메인루프
│   ├── analyzer.go            # 분석 메인루프 (Box/DefBox + 돌파 게이트)
│   ├── buy_rule_engine.go     # YAML 매수 룰 평가 엔진
│   ├── buy_conditions_registry.go # 조건명 → cond 함수 매핑 (81종 등록)
│   ├── buy_followup.go        # S13~S20 후속 매수 처리 (REST2)
│   ├── buy_settings.go        # 분석 설정값 (Settings struct)
│   │
│   ├── trigger_registry.go    # 트리거(edge 이벤트) 등록소 (15종 등록)
│   ├── armed_trigger.go       # Armed Trigger (장전→발화) 상태머신 엔진
│   ├── armed_trigger_registry.go # Armed 트리거 등록소 (8종 등록)
│   ├── trigger_scan.go        # 범용 Trigger×Condition 조합 측정 러너
│   │
│   ├── sell_rule_engine.go    # YAML 매도 룰·5-Path 결정 엔진
│   ├── sell_executor.go       # 부분 매도 실행·가중평균 수익률
│   ├── sell_conditions_registry.go # 매도 조건명 레지스트리 (24종 등록)
│   ├── sell_settings.go       # 매도 설정값 (SellSettings struct)
│   ├── sell_tracker.go        # 매도 트래킹 (count_min, ratio_min)
│   ├── sell_15m.go            # 15분봉 매도 평가
│   │
│   ├── types.go               # 신호·결과 타입 (AnalysisResult, Positions)
│   ├── wpattern_analyze.go    # W-패턴 분석
│   ├── wpattern_defbox.go     # W-패턴 + DefBox 결합 분석
│   ├── combined_analyze.go    # 복합 시간봉 분석 (일봉+15분봉)
│   ├── volume_wave_analyze.go # Volume Wave 신호 분석기
│   ├── volume_wave_pullback.go# Volume Wave + Pullback 결합 분석
│   ├── descending_trendline_analyze.go # 하락추세선 분석기
│   ├── mainbox_retest_analyze.go # MainBox 재시험 분석기
│   ├── hns_analyze.go         # HNS(헤드앤숄더) 분석기
│   ├── mpattern_analyze.go    # MTop 패턴 분석기
│   ├── pullback_analyze.go    # Pullback 분석기
│   ├── wgc_analyze.go         # WGC(W바텀 GoldenCross) 분석기
│   └── overlay_density.go     # Overlay 밀도 판정 (W중력)
│
├── stock/                     # [CLI 핸들러] 명령어 → 분석 파이프라인
│   └── handler.go             # CLI 명령어 라우팅 (22개 연구 명령 포함)
│
├── study/                     # [연구 도구] 백테스트·통계·스캔·차트
│   ├── grid.go                # Grid Search
│   ├── edge.go                # Edge Test / Baseline
│   ├── walk_forward.go        # Walk-Forward 분석
│   ├── baseline_30m.go        # 30분봉 베이스라인
│   ├── breakdown.go           # 브레이크다운 분석
│   ├── event_study.go         # 이벤트 스터디
│   ├── pair.go                # Pair 트레이딩 분석
│   ├── stats.go               # 통계 유틸리티
│   ├── wbottom_scan.go        # W-bottom 스캔
│   ├── miiib_scan.go          # MIIIB 패턴 스캔
│   ├── wdefbox_scan.go        # W-DefBox 조합 스캔
│   ├── combined_scan.go       # 복합 시간봉 스캔
│   ├── mtop_scan.go           # MTop 패턴 스캔
│   ├── hns_scan.go            # HNS 패턴 스캔
│   ├── pullback_scan.go       # Pullback 스캔
│   ├── wgc_scan.go            # WGC 스캔
│   ├── trigger_scan.go        # Trigger Scan
│   ├── volume_wave_scan.go    # Volume Wave 스캔
│   ├── volume_wave_matrix.go  # Volume Wave 매트릭스 분석
│   ├── volume_wave_chart.go   # Volume Wave 차트 샘플
│   ├── volume_wave_box_study.go   # Volume Wave + Box 스터디
│   ├── volume_wave_strict_study.go# Volume Wave 엄격 스터디
│   ├── mainbox_retest_study.go    # MainBox 재시험 스터디
│   ├── mainbox_retest_s1_study.go # S1 MainBox 재시험
│   ├── mainbox_retest_refine_study.go  # MainBox 정제 스터디
│   ├── mainbox_retest_temporal.go # MainBox 시간적 분석
│   ├── descending_trendline_study.go      # 하락추세선 스터디
│   ├── descending_trendline_ma_study.go   # 하락추세선+MA 스터디
│   ├── descending_trendline_sideways_study.go # 하락추세선 횡보 스터디
│   └── descending_trendline_chart.go      # 하락추세선 차트
│
├── rules/                     # [전략 정의] YAML 룰 파일 (65개)
│   ├── strategy1.yaml         # 기본 매수 전략 (SingleDef 5종 + MultiDef 3종)
│   ├── strategy1_gc.yaml      # Golden Cross 확증 변형
│   ├── strategy1_s03s23.yaml  # S03+S23 조합 전략
│   ├── buy_indicator.yaml     # 지표 기반 매수 전략 6종 (I01~I06)
│   ├── buy_bb_pure.yaml       # Bollinger 3대 방법 (MIIIb~MI)
│   ├── buy_bb_hybrid.yaml     # Box + BB 복합 4룰 (SH1~SH4)
│   ├── buy_trigger_example.yaml # 트리거 문법 예시
│   ├── buy_crypto_15m.yaml    # 암호화폐 15분봉 다중 트리거 (보류)
│   ├── buy_wdefbox.yaml       # W-DefBox 결합 매수 전략
│   ├── buy_wdefbox_gc.yaml    # W-DefBox + Golden Cross
│   ├── buy_stg11_15m.yaml     # STG11 15분봉 전략
│   ├── grid_crypto_*.yaml     # 암호화폐 그리드 서치 (3개)
│   ├── grid_stg11.yaml        # STG11 그리드
│   ├── overlay_wdefbox.yaml   # W중력 오버레이 밀도 게이트
│   ├── sell_default.yaml      # 기본 매도 전략 (21룰 + 5-Path)
│   ├── sell_positive_only.yaml    # 익절 전용
│   ├── sell_positive_only_mh25.yaml # 익절 + max_holding 25
│   ├── sell_s03s23.yaml       # S03+S23 전용 매도
│   ├── sell_wdefbox.yaml      # W-DefBox 전용 매도
│   ├── ablation/              # 소거 실험 YAML (34개)
│   └── archive/               # 보관된 과거 실험 YAML (10개)
│
├── py/                        # [Python 분석] 차트·백테스트·테마 전략
│   ├── analysis/              # Box 차트 생성, MA5 변곡 분석
│   ├── batch/                 # 차트 일괄 생성, Telegram 발송
│   ├── backtest/              # 테마 전략 공통 백테스트 엔진
│   ├── strategy/theme/        # 외국인 수급 기반 테마 전략 4종
│   └── common/                # DB 연결 공통 모듈
│
├── r_stg/                     # [레거시] 과거 전략 노트 (AU3~전략16)
├── pitasks/                   # [Pi 작업 명세] 연구 태스크 정의 (28개)
├── pidocs/                    # [Pi 문서] 프로젝트 기술 문서
├── deploy/                    # 배포 스크립트
├── .telegram/                 # Telegram 봇 설정
└── zpicture/                  # 분석 결과 저장소 (JSON + 차트 이미지)
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
- SSH 키: `~/.ssh/id_ed25519` (ED25519)
- C# 참조 프로젝트: `/home/feihong/code/REST/RESTG/Stock1/` (branch: `feature/multi-position-sell-strategy`)

---

## 11. 설계 평가

### 강점
1. **관심사 분리 양호**: console(인프라) / box(도메인) / cond(조건함수) / stg(엔진) / study(연구) 경계 명확
2. **순수 함수 지향**: cond 패키지 함수들은 대부분 부작용 없는 순수 함수 → 테스트 용이
3. **YAML 기반 전략**: 전략 수정 시 재빌드 불필요, ablation 실험 체계 우수
4. **O(N) 지표 계산**: Rolling sum 기법으로 Bollinger, MA 등 효율적 계산
5. **C# 포팅 충실도**: 원본 로직을 정확히 포팅하면서 Go 이디엄 적용
6. **확장성**: Trigger/Armed Trigger 아키텍처로 코드 변경 없이 YAML에서 무한 전략 조합 가능

### 약점 / 개선 포인트
1. **싱글턴 남용**: 전역 변수로 인해 테스트 격리 어려움, 의존성 주입 부재
2. **RabbitMQ 의존성**: 로깅 시스템이 RabbitMQ에 강결합 → 연결 실패 시 로그 유실 위험
3. **에러 처리 불일치**: 일부 함수는 에러를 반환하지 않고 내부에서 삼키는 패턴
4. **테스트 커버리지**: 306개 중 58개(19%)만 테스트 파일 존재, Study 패키지 테스트 전무
5. **15분봉·일봉 결합**: `combined_analyze.go`가 두 시간 프레임을 단단히 결합 → 확장성 제한

---

## 12. 위험 요소

| 위험 | 심각도 | 설명 |
|------|--------|------|
| RabbitMQ 장애 | **HIGH** | 로그 유실 + Telegram 알림 중단 |
| config.yaml 유출 | **CRITICAL** | DB 크리덴셜 노출 (AES 암호화는 방어선) |
| Go 버전 의존성 | LOW | 1.25.0 — 마이그레이션 필요 가능 |
| C#-Go 불일치 | MEDIUM | 포팅 오류 시 분석 결과 차이 발생 (ablation으로 검증 중) |
| 문서 최신성 | MEDIUM | 코드 변경 속도가 문서 업데이트보다 빠름 |
