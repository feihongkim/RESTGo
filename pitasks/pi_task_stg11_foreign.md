# 작업: 전략11 해외 4시장 검증 (JP/CN/HK/US × 96/128봉) — 연도 분해 필수 (코드 수정 금지)

## 배경 (중요 — 프레임 변경)
전략11 일봉(한국)은 **2026 강세장 아티팩트로 철회됨** (128봉 +6.00 t=4.02 → 2026 제외 +1.77 t=1.08).
크립토 15분봉만 생존 (h10 +2.06 t=3.61, 연도 분산 건전). 이번 해외 검증의 목적은 빈도 확대가 아니라
**독립 데이터에서 일봉 신호의 재활 여부 판정**: 4개 해외 시장(각 ~5-6년)에서 연도 분해를 통과하는
일관된 엣지가 나오면 일봉 재평가, 아니면 일봉 기각 확정.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 스캔 시작/완료 보고.

## 단계 — 스캔 8회 순차 (각 ~15-25분)
```
go build -o RESTGo_pisim .
for MKT in us jp cn hk; do
  ./RESTGo_pisim stock trigger_scan --trigger Stg11MA60Breakdown --foreign-$MKT --set Stg11AlignedBars=96 --out zpicture/stg11_${MKT}96_scan.json
  ./RESTGo_pisim stock trigger_scan --trigger Stg11MA60Breakdown --foreign-$MKT --set Stg11AlignedBars=128 --out zpicture/stg11_${MKT}128_scan.json
done
```

## 분석 (Python, /tmp에만) — **연도 분해가 핵심 판정**
- 시장×봉수별 h5/h20 표 (n/mean/median/승률/edge/t)
- **필수: 각 셀의 연도별 분해** — 특정 1~2개 연도가 edge의 50%+를 만들면 "국면 효과 의심" 표시.
  전체 t가 유의해도 연도 분해를 못 넘으면 유효 판정 금지 (한국 일봉의 실패 사례와 동일 기준)
- 시장 간 일관성: 4시장 중 몇 곳에서 같은 방향인가
- 판정: "일봉 전략11 재활 가능 여부" 명시적 답

## 보고서 — `zpicture/stg11_foreign_report.md`
- 요약 3줄 (재활 여부 / 시장별 성적 / 크립토와의 대비)
- 표 전부 + 한계 (해외 5-6년, 활성 종목만 목록 조회 = 생존 편향 가능성 명시)
- 수치 지어내지 말 것. **모든 부분집합에 t값**

## 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 시장×봉수 8셀 요약 + 연도 분해 통과 여부 + 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
