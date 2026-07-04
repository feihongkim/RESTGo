#!/usr/bin/env python3
"""
W중력 전략 포트폴리오 자본 제약 시뮬레이션 (v2 — 버그 수정).

v2 수정사항 (2026-07-03):
- BugFix 1: 자본곡선을 equity(현금 + 보유포지션 원가)로 계산. v1은 현금만 기록해 보유분이 소멸.
- BugFix 2: invested = min(cash, equity/N) — 총자산 기준 균등배분. v1은 현금/N이라 만석 시 37% 유휴.
- simulate()와 simulate_detailed() 중복 제거, 단일 함수로 통합.
- 평균/최대 동시 포지션을 시뮬레이션 이벤트에서 실제 len(slots) 일별 추적으로 계산.

입력: strategy_study JSON의 sell_trades 배열
- strategy, shcode, buy_date, sell_date, holding_bars, net_return_pct, weight
- weight: 남은 잔량 대비 매도 비율 (fraction of remaining)

규칙:
- 초기자본 1.0, 슬롯 N개
- 신호 발생일(buy_date)에 빈 슬롯 있으면 equity/N 진입 (현금 부족 시 그만큼만), 없으면 스킵
- 같은 날 다중 신호: 종목코드 오름차순
- 미투자 현금 수익률 0%
- 매도일(sell_date)에 해당 비중만큼 net_return 실현
- 부분매도: sell_quantity = remaining * weight 로 원본 대비 실제 청산 비중 계산
- 슬롯은 최종 매도 시점에 해제
"""

import json
import sys
import os
from collections import defaultdict
from datetime import datetime, timedelta
import math


def load_sell_trades(path):
    with open(path) as f:
        data = json.load(f)
    return data.get("sell_trades", [])


def group_positions(trades):
    """sell_trades 행을 포지션별로 그룹화하고 실제 청산 수량(sell_quantity)을 계산."""
    positions = defaultdict(list)
    for row in trades:
        key = (row["shcode"], row["buy_date"], row["strategy"])
        positions[key].append(row)

    result = []
    for (shcode, buy_date, strategy), rows in positions.items():
        rows.sort(key=lambda r: r["sell_date"])
        remaining = 1.0
        sell_events = []
        final_sell_date = buy_date
        for r in rows:
            sell_qty = remaining * r["weight"]
            sell_events.append({
                "sell_date": r["sell_date"],
                "sell_quantity": sell_qty,
                "net_return_pct": r["net_return_pct"],
                "holding_bars": r["holding_bars"],
            })
            remaining -= sell_qty
            if r["sell_date"] > final_sell_date:
                final_sell_date = r["sell_date"]

        result.append({
            "shcode": shcode,
            "strategy": strategy,
            "buy_date": buy_date,
            "final_sell_date": final_sell_date,
            "sell_events": sell_events,
        })
    return result


def parse_date(d):
    """Parse YYYYMMDD string to date."""
    return datetime.strptime(d, "%Y%m%d").date()


def simulate(positions, N, initial_capital=1.0):
    """
    N-slot 포트폴리오 시뮬레이션 (통합 버전).

    positions: 그룹화된 포지션 리스트
    N: 슬롯 수 (None = 무제한)
    initial_capital: 초기 자본

    Returns: metrics dict + yearly dict 포함
    """
    if N is None:
        N = len(positions) + 1  # effectively unlimited

    # ── 이벤트 타임라인 구성 ──
    events = []
    for pos in positions:
        buy_date = parse_date(pos["buy_date"])
        events.append((buy_date, "buy", pos))
        for se in pos["sell_events"]:
            sell_date = parse_date(se["sell_date"])
            events.append((sell_date, "sell", {
                "pos": pos,
                "sell_quantity": se["sell_quantity"],
                "net_return_pct": se["net_return_pct"],
            }))

    # 날짜순, 같은 날: buy 먼저(shcode 오름차순), sell 나중
    def event_sort_key(e):
        d, typ, data = e
        if typ == "buy":
            return (d, 0, data["shcode"])
        else:
            return (d, 1, "")

    events.sort(key=event_sort_key)

    cash = initial_capital
    slots = []  # [(key, invested, remaining_quantity)]
    total_signals = 0
    entered_signals = 0

    # ── 일별 추적: equity = cash + Σ(invested × remaining_quantity) ──
    daily_equity = {}   # date -> equity
    daily_slots = {}    # date -> len(slots)
    current_date = None

    def record(date, c, s):
        equity = c + sum(sl[1] * sl[2] for sl in s)
        daily_equity[date] = equity
        daily_slots[date] = len(s)

    # ── 검증용: 만석 시 투자비중 ──
    full_slot_ratios = []  # (date, invested_total/equity) when len(slots)==N

    for date, typ, data in events:
        # 이전 거래일 종가 기준 기록
        if current_date is not None and date > current_date:
            record(current_date, cash, slots)
        current_date = date

        if typ == "buy":
            total_signals += 1
            key = (data["shcode"], data["buy_date"], data["strategy"])
            # 중복 방지
            if any(s[0] == key for s in slots):
                continue
            if len(slots) < N:
                equity = cash + sum(s[1] * s[2] for s in slots)
                invested = min(cash, equity / N)
                cash -= invested
                slots.append([key, invested, 1.0])
                entered_signals += 1

        elif typ == "sell":
            pos = data["pos"]
            key = (pos["shcode"], pos["buy_date"], pos["strategy"])
            slot_idx = None
            for i, s in enumerate(slots):
                if s[0] == key:
                    slot_idx = i
                    break
            if slot_idx is None:
                continue

            invested = slots[slot_idx][1]
            sq = data["sell_quantity"]
            nr = data["net_return_pct"]
            realized = invested * sq * (1 + nr / 100.0)
            cash += realized
            slots[slot_idx][2] -= sq

            # 잔량 0이면 슬롯 해제
            if slots[slot_idx][2] <= 0.0001:
                # 잔여 원금 회수 (안전장치)
                cash += slots[slot_idx][1] * slots[slot_idx][2]
                slots.pop(slot_idx)

        # 만석 검증: len(slots) == N 일 때 투자비중 기록
        if len(slots) == N:
            eq = cash + sum(s[1] * s[2] for s in slots)
            inv_total = sum(s[1] * s[2] for s in slots)
            if eq > 0:
                full_slot_ratios.append((date, inv_total / eq))

    # 최종일 기록
    if current_date is not None:
        record(current_date, cash, slots)

    if not daily_equity:
        return {
            "N": N_orig,
            "CAGR": 0.0, "MDD": 0.0, "Sharpe": 0.0,
            "final_capital": initial_capital,
            "avg_concurrent": 0.0, "max_concurrent": 0,
            "entered_signals": 0, "total_signals": total_signals,
            "digestion_rate": 0.0,
            "yearly": {},
            "full_slot_ratios": [],
        }

    dates = sorted(daily_equity.keys())
    equities = [daily_equity[d] for d in dates]

    # ── CAGR ──
    start_date = dates[0]
    end_date = dates[-1]
    years = (end_date - start_date).days / 365.25
    if years <= 0:
        years = 0.01
    final_capital = equities[-1]
    cagr = (final_capital / initial_capital) ** (1.0 / years) - 1.0

    # ── MDD (equity 곡선 기준) ──
    peak = equities[0]
    mdd = 0.0
    for v in equities:
        if v > peak:
            peak = v
        dd = (peak - v) / peak
        if dd > mdd:
            mdd = dd

    # ── 월별 수익률 → 연환산 Sharpe ──
    monthly_returns = []
    month_map = defaultdict(list)
    for d in dates:
        mk = (d.year, d.month)
        month_map[mk].append((d, daily_equity[d]))
    for mk in sorted(month_map.keys()):
        entries = sorted(month_map[mk])
        start_v = entries[0][1]
        end_v = entries[-1][1]
        if start_v > 0:
            monthly_returns.append((end_v - start_v) / start_v)

    if len(monthly_returns) > 1:
        mean_ret = sum(monthly_returns) / len(monthly_returns)
        var = sum((r - mean_ret) ** 2 for r in monthly_returns) / (len(monthly_returns) - 1)
        std_ret = math.sqrt(var)
        sharpe = (mean_ret / std_ret) * math.sqrt(12) if std_ret > 0 else 0.0
    else:
        sharpe = 0.0

    # ── 동시 포지션 (실제 len(slots) 일별 추적) ──
    slot_counts = [daily_slots[d] for d in dates]
    avg_concurrent = sum(slot_counts) / len(slot_counts) if slot_counts else 0.0
    max_concurrent = max(slot_counts) if slot_counts else 0

    # ── 연도별 수익률 ──
    yearly = {}
    for yr in sorted(set(d.year for d in dates)):
        yr_dates = [d for d in dates if d.year == yr]
        if len(yr_dates) >= 2:
            start_eq = daily_equity[min(yr_dates)]
            end_eq = daily_equity[max(yr_dates)]
            ret = (end_eq - start_eq) / start_eq if start_eq > 0 else 0.0
            yearly[yr] = {"start": start_eq, "end": end_eq, "return": ret}

    # ── 만석 투자비중 통계 ──
    full_stats = None
    if full_slot_ratios:
        ratios = [r for _, r in full_slot_ratios]
        full_stats = {
            "count": len(ratios),
            "mean_ratio": sum(ratios) / len(ratios),
            "min_ratio": min(ratios),
            "max_ratio": max(ratios),
        }

    return {
        "N": N if N <= len(positions) else "∞",
        "N_actual": N,
        "CAGR": cagr,
        "MDD": mdd,
        "Sharpe": sharpe,
        "final_capital": final_capital,
        "avg_concurrent": avg_concurrent,
        "max_concurrent": max_concurrent,
        "entered_signals": entered_signals,
        "total_signals": total_signals,
        "digestion_rate": entered_signals / total_signals if total_signals > 0 else 0.0,
        "yearly": yearly,
        "full_slot_stats": full_stats,
        "start_date": str(start_date),
        "end_date": str(end_date),
        "years": round(years, 2),
    }


def main():
    if len(sys.argv) < 2:
        json_path = "zpicture/wdefbox_portfolio_trades.json"
    else:
        json_path = sys.argv[1]

    if not os.path.exists(json_path):
        print(f"ERROR: 파일 없음: {json_path}")
        sys.exit(1)

    trades = load_sell_trades(json_path)
    print(f"sell_trades 행 수: {len(trades)}")

    positions = group_positions(trades)
    print(f"고유 포지션 수: {len(positions)}")

    # sell_quantity 합 검증
    total_sq = 0.0
    for p in positions:
        pos_sq = sum(se["sell_quantity"] for se in p["sell_events"])
        total_sq += pos_sq
    print(f"평균 sell_quantity 합계: {total_sq/len(positions):.6f} (1.0에 가까워야 함)")

    # ── N 스윕 ──
    N_values = [10, 20, 35, 50, None]
    results = []
    for N in N_values:
        label = f"N={N}" if N is not None else "N=∞"
        print(f"\n[{label}] 시뮬레이션 중...")
        r = simulate(positions, N)
        results.append(r)
        print(f"  CAGR: {r['CAGR']*100:.2f}%  MDD: {r['MDD']*100:.2f}%  Sharpe: {r['Sharpe']:.2f}")
        print(f"  최종자본: {r['final_capital']:.4f}  평균동시: {r['avg_concurrent']:.1f}  최대동시: {r['max_concurrent']}")
        print(f"  신호 소화율: {r['entered_signals']}/{r['total_signals']} = {r['digestion_rate']*100:.1f}%")
        if r.get("full_slot_stats"):
            fs = r["full_slot_stats"]
            print(f"  만석 투자비중: 평균 {fs['mean_ratio']*100:.1f}% (범위 {fs['min_ratio']*100:.1f}~{fs['max_ratio']*100:.1f}%, n={fs['count']})")

    # ── 연도별 수익률 (N=35) ──
    r35 = results[2]  # N=35 is index 2
    print("\n── N=35 연도별 수익률 ──")
    for yr in sorted(r35["yearly"].keys()):
        yr_data = r35["yearly"][yr]
        print(f"  {yr}: {yr_data['return']*100:+.2f}%  (연초 {yr_data['start']:.4f} → 연말 {yr_data['end']:.4f})")

    # ── 결과 JSON 저장 ──
    output = {
        "params": {
            "initial_capital": 1.0,
            "input": json_path,
            "version": "v2 (bugfix: equity-based curve, equity/N sizing, actual slot tracking)",
            "note": "MDD는 실현+원가평가 기준 — 보유 중 시세 평가손 미반영 → MDD 과소추정 가능",
        },
        "results": [],
    }
    for r in results:
        out_r = {
            "N": r["N"],
            "CAGR_pct": round(r["CAGR"] * 100, 2),
            "MDD_pct": round(r["MDD"] * 100, 2),
            "Sharpe": round(r["Sharpe"], 2),
            "final_capital": round(r["final_capital"], 4),
            "avg_concurrent": round(r["avg_concurrent"], 1),
            "max_concurrent": r["max_concurrent"],
            "entered_signals": r["entered_signals"],
            "total_signals": r["total_signals"],
            "digestion_rate_pct": round(r["digestion_rate"] * 100, 1),
            "start_date": r["start_date"],
            "end_date": r["end_date"],
            "years": r["years"],
            "yearly": {str(yr): {
                "return_pct": round(v["return"] * 100, 2),
                "start": round(v["start"], 4),
                "end": round(v["end"], 4),
            } for yr, v in r["yearly"].items()},
        }
        if r.get("full_slot_stats"):
            out_r["full_slot_stats"] = {
                "count": r["full_slot_stats"]["count"],
                "mean_ratio_pct": round(r["full_slot_stats"]["mean_ratio"] * 100, 1),
                "min_ratio_pct": round(r["full_slot_stats"]["min_ratio"] * 100, 1),
                "max_ratio_pct": round(r["full_slot_stats"]["max_ratio"] * 100, 1),
            }
        output["results"].append(out_r)

    out_path = "zpicture/portfolio_sim_result.json"
    with open(out_path, "w") as f:
        json.dump(output, f, indent=2, ensure_ascii=False, default=str)
    print(f"\n결과 저장: {out_path}")


if __name__ == "__main__":
    main()
