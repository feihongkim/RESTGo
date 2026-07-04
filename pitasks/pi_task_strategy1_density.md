# 작업: ① strategy1 밀도 가설 검증 ② hannam 일별 신호수 테이블 생성·백필 (코드 수정 금지)

## 배경
W중력에서 확인된 "신호 밀도가 품질을 예측한다" 패턴(Q1 +0.42% → Q5 +4.89%)이
strategy1(DefBox 돌파)에도 성립하는지 검증한다. 가설(사용자): "돌파 신호 밀도가 높은 구간 = 강세장
= 돌파 매수 품질 상승". W와 방향이 같을지(고밀도=좋음) 다를지는 데이터가 답한다.
아울러 실운영을 위해 일별 신호 수를 hannam DB 테이블로 적재한다 (사용자 승인 완료).

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 단계 시작/완료 보고.

## 단계

### 1) 빌드 (커밋된 ./RESTGo 는 sell_trades 덤프 이전 버전)
```
cd /workspace && go test ./cond/ ./stg/ ./indicator/ && go build -o RESTGo_pisim .
```

### 2) strategy1 이벤트 스터디 실행 (~20분)
```
RESTGO_SELL_RULES=rules/sell_default.yaml ./RESTGo_pisim stock strategy_study --with-sell rules/strategy1.yaml zpicture/strategy1_density_study.json
```
- stderr "매도 룰 로드:" 경로 확인
- 참고 비교: 기존 `zpicture/strategy1_event_study_ref.json`과 매수측 stats(fire_count 등)를 비교해
  차이가 있으면 그대로 보고 (그 사이 box Resist 수정 커밋이 있어 다를 수 있음 — 하드 실패 아님, 기록만)

### 3) 밀도 진단 실행 (수 초)
```
python3 py/analysis/portfolio_sim_policy.py --smoke zpicture/strategy1_density_study.json
```
- 출력의 "밀도 5분위별 품질" 표가 핵심 산출물. strategy1은 룰별 strategy명이 여러 개일 수 있으나
  진단은 전체 신호 흐름 통합 기준(그대로 두면 됨)
- W중력 결과(Q1 +0.415/49.9% → Q5 +4.889/71.9%)와 나란히 비교

### 4) hannam DB 테이블 생성 + 백필
테이블 설계 (이대로 생성):
```sql
CREATE TABLE StrategySignalDaily (
    strategy     VARCHAR(40) NOT NULL,
    trade_date   CHAR(8)     NOT NULL,
    signal_count INT         NOT NULL,
    updated_at   DATETIME    NOT NULL DEFAULT GETDATE(),
    CONSTRAINT PK_StrategySignalDaily PRIMARY KEY (strategy, trade_date)
);
```
실행: `./RESTGo_pisim sqlquery -db han "CREATE TABLE ..."` (이미 존재하면 중단하고 보고)

백필 (Python으로 JSON → INSERT문 생성, 500행 단위 배치로 sqlquery 실행):
- `W_DefBoxGravity`: zpicture/wdefbox_portfolio_trades.json 의 sell_trades에서
  고유 포지션(shcode, buy_date) 기준 buy_date별 카운트 (총 7,534건이어야 함)
- strategy1: zpicture/strategy1_density_study.json 의 sell_trades에서 strategy명별·buy_date별 카운트
  (rule별 strategy명 그대로 사용)
검증 쿼리 실행·기록:
```sql
SELECT strategy, COUNT(*) AS days, SUM(signal_count) AS total FROM StrategySignalDaily GROUP BY strategy ORDER BY strategy
```
→ W_DefBoxGravity total=7,534 및 strategy1 total=JSON 포지션 수와 일치 확인

### 5) 보고서 — `zpicture/strategy1_density_report.md`
- 요약 3줄 (가설 지지 여부 / W와 방향 동일한지 / 함의)
- strategy1 신호 규모 (총 신호 수, 연평균, 기간)
- 밀도 5분위 표: strategy1 vs W중력 나란히
- 단조성 판정: 밀도↑ → 품질↑ 인가, 아니면 다른 형태(역U 등)인가. 수치 그대로
- 매수측 stats 참고 비교 (ref와 차이 있으면 기록)
- 테이블 백필 결과 (검증 쿼리 출력 포함)
- 주의: 수치 지어내지 말 것. 진단 출력·JSON·쿼리 결과에서 그대로

### 6) 마무리
- 생성 파일 chown 1000:1000, `RESTGo_pisim` 삭제
- 마지막 응답: strategy1 밀도 Q1~Q5 평균수익·승률, 가설 판정, 테이블 검증 쿼리 결과

## 제약
- **Go/Python 코드·rules YAML 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- DB는 테이블 생성·INSERT·SELECT만. 기존 테이블 변경 금지.
- 이 지시서는 삭제하지 말 것.
