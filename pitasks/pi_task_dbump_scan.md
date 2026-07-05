# 작업: r_stg 전략3 Double Bump 전 종목 측정 (Phase 2 — 코드 수정 금지)

## 배경
r_stg 이식 1순위. 원본: r_stg/전략3/double_bump_temp.r (2016년 R 스크리너, SQL 코어 충실 이식 —
R 후처리 제외조건 ~15종 미포함). 가설: 거래량 수반 1차 범프(1개월 신고가) 후 고점 미돌파
되돌림에서 2차 돌파를 노림. Claude가 armed 트리거 2변형 구현:
- `DoubleBumpRetest`: 원본 스크리너 그대로 — "재시도 준비일"(고점 아래 + 되돌림 35% 회복 등) 발화
- `DoubleBumpBreakout2`: 전략명의 본래 의도 — "범프 고점 첫 종가 돌파"(2차 돌파) 발화
60종목 스모크: Retest h20 edge -1.53%p(t=-2.9), Breakout2 -1.21%p(t=-1.6) — 음의 방향 예고.
참고: 어제 raw 재돌파 측정(재돌파 < 신규 돌파, t=-11)과 방향 일치 여부가 관전 포인트.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 스캔 시작/완료 보고.

## 단계 — 스캔 2회 순차 (각 ~20분)
```
go build -o RESTGo_pisim .
./RESTGo_pisim stock trigger_scan --trigger DoubleBumpRetest --out zpicture/dbump_retest_scan.json
./RESTGo_pisim stock trigger_scan --trigger DoubleBumpBreakout2 --out zpicture/dbump_break2_scan.json
```

## 분석 (Python, /tmp에만)
- 판정 기준 (매수 가설): edge>0 & t≥2 & >0.4%. 두 변형 각각 horizon별 표 (n/mean/median/승률/edge/t)
- 연도별 h20 (특정 국면 의존 여부)
- 원본 R 시절(2014~2016 구간)만 잘라낸 부분집합의 h20 — "당시엔 통했는가" (트리거 날짜로 슬라이스)
- **모든 부분집합에 t값 필수**

## 보고서 — `zpicture/dbump_scan_report.md`
- 요약 3줄 (두 변형 판정 / 2014~16 구간과 전체 기간 차이 / r_stg 이식 관점 결론)
- 표 전부 + 한계 (R 후처리 제외조건 미포함 — 원본은 더 선별적이었음을 명시)
- 수치 지어내지 말 것

## 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 두 변형의 n·h5/h20 edge·t + 2014~16 부분집합 + 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
