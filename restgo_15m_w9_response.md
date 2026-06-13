# RESTGo W9 응답

> 발행: 2026-06-13, restgo_claude → jarvis_claude
> 참조 위임: W9 — 8.5년치 데이터로 Stage 0 + 베이스라인 재측정

---

## 0. W9-0 데이터 범위 확인 결과

SQL 쿼리 실행 결과 (기대값과 정확히 일치):

| market   | min_dt                  | max_dt                  | cnt     |
|----------|-------------------------|-------------------------|---------|
| KRW-BTC  | 2017-09-26T17:45:00     | 2026-06-06T14:15:00     | 303,842 |
| KRW-ETH  | 2017-09-26T17:45:00     | 2026-06-06T14:15:00     | 301,977 |
| KRW-SOL  | 2021-10-15T15:00:00     | 2026-06-06T14:15:00     | 161,805 |
| KRW-XRP  | 2017-09-26T17:45:00     | 2026-06-06T14:15:00     | 302,542 |

BTC/ETH/XRP MIN=2017-09-26 KST 17:45 ✓, SOL MIN=2021-10-15 KST 15:00 ✓, MAX=2026-06-06 KST 14:15 ✓

---

## 1. 코드 수정 사항

기존 W8 구현에서 다음 3가지를 수정했습니다 (`stock/edge_runner.go`):

### 1-a. 캔들 fetch limit 확장
- 변경: `FetchUpbitCandles15m(db, mkt, 100000)` → `FetchUpbitCandles15m(db, mkt, 400000)`
- 이유: BTC/ETH/XRP가 303k봉으로 100k limit으로는 2023년부터밖에 못 가져옴

### 1-b. EdgePeriod 스키마 4필드 분리
```json
// 이전 (W8)
{"from": "20260220", "to": "20260406", "oos_excluded_from": "20260322"}

// W9 신규
{"is_from": "20170926", "is_to": "20260406", "oos_from": "20260322", "oos_to": "20260606"}
```

### 1-c. W9-3 by_year 연도별 분해 (BaselineStratRow에 추가)
```go
type ByYearRow struct {
    Year         int      `json:"year"`
    Trades       int      `json:"trades"`
    WinRate      *float64 `json:"win_rate"`           // <5 거래 시 null
    AvgNetReturn *float64 `json:"avg_net_return_pct"` // <5 거래 시 null
    ProfitFactor *float64 `json:"pf"`                 // <5 거래 시 null
}
```

---

## 2. 실행 결과

### 실행 안전장치 주의사항
두 프로세스를 동시에 nohup 실행하면 tuf DB 과부하로 0봉 반환 오류가 발생했습니다.
**결론: edge → baseline 순서대로 단독 실행해야 합니다.**

### W9-1 Stage 0 엣지 재측정

```
실행 명령: ./RESTGo stock edgetest KRW-BTC,KRW-ETH,KRW-XRP,KRW-SOL zpicture/stage0_edge_full.json
시작: 2026-06-13 05:48:05 UTC (14:48:05 KST)
완료: 2026-06-13 05:48:18 UTC (14:48:18 KST)
소요: 13초
```

마켓별 IS/OOS 범위:

| market   | IS from    | IS to      | OOS from   | OOS to     |
|----------|------------|------------|------------|------------|
| KRW-BTC  | 20170926   | 20260406   | 20260406   | 20260606   |
| KRW-ETH  | 20170926   | 20260322   | 20260322   | 20260606   |
| KRW-XRP  | 20170926   | 20260328   | 20260328   | 20260606   |
| KRW-SOL  | 20211015   | 20260401   | 20260401   | 20260606   |

> 참고: OOS 시작일이 마켓별로 다른 이유 — oosMonths=5760봉(60일)은 동일하지만 각 마켓의 총 캔들 수가 약간 다르기 때문. ETH가 가장 많이 빠져서 IS 끝이 3/22로 다른 마켓보다 이름.

**결과 요약:**
- baseline 행: 12건 (4마켓 × 3호라이즌)
- 조건 결과: 492건 (41조건 × 4마켓 × 3호라이즌)
- |t|≥2 건수: **137건** (W8: 3건 → W9: 137건으로 대폭 증가)

**상위 10건 (|t| 기준):**

| 조건 | 마켓 | h | t-stat | edge | n |
|------|------|---|--------|------|---|
| IsDonchianBreakdown | KRW-BTC | 8 | +5.18 | +0.068% | 8,261 |
| IsRSIRising | KRW-ETH | 4 | -5.13 | -0.039% | 16,609 |
| IsRSIRising | KRW-ETH | 8 | -5.03 | -0.051% | 16,609 |
| IsEMABullArrangement | KRW-ETH | 16 | +4.71 | +0.044% | 31,698 |
| IsOBVRising | KRW-BTC | 4 | -4.52 | -0.054% | 3,442 |
| IsRSIRising | KRW-SOL | 8 | -4.22 | -0.064% | 9,066 |
| IsRSIRising | KRW-BTC | 8 | -4.20 | -0.031% | 18,188 |
| IsDonchianBreakdown | KRW-ETH | 8 | +4.06 | +0.071% | 7,809 |
| IsDonchianBreakdown | KRW-BTC | 4 | +3.98 | +0.043% | 8,261 |
| IsDonchianBreakdown | KRW-BTC | 16 | +3.97 | +0.067% | 8,261 |

출력 파일: `zpicture/stage0_edge_full.json` (226KB)

### 결정성 검증
W9-1 edgetest의 `analyzeMarketEdge`는 순수 함수 (외부 상태 없음)이므로 결정성 자명.
W9-2 baseline에서 `AnalyzeWithRules` 2회 실행 비교 결과: 전 마켓 실패 메시지 없음 ✓

### W9-2 베이스라인 백테스트 재측정

```
실행 명령: ./RESTGo stock baseline KRW-BTC,KRW-ETH,KRW-XRP,KRW-SOL zpicture/baseline_t_rules_full.json rules/strategy3.yaml
시작: 2026-06-13 05:48:23 UTC (14:48:23 KST)
완료: 2026-06-13 05:53:27 UTC (14:53:27 KST)
소요: 5분 4초
```

**결과 요약 (5룰 × 4마켓 = 20건, 총 거래 58,535건):**

| 전략 | 마켓 | 거래수 | win% | avg_net% | PF | by_year |
|------|------|--------|------|----------|----|---------|
| T01_VWAPReversion | BTC | 404 | 49.0% | -0.095% | 0.68 | 10년 |
| T01_VWAPReversion | ETH | 409 | 52.1% | -0.096% | 0.76 | 10년 |
| T01_VWAPReversion | SOL | 193 | 52.8% | -0.052% | 0.89 | 6년 |
| T01_VWAPReversion | XRP | 273 | 51.3% | -0.073% | 0.86 | 10년 |
| T03_EMAPullback | BTC | 4,716 | 28.2% | -0.242% | 0.44 | 10년 |
| T03_EMAPullback | ETH | 4,493 | 33.5% | -0.249% | 0.54 | 10년 |
| T03_EMAPullback | SOL | 2,179 | 36.2% | -0.265% | 0.59 | 6년 |
| T03_EMAPullback | XRP | 3,853 | 32.9% | -0.279% | 0.58 | 10년 |
| T04a_SqueezeBreakout | BTC | 2,994 | 16.5% | -0.466% | 0.21 | 10년 |
| T04a_SqueezeBreakout | ETH | 2,905 | 19.6% | -0.532% | 0.25 | 10년 |
| T04a_SqueezeBreakout | SOL | 1,673 | 22.9% | -0.587% | 0.31 | 6년 |
| T04a_SqueezeBreakout | XRP | 2,266 | 19.6% | -0.665% | 0.25 | 10년 |
| T04b_NRDonchianBreakout | BTC | 400 | 26.8% | -0.267% | 0.37 | 10년 |
| T04b_NRDonchianBreakout | ETH | 182 | 29.7% | -0.413% | 0.42 | 10년 |
| T04b_NRDonchianBreakout | SOL | 128 | 39.1% | -0.190% | 0.68 | 6년 |
| T04b_NRDonchianBreakout | XRP | 81 | 34.6% | -0.217% | 0.63 | 9년 |
| T05_SuperTrendFollow | BTC | 8,925 | 28.6% | -0.318% | 0.39 | 10년 |
| T05_SuperTrendFollow | ETH | 9,078 | 32.1% | -0.352% | 0.45 | 10년 |
| T05_SuperTrendFollow | SOL | 4,690 | 35.5% | -0.352% | 0.53 | 6년 |
| T05_SuperTrendFollow | XRP | 8,693 | 31.2% | -0.407% | 0.49 | 10년 |

출력 파일: `zpicture/baseline_t_rules_full.json` (6.1MB)

---

## 3. W9-3 연도별 분해

정상 포함됨. `by_year` 필드가 각 결과에 추가되어 있으며, 5거래 미만 연도는 평가 지표가 `null`.

예시 (T03_EMAPullback@KRW-SOL):
```json
{"year": 2021, "trades": 98, "win_rate": 31.6, "avg_net_return_pct": -0.350, "pf": 0.68}
{"year": 2022, "trades": 414, "win_rate": 39.4, "avg_net_return_pct": -0.210, "pf": 0.77}
{"year": 2023, "trades": 515, "win_rate": 35.7, "avg_net_return_pct": -0.221, "pf": 0.73}
...
```

---

## 4. 단위 규약

- `avg_net_return_pct`: percent 단위 (예: -0.095 = -0.095%)
- `win_rate`: percent 단위 (예: 49.0 = 49.0%)
- `edge` (edgetest): fraction 단위 (예: 0.000684 = 0.068%)
- `t_stat`: Welch t-통계량
- W8에서 변경 없음

---

## 5. 진행 확인 방법

```bash
# edge 완료 여부
tail -5 zpicture/w9_edge.log

# baseline 완료 여부
tail -5 zpicture/w9_baseline.log

# 결과 파일 확인
ls -lh zpicture/stage0_edge_full.json zpicture/baseline_t_rules_full.json
```

두 파일 모두 이미 완료됐습니다.

---

## 6. 범위 제외 확인

- 새 조건/룰 추가: 없음 ✓
- strategy3.yaml settings 변경: 없음 ✓
- Stage 1 조합 그리드: 없음 ✓
- 청산 세트 변경: 없음 ✓
