# 작업: 상방 M자 패턴 전 종목 스캔 실행 + 음의 엣지 분석 보고서 (코드 수정 금지)

## 배경
사용자 설계 매도 신호: 5이평 변곡 3박스 resist(P1, BB상단 이탈)–support(MA20 위)–resist(P2, BB 내부)로
M 완성(장전) 후, 20봉 내 20이평 부근에서 음-양-음(연속 동색은 런으로 묶음) 반등 실패 붕괴가 트리거.
DefBox 무관. Claude가 구현·단위테스트 7종·30종목 스모크 완료 (신호 21건, h20 edge -2.4%p 방향 확인).
구현: cond/sell_mtop.go, stg/mpattern_analyze.go, study/mtop_scan.go

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 빌드 + 전 종목 스캔 (~20~30분)
```
cd /workspace && go build -o RESTGo_pisim . && ./RESTGo_pisim stock mtop_scan --out zpicture/mtop_scan.json
```
(hannam 4,307종목 × 4200봉. 진행 로그가 500종목 단위로 출력됨)

### 2) 분석 — JSON의 stats + examples 사용
- **음의 엣지 판정(핵심)**: h1/h5/h10/h20의 mean, median, 하락률, baseline 대비 edge, t-stat.
  매도 신호이므로 edge가 음수이고 |t|가 클수록 좋다. t ≤ -2 수준이면 유의미.
- 신호 규모: 총 신호 수, 종목당 빈도, 연도별 분포(trigger_date 기준 — 특정 연도 쏠림 여부)
- arm_to_trigger_bars 분포 (장전→발화 소요 봉수)
- 연도별 h20 평균 (강세장/약세장에서 신호 품질 차이 — 예: 2008, 2020, 2022)
- 참고: W중력 밀도 게이트처럼 M신호도 밀도(군집) 효과가 있는지 —
  `python3 py/analysis/portfolio_sim_policy.py`의 진단은 sell_trades 형식이라 직접 못 쓰니,
  examples의 trigger_date로 간단히 월별 신호 수 상위/하위 구간의 h20 평균만 비교해 기록
- 백테스트 지표는 아직 없음(진입/청산 룰 미정) — 전방 수익률 엣지 측정까지가 이번 범위

### 3) 보고서 — `zpicture/mtop_scan_report.md`
- 요약 3줄 (매도 엣지 존재 여부 / 어느 horizon에서 / short vs 보유분 청산 관점 함의)
- horizon별 표 (stats 그대로)
- 연도별 표, arm_to_trigger 분포
- 한계: 전방 수익률은 종가 기준, 거래비용 미반영(매도/short 실행 층은 미설계), in-sample
- 수치 지어내지 말 것. JSON에 없는 값은 "측정 불가"

### 4) 마무리
- 생성 파일 chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 총 신호 수, h5/h20 edge와 t-stat, 하락률, 판정

## 제약
- **코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
