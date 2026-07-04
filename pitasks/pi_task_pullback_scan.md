# 작업: 20이평 눌림 돌파 매수 전 종목 스캔 + 분석 보고서 (코드 수정 금지)

## 배경
사용자 설계 매수 신호 (추세 지속형): ① MA20 상승 3연속(+++, 이전<지금) 유지 중 ② 가격이 이평 아래 눌림
③ 이평 아래 resist box — box ±3봉에서 고가>MA20 ≥1봉 AND 종가>MA20 0봉 (첫 시도 거부)
④ support box (종가가 이평 아래) ⑤ 트리거: support 인지 후 20봉 내, 양봉이 종가 기준 MA20 상향 돌파
(전일 종가≤전일 MA20 edge) + 트리거 시점도 MA20 +++.
구현: cond/buy_pullback.go, stg/pullback_analyze.go, study/pullback_scan.go (Claude, 단위테스트 7종 통과)
50종목 스모크: 신호 10건 — **설계상 희소한 신호** (상승 MA20 아래 눌림 국면이 짧음). 전 종목 ~900건 예상.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 빌드 + 전 종목 스캔 (~20분)
```
cd /workspace && go build -o RESTGo_pisim . && ./RESTGo_pisim stock pullback_scan --out zpicture/pullback_scan.json
```

### 2) 분석 (Python, 스크립트는 /tmp에만)
- 전체 h1/5/10/20: mean/median/승률/edge/t — **매수 신호이므로 edge 양수 & t≥2가 유효 기준**
- 속성별 부분집합 (h5·h20 edge):
  - depth_pct (눌림 깊이) 3분위 — 얕은 눌림 vs 깊은 눌림
  - ma20_slope_pct (추세 강도) 중앙값 상/하
  - touch_count 1 vs ≥2 (거부 횟수)
  - r_to_s_bars, arm_to_trigger_bars 분포와 상/하 분할
- 연도별 신호 수·h20 평균
- n이 작은 부분집합(<100)은 "소표본" 표기하고 결론에 쓰지 말 것

### 3) 보고서 — `zpicture/pullback_scan_report.md`
- 요약 3줄 (엣지 존재 여부 / 유효 조건 / 다음 단계 권고)
- 전체 표 + 부분집합 표 + 연도별 표
- 신호 희소성 자체에 대한 코멘트 (총 n, 종목당·연간 빈도)
- W중력(+2.55%)·strategy1과의 성격 차이 한 단락 (반전형 vs 지속형)
- 한계: in-sample, 종가 기준, n 규모, 다중비교 주의
- 수치 지어내지 말 것

### 4) 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 총 신호 수, h5/h20 edge·t·승률, 최고 부분집합, 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
