# 작업: portfolio_sim.py 치명 버그 2건 수정 + 재시뮬레이션 + 보고서 재작성

이전 작업(pi_task_portfolio_sim.md) 결과를 검증한 결과, **데이터 덤프(sell_trades)는 무결**하나
`py/analysis/portfolio_sim.py`에 치명적 버그 2건이 있어 CAGR/MDD/Sharpe/연도별 수치가 전부 무효다.
백테스트 재실행은 불필요 — `zpicture/wdefbox_portfolio_trades.json` 그대로 사용하라.

## 버그 1 (치명): 자본곡선이 "현금 잔고"만 기록
`daily_capital[date] = capital`에서 `capital`은 매수 시 `capital -= invested`로 차감된 **현금**이다.
보유 중 포지션의 원금이 곡선에서 사라져, 슬롯이 차 있을 때가 전부 "낙폭"으로 보인다.
→ MDD 69.88%, 2021년 -46.95% 등은 손실이 아니라 회계 착시다.

**수정**: `equity = cash + Σ(열린 슬롯의 invested × remaining_quantity)` (보유분은 원가 평가)를 매일 기록하고,
CAGR/MDD/월별 Sharpe/연도별 수익률을 전부 equity 곡선으로 계산하라.

## 버그 2 (구조적 과소투자): 포지션 크기가 현금/N
`invested = capital / N` (현금/N)이라 슬롯이 찰수록 신규 포지션이 기하급수로 작아지고,
35슬롯 만석에도 자본의 ~37%가 영구 유휴다((34/35)^35 ≈ 0.375).

**수정**: `invested = min(cash, equity / N)` 으로 변경 (총자산 기준 균등 배분, 현금 부족 시 그만큼만).

## 추가 수정 (부수)
- 평균/최대 동시 포지션: 근사(min(all_avg,N)) 대신 시뮬레이션 이벤트 처리 중 실제 `len(slots)`를 일별 추적해 계산
- simulate()와 simulate_detailed() 중복 구현을 하나로 통합 (같은 로직 두 벌이라 이번처럼 버그가 복제됨)
- 검증 출력 추가: N=35에서 "만석 시 투자비중(invested_total/equity)"이 ~1.0 근처인지 출력

## 재실행·검증
- N 스윕 동일: 10/20/35/50/무제한
- 자가 검증(sanity): 거래당 평균 +2.55%, 보유 ~19.3봉(≈1개월)이므로 만석 유지 구간의 equity 성장률은
  월 ~+2%대가 나와야 정상이다. N=35 CAGR이 한 자릿수 초반%로 다시 나오면 버그가 남은 것이니 원인 규명 후 보고하라.
- MDD는 여전히 "실현+원가평가" 기준(보유 중 시세 평가손 미반영) — 한계 문구 유지

## 산출물
- `py/analysis/portfolio_sim.py` 수정 (덮어쓰기)
- `zpicture/portfolio_sim_result.json` 재생성
- `zpicture/wdefbox_portfolio_sim_report.md` **전면 재작성** (덮어쓰기): 이전 버전 수치는 폐기.
  보고서 서두에 "1차 버전은 자본곡선 버그로 폐기, 본 버전이 유효" 한 줄 명시
- 생성·수정 파일 chown 1000:1000

## Telegram 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로: 시작 시 "1차 수치는 버그로 무효, 수정 재실행 중" 정정 공지 + 완료 시 핵심 수치(N=35 CAGR/MDD/Sharpe/소화율).

## 제약
- rules/*.yaml, study/*.go 수정 금지 (Go 재실행 불필요)
- 수치를 지어내지 말 것. 이 지시서는 삭제하지 말 것.
