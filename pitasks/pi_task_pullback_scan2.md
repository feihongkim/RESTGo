# 작업: 눌림 돌파 재측정 (+++ 폐지, streak=0) + slope 속성 분석 (코드 수정 금지)

## 배경
1차(streak=3, +++ 이중 요구)는 신호 296건·유의성 없음·복권형으로 기각 (zpicture/pullback_scan_report.md).
사용자 결정으로 +++ 폐지 → streak 파라미터화 완료 (기본 0). 50종목 스모크: 신호 423건 (42배 증가),
엣지는 중립 — 추세 조건 제거로 하락장 반등이 섞였을 가능성. **핵심 질문: 기록된 ma20_slope_pct
속성으로 사후 분해했을 때 "완만한 상승 이평" 중간 지대에 엣지가 있는가** (+++보다 부드러운 필터 탐색).

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 빌드 + 전 종목 스캔 (~20분, streak 기본 0)
```
cd /workspace && go build -o RESTGo_pisim . && ./RESTGo_pisim stock pullback_scan --out zpicture/pullback_scan_s0.json
```
(1차 결과 zpicture/pullback_scan.json은 덮어쓰지 말 것 — 출력 파일명 주의)

### 2) 분석 (Python, /tmp에만)
- 전체 h1/5/10/20 (edge 양수 & t≥2 기준)
- **ma20_slope_pct 분해 (핵심)**: 5분위별 h5·h20 (n/mean/edge/승률) — slope>0 구간에 엣지가 있는지,
  단조 관계인지. 추가로 slope>0 vs ≤0 이분, slope 상위 25%
- depth_pct 3분위, touch_count 1 vs ≥2, arm_to_trigger_bars 분포
- slope>0 AND depth 얕음 같은 2차 교차는 n≥300인 조합만
- 연도별 신호 수·h20 평균
- 1차(streak=3, 296건, h5 +0.70 t=1.06) 결과와 비교 표

### 3) 보고서 — `zpicture/pullback_scan_s0_report.md`
- 요약 3줄 (전체 엣지 / slope 분해에서 중간 지대 발견 여부 / 권고)
- 위 표들 + 1차 대비 비교
- 다중비교 주의 명시 (slope 분해는 사후 선택)
- 수치 지어내지 말 것

### 4) 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 총 신호 수, 전체 h5/h20 edge·t, slope 최고 분위의 n·edge·t, 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
