"""
전략③: 테마+섹터 통합 수급 서지 (Combined Surge)
- 테마(302개) + 업종(~35개) 통합 그룹
- 시가총액 필터 적용 (대형주 포함)
- 수급 + 모멘텀 동시 상승

Usage:
  python py/strategy/theme/sector_surge.py analyze
  python py/strategy/theme/sector_surge.py backtest
"""
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", ".."))

import pandas as pd
import numpy as np
from common.db import open_connection
from backtest.engine import (load_themes, load_sectors, load_foreign,
                             load_prices, load_kospi, run_backtest)

STRATEGY_NAME = "테마+섹터 통합 서지"
LOOKBACK = 5
TOP_N = 10
MIN_MCAP = 3000  # 시가총액 하한 (억원)


def build_groups(conn):
    themes = load_themes(conn)
    themes["group_type"] = "테마"
    themes.rename(columns={"theme_code": "group_code", "theme_name": "group_name"}, inplace=True)

    sectors = load_sectors(conn)
    sectors["mcap"] = sectors["lstg_stqt"] * sectors["last_close"] / 100000000
    mcap_map = sectors.set_index("stock_code")["mcap"].to_dict()

    sector_groups = sectors[["stock_code", "sector_code", "sector_name"]].copy()
    sector_groups.rename(columns={"sector_code": "group_code", "sector_name": "group_name"}, inplace=True)
    sector_groups["group_code"] = "SEC_" + sector_groups["group_code"].astype(str)
    sector_groups["group_type"] = "섹터"

    all_groups = pd.concat([themes, sector_groups], ignore_index=True)
    valid_stocks = {code for code, mcap in mcap_map.items() if mcap >= MIN_MCAP}
    all_groups = all_groups[all_groups["stock_code"].isin(valid_stocks)].copy()
    return all_groups


def score_fn(groups, foreign_window, prices_pivot, window_dates):
    frgn_sum = foreign_window.groupby("code")["frgn_net"].sum()
    frgn_days = foreign_window.assign(buy=lambda x: x["frgn_net"] > 0).groupby("code")["buy"].sum()

    first_d, last_d = window_dates[0], window_dates[-1]
    if first_d in prices_pivot.columns and last_d in prices_pivot.columns:
        price_ret = (prices_pivot[last_d] - prices_pivot[first_d]) / prices_pivot[first_d].replace(0, np.nan)
    else:
        price_ret = pd.Series(dtype=float)

    stock_data = groups[["stock_code", "group_code", "group_name", "group_type"]].copy()
    stock_data = stock_data.join(frgn_sum.rename("frgn_net_total"), on="stock_code")
    stock_data = stock_data.join(frgn_days.rename("frgn_buy_days"), on="stock_code")
    stock_data = stock_data.join(price_ret.rename("price_return"), on="stock_code")

    gs = stock_data.groupby(["group_code", "group_name", "group_type"]).agg(
        stock_cnt=("stock_code", "count"),
        frgn_net_avg=("frgn_net_total", "mean"),
        frgn_buy_days_avg=("frgn_buy_days", "mean"),
        frgn_buy_ratio=("frgn_net_total", lambda x: (x > 0).mean()),
        price_return_avg=("price_return", "mean"),
        rising_ratio=("price_return", lambda x: (x > 0).mean()),
    ).reset_index()
    gs = gs[gs["stock_cnt"] >= 3].copy()
    if len(gs) == 0:
        return gs

    gs["supply_score"] = (
        gs["frgn_buy_ratio"].rank(pct=True) * 15
        + gs["frgn_buy_days_avg"].rank(pct=True) * 10
        + gs["frgn_net_avg"].rank(pct=True) * 15
    )
    gs["momentum_score"] = (
        gs["price_return_avg"].rank(pct=True) * 40
        + gs["rising_ratio"].rank(pct=True) * 20
    )
    gs["score"] = gs["supply_score"] + gs["momentum_score"]
    return gs.sort_values("score", ascending=False)


def analyze():
    with open_connection() as conn:
        groups = build_groups(conn)
        foreign = load_foreign(conn)
        prices = load_prices(conn)

    prices_pivot = prices.pivot_table(index="code", columns="date", values="close_price")

    dates = sorted(foreign["date"].unique())
    window_dates = dates[-LOOKBACK:]
    foreign_window = foreign[foreign["date"].isin(window_dates)]

    scores = score_fn(groups, foreign_window, prices_pivot, window_dates)
    result = scores.head(20)

    print(f"\n{'='*80}\n{STRATEGY_NAME} TOP 20 (시총 {MIN_MCAP}억+)\n{'='*80}")
    for _, row in result.iterrows():
        tag = f"[{row['group_type']}]"
        print(f"  {tag:6s} {row['group_name']:20s} | 종목 {row['stock_cnt']:3.0f}개 | "
              f"외인매수 {row['frgn_buy_ratio']:.0%} | 등락 {row['price_return_avg']*100:+.2f}% | "
              f"점수 {row['score']:.1f}")
    return result


def backtest():
    with open_connection() as conn:
        groups = build_groups(conn)
        foreign = load_foreign(conn)
        prices = load_prices(conn)
        kospi = load_kospi(conn)

    prices_pivot = prices.pivot_table(index="code", columns="date", values="close_price")
    kospi_dict = kospi.set_index("date")["kospi_close"].to_dict()
    trade_dates = sorted(foreign["date"].unique())

    return run_backtest(groups, score_fn, foreign, prices_pivot, kospi_dict,
                        trade_dates, LOOKBACK, TOP_N, strategy_name=STRATEGY_NAME)


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description=STRATEGY_NAME)
    parser.add_argument("command", choices=["analyze", "backtest"])
    args = parser.parse_args()
    if args.command == "analyze":
        analyze()
    else:
        backtest()
