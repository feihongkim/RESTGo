# 작업: 배분 정책 배터리 실행 + 보고서 작성 (코드 수정 금지 — 실행·집계·보고만)

## 배경
FIFO 슬롯 역선택(진입 +0.75% vs 스킵 +4.26%)을 해소하기 위한 배분 정책 실험.
시뮬레이터 `py/analysis/portfolio_sim_policy.py`(v3)는 Claude가 설계·구현·검증 완료:
- fifo N=35가 v2 기준선을 정확히 재현함을 확인 (CAGR 3.68%, 최종자본 1.9529)
- 진단(밀도 5분위)도 내장: Q1(저밀도) +0.42% → Q5(군집 심부) +4.89%/승률 71.9% 단조 증가 — "군집 심부일수록 신호가 좋다" 확인됨

정책 4종: fifo(기준) / cash_frac(슬롯 없이 equity/K, 현금이 자연 한도) / burst_half(밀도 T 이상이면 절반 크기) / density_filter(밀도 밴드만 진입). 상세는 스크립트 docstring 참조.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 배터리 실행 (수 초 소요)
```
cd /workspace && python3 py/analysis/portfolio_sim_policy.py --battery zpicture/wdefbox_portfolio_trades.json
```
→ `zpicture/alloc_policy_results.json` 생성 (진단 + 11개 설정 결과)

**무결성 앵커**: 결과 중 fifo N=35 행이 CAGR 3.68% / MDD 37.32% / 최종 1.9529 / 소화율 54.4%와 일치해야 한다. 불일치 시 중단하고 보고하라.

### 2) 보고서 작성 — `zpicture/wdefbox_alloc_policy_report.md`
JSON에서 그대로 읽은 수치로:
- 요약 3줄 (최고 정책과 기준 대비 개선폭 / 역선택 해소 여부 / 권고)
- 진단 표: 밀도 5분위 × (건수/평균/승률) — diagnostic_density 절
- 정책 비교표: 정책·파라미터 / CAGR / MDD / Sharpe / 소화율 / 진입평균 / 스킵평균 / 평균·최대 동시 포지션
- 역선택 해소 분석: 각 정책의 진입평균 vs 전체평균(+2.35%) — 진입평균이 전체평균에 근접할수록 역선택 해소
- 최고 CAGR 정책 + 최고 Sharpe 정책의 연도별 수익률 표 (fifo N=35와 나란히 비교)
- 해석 주의 사항 (반드시 포함):
  - burst_half·density_filter의 임계값 T는 전체 기간 밀도 분위수(in-sample) — 과최적화 단서. 단 밀도 자체는 트레일링 28일 계산이라 미래 정보 아님(causal)
  - MDD는 실현+원가 기준 하한 추정치 (보유 중 시세 평가손 미반영)
  - density_filter 소강기 전용(lo=0, hi=q40)은 대조군 — 성과가 나쁠 것으로 예상되는 설정
- 결론: W중력 실운용 배분 정책 권고 1개 + 근거

### 3) 마무리
- 생성 파일 chown 1000:1000
- 마지막 응답으로 핵심 수치(최고 정책의 CAGR/MDD/Sharpe/소화율, fifo 대비 개선폭) 보고

## 제약
- **코드·rules 수정 금지.** 실행이 에러나면 고치지 말고 에러 전문을 보고하라.
- 수치를 지어내지 말 것. JSON에 없는 값은 "측정 불가"로 표기.
- 이 지시서는 삭제하지 말 것.
