# 작업: S03+S23 매도 전략 A/B 4종 (코드·rules 수정 금지)

## 배경
strategy1 축소판(rules/strategy1_s03s23.yaml, S03+S23 2룰)의 전용 매도 설계.
진단(sell_default 기준): Technical 버킷 -2.7%/승률 5~11% 최악, 0~2봉 조기청산 평균 음수, 21봉 초과 +7.75%/승률 96%.
가설: Technical 제거·손절 유예가 개선. 단 W와 달리 S03 손절은 제 몫일 수 있음 — 데이터로 판단.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 각 arm 시작/완료 보고 (핵심 수치 포함).

## 단계 — 4 arm 순차 (각 ~30-40분, hannam 전 종목)
```
go build -o RESTGo_pisim .
RESTGO_SELL_RULES=rules/sell_default.yaml                    ./RESTGo_pisim stock strategy_study --with-sell rules/strategy1_s03s23.yaml zpicture/s0323_sell_default.json
RESTGO_SELL_RULES=rules/ablation/sell_notechnical.yaml       ./RESTGo_pisim stock strategy_study --with-sell rules/strategy1_s03s23.yaml zpicture/s0323_sell_notech.json
RESTGO_SELL_RULES=rules/ablation/sell_minhold5.yaml          ./RESTGo_pisim stock strategy_study --with-sell rules/strategy1_s03s23.yaml zpicture/s0323_sell_minhold5.json
RESTGO_SELL_RULES=rules/ablation/sell_mh20_notechnical.yaml  ./RESTGo_pisim stock strategy_study --with-sell rules/strategy1_s03s23.yaml zpicture/s0323_sell_wstyle.json
```

## 무결성 앵커
arm①(sell_default)의 S03+S23 합산 가중평균은 기존 측정 +1.63%/체결·PF 1.65와 유사해야 한다
(주의: 축소판은 약한 룰 제거로 신호 귀속이 바뀌지 않지만, 자본 경쟁이 없는 event study라 정확 일치 기대.
S03 개별 +2.15%/PF 1.94, S23 +1.12%/1.41). 크게 어긋나면 중단·보고.

## 분석 (Python, /tmp에만)
- arm별: 체결 수(가중), 가중평균 수익/체결, PF, 승률, 평균 보유봉, 매도 사유별 분해
- **연도별 분해 필수** (특정 1~2연도 지배 시 표시)
- 판정 기준: 가중평균·PF 동시 개선 + 연도 안정. **모든 비교에 t값**(arm간 평균 차이의 t)

## 보고서 — `zpicture/s0323_sell_ab_report.md`
- 요약 3줄 (승자 arm / 개선폭 / 신뢰 수준) + 표 전부
- 수치 지어내지 말 것

## 마무리
- chown 1000:1000, RESTGo_pisim 삭제
- 마지막 응답: 4 arm 핵심 수치 + 승자 + 근거

## 제약
- **리포지토리 코드·rules 수정 금지.** 에러 시 고치지 말고 에러 전문 보고. 지시서 삭제 금지.
