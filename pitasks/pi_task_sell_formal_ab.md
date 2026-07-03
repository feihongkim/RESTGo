# 작업: sell_wdefbox 정식화 A/B 백테스트 실행 + 분석 보고서

## 배경
W중력 전략 청산 스윕(zpicture/wdefbox_minhold_sweep_report.md)에서 "손절·기술적 청산 무용, 시간청산+익절만" 결론이 나왔다.
이를 유예 해킹(min_holding_period)이 아닌 명시적 룰로 정식화한 YAML 2종이 준비되어 있다 (수정 금지):
- A: `rules/sell_wdefbox.yaml` — 익절 2종 + 기간만료(20봉, 연장 포함)만
- B: `rules/ablation/sell_wdefbox_dstop20.yaml` — A + 재난 손절 DisasterStop(-20%, Critical, 즉시 전량)

이번에는 네가 DB에 접속 가능하므로 백테스트 실행까지 네가 수행한다.

## Telegram 진행 보고 (필수)
각 단계 시작/완료 시 telegram_send 도구로 chat_id 7723743534 에 `[pi]` 접두사로 짧게 보고하라.
(예: "[pi] A안 백테스트 시작 (hannam 16년, ~20분 예상)", "[pi] A안 완료, B안 시작", "[pi] 분석 중", "[pi] 완료: 보고서 저장. 핵심수치=...")

## 단계

### 1) 백테스트 2회 실행 (프로젝트 루트 /workspace 에서, 순차 실행)
```
RESTGO_SELL_RULES=rules/sell_wdefbox.yaml ./RESTGo stock strategy_study --with-sell rules/buy_wdefbox.yaml zpicture/wdefbox_sell_formal_event_study.json
RESTGO_SELL_RULES=rules/ablation/sell_wdefbox_dstop20.yaml ./RESTGo stock strategy_study --with-sell rules/buy_wdefbox.yaml zpicture/wdefbox_sell_dstop20_event_study.json
```
각 실행 시작 직후 stderr에 "매도 룰 로드: <경로>"가 올바른 YAML인지 확인하라. 각 ~20분 소요.

### 2) 분석 (JSON 구조는 study/event_study.go 의 SESOutput 참조)
비교 기준: `zpicture/wdefbox_mh20_notech_event_study.json` (유예 해킹 버전, ALL 평균 +2.57% / 승률 59.4% / 보유 21.53봉 / 흑자연도 17/19)

측정할 것:
1. **A안 vs 기준 동등성 검증**: sell_stats ALL(건수/평균/중앙값/승률/보유) + 버킷 구성 비교. 정식화가 유예 해킹과 동등한 성과를 내는가? 차이가 있으면 어느 버킷에서 왜 다른지 (특히 연장(Extension) 경로 — 기준 버전은 Loss 룰이 유예 후 연장 청산을 담당했지만 A안은 MA5BreakdownDuringExtension만 담당).
2. **B안 재난손절 효과**: DisasterStop 발동 건수/평균수익/평균보유. sell_by_year에서 2008년(기준 -3.41%)과 2026년이 얼마나 개선되는가. 전체 ALL 평균의 비용(A 대비 몇 %p 하락)은.
3. **A vs B 권고**: 꼬리 절감 대비 비용으로 어느 쪽을 W중력 기본 매도로 삼을지 근거와 함께.
4. 매수측 h20 stats가 세 파일 모두 동일한지 확인 (fire_count 7,534, t=6.80) — 매도 단일변수 실험 무결성.

### 3) 보고서 작성
`zpicture/wdefbox_sell_formal_ab_report.md` 에 markdown으로 저장:
- 요약 3줄 (동등성 여부 / 재난손절 효과 / 권고)
- A vs 기준 vs B 비교표 (ALL + 버킷별)
- 연도별 표 (특히 2008, 2012, 2021, 2026)
- 수치를 지어내지 말 것. JSON에 없는 값은 "측정 불가"로 표기. 모든 수치는 JSON에서 그대로 읽은 값이어야 한다.

### 4) 마무리
- 생성한 모든 파일(JSON 2개 + 보고서)을 `chown 1000:1000` 하라.
- 마지막 응답으로 핵심 수치(A/B의 ALL 평균·승률, 2008년 개선폭, 권고)를 보고하라.

## 제약
- 코드·rules YAML 수정 금지. 실행·분석·보고서 작성만.
- 이 지시서(pi_task_sell_formal_ab.md)는 삭제하지 말 것 (작업 후 Claude가 정리).
