# 작업: 해드앤숄더(H&S) 전 종목 스캔 + 속성별 부분집합 분석 보고서 (코드 수정 금지)

## 배경
M패턴 폐기 후 정식 H&S 구현 (Claude, 5이평 박스 기준 — 캔들 기준 아님):
- 5박스 R(왼어깨)–S(골1)–R(머리)–S(골2)–R(오른어깨), Box.Price = 구간 극값
- 경성 게이트: 머리 최고점 / 어깨 대칭 ≤10% / 골이 어깨 아래 / 머리 시점 MA20>MA60(선행 상승)
- 트리거: 오른어깨 인지 후 20봉 내 종가가 넥라인(골1–골2 직선) 하향 이탈
- **연성 속성(게이트 아님, 신호마다 기록)**: volume_ratio(오른어깨/왼어깨 거래량 — <1이면 정식 감소 충족),
  head_prom_pct(머리 돌출), neck_slope_pct(넥라인 기울기), shoulder_diff_pct, pattern_width
구현: cond/sell_hns.go, stg/hns_analyze.go, study/hns_scan.go. 단위테스트 6종 통과.
50종목 스모크: 528신호(빈도 높음), 전체 엣지 중립 — **속성별 부분집합이 이번 분석의 핵심**.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 빌드 + 전 종목 스캔 (~30분)
```
cd /workspace && go build -o RESTGo_pisim . && ./RESTGo_pisim stock hns_scan --out zpicture/hns_scan.json
```

### 2) 분석 (Python으로 JSON 처리 — 리포지토리 코드 수정 금지, 스크립트는 /tmp에)
전체 h1/5/10/20 통계에 더해 **속성별 부분집합의 h5·h20 (n / mean / edge=mean-baseline / 하락률)**:
- volume_ratio < 1 vs ≥ 1, 그리고 < 0.7 (정식 거래량 감소의 효과)
- neck_slope_pct ≤ 0 vs > 0 (정식은 수평·하향 넥라인)
- head_prom_pct 중앙값 상/하 및 > 5%
- shoulder_diff_pct < 5 vs 5~10
- **정식 풀셋**: volume_ratio<1 AND neck_slope_pct≤0 AND head_prom_pct>3 — n이 충분한지와 엣지
- 연도별 신호 수·h20 평균 (2025 쏠림 여부 — M패턴 때 2025년 361건 폭증 전례)
- arm_to_trigger_bars 분포
baseline은 JSON의 stats에 있는 baseline_mean_pct를 사용 (부분집합에도 동일 baseline 적용 명시).

### 3) 보고서 — `zpicture/hns_scan_report.md`
- 요약 3줄 (전체 엣지 / 정식 확증 조건이 엣지를 만드는가 / 판정·권고)
- 전체 horizon 표 + 속성별 부분집합 표
- 판정 기준: 부분집합 h5 또는 h20 edge가 음수이고 t 유의(≤-2) 또는 명확한 단조 경향이면 "조건부 유효"
- M패턴 결과(엣지 없음)와의 비교 한 단락
- 한계: in-sample, 종가 기준, 실행 층 미설계, 부분집합 다중비교 주의(사후 선택 편향)
- 수치 지어내지 말 것

### 4) 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 총 신호 수, 전체 h5/h20 edge·t, 최고 부분집합(조건·n·edge·t), 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 분석 스크립트는 /tmp 아래에만.
- 이 지시서는 삭제하지 말 것.
