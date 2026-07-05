# 작업: 골든크로스 임박(GC-pending) 국면 × W패턴/strategy1 측정 (코드 수정 금지)

## 배경 (사용자 설계, 2026-07-05)
가설: MA60<MA120 역배열인데 간격이 지속 축소돼 골든크로스 임박 = 하락 관성 소진 국면.
이 국면에서 W패턴/strategy1 발동 시 품질이 오르는가. W는 BB 하단 이탈 조건과 국면이 상충할 수 있어
BB 급락 조건을 완화판으로 풀고 속성으로 기록 — 한 스캔에서 2×2(bb_crash × gc_pending) 분해.

GC 정의: MA60<MA120 + 간격 (MA120-MA60)/MA120이 20봉 지속 축소(5봉 체크포인트 4개) + 간격 ≤3%.
연속량(gc_gap_pct, gc_shrink)도 기록됨 — 임계 스윕 가능.

구현(Claude): cond/buy_regime.go(GoldenCrossPendingInfo, 테스트 4종), FindBBWBottomBoxPatternRelaxed
(운영 경로 불변 — 기존 테스트 전부 통과), stg/wgc_analyze.go, study/wgc_scan.go(strategy1 후계산 포함).
60종목 스모크: W완화 743건(그중 bb_crash 117), GC임박 3.8%, S1 대응 98건.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 빌드 + 전 종목 스캔 (~25분)
```
cd /workspace && go build -o RESTGo_pisim . && ./RESTGo_pisim stock wgc_scan --out zpicture/wgc_scan.json
```
완료 시 stdout에 2×2 h20 요약이 출력됨 — 보고서에 그대로 인용 가능.

### 2) 분석 (Python, /tmp에만) — h5·h20 기준 (n / mean / median / 승률 / edge=mean-baseline / t)
**W패턴 (w_examples):**
- 2×2: bb_crash × gc_pending 4셀 + 행/열 합계
- GC 연속량 스윕: gc_gap_pct 3분위 × 성과, gc_shrink(4~8) 별 성과 — 임계 3%가 적절한지
- 3중 참고: bb_crash × gc_pending × has_defbox (n<100 셀은 "소표본" 표기)
- **핵심 질문 2개에 명시적 답**: ① BB급락과 GC임박은 실제로 상충하는가(교집합 n)
  ② GC임박이 W 품질을 올리는가 (완화판·급락판 각각에서)
**strategy1 (s1_examples):**
- gc_pending vs not: h5/h20 비교 (참고: s1 신호는 매수 h20 전방수익 기준 — 매도 실현과 다름을 명시)
- gc_gap_pct 3분위
- 전략(rule)별 분해는 n≥100인 것만
**공통:** 연도별 GC임박 신호 분포 (특정 국면 연도 쏠림 확인 — 2009, 2020 등 회복기 예상)

### 3) 보고서 — `zpicture/wgc_scan_report.md`
- 요약 3줄 (가설 지지 여부 W/strategy1 각각 / BB 상충 여부 / 권고)
- 위 표 전부. 판정 기준: edge>0 & t≥2 & 비용(0.4%) 초과
- 한계: in-sample, GC임박 신호 희소(~4%)로 t 검정력 주의, 다중비교
- 수치 지어내지 말 것. **부분집합에는 반드시 t값을 붙여라** (이전 실수 재발 금지)

### 4) 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: W 2×2 4셀(n/h20 edge/t), S1 gc_pending vs not(n/edge/t), 핵심 질문 2개 답

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
