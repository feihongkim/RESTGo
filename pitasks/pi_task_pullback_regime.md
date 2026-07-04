# 작업: 눌림돌파 조건 조정 + 시장국면 2종 테스트 (코드 수정 금지)

## 배경 (사용자 설계 추가, 2026-07-04)
1) 패턴 조정: R→S 봉 간격 ≤ 7 (저항 후 오래 흐르면 안 됨) — 기록된 r_to_s_bars로 슬라이스
2) 국면a: 트리거 시점에 최근 DefBox가 MA20 위 (W중력 유사 상방 중력) — defbox_above_ma20
3) 국면b: 트리거 시점에 볼린저 밴드 확장 중 (스퀴즈 아님, W바텀 P1과 동일 정의) — bb_expanding

Claude가 스캐너에 국면 속성 기록을 추가했다 (탐지 로직 불변 — 60종목 회귀에서 신호 473건 완전 동일 확인).
DefBox 생성은 stg/analyzer.go와 동일 경로(CheckAndCreateDefBox), 패턴 탐지에서는 DefBox 제외.
스모크 속성 비율: rs≤7 54% / defbox 존재 97% / 국면a 85% / 국면b 15%.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 빌드 + 전 종목 스캔 (~20분)
```
cd /workspace && go build -o RESTGo_pisim . && ./RESTGo_pisim stock pullback_scan --out zpicture/pullback_scan_regime.json
```
(출력 파일명 주의 — 기존 pullback_scan_s0.json 덮어쓰기 금지)

### 2) 분석 (Python, /tmp에만) — 변형별 h5·h20 (n / mean / median / 승률 / edge / t)
**주 변형 (사용자 요청 2가지 + 참조):**
- V0 (조정 베이스): r_to_s_bars ≤ 7
- **VA (국면a)**: rs≤7 AND defbox_above_ma20
- **VB (국면b)**: rs≤7 AND bb_expanding
- VAB: rs≤7 AND a AND b (n 확인 후 소표본이면 표기만)
**귀속 분석 (어느 조건이 기여하는지):**
- rs≤7 vs rs>7 (조건1 단독 효과)
- 국면a 단독 (rs 무관) vs ¬a (defbox 아래/없음 — 대조군)
- 국면b 단독 vs ¬b
- defbox_dist_pct 3분위 (중력 거리 효과)
- 판정 기준: edge>0 & t≥2, 그리고 실용성은 edge가 왕복비용 0.4%를 넘는가
- 연도별 분포 (주 변형 기준)

### 3) 보고서 — `zpicture/pullback_regime_report.md`
- 요약 3줄 (조건1 효과 / 국면a vs 국면b 어느 쪽이 유효한가 / 권고)
- 변형·귀속 표 전부
- 이전 결과와의 비교: streak=0 전체(23,773건, h20 edge 0) 대비 개선폭
- 한계: in-sample, 다중비교(변형이 늘수록 우연 통과 위험), 국면a 기저율 85%(선택성 낮음 주의)
- 수치 지어내지 말 것

### 4) 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: V0/VA/VB/VAB 각각의 n·h5·h20 edge·t + 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
