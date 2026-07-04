# 작업: W중력 전략 포트폴리오 자본 제약 시뮬레이션 (코드 보완 + 백테스트 + 시뮬 + 보고서)

## 배경
이벤트 스터디 결과(A안: 거래당 +2.549%, 승률 60.0%, 보유 19.3봉, 7,534건)는 포지션당 독립 전액 진입 가정이다.
월평균 ~35신호 × 보유 ~1개월이면 평균 ~35개 동시 포지션이 되므로, 실제 포트폴리오 성과(CAGR·MDD·연환산 Sharpe)는
자본 제약 시뮬레이션으로만 알 수 있다. 이번 작업의 목표가 그것이다.

전제: 현재 `zpicture/wdefbox_sell_formal_event_study.json`에는 매도 체결별 행(sell trade rows)이 없어
시뮬레이션 입력이 부족하다. 먼저 study 코드에 dump를 추가해야 한다.

## Telegram 진행 보고 (필수)
각 단계 시작/완료 시 telegram_send 도구로 chat_id 7723743534 에 `[pi]` 접두사로 짧게 보고하라.

## 단계

### 1) study/event_study.go에 매도 체결 전체 기록 dump 추가 (additive only)
- 출력 JSON에 새 필드 `sell_trades` 배열 추가: strategy, shcode, buy_date, sell_date, holding_bars, net_return_pct(왕복비용 0.4% 차감 후), weight(부분매도 비중)
- **기존 출력 필드·수치는 절대 변경 금지** (기존 보고서 재현성). 참고: `stg/types.go`(Positions·체결시각), `stg/sell_rule_engine.go`
- `go test ./...` 통과 확인 후 `go build -o RESTGo_pisim .` (커밋된 ./RESTGo 바이너리는 건드리지 말 것)

### 2) 백테스트 재실행 (~20분)
```
RESTGO_SELL_RULES=rules/sell_wdefbox.yaml ./RESTGo_pisim stock strategy_study --with-sell rules/buy_wdefbox.yaml zpicture/wdefbox_portfolio_trades.json
```
- 시작 직후 stderr의 "매도 룰 로드:" 경로 확인
- **무결성 체크**: 새 JSON의 sell_stats ALL이 기존 A안(7,534건 / +2.549% / 승률 60.0% / 보유 19.3봉)과 일치해야 한다. 불일치하면 중단하고 원인을 보고하라.

### 3) 포트폴리오 시뮬레이션 (Python, `py/analysis/portfolio_sim.py`로 저장)
- 입력: `sell_trades` (부분매도는 weight로 반영 — 한 포지션의 매도 행들 weight 합이 1이 되는 구조인지 먼저 확인하고, 아니면 실제 구조를 파악해 보고서에 명시)
- 규칙:
  - 초기자본 1.0, 슬롯 N개. 신호 발생일(buy_date)에 빈 슬롯이 있으면 `현재자본/N`을 진입, 없으면 그 신호는 스킵
  - 같은 날 다중 신호는 종목코드 오름차순으로 처리 (결정적 — 난수 사용 금지)
  - 미투자 현금 수익률 0. 매도일(sell_date)에 해당 비중만큼 net_return 실현
- N 스윕: 10 / 20 / 35 / 50 / 무제한(참고용)
- 산출 (N별): CAGR, MDD(실현 기준 자본곡선), 연환산 Sharpe(월별 수익률 기반), 평균·최대 동시 포지션 수, 신호 소화율(진입건/전체신호)
- 한계 명시: 자본곡선이 실현(매도 시점) 기준이라 보유 중 평가손 미반영 → MDD 과소추정 가능. 보고서에 반드시 기록

### 4) 보고서 작성 — `zpicture/wdefbox_portfolio_sim_report.md`
- 요약 3줄 (현실적 기대 CAGR / MDD / 권장 N)
- N별 비교표, N=35 기준 연도별 수익률표, 신호 소화율
- 방법·한계 절 포함. **수치를 지어내지 말 것** — 모든 수치는 시뮬레이션 출력에서 그대로.

### 5) 마무리
- 생성·수정 파일 전부 `chown 1000:1000`
- `RESTGo_pisim` 바이너리 삭제
- 마지막 응답으로 핵심 수치(N=35 CAGR·MDD·Sharpe·소화율, 권장 N) 보고

## 제약
- rules/*.yaml 수정 금지. study 코드 수정은 1)의 additive dump에 한정.
- 이 지시서는 삭제하지 말 것.
