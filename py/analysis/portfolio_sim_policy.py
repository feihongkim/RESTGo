#!/usr/bin/env python3
"""
W중력 포트폴리오 배분 정책 실험 (v3).

배경: v2(portfolio_sim.py)에서 FIFO 슬롯 배분의 역선택 발견 —
진입 4,100건 평균 +0.75% vs 스킵 3,434건 평균 +4.26%.
W바텀 신호는 하락장에 군집 발생하고, 선착순 슬롯은 군집 초반(떨어지는 칼)
신호로 만석이 되어 바닥 근처의 군집 후반 신호를 놓친다.

이 스크립트는 배분 정책을 파라미터화해 역선택 해소 여부를 실험한다.

정책:
- fifo:           v2와 동일한 기준선. N슬롯, 빈 슬롯 있으면 min(cash, equity/N) 진입.
- cash_frac:      슬롯 캡 없음. 모든 신호에 equity/K 목표 진입, 현금이 자연 한도.
                  cash < 0.5×목표면 스킵(먼지 포지션 방지). 군집 후반까지 진입 여력 보존.
- burst_half:     cash_frac 변형. 직전 28일(달력일) 신호 밀도 D ≥ T면 목표를 equity/(2N)으로
                  절반 축소, 아니면 equity/N. 군집에서 자동으로 잘게 나눠 담는다.
- density_filter: 밀도 밴드 [lo, hi] 안의 신호만 equity/K 진입. 군집 초반/소강기 배제 실험.

진단(--diagnostic 포함 battery): 신호별 직전 28일 밀도 5분위 × (건수/수량가중 평균수익/승률)
— "군집 깊이와 신호 품질" 관계를 정책과 독립적으로 측정.

밀도 정의: D(signal) = buy_date 기준 [d-28일, d) 구간의 전체 신호 수 (당일 제외, 달력일).

자본곡선·지표는 v2와 동일: equity = 현금 + Σ(보유원가×잔량), 보유분 원가 평가(시세 미반영),
CAGR/MDD/월별 Sharpe/연도별 수익률. MDD는 하한 추정치.

사용:
  python3 portfolio_sim_policy.py --battery [trades_json]   # 사전 정의 정책 일괄 실행 + JSON 저장
  python3 portfolio_sim_policy.py --smoke [trades_json]     # fifo N=35만 실행 (v2 회귀 검증용)
"""

import json
import sys
import math
import bisect
from collections import defaultdict
from datetime import timedelta

from portfolio_sim import load_sell_trades, group_positions, parse_date

DENSITY_WINDOW_DAYS = 28


# ──────────────────────────── 준비 ────────────────────────────

def build_events(positions):
    """v2와 동일한 이벤트 타임라인. 같은 날: buy(종목코드 오름차순) → sell."""
    events = []
    for pos in positions:
        events.append((parse_date(pos["buy_date"]), "buy", pos))
        for se in pos["sell_events"]:
            events.append((parse_date(se["sell_date"]), "sell", {
                "pos": pos,
                "sell_quantity": se["sell_quantity"],
                "net_return_pct": se["net_return_pct"],
            }))
    events.sort(key=lambda e: (e[0], 0 if e[1] == "buy" else 1,
                               e[2]["shcode"] if e[1] == "buy" else ""))
    return events


def compute_densities(positions):
    """포지션별 직전 28일(달력일, 당일 제외) 신호 밀도."""
    all_dates = sorted(parse_date(p["buy_date"]) for p in positions)
    density = {}
    for p in positions:
        d = parse_date(p["buy_date"])
        lo = bisect.bisect_left(all_dates, d - timedelta(days=DENSITY_WINDOW_DAYS))
        hi = bisect.bisect_left(all_dates, d)  # 당일 제외
        density[poskey(p)] = hi - lo
    return density


def poskey(pos):
    return (pos["shcode"], pos["buy_date"], pos["strategy"])


def pos_return(pos):
    """포지션의 수량가중 순수익(%). sell_quantity 합은 1.0."""
    return sum(se["sell_quantity"] * se["net_return_pct"] for se in pos["sell_events"])


def quantiles(sorted_vals, qs):
    out = {}
    n = len(sorted_vals)
    for q in qs:
        idx = min(n - 1, max(0, int(round(q * (n - 1)))))
        out[q] = sorted_vals[idx]
    return out


# ──────────────────────────── 정책 ────────────────────────────

def make_target_fn(policy, params, density):
    """buy 이벤트에서 (진입 목표금액 or None=스킵)을 결정하는 함수를 반환.

    반환 함수 시그니처: fn(equity, cash, n_open, key) -> (target, use_floor)
    use_floor=True면 cash < 0.5×target 시 스킵, False면 min(cash, target)로 무조건 진입(fifo 호환).
    """
    if policy == "fifo":
        N = params["N"]

        def fn(equity, cash, n_open, key):
            if n_open >= N:
                return None, False
            return equity / N, False
        return fn

    if policy == "cash_frac":
        K = params["K"]

        def fn(equity, cash, n_open, key):
            return equity / K, True
        return fn

    if policy == "burst_half":
        N, T = params["N"], params["T"]

        def fn(equity, cash, n_open, key):
            k = 2 * N if density[key] >= T else N
            return equity / k, True
        return fn

    if policy == "density_filter":
        lo, hi, K = params["lo"], params["hi"], params["K"]

        def fn(equity, cash, n_open, key):
            d = density[key]
            if d < lo or d > hi:
                return None, False
            return equity / K, True
        return fn

    raise ValueError(f"unknown policy: {policy}")


# ──────────────────────────── 엔진 ────────────────────────────

def simulate_policy(positions, events, density, policy, params, initial_capital=1.0):
    target_fn = make_target_fn(policy, params, density)

    cash = initial_capital
    open_pos = {}  # key -> [invested, remaining_quantity]
    daily_equity = {}
    daily_count = {}
    current_date = None
    total_signals = 0
    entered_keys = set()

    def equity_now():
        return cash + sum(v[0] * v[1] for v in open_pos.values())

    def record(date):
        daily_equity[date] = equity_now()
        daily_count[date] = len(open_pos)

    for date, typ, data in events:
        if current_date is not None and date > current_date:
            record(current_date)
        current_date = date

        if typ == "buy":
            total_signals += 1
            key = poskey(data)
            if key in open_pos:
                continue
            target, use_floor = target_fn(equity_now(), cash, len(open_pos), key)
            if target is None or target <= 0:
                continue
            if use_floor and cash < 0.5 * target:
                continue
            invested = min(cash, target)
            # fifo(use_floor=False)는 v2 재현을 위해 0원 진입도 슬롯 소모 (v2 동작 그대로).
            # floor 정책들은 0.5×목표 미만이면 위에서 이미 스킵됨.
            if use_floor and invested <= 0:
                continue
            cash -= invested
            open_pos[key] = [invested, 1.0]
            entered_keys.add(key)

        else:  # sell
            key = poskey(data["pos"])
            slot = open_pos.get(key)
            if slot is None:
                continue
            sq = data["sell_quantity"]
            cash += slot[0] * sq * (1 + data["net_return_pct"] / 100.0)
            slot[1] -= sq
            if slot[1] <= 0.0001:
                cash += slot[0] * slot[1]
                del open_pos[key]

    if current_date is not None:
        record(current_date)

    return build_metrics(policy, params, positions, entered_keys, total_signals,
                         daily_equity, daily_count, initial_capital)


def build_metrics(policy, params, positions, entered_keys, total_signals,
                  daily_equity, daily_count, initial_capital):
    dates = sorted(daily_equity.keys())
    eq = [daily_equity[d] for d in dates]
    years = max(0.01, (dates[-1] - dates[0]).days / 365.25)
    final = eq[-1]
    cagr = (final / initial_capital) ** (1.0 / years) - 1.0

    peak, mdd = eq[0], 0.0
    for v in eq:
        peak = max(peak, v)
        mdd = max(mdd, (peak - v) / peak)

    month_map = defaultdict(list)
    for d in dates:
        month_map[(d.year, d.month)].append((d, daily_equity[d]))
    mrets = []
    for mk in sorted(month_map):
        ent = sorted(month_map[mk])
        if ent[0][1] > 0:
            mrets.append((ent[-1][1] - ent[0][1]) / ent[0][1])
    if len(mrets) > 1:
        mu = sum(mrets) / len(mrets)
        sd = math.sqrt(sum((r - mu) ** 2 for r in mrets) / (len(mrets) - 1))
        sharpe = (mu / sd) * math.sqrt(12) if sd > 0 else 0.0
    else:
        sharpe = 0.0

    yearly = {}
    for yr in sorted(set(d.year for d in dates)):
        yd = [d for d in dates if d.year == yr]
        if len(yd) >= 2 and daily_equity[min(yd)] > 0:
            s, e = daily_equity[min(yd)], daily_equity[max(yd)]
            yearly[yr] = {"return_pct": round((e - s) / s * 100, 2),
                          "start": round(s, 4), "end": round(e, 4)}

    # 역선택 지표: 진입/스킵 부분집합의 (포지션당 동일가중) 평균 순수익
    ent_rets, skip_rets = [], []
    for p in positions:
        (ent_rets if poskey(p) in entered_keys else skip_rets).append(pos_return(p))
    mean = lambda xs: sum(xs) / len(xs) if xs else 0.0

    counts = [daily_count[d] for d in dates]
    return {
        "policy": policy,
        "params": params,
        "CAGR_pct": round(cagr * 100, 2),
        "MDD_pct": round(mdd * 100, 2),
        "Sharpe": round(sharpe, 2),
        "final_capital": round(final, 4),
        "avg_concurrent": round(sum(counts) / len(counts), 1),
        "max_concurrent": max(counts),
        "entered_signals": len(entered_keys),
        "total_signals": total_signals,
        "digestion_rate_pct": round(100 * len(entered_keys) / total_signals, 1),
        "entered_mean_net_pct": round(mean(ent_rets), 3),
        "skipped_mean_net_pct": round(mean(skip_rets), 3),
        "yearly": {str(y): v for y, v in yearly.items()},
        "start_date": str(dates[0]),
        "end_date": str(dates[-1]),
        "years": round(years, 2),
    }


# ──────────────────────────── 진단 ────────────────────────────

def diagnostic_density(positions, density):
    """밀도 5분위별 신호 품질 — 정책과 독립적인 데이터 사실."""
    rows = [(density[poskey(p)], pos_return(p)) for p in positions]
    ds = sorted(d for d, _ in rows)
    edges = quantiles(ds, [0.2, 0.4, 0.6, 0.8])
    bounds = [edges[0.2], edges[0.4], edges[0.6], edges[0.8]]

    bins = [[] for _ in range(5)]
    for d, r in rows:
        i = sum(1 for b in bounds if d > b)
        bins[i].append((d, r))

    out = []
    labels = ["Q1(저밀도)", "Q2", "Q3", "Q4", "Q5(고밀도=군집 심부)"]
    for i, b in enumerate(bins):
        rets = [r for _, r in b]
        out.append({
            "bin": labels[i],
            "density_range": f"{min(d for d, _ in b)}~{max(d for d, _ in b)}" if b else "-",
            "count": len(b),
            "mean_net_pct": round(sum(rets) / len(rets), 3) if rets else 0.0,
            "win_rate_pct": round(100 * sum(1 for r in rets if r > 0) / len(rets), 1) if rets else 0.0,
        })
    return {"window_days": DENSITY_WINDOW_DAYS, "quantile_edges": bounds, "bins": out}


# ──────────────────────────── 배터리 ────────────────────────────

def battery_configs(density_values_sorted):
    q = quantiles(density_values_sorted, [0.4, 0.5, 0.6, 0.75, 0.8])
    return [
        ("fifo", {"N": 35}),                      # v2 회귀 기준선 (CAGR 3.68% 재현되어야 함)
        ("fifo", {"N": 50}),
        ("cash_frac", {"K": 35}),
        ("cash_frac", {"K": 50}),
        ("cash_frac", {"K": 70}),
        ("cash_frac", {"K": 100}),
        ("burst_half", {"N": 35, "T": q[0.5]}),
        ("burst_half", {"N": 35, "T": q[0.75]}),
        ("density_filter", {"lo": q[0.8], "hi": 10 ** 9, "K": 35}),   # 군집 심부만
        ("density_filter", {"lo": q[0.6], "hi": 10 ** 9, "K": 50}),
        ("density_filter", {"lo": 0, "hi": q[0.4], "K": 35}),         # 소강기만 (대조군)
    ]


def main():
    mode = "--battery"
    args = [a for a in sys.argv[1:]]
    if args and args[0].startswith("--"):
        mode = args.pop(0)
    json_path = args[0] if args else "zpicture/wdefbox_portfolio_trades.json"

    trades = load_sell_trades(json_path)
    positions = group_positions(trades)
    events = build_events(positions)
    density = compute_densities(positions)
    print(f"positions={len(positions)}  events={len(events)}  density_window={DENSITY_WINDOW_DAYS}d")

    diag = diagnostic_density(positions, density)
    print("\n── 진단: 직전 28일 신호 밀도 5분위별 품질 ──")
    for b in diag["bins"]:
        print(f"  {b['bin']:<22} 밀도 {b['density_range']:<10} n={b['count']:<6} "
              f"평균 {b['mean_net_pct']:+.3f}%  승률 {b['win_rate_pct']:.1f}%")

    if mode == "--smoke":
        configs = [("fifo", {"N": 35})]
    else:
        configs = battery_configs(sorted(density.values()))

    results = []
    for policy, params in configs:
        r = simulate_policy(positions, events, density, policy, params)
        results.append(r)
        print(f"\n[{policy} {params}]")
        print(f"  CAGR {r['CAGR_pct']}%  MDD {r['MDD_pct']}%  Sharpe {r['Sharpe']}  "
              f"최종 {r['final_capital']}")
        print(f"  소화율 {r['digestion_rate_pct']}% ({r['entered_signals']}/{r['total_signals']})  "
              f"진입평균 {r['entered_mean_net_pct']}%  스킵평균 {r['skipped_mean_net_pct']}%  "
              f"평균동시 {r['avg_concurrent']}  최대 {r['max_concurrent']}")

    if mode == "--battery":
        out = {
            "params": {
                "input": json_path,
                "version": "v3 (allocation policy battery)",
                "note": "equity=현금+보유원가. MDD 하한 추정치(시세 평가손 미반영). "
                        "밀도=직전 28달력일 신호 수(당일 제외). 임계값은 in-sample 분위수 — 과최적화 주의.",
            },
            "diagnostic_density": diag,
            "results": results,
        }
        out_path = "zpicture/alloc_policy_results.json"
        with open(out_path, "w") as f:
            json.dump(out, f, indent=2, ensure_ascii=False, default=str)
        print(f"\n결과 저장: {out_path}")


if __name__ == "__main__":
    main()
