# 작업: density_filter 임계값 walkforward IS/OOS 검증 실행 + 보고서 (코드 수정 금지)

## 배경
배분 정책 실험(zpicture/wdefbox_alloc_policy_report.md)의 승자 density_filter(lo=47=전체기간 q60, K=50)의
임계값이 in-sample이라 과최적화 검증이 필요하다. Claude가 walkforward 스크립트
`py/analysis/walkforward_density.py`를 설계·구현·스모크 검증 완료 (OOS 2022 폴드 정상 동작 확인).

설계 요지 (스크립트 docstring 참조):
- IS 4년 → OOS 1년 슬라이딩, OOS 2012~2026 (~15폴드), K=50 고정
- 규칙 A: lo = IS 밀도 q60 (고정 규칙의 일반화 검증)
- 규칙 B: IS 그리드(q40~q80)에서 Sharpe 최대 선택 (튜닝 절차의 과최적화 측정)
- 폴드별: OOS 신호 수준(통과군 vs 미달군 평균 수익 diff — 1차 근거) + 포트폴리오 연수익(A/B/fifo)

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/완료 보고.

## 단계

### 1) 실행 (수 초)
```
cd /workspace && python3 py/analysis/walkforward_density.py zpicture/wdefbox_portfolio_trades.json
```
→ `zpicture/walkforward_density_results.json` 생성

**무결성 앵커**: OOS 2022 폴드가 A: lo=46, pass n=937, pass μ=7.311%, diff=+4.851pp, 연수익 21.35% /
B: lo=67(q0.8), 연수익 33.57% / fifo 연수익 2.12% 와 일치해야 한다. 불일치 시 중단·보고.

### 2) 보고서 — `zpicture/wdefbox_walkforward_density_report.md`
JSON 수치 그대로:
- 요약 3줄 (강건성 판정 / A vs B / 권고)
- 폴드별 표: OOS연도, IS구간, lo(A/B), OOS 신호 diff(A/B), OOS 연수익(A/B/fifo)
- **강건성 핵심 지표**:
  - 신호 수준: pass_beats_fail 폴드 비율 (예: 14/15), diff 중앙값·평균 — 이것이 1차 판정 기준
  - lo 안정성: 폴드별 lo 값 나열 + 최소/최대/중앙값. 전체기간 값 47과 비교
  - 포트폴리오: OOS 체인 CAGR (A vs B vs fifo). B가 A보다 크게 나쁘면 튜닝 과최적화 신호,
    B ≥ A면 그리드 튜닝도 안전하다는 뜻
- 실패 폴드 분석: diff가 음수인 폴드가 있으면 해당 연도 특성(신호 수, 시장 국면) 서술
- 해석 주의 (반드시): trade-complete 연도 귀속(12월 진입분), 연 단위 포트폴리오 수익 노이즈,
  밀도 causal, K=50 고정(임계값만 격리), MDD류 원가 기준
- 결론: "density_filter 임계값은 OOS에서 강건한가?" 명시적 답 + 실운용 임계값 규칙 권고

### 3) 마무리
- 생성 파일 chown 1000:1000
- 마지막 응답으로 핵심 수치(pass_beats_fail 비율, diff 중앙값, OOS 체인 CAGR A/B/fifo, lo 범위) 보고

## 제약
- **코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고.
- 수치 지어내지 말 것. JSON에 없는 값은 "측정 불가".
- 이 지시서는 삭제하지 말 것.
