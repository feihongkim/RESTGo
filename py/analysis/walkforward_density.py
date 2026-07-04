#!/usr/bin/env python3
"""
density_filter 임계값 walkforward IS/OOS 강건성 검증 (v1).

배경: alloc_policy 실험(zpicture/wdefbox_alloc_policy_report.md)에서 density_filter(lo=q60=47, K=50)가
승자였으나, 임계값이 전체 기간 분위수(in-sample)라 과최적화 여부 검증이 필요하다.

핵심 아이디어: 매수·매도 룰은 고정이므로 Go 백테스트 재실행 없이 sell_trades 위에서
배분 정책 레이어만 walkforward한다. 밀도(직전 28달력일 신호 수)는 트레일링이라 causal.

설계:
- 슬라이딩 폴드: IS = 직전 4개 연도(buy_date 기준), OOS = 다음 1개 연도. OOS 2012~2026 (~15폴드).
- 규칙 A (고정 규칙): lo = IS 신호 밀도의 q60. "분위수 규칙 자체"의 일반화를 검증.
- 규칙 B (IS 최적화): q ∈ {0.4, 0.5, 0.6, 0.7, 0.8} 그리드에서 IS 포트폴리오 Sharpe 최대
  (동률 시 최종자본)를 선택해 OOS 적용. 튜닝 절차의 과최적화 정도를 측정.
- K=50 고정 (임계값 강건성만 격리).
- 폴드별 평가:
  * 신호 수준: OOS에서 밀도≥lo 통과군 vs 미달군의 (건수, 평균 순수익, 승률) — 핵심 검증
  * 포트폴리오 수준: OOS 연도 포지션만으로 density_filter(lo, K=50) vs fifo N=35 시뮬 수익률
- 집계: 통과군>미달군 폴드 비율, 폴드 중앙값, OOS 연수익 체인(복리) A/B/fifo 비교, lo 안정성.

주의(보고서에 반드시 기재):
- OOS 연도 경계: 12월 진입 포지션의 매도가 이듬해로 넘어가며 해당 폴드에 귀속(trade-complete 방식).
- 포트폴리오 수치는 연 단위 짧은 구간이라 노이즈가 큼 — 신호 수준 검증이 1차 근거.
- 밀도는 전체 신호 흐름으로 계산(causal). MDD 등은 원가 기준 하한.

사용:
  python3 walkforward_density.py [trades_json]            # 전체 폴드 실행 + JSON 저장
  python3 walkforward_density.py --smoke [trades_json]    # OOS 2022 한 폴드만 (기계 검증용)
"""

import json
import sys
from collections import defaultdict

from portfolio_sim import load_sell_trades, group_positions, parse_date
from portfolio_sim_policy import (build_events, compute_densities, poskey, pos_return,
                                  quantiles, simulate_policy)

IS_YEARS = 4
K_FIXED = 50
RULE_A_Q = 0.6
GRID_QS = [0.4, 0.5, 0.6, 0.7, 0.8]


def buy_year(pos):
    return int(pos["buy_date"][:4])


def subset(positions, y0, y1):
    return [p for p in positions if y0 <= buy_year(p) <= y1]


def signal_split(positions_oos, density, lo):
    """OOS 신호를 밀도 임계값으로 이분해 품질 비교."""
    pas, fail = [], []
    for p in positions_oos:
        (pas if density[poskey(p)] >= lo else fail).append(pos_return(p))
    stat = lambda xs: {
        "n": len(xs),
        "mean_net_pct": round(sum(xs) / len(xs), 3) if xs else None,
        "win_rate_pct": round(100 * sum(1 for r in xs if r > 0) / len(xs), 1) if xs else None,
    }
    return {"pass": stat(pas), "fail": stat(fail),
            "diff_pp": round(stat(pas)["mean_net_pct"] - stat(fail)["mean_net_pct"], 3)
                       if pas and fail else None}


def sim_return(positions_sub, density, policy, params):
    """부분 기간 시뮬 총수익률(%). 진입 0건이면 0%."""
    if not positions_sub:
        return {"return_pct": 0.0, "entered": 0, "sharpe": 0.0, "final": 1.0}
    ev = build_events(positions_sub)
    r = simulate_policy(positions_sub, ev, density, policy, params)
    return {"return_pct": round((r["final_capital"] - 1.0) * 100, 2),
            "entered": r["entered_signals"], "sharpe": r["Sharpe"],
            "final": r["final_capital"]}


def pick_lo_rule_a(positions_is, density):
    ds = sorted(density[poskey(p)] for p in positions_is)
    return quantiles(ds, [RULE_A_Q])[RULE_A_Q]


def pick_lo_rule_b(positions_is, density):
    """IS 그리드에서 Sharpe 최대(동률 시 최종자본) q 선택."""
    ds = sorted(density[poskey(p)] for p in positions_is)
    qmap = quantiles(ds, GRID_QS)
    best = None
    trials = []
    for q in GRID_QS:
        lo = qmap[q]
        r = sim_return(positions_is, density, "density_filter",
                       {"lo": lo, "hi": 10 ** 9, "K": K_FIXED})
        trials.append({"q": q, "lo": lo, "is_sharpe": r["sharpe"],
                       "is_return_pct": r["return_pct"], "is_entered": r["entered"]})
        key = (r["sharpe"], r["final"])
        if best is None or key > best[0]:
            best = (key, q, lo)
    return best[2], best[1], trials


def main():
    args = sys.argv[1:]
    smoke = args and args[0] == "--smoke"
    if smoke:
        args.pop(0)
    json_path = args[0] if args else "zpicture/wdefbox_portfolio_trades.json"

    positions = group_positions(load_sell_trades(json_path))
    density = compute_densities(positions)  # 전체 신호 흐름 기준 (causal)
    years = sorted(set(buy_year(p) for p in positions))
    print(f"positions={len(positions)}  years={years[0]}..{years[-1]}")

    oos_years = [2022] if smoke else [y for y in years if y - IS_YEARS >= years[0] + (1 if years[0] == 2007 else 0)]
    # 2007은 신호 1건뿐이라 IS 시작 연도로 취급하지 않음
    oos_years = [y for y in oos_years if y >= 2012]

    folds = []
    for oy in oos_years:
        is0, is1 = oy - IS_YEARS, oy - 1
        pos_is = subset(positions, is0, is1)
        pos_oos = subset(positions, oy, oy)
        if len(pos_is) < 100 or not pos_oos:
            continue

        lo_a = pick_lo_rule_a(pos_is, density)
        lo_b, q_b, trials_b = pick_lo_rule_b(pos_is, density)

        fold = {
            "oos_year": oy,
            "is_range": f"{is0}~{is1}",
            "is_signals": len(pos_is),
            "oos_signals": len(pos_oos),
            "rule_a": {"lo": lo_a,
                       "signal": signal_split(pos_oos, density, lo_a),
                       "oos_sim": sim_return(pos_oos, density, "density_filter",
                                             {"lo": lo_a, "hi": 10 ** 9, "K": K_FIXED})},
            "rule_b": {"lo": lo_b, "picked_q": q_b, "is_grid": trials_b,
                       "signal": signal_split(pos_oos, density, lo_b),
                       "oos_sim": sim_return(pos_oos, density, "density_filter",
                                             {"lo": lo_b, "hi": 10 ** 9, "K": K_FIXED})},
            "fifo_baseline": sim_return(pos_oos, density, "fifo", {"N": 35}),
        }
        folds.append(fold)
        sa, sb = fold["rule_a"]["signal"], fold["rule_b"]["signal"]
        print(f"\nOOS {oy} (IS {is0}~{is1}, n_is={len(pos_is)}, n_oos={len(pos_oos)})")
        print(f"  A: lo={lo_a:<4} pass n={sa['pass']['n']:<4} μ={sa['pass']['mean_net_pct']}% "
              f"| fail μ={sa['fail']['mean_net_pct']}% | diff={sa['diff_pp']}pp "
              f"| 연수익 {fold['rule_a']['oos_sim']['return_pct']}%")
        print(f"  B: lo={lo_b:<4} (q{q_b}) diff={sb['diff_pp']}pp "
              f"| 연수익 {fold['rule_b']['oos_sim']['return_pct']}% "
              f"| fifo 연수익 {fold['fifo_baseline']['return_pct']}%")

    # ── 집계 ──
    def agg(rule):
        diffs = [f[rule]["signal"]["diff_pp"] for f in folds if f[rule]["signal"]["diff_pp"] is not None]
        wins = sum(1 for d in diffs if d > 0)
        chain = 1.0
        for f in folds:
            chain *= 1 + f[rule]["oos_sim"]["return_pct"] / 100.0
        n_years = len(folds)
        cagr = (chain ** (1.0 / n_years) - 1) * 100 if n_years else 0.0
        srt = sorted(diffs)
        median = srt[len(srt) // 2] if srt else None
        return {"folds_with_diff": len(diffs), "pass_beats_fail": wins,
                "median_diff_pp": median,
                "mean_diff_pp": round(sum(diffs) / len(diffs), 3) if diffs else None,
                "oos_chain_final": round(chain, 4),
                "oos_chain_cagr_pct": round(cagr, 2),
                "lo_values": [f[rule]["lo"] for f in folds]}

    fifo_chain = 1.0
    for f in folds:
        fifo_chain *= 1 + f["fifo_baseline"]["return_pct"] / 100.0
    summary = {
        "rule_a_fixed_q60": agg("rule_a"),
        "rule_b_is_optimized": agg("rule_b"),
        "fifo_chain_final": round(fifo_chain, 4),
        "fifo_chain_cagr_pct": round((fifo_chain ** (1.0 / len(folds)) - 1) * 100, 2) if folds else 0.0,
        "n_folds": len(folds),
    }

    print("\n══ 집계 ══")
    for k, v in summary.items():
        print(f"  {k}: {v}")

    if not smoke:
        out = {
            "params": {"input": json_path, "is_years": IS_YEARS, "K": K_FIXED,
                       "rule_a_q": RULE_A_Q, "grid_qs": GRID_QS,
                       "note": "밀도=직전 28달력일(causal). trade-complete 귀속(12월 진입분 매도는 이듬해로 넘어감). "
                               "포트폴리오 연수익은 노이즈 큼 — 신호 수준 diff가 1차 근거. K=50 고정."},
            "folds": folds,
            "summary": summary,
        }
        out_path = "zpicture/walkforward_density_results.json"
        with open(out_path, "w") as f:
            json.dump(out, f, indent=2, ensure_ascii=False, default=str)
        print(f"\n결과 저장: {out_path}")


if __name__ == "__main__":
    main()
