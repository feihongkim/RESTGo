# 작업: slope(기울기) 조건 검증 — 배터리 + walkforward 재실행 + 보고서 (코드 수정 금지)

## 배경
관성(inertia) 검증(2026-07-04)에서 "고밀도+기울기 상승 +4.90%/승률 73% vs 고밀도+하강 +2.62%/55.9%" 발견.
2023 폴드 역전(-2.10pp)의 주범이 통과+하강 143건(+0.59%)이었다. Claude가 시뮬레이터에
`density_slope_filter` 정책(밀도≥lo AND 기울기≥slope_min; 기울기=최근14일-이전14일 신호 수, causal)과
walkforward 규칙 C(A의 lo + slope≥1)를 구현·스모크 검증 완료.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 배터리 재실행 (수 초)
```
cd /workspace && python3 py/analysis/portfolio_sim_policy.py --battery zpicture/wdefbox_portfolio_trades.json
```
→ `zpicture/alloc_policy_results.json` 갱신 (기존 11개 + slope 4개 = 15개 설정)

**무결성 앵커 1**: fifo N=35 = CAGR 3.68% / 최종 1.9529, density_filter lo=47 K=50 = CAGR 5.09% / 최종 2.5102.
불일치 시 중단·보고.

### 2) walkforward 재실행 (수 초)
```
cd /workspace && python3 py/analysis/walkforward_density.py zpicture/wdefbox_portfolio_trades.json
```
→ `zpicture/walkforward_density_results.json` 갱신 (규칙 C 추가)

**무결성 앵커 2**: 규칙 A 집계 = 11/14승, diff 중앙값 +1.24pp, 체인 CAGR 4.20% / 규칙 B 체인 4.75% /
fifo 체인 3.73%가 기존과 동일해야 함 (규칙 C는 순수 추가). OOS 2022 폴드 C = pass n=898, diff +4.772pp, 연수익 19.8%.

### 3) 보고서 — `zpicture/wdefbox_slope_filter_report.md`
JSON 수치 그대로:
- 요약 3줄 (slope 조건이 in-sample과 OOS 모두에서 개선되는가 / 2023 구제 여부 / 권고)
- 배터리 표: slope 4개 설정 vs 기존 승자 density_filter(lo=47,K=50) vs fifo — CAGR/MDD/Sharpe/소화율/진입평균
  - 특히 slope 단독(lo=0) ablation이 수준 필터 없이 어디까지 가는지 명시
- walkforward 표: 폴드별 A vs C (lo/diff/연수익) 나란히 + 집계 비교 (pass_beats_fail, diff 중앙값, 체인 CAGR)
- **2023 폴드 집중 분석**: A(-2.10pp) → C에서 얼마나 개선되는가. 관성 가설의 OOS 최종 판정
- 상승장 비용: 2022처럼 A가 이기는 해에 C가 얼마나 양보하는지 (스모크에서 21.35%→19.8% 관찰됨)
- 해석 주의: slope 발견 자체가 in-sample이었으므로 walkforward 규칙 C 결과가 최종 근거라는 점,
  기울기·밀도 모두 causal(트레일링), K=50 고정, MDD 원가 하한
- 결론: density_filter(q60) vs density_slope_filter(q60+slope≥1) 중 실운용 권고 + 근거

### 4) 마무리
- 생성·갱신 파일 chown 1000:1000
- 마지막 응답: 배터리 slope 주 후보(lo=q60,K=50,slope≥1)의 CAGR/MDD/Sharpe/진입평균 + walkforward C 집계(승수, diff 중앙값, 체인 CAGR) + 2023 폴드 A vs C

## 제약
- **코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 수치 지어내지 말 것. JSON에 없는 값은 "측정 불가".
- 이 지시서는 삭제하지 말 것.
