# 작업: W중력 전략(buy_wdefbox) 백테스트 결과 측정·분석 보고서 (2차 — 분석 전용)

## 배경
1차 시도에서 이 컨테이너는 DB 네트워크 도달 불가로 실행이 안 됐다 (올바르게 보고했음).
이번에는 **백테스트 실행이 이미 완료**되어 결과 JSON이 준비돼 있다. 실행은 하지 말고
아래 두 파일을 읽어 측정·분석만 하라.

- `zpicture/wdefbox_event_study.json` — W중력 전략 (rules/buy_wdefbox.yaml, hannam 16년 일봉, 5-Path 매도)
- `zpicture/strategy1_event_study_ref.json` — 비교 기준 strategy1 (동일 조건)

JSON 구조는 `study/event_study.go`의 `SESOutput` 참고 (stats, by_year, baseline_mean_by_horizon,
net_return_distribution, sell_stats, sell_by_year, trades).

## 측정·분석할 것

각 전략에 대해:
1. 신호 수 (전략별 stats)
2. horizon별 평균 수익률과 baseline(전체 시장 평균) 대비 초과수익
3. 매도 통계 (sell_stats): 승률, 평균 net 수익률, 수익 팩터(PF), 평균 보유기간
4. 연도별 분해 (by_year, sell_by_year): 특정 연도 쏠림 여부, 연도별 안정성
5. W중력(W_DefBoxGravity) vs strategy1 비교표: 신호 빈도, 승률, 평균수익, 매도 알파

## 보고서 작성

`zpicture/wdefbox_backtest_report.md` 를 **덮어써서** markdown으로 저장 (1차 실패 보고서 대체):
- 요약 (전략 가설 "DefBox 중력 + W 반등 = 강한 상승"이 데이터로 지지되는가 — 3줄)
- 위 측정 항목 표
- W중력 전략의 강점/약점 관찰
- 주의: 수치를 지어내지 말 것. JSON에 없는 값은 "측정 불가"로 표기.
  보고서에 쓰는 모든 수치는 JSON에서 그대로 읽은 값이어야 한다.

## 제약
- 코드·rules 수정 금지, 명령 실행 금지 — 파일 읽기와 보고서 작성만
- 완료 후 보고서 핵심 수치(양 전략 신호 수·승률·평균수익·PF)를 응답으로 보고
