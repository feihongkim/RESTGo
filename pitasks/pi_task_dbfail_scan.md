# 작업: DefBox 돌파 실패(재붕괴) short 가설 측정 (코드 수정 금지)

## 배경 (사용자 가설, 2026-07-05)
strategy1의 트리거인 DefBox 돌파는 성공 시 수익이 크지만 실패(재붕괴)가 절반 이상.
실패 자체를 short 트리거로 활용 가능한가 + M패턴 구조와 겹치는 경우는 더 강한가.
Claude 구현: armed 트리거 `DefBoxBreakoutFailure`(돌파[가격+거래대금+ATR 게이트]=장전 → 20봉 내
종가가 박스가 아래 재붕괴=발화) + 조건 `HasMTopStructure`(M 3박스 구조 존재).
engine-parity 검증 완료(5케이스 6,352신호 불일치 0). 60종목 스모크: 돌파 3,282 vs 실패 2,457(≈75%),
M 겹침은 15건뿐(0.6% — 겹침 자체가 희소하다는 사전 관찰).

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 스캔 시작/완료 보고.

## 단계 — 스캔 3회 순차 실행 (각 ~20분, 빌드 1회)
```
go build -o RESTGo_pisim .
./RESTGo_pisim stock trigger_scan --trigger DefBoxBreakoutFailure --out zpicture/dbfail_scan.json
./RESTGo_pisim stock trigger_scan --trigger DefBoxBreakoutFailure --when HasMTopStructure --out zpicture/dbfail_mtop_scan.json
./RESTGo_pisim stock trigger_scan --trigger DefBoxBreakout --out zpicture/dbbreak_scan.json
```

## 분석 (Python, /tmp에만)
- **판정 기준 (short 가설)**: edge가 음수 & t ≤ -2 & |edge| > 0.4%(왕복비용) — h5·h20 중심
- ① 실패 전체: horizon별 표 (n/mean/median/하락률/edge/t)
- ② M 겹침 부분집합: 같은 표 + n이 작으면(<300 예상) "소표본, 참고만" 명시
- ③ 실패-비겹침 = ①의 examples에서 ②의 (shcode,trigger_date) 키를 뺀 나머지로 직접 계산 (스캔 불필요)
- ④ 실패율 추정: ③번 스캔(돌파 전체) 신호 수 vs ① 신호 수 → 실패율 %. 재교차 중복 가능성 명시
- ⑤ 연도별 분포 (① 기준) — 약세장 집중 여부
- 참고: h20 전방수익이 양수라도 baseline보다 낮으면 "덜 오름"이지 "떨어짐"이 아님 —
  H&S 때와 동일 기준으로 절대수익 부호도 명시할 것

## 보고서 — `zpicture/dbfail_scan_report.md`
- 요약 3줄 (실패의 short 엣지 / M 겹침 효과 / 실패율)
- 위 표 전부. **모든 부분집합에 t값 필수**
- strategy1 관점 함의 한 단락: 실패율이 이 수준일 때 돌파 매수의 비대칭 구조
- 한계: in-sample, 재교차 중복, M겹침 소표본
- 수치 지어내지 말 것

## 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 실패율, ① h5/h20 edge·t·절대수익 부호, ② n·edge·t, 판정

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
