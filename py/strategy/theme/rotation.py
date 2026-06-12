"""
전략④: 섹터 내 순환 매매 (Intra-Sector Rotation)
- 핫 섹터(반도체, 로봇 등) 내에서 자금 순환을 포착
- 섹터 평균 대비 저성과 + 수급 유입 시작 = 다음 순환 타겟

Usage:
  python py/strategy/theme/rotation.py analyze
  python py/strategy/theme/rotation.py backtest
"""
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", ".."))

import pandas as pd
import numpy as np
from common.db import get_connection, open_connection
from backtest.engine import load_foreign, load_prices, load_kospi, run_backtest

STRATEGY_NAME = "섹터 내 순환 매매"
LOOKBACK = 5
TOP_N = 10

# 핫 섹터 테마 목록 (사용자가 수정 가능)
HOT_THEMES = [
    '반도체/반도체장비', '반도체재료', '시스템반도체', 'HBM', '3D낸드', 'CXL',
    '로봇', '인공지능', '온디바이스AI', '드론', '자율주행차', '전력기기',
]


def load_hot_stocks(conn):
    """핫 테마에 속한 종목 목록 로드"""
    placeholders = ",".join([f"N'{t.replace(chr(39), chr(39)+chr(39))}'" for t in HOT_THEMES])
    cur = conn.cursor()
    cur.execute(f"""
        SELECT stock_code, stock_name, theme_code, theme_name
        FROM MS.ThemeGroup
        WHERE theme_name IN ({placeholders})
    """)
    cols = [d[0] for d in cur.description]
    rows = cur.fetchall()
    df = pd.DataFrame(rows, columns=cols)
    return df


def score_fn(groups, foreign_window, prices_pivot, window_dates):
    first_d, last_d = window_dates[0], window_dates[-1]

    if first_d in prices_pivot.columns and last_d in prices_pivot.columns:
        stock_ret = (prices_pivot[last_d] - prices_pivot[first_d]) / prices_pivot[first_d].replace(0, np.nan)
    else:
        return pd.DataFrame()

    frgn_daily = foreign_window.pivot_table(index="code", columns="date", values="frgn_net", fill_value=0)
    frgn_dates = sorted(frgn_daily.columns)

    frgn_total = foreign_window.groupby("code")["frgn_net"].sum()

    recent_dates = frgn_dates[-2:] if len(frgn_dates) >= 2 else frgn_dates
    frgn_recent = foreign_window[foreign_window["date"].isin(recent_dates)].groupby("code")["frgn_net"].sum()

    prev_dates = frgn_dates[:-2] if len(frgn_dates) > 2 else []
    if prev_dates:
        frgn_prev = foreign_window[foreign_window["date"].isin(prev_dates)].groupby("code")["frgn_net"].sum()
    else:
        frgn_prev = pd.Series(dtype=float)

    frgn_turn = pd.DataFrame({
        "frgn_recent": frgn_recent,
        "frgn_prev": frgn_prev,
    })
    frgn_turn["turning"] = (frgn_turn["frgn_recent"] > 0).astype(int)

    stock_data = groups[["stock_code", "group_code", "group_name"]].copy()
    stock_data = stock_data.join(stock_ret.rename("ret_5d"), on="stock_code")
    stock_data = stock_data.join(frgn_total.rename("frgn_total"), on="stock_code")
    stock_data = stock_data.join(frgn_recent.rename("frgn_recent"), on="stock_code")
    stock_data = stock_data.join(frgn_turn["turning"].rename("frgn_turning"), on="stock_code")

    theme_avg_ret = stock_data.groupby("group_code")["ret_5d"].mean()
    stock_data = stock_data.join(theme_avg_ret.rename("theme_avg_ret"), on="group_code")

    stock_data["relative_lag"] = stock_data["theme_avg_ret"] - stock_data["ret_5d"]
    stock_data = stock_data.dropna(subset=["ret_5d", "frgn_total"])

    if len(stock_data) == 0:
        return pd.DataFrame()

    stock_data["lag_rank"] = stock_data["relative_lag"].rank(pct=True)
    stock_data["frgn_recent_rank"] = stock_data["frgn_recent"].rank(pct=True)
    stock_data["turn_bonus"] = stock_data["frgn_turning"] * 20

    stock_data["rotation_score"] = (
        stock_data["lag_rank"] * 40
        + stock_data["frgn_recent_rank"] * 40
        + stock_data["turn_bonus"]
    )

    top_per_theme = stock_data.sort_values("rotation_score", ascending=False).groupby("group_code").head(3)

    gs = top_per_theme.groupby(["group_code", "group_name"]).agg(
        stock_cnt=("stock_code", "count"),
        avg_rotation_score=("rotation_score", "mean"),
        avg_lag=("relative_lag", "mean"),
        avg_frgn_recent=("frgn_recent", "mean"),
        turn_ratio=("frgn_turning", "mean"),
        avg_ret=("ret_5d", "mean"),
    ).reset_index()

    gs = gs[gs["stock_cnt"] >= 2].copy()
    if len(gs) == 0:
        return gs

    gs["score"] = gs["avg_rotation_score"]
    return gs.sort_values("score", ascending=False)


def analyze():
    with open_connection() as conn:
        hot_stocks = load_hot_stocks(conn)
        hot_stocks.rename(columns={"theme_code": "group_code", "theme_name": "group_name"}, inplace=True)
        foreign = load_foreign(conn)
        prices = load_prices(conn)

    prices_pivot = prices.pivot_table(index="code", columns="date", values="close_price")

    dates = sorted(foreign["date"].unique())
    window_dates = dates[-LOOKBACK:]
    foreign_window = foreign[foreign["date"].isin(window_dates)]

    scores = score_fn(hot_stocks, foreign_window, prices_pivot, window_dates)

    print(f"\n{'='*80}")
    print(f"{STRATEGY_NAME} — 핫 섹터 내 순환 타겟")
    print(f"분석 기간: {window_dates[0]} ~ {window_dates[-1]}")
    print(f"대상 테마: {', '.join(HOT_THEMES[:6])}...")
    print(f"{'='*80}")

    if scores.empty:
        print("결과 없음")
        return scores

    for _, row in scores.iterrows():
        print(
            f"\n  {row['group_name']:20s} | "
            f"대표종목 {row['stock_cnt']:.0f}개 | "
            f"래거드 {row['avg_lag']*100:+.2f}% | "
            f"최근수급 {row['avg_frgn_recent']:+.0f} | "
            f"전환비 {row['turn_ratio']:.0%} | "
            f"점수 {row['score']:.1f}"
        )

    print(f"\n{'='*80}")
    print("테마별 순환 타겟 종목 TOP 3")
    print(f"{'='*80}")

    hot_stocks2 = hot_stocks.copy()
    dates2 = sorted(foreign["date"].unique())
    window_dates2 = dates2[-LOOKBACK:]
    foreign_window2 = foreign[foreign["date"].isin(window_dates2)]

    first_d, last_d = window_dates2[0], window_dates2[-1]
    stock_ret = (prices_pivot[last_d] - prices_pivot[first_d]) / prices_pivot[first_d].replace(0, np.nan)
    frgn_recent_dates = sorted(foreign_window2["date"].unique())[-2:]
    frgn_recent = foreign_window2[foreign_window2["date"].isin(frgn_recent_dates)].groupby("code")["frgn_net"].sum()

    stock_detail = hot_stocks2.copy()
    stock_detail = stock_detail.join(stock_ret.rename("ret_5d"), on="stock_code")
    stock_detail = stock_detail.join(frgn_recent.rename("frgn_recent"), on="stock_code")
    theme_avg = stock_detail.groupby("group_code")["ret_5d"].mean()
    stock_detail = stock_detail.join(theme_avg.rename("theme_avg"), on="group_code")
    stock_detail["lag"] = stock_detail["theme_avg"] - stock_detail["ret_5d"]
    stock_detail = stock_detail.dropna(subset=["ret_5d"])

    for theme in scores.head(5)["group_name"]:
        sub = stock_detail[stock_detail["group_name"] == theme].copy()
        sub["combo"] = sub["lag"].rank(pct=True) * 50 + sub["frgn_recent"].rank(pct=True) * 50
        top3 = sub.sort_values("combo", ascending=False).head(3)
        print(f"\n  [{theme}]")
        for _, s in top3.iterrows():
            print(f"    {s['stock_name']:12s} ({s['stock_code']}) | "
                  f"등락 {s['ret_5d']*100:+.2f}% (테마평균 {s['theme_avg']*100:+.2f}%) | "
                  f"최근수급 {s['frgn_recent']:+.0f}")

    return scores


def backtest():
    with open_connection() as conn:
        hot_stocks = load_hot_stocks(conn)
        hot_stocks.rename(columns={"theme_code": "group_code", "theme_name": "group_name"}, inplace=True)
        foreign = load_foreign(conn)
        prices = load_prices(conn)
        kospi = load_kospi(conn)

    prices_pivot = prices.pivot_table(index="code", columns="date", values="close_price")
    kospi_dict = kospi.set_index("date")["kospi_close"].to_dict()
    trade_dates = sorted(foreign["date"].unique())

    print(f"대상 테마: {len(HOT_THEMES)}개")
    print(f"대상 종목: {hot_stocks['stock_code'].nunique()}개")

    return run_backtest(hot_stocks, score_fn, foreign, prices_pivot, kospi_dict,
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
