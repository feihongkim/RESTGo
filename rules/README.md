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
| `buy_wdefbox.yaml` | trigger | **W중력 전략.** WBottomBox 트리거 + HasDefBoxBeforeWPattern (DefBox 중력 가설). hannam 16년 검증 +2.55%/승률 60% (zpicture/wdefbox_sell_formal_ab_report.md) |
| `buy_wdefbox_gc.yaml` | trigger | W패턴 × 골든크로스 임박(MA60→MA120 수렴). 일봉 단독 결합 이득 중립 — **주봉(거시) 결합 전제 등록** (zpicture/wgc_scan_report.md) |
| `strategy1_gc.yaml` | on_breakout | strategy1 전 룰에 IsGoldenCrossPending 게이트 추가한 변형. 일봉 단독 무차별 — 주봉 결합 전제 등록. 원본 갱신 시 재생성 필요 |

## 매도 전략 (RESTGO_SELL_RULES로 선택, 기본 sell_default.yaml)

| 파일 | 내용 |
|------|------|
| `sell_default.yaml` | (구 sell_strategy1) **기본 매도.** 21룰 + 5-Path 결정 + 부분매도. ※ 매도 로직은 재설계 예정 |
| `sell_positive_only.yaml` | 양수 수익 구간만 매도하는 변형 |
| `sell_positive_only_mh25.yaml` | 위 + max_holding 25로 조정 |
| `sell_wdefbox.yaml` | **W중력 전용 매도.** 익절 2종 + 기간만료(20봉, 연장) — 손절 없음. 손절 무용 스윕 + A/B 검증으로 확정 |

## 포트폴리오 오버레이 (stock densitygate, RESTGO_OVERLAY_RULES로 선택)

| 파일 | 내용 |
|------|------|
| `overlay_wdefbox.yaml` | **W중력 밀도 게이트.** 직전 28일 신호 밀도 ≥ 롤링 4년 q60이면 진입 허용(equity/50) — walkforward 15폴드 검증. 매수 룰과 별개인 2단계 포트폴리오 결정 (`stg/overlay_density.go`, hannam StrategySignalDaily) |

## 그리드 서치 (stock gridtest, 가상자산 — 보류 영역)

| 파일 | 내용 |
|------|------|
| `grid_crypto_example.yaml` | 그리드 정의 예시 (ADX/VolumeZScore/Cooldown 스윕) |
| `grid_crypto_stage2.yaml` | Stage 2 플래토 그리드 (135조합×4마켓) |
| `grid_crypto_w10b.yaml` | W10-B 청산 파라미터 그리드 (192시나리오) |

## 평가 방식 요약

- **on_breakout** (trigger 미지정): C# DamChecker 상태머신 — DefBox당 게이트 통과 1회만 룰 평가. strategy1 전용.
- **trigger**: 룰별 메인이벤트(edge)가 발화한 캔들에서만 when/when_not/any_of 평가. `once_per: defbox|cooldown|none`으로 중복 제어. 같은 트리거 그룹 내 첫 매칭 승리. 등록 트리거: `DefBoxBreakout`, `PriceBreakout`, `WBottomBox`, `BBLowerBreakdown`, `BBLowerReentry`, `BBSqueezeBreakout` (`stg/trigger_registry.go`).
- **armed 트리거** (2026-07-05, `stg/armed_trigger*.go`): 장전→발화 2단계 패턴도 같은 `trigger:` 필드로 사용 가능 — 패턴 완성(장전) 후 유효기간 내 확인 이벤트(발화)에서 룰 평가. 등록: `MTopCollapse`(M자 완성→음양음 붕괴), `HNSNecklineBreak`(오른어깨→넥라인 이탈), `MA20PullbackBreakout`(눌림 R→S→양봉 MA20 돌파, `settings.PullbackStreak`). ※ 3종 모두 단독 엣지 기각된 실험 신호 — 상황 조건과의 조합 실험용. 전용 분석기와 발화 동일성 검증 완료(113종목·2,091신호 불일치 0).
- **per_candle**: 매 캔들 평가 + 쿨다운 (가상자산 15분봉 전용, 보류 영역).
