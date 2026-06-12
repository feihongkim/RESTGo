"""
공통 백테스트 엔진
- 데이터 로드, 수익률 계산, 벤치마크 비교, 결과 출력
"""
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pandas as pd
import numpy as np
from common.db import get_connection


def load_themes(conn) -> pd.DataFrame:
    return pd.read_sql("SELECT stock_code, theme_code, theme_name FROM MS.ThemeGroup", conn)


def load_sectors(conn) -> pd.DataFrame:
    return pd.read_sql("""
        SELECT stck_shrn_iscd AS stock_code,
               idx_bztp_mcls_cd AS sector_code,
               idx_bztp_mcls_cd_name AS sector_name,
               CAST(lstg_stqt AS BIGINT) AS lstg_stqt,
               CAST(thdt_clpr AS FLOAT) AS last_close
        FROM DM.SI_StockBasic
        WHERE idx_bztp_mcls_cd_name IS NOT NULL AND idx_bztp_mcls_cd_name != ''
    """, conn)


def load_foreign(conn) -> pd.DataFrame:
    return pd.read_sql("""
        SELECT stck_shrn_iscd AS code, stck_bsop_date AS date,
               SUM(frgn_ntby_qty_icdc) AS frgn_net
        FROM DM.PA_ForeignNetBuyTrend
        GROUP BY stck_shrn_iscd, stck_bsop_date
    """, conn)


def load_prices(conn) -> pd.DataFrame:
    return pd.read_sql("""
        SELECT stck_shrn_iscd AS code, stck_bsop_date AS date,
               CAST(stck_clpr AS FLOAT) AS close_price
        FROM DM.BP_PeriodPrice WHERE period_type = 'D'
    """, conn)


def load_kospi(conn) -> pd.DataFrame:
    return pd.read_sql("""
        SELECT stck_bsop_date AS date,
               CAST(bstp_nmix_prpr AS FLOAT) AS kospi_close
        FROM SC.Sector_DailyIndex WHERE bstp_cls_code = '0001'
    """, conn)


def calc_returns(prices_pivot, date, holding_days):
    dates = sorted(prices_pivot.columns.tolist())
    if date not in dates:
        return None
    idx = dates.index(date)
    if idx + holding_days >= len(dates):
        return None
    entry = prices_pivot[dates[idx]].replace(0, np.nan)
    exit_ = prices_pivot[dates[idx + holding_days]]
    return (exit_ - entry) / entry


def run_backtest(groups, score_fn, foreign, prices_pivot, kospi_dict,
                 trade_dates, lookback=5, top_n=10, holdings=(1, 3, 5),
                 strategy_name="전략"):
    signal_dates = trade_dates[lookback:]
    print(f"\n백테스트: {signal_dates[0]} ~ {signal_dates[-1]} ({len(signal_dates)}일)")
    print("시뮬레이션 중...")

    results = []
    for i, sig_date in enumerate(signal_dates):
        sig_idx = trade_dates.index(sig_date)
        window_dates = trade_dates[sig_idx - lookback + 1: sig_idx + 1]
        foreign_window = foreign[foreign["date"].isin(window_dates)]

        scores = score_fn(groups, foreign_window, prices_pivot, window_dates)
        if len(scores) == 0:
            continue

        top = scores.head(top_n)
        top_codes = set(top["group_code"])
        selected = groups[groups["group_code"].isin(top_codes)]["stock_code"].unique()
        selected = [s for s in selected if s in prices_pivot.index]

        if not selected or sig_idx + 1 >= len(trade_dates):
            continue
        next_date = trade_dates[sig_idx + 1]

        for hold in holdings:
            rets = calc_returns(prices_pivot, next_date, hold)
            if rets is None:
                continue
            strat_codes = [s for s in selected if s in rets.index and not np.isnan(rets[s])]
            if not strat_codes:
                continue
            strat_ret = rets[strat_codes].mean()

            ds = sorted(prices_pivot.columns.tolist())
            bi = ds.index(next_date)
            if bi + hold >= len(ds):
                continue
            exit_d = ds[bi + hold]
            ke, kx = kospi_dict.get(next_date), kospi_dict.get(exit_d)
            bench_ret = (kx - ke) / ke if ke and kx and ke > 0 else 0

            theme_cnt = int((top.get("group_type", pd.Series()) == "테마").sum())

            results.append({
                "signal_date": sig_date, "buy_date": next_date, "holding": hold,
                "strategy_ret": strat_ret, "benchmark_ret": bench_ret,
                "excess_ret": strat_ret - bench_ret, "n_stocks": len(strat_codes),
                "top_group": f"[{top.iloc[0].get('group_type','')}]{top.iloc[0]['group_name']}",
                "theme_cnt": theme_cnt, "sector_cnt": top_n - theme_cnt,
            })

        if (i + 1) % 10 == 0:
            print(f"  {i+1}/{len(signal_dates)} 완료...")

    df = pd.DataFrame(results)
    if df.empty:
        print("결과 없음!")
        return df

    print_results(df, strategy_name, top_n, lookback, holdings)
    return df


def print_results(df, strategy_name, top_n, lookback, holdings):
    print("\n" + "=" * 80)
    print(f"{strategy_name} 백테스트 결과")
    print(f"기간: {df['signal_date'].min()} ~ {df['signal_date'].max()}")
    print(f"TOP {top_n}, 룩백 {lookback}일, 벤치마크: KOSPI")
    print("=" * 80)

    for hold in holdings:
        sub = df[df["holding"] == hold]
        if sub.empty:
            continue
        n = len(sub)
        avg_ret = sub["strategy_ret"].mean() * 100
        avg_bench = sub["benchmark_ret"].mean() * 100
        avg_excess = sub["excess_ret"].mean() * 100
        win_rate = (sub["excess_ret"] > 0).mean() * 100
        ir = sub["excess_ret"].mean() / sub["excess_ret"].std() if sub["excess_ret"].std() > 0 else 0
        cum_ret = (1 + sub["strategy_ret"]).prod() - 1

        print(f"\n--- {hold}일 보유 ({n}회) ---")
        print(f"  전략 평균:    {avg_ret:+.3f}%")
        print(f"  KOSPI 평균:   {avg_bench:+.3f}%")
        print(f"  초과수익:     {avg_excess:+.3f}%")
        print(f"  승률:         {win_rate:.1f}%")
        print(f"  IR:           {ir:.3f}")
        print(f"  누적 수익:    {cum_ret*100:+.2f}%")
        print(f"  평균 종목수:   {sub['n_stocks'].mean():.0f}개")

    print("\n--- 1일 보유 일별 상세 (최근 10일) ---")
    sub1 = df[df["holding"] == 1].tail(10)
    for _, row in sub1.iterrows():
        marker = "○" if row["excess_ret"] > 0 else "●"
        print(
            f"  {marker} {row['signal_date']} → {row['buy_date']} | "
            f"전략 {row['strategy_ret']*100:+.2f}% | "
            f"KOSPI {row['benchmark_ret']*100:+.2f}% | "
            f"초과 {row['excess_ret']*100:+.2f}% | "
            f"{row['top_group']}"
        )
