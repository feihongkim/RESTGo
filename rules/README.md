# rules/ — 전략 YAML 인덱스

파일명 규칙: `buy_*` 매수 전략 / `sell_*` 매도 전략 / `grid_*` 그리드 서치 정의 / `ablation/` 소거 실험 / `archive/` 과거 스냅샷(수정 금지).

## 매수 전략 (RESTGO_BUY_RULES로 선택, 기본 strategy1.yaml)

| 파일 | 평가 방식 | 내용 |
|------|-----------|------|
| `strategy1.yaml` | on_breakout (C# 정합) | **기본 전략.** C# Stock1 REST1 포팅 — Box 구조 8룰 + Core-3 게이트. C# 매수 정합의 기준이므로 트리거 형식으로 바꾸지 않는다 |
| `buy_indicator.yaml` | trigger | (구 strategy2) DefBox 돌파 순간 지표(RSI/BB/MA) 확증 6룰 (I01~I06) |
| `buy_bb_pure.yaml` | trigger | (구 strategy_bb_pure) John Bollinger 원서 3대 방법 — MIIIb/MIII W바텀 → MII 밴드워크 → MI 역사적 스퀴즈 |
| `buy_bb_hybrid.yaml` | trigger | (구 strategy_bb_hybrid) Box 구조 + BB 복합 4룰 (SH1~SH4) |
| `buy_trigger_example.yaml` | trigger | 트리거 문법 예시 3룰 (BB하단복귀 / 스퀴즈돌파 / DefBox재돌파) — 문법 설명 주석 포함 |
| `buy_crypto_15m.yaml` | per_candle (보류 영역) | (구 strategy3) 가상자산 15분봉 다중 트리거 OR — 추후 보완 예정 |

## 매도 전략 (RESTGO_SELL_RULES로 선택, 기본 sell_default.yaml)

| 파일 | 내용 |
|------|------|
| `sell_default.yaml` | (구 sell_strategy1) **기본 매도.** 21룰 + 5-Path 결정 + 부분매도. ※ 매도 로직은 재설계 예정 |
| `sell_positive_only.yaml` | 양수 수익 구간만 매도하는 변형 |
| `sell_positive_only_mh25.yaml` | 위 + max_holding 25로 조정 |

## 그리드 서치 (stock gridtest, 가상자산 — 보류 영역)

| 파일 | 내용 |
|------|------|
| `grid_crypto_example.yaml` | 그리드 정의 예시 (ADX/VolumeZScore/Cooldown 스윕) |
| `grid_crypto_stage2.yaml` | Stage 2 플래토 그리드 (135조합×4마켓) |
| `grid_crypto_w10b.yaml` | W10-B 청산 파라미터 그리드 (192시나리오) |

## 평가 방식 요약

- **on_breakout** (trigger 미지정): C# DamChecker 상태머신 — DefBox당 게이트 통과 1회만 룰 평가. strategy1 전용.
- **trigger**: 룰별 메인이벤트(edge)가 발화한 캔들에서만 when/when_not/any_of 평가. `once_per: defbox|cooldown|none`으로 중복 제어. 같은 트리거 그룹 내 첫 매칭 승리. 등록 트리거: `DefBoxBreakout`, `PriceBreakout`, `BBLowerBreakdown`, `BBLowerReentry`, `BBSqueezeBreakout` (`stg/trigger_registry.go`).
- **per_candle**: 매 캔들 평가 + 쿨다운 (가상자산 15분봉 전용, 보류 영역).
