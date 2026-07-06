# 작업: "돌파 전 선매수" 매트릭스 측정 — X(반전 이벤트) × S1 조건 셋 (코드 수정 금지)

## 배경
사용자 구상: strategy1의 돌파일 조건을 사전 평가해 DefBox 아래에서 선매수 (zpicture/s1_condition_taxonomy.md).
Claude가 조건 분류·복합 조건 등록 완료: IsS1SetupTerrain(지형 9조건+윗꼬리NOT), IsS1EntryQuality(양봉+MA수렴).
이미 판정된 X: BB하단이탈+상방DefBox = 무의미(228,421신호, h20 +0.19%p t=1.25 — 떨어지는 칼 확인).

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 스캔 시작/완료 보고.

## 단계 — 스캔 9회 순차 (hannam 일봉, 각 ~20-35분)
```
go build -o RESTGo_pisim .
for X in DefBoxApproach BBLowerReentry WBottomBox; do
  ./RESTGo_pisim stock trigger_scan --trigger $X --out zpicture/pbm_${X}_alone.json
  ./RESTGo_pisim stock trigger_scan --trigger $X --when IsS1SetupTerrain --out zpicture/pbm_${X}_terrain.json
  ./RESTGo_pisim stock trigger_scan --trigger $X --when IsS1SetupTerrain,IsS1EntryQuality --out zpicture/pbm_${X}_full.json
done
```

## 분석 (Python, /tmp에만)
- 3×3 매트릭스 표: X × {단독/지형/지형+품질} — h5/h20 (n/mean/median/승률/edge/t)
- **연도별 분해 필수** — 특정 1~2연도가 edge 50%+면 국면 효과 표시 (유효 판정 금지)
- 비교 기준 명시: strategy1 본판(돌파 후, 매수 h20 edge +1.31%p t=6.8 — wgc_scan bb_crash 셀 기준은 W중력)
  ↔ 실제 strategy1 h20은 zpicture/strategy1_density_study.json stats 참조
- 판정: 조건 셋이 얼마나 보태는가(단독→지형→풀 증분), 어떤 X가 최선인가, 선매수가 돌파 후 매수를 이기는가

## 보고서 — `zpicture/prebreakout_matrix_report.md`
- 요약 3줄 + 매트릭스 표 + 연도 분해 + 판정
- 한계: in-sample, 다중비교(9셀), 매도 미결합(전방수익 기준)
- **모든 셀에 t값**. 수치 지어내지 말 것

## 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 9셀 요약 + 최선 조합 + 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
