# 작업: r_stg 일봉 전략 3종(9·7·14) 전 종목 측정 + r_stg 종합 판정 보고서 (코드 수정 금지)

## 배경
r_stg Phase 2 마무리. 현재까지 판정: 전략3 기각 / 전략6 기각(크립토) / 전략2 기각(크립토, 신호 747 엣지 0) /
전략4·5 구조적 부적합(갭 의존) / **전략11 크립토 핵심형 유력**(h10 +2.06%p t=3.61, 스윕 단조 강건).
남은 일봉 3종을 Claude가 이식 완료 (cond/buy_rstg_more.go — 스크리너의 미래 참조 조건은 제거):
- `Stg9ApexPerch`: 비율 정배열 + 5이평 상승변곡 + 박스권 천장(직전 10일 고점의 93~109%) 근접 + 거래대금 9억
- `Stg7GCAccel`: 20-60 GC 후 5~30일 + 20이 8일 전 대비 1%+ 상승 + 20이 60보다 빠름 (성립 순간 edge)
- `Stg14Oversold`: 정배열 10일 유지 + 20이평 3일 연속 상승 중 당일 시가>60이평 → 종가<120이평 극단 급락

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 스캔 시작/완료 보고.

## 단계 — 스캔 3회 순차 (각 ~20분, hannam 일봉)
```
go build -o RESTGo_pisim .
./RESTGo_pisim stock trigger_scan --trigger Stg9ApexPerch --out zpicture/stg9_scan.json
./RESTGo_pisim stock trigger_scan --trigger Stg7GCAccel --out zpicture/stg7_scan.json
./RESTGo_pisim stock trigger_scan --trigger Stg14Oversold --out zpicture/stg14_scan.json
```

## 분석 (Python, /tmp에만)
- 판정 기준 (매수): edge>0 & t≥2 & >0.4%. 전략별 horizon 표 + 연도별 h20 + **부분집합/비교에 t값 필수**
- 전략7은 W중력과의 개념 겹침이 있으므로: 신호 빈도·시기 특성을 W중력(공황 바닥 후)과 대비해 서술

## 보고서 — `zpicture/rstg_phase2_report.md` (r_stg Phase 2 종합)
- 요약: r_stg 전체 전략의 최종 성적표 (측정 완료 전부 — 3/6/2/4·5/11/9/7/14 + 미측정 보류 목록)
- 이번 3종의 상세 표
- 전략11 요약 재기록 (유력 후보 — 이미 측정된 수치 인용: zpicture/stg11_upbit_scan.json)
- 한계: in-sample, 이식 시 변환 판단 목록(명세 참조), 원본 후처리 필터 미포함
- 수치 지어내지 말 것

## 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 3종 각각 n·h5/h20 edge·t·판정 + r_stg 종합 성적 한 줄

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 이 지시서는 삭제하지 말 것.
