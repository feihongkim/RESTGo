"""
MA5 변곡점 분석 + 차트
- 5일 이동평균의 의미 있는 반전(고점/저점)을 감지
- 자잘한 노이즈는 진폭 필터(min_pct)로 제거

Usage:
  python py/analysis/ma5_inflection.py <code>
  python py/analysis/ma5_inflection.py <code> --chart
  python py/analysis/ma5_inflection.py <code> --min-pct 2.0
  python py/analysis/ma5_inflection.py <code> --days 120
  python py/analysis/ma5_inflection.py <code> --index

Examples:
  python py/analysis/ma5_inflection.py 005930 --chart
  python py/analysis/ma5_inflection.py 0001 --index --chart
"""
import sys
import argparse
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pandas as pd
from common.db import open_connection

# zpicture 기본 경로: 프로젝트 루트 기준
_PROJECT_ROOT = os.path.join(os.path.dirname(__file__), "..", "..")
_DEFAULT_OUT_DIR = os.path.normpath(os.path.join(_PROJECT_ROOT, "zpicture"))


# ── 한글 폰트 설정 ──────────────────────────────────────────
def _setup_font():
    import matplotlib.pyplot as plt
    import matplotlib.font_manager as fm

    nanum_path = None
    for f in fm.fontManager.ttflist:
        if "NanumGothic.ttf" in f.fname:
            nanum_path = f.fname
            break

    candidates = [
        "/home/feihong/.fonts/NanumGothic.ttf",
        nanum_path,
        "/mnt/c/Windows/Fonts/malgun.ttf",
        "/usr/share/fonts/truetype/nanum/NanumGothic.ttf",
        "/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
    ]

    for path in candidates:
        if path and os.path.exists(path):
            fm.fontManager.addfont(path)
            fp = fm.FontProperties(fname=path)
            plt.rcParams["axes.unicode_minus"] = False
            return fp

    return fm.FontProperties(family="DejaVu Sans")


# ── DB 로드 ─────────────────────────────────────────────────
def load_stock_ohlcv(conn, code: str, days: int) -> pd.DataFrame:
    df = pd.read_sql(f"""
        SELECT TOP {days}
               stck_bsop_date           AS dt,
               CAST(stck_oprc AS FLOAT) AS o,
               CAST(stck_hgpr AS FLOAT) AS h,
               CAST(stck_lwpr AS FLOAT) AS l,
               CAST(stck_clpr AS FLOAT) AS c,
               CAST(acml_vol  AS BIGINT) AS v
        FROM DM.BP_PeriodPrice
        WHERE stck_shrn_iscd = '{code}' AND period_type = 'D'
        ORDER BY stck_bsop_date DESC
    """, conn)
    df = df.sort_values("dt")
    df["dt"] = pd.to_datetime(df["dt"])
    df = df.set_index("dt")
    df.index.name = "date"
    df.columns = ["Open", "High", "Low", "Close", "Volume"]
    return df


def load_index_ohlcv(conn, code: str, days: int) -> pd.DataFrame:
    df = pd.read_sql(f"""
        SELECT TOP {days}
               stck_bsop_date                AS dt,
               CAST(bstp_nmix_oprc AS FLOAT) AS o,
               CAST(bstp_nmix_hgpr AS FLOAT) AS h,
               CAST(bstp_nmix_lwpr AS FLOAT) AS l,
               CAST(bstp_nmix_prpr AS FLOAT) AS c,
               CAST(acml_vol       AS BIGINT) AS v
        FROM SC.Sector_DailyIndex
        WHERE bstp_cls_code = '{code}'
        ORDER BY stck_bsop_date DESC
    """, conn)
    df = df.sort_values("dt")
    df["dt"] = pd.to_datetime(df["dt"])
    df = df.set_index("dt")
    df.index.name = "date"
    df.columns = ["Open", "High", "Low", "Close", "Volume"]
    return df


# ── 변곡점 감지 ─────────────────────────────────────────────
def detect_ma5_inflection(prices: pd.Series, min_pct: float = 1.5) -> pd.DataFrame:
    ma5 = prices.rolling(5).mean().dropna()
    slope = ma5.diff()

    raw = []
    for i in range(1, len(ma5)):
        prev_s = slope.iloc[i - 1]
        curr_s = slope.iloc[i]
        if pd.isna(prev_s) or pd.isna(curr_s):
            continue
        if prev_s > 0 and curr_s < 0:
            raw.append((ma5.index[i], "peak", ma5.iloc[i]))
        elif prev_s < 0 and curr_s > 0:
            raw.append((ma5.index[i], "valley", ma5.iloc[i]))

    if not raw:
        return pd.DataFrame()

    confirmed = [raw[0]]
    for date, kind, val in raw[1:]:
        last_date, last_kind, last_val = confirmed[-1]
        if kind == last_kind:
            if (kind == "peak" and val > last_val) or \
               (kind == "valley" and val < last_val):
                confirmed[-1] = (date, kind, val)
        else:
            change_pct = abs(val - last_val) / last_val * 100
            if change_pct >= min_pct:
                confirmed.append((date, kind, val))

    result = pd.DataFrame(confirmed, columns=["date", "type", "ma5"])
    result = result.set_index("date")
    result["price"] = prices.reindex(result.index)
    result["ma5"] = result["ma5"].round(1)
    return result


# ── 텍스트 출력 ─────────────────────────────────────────────
def print_result(df: pd.DataFrame, code: str, prices: pd.Series, min_pct: float):
    if df.empty:
        print(f"[{code}] 변곡점 없음 (데이터 부족 또는 min_pct={min_pct}% 조건 미충족)")
        return

    current_price = prices.iloc[-1]
    current_ma5 = prices.rolling(5).mean().iloc[-1]
    above = current_price >= current_ma5

    print(f"\n{'='*60}")
    print(f" {code}  |  현재가: {current_price:,.0f}  |  MA5: {current_ma5:,.1f}  |  {'MA5 위' if above else 'MA5 아래'}")
    print(f" 필터: min_pct={min_pct}%  |  변곡점 {len(df)}개")
    print(f"{'='*60}")
    print(f"  {'날짜':10s}  {'구분':6s}  {'MA5':>10s}  {'종가':>10s}  {'변동폭':>8s}")
    print(f"  {'-'*52}")

    prev_ma5 = None
    for date, row in df.iterrows():
        marker = "▲ 고점" if row["type"] == "peak" else "▼ 저점"
        chg = ""
        if prev_ma5 is not None:
            pct = (row["ma5"] - prev_ma5) / prev_ma5 * 100
            chg = f"{pct:+.1f}%"
        print(f"  {str(date)[:10]}  {marker}  {row['ma5']:>10,.1f}  {row['price']:>10,.0f}  {chg:>8s}")
        prev_ma5 = row["ma5"]

    last = df.iloc[-1]
    days_since = (prices.index[-1] - df.index[-1]).days
    trend = "하락 중" if last["type"] == "peak" else "상승 중"
    print(f"\n  최근 변곡: {str(df.index[-1])[:10]} ({last['type']}) → 현재 {trend} ({days_since}일 경과)")
    print()


# ── 차트 그리기 ─────────────────────────────────────────────
def draw_chart(ohlcv: pd.DataFrame, inflection: pd.DataFrame, code: str,
               min_pct: float, out_dir: str = None) -> str:
    import mplfinance as mpf
    import matplotlib.pyplot as plt
    import matplotlib.font_manager as fm
    import numpy as np

    if out_dir is None:
        out_dir = _DEFAULT_OUT_DIR

    fontprop = _setup_font()
    plt.rcParams["grid.linewidth"] = 0.5
    plt.rcParams["grid.linestyle"] = ":"

    ohlcv = ohlcv.copy()
    ohlcv["MA5"] = ohlcv["Close"].rolling(5).mean()
    ohlcv["MA20"] = ohlcv["Close"].rolling(20).mean()

    peak_vals   = pd.Series(np.nan, index=ohlcv.index)
    valley_vals = pd.Series(np.nan, index=ohlcv.index)

    if not inflection.empty:
        for date, row in inflection.iterrows():
            if date in ohlcv.index:
                if row["type"] == "peak":
                    peak_vals[date] = ohlcv.loc[date, "High"] * 1.005
                else:
                    valley_vals[date] = ohlcv.loc[date, "Low"] * 0.995

    add_plots = [
        mpf.make_addplot(ohlcv["MA5"],  color="blue",   linestyle="-",  width=1.2, label="MA5"),
        mpf.make_addplot(ohlcv["MA20"], color="orange",  linestyle="-",  width=0.8, label="MA20"),
        mpf.make_addplot(peak_vals,   type="scatter", markersize=80,
                         marker="v", color="red",      label="Peak"),
        mpf.make_addplot(valley_vals, type="scatter", markersize=80,
                         marker="^", color="royalblue", label="Valley"),
    ]

    market_colors = mpf.make_marketcolors(
        up="r", down="b",
        volume={"up": "r", "down": "b"},
        edge={"up": "r", "down": "b"},
        wick={"up": "r", "down": "b"},
    )
    custom_style = mpf.make_mpf_style(
        marketcolors=market_colors, gridstyle=":", gridcolor="gray"
    )

    fig, axes = mpf.plot(
        ohlcv,
        type="candle",
        style=custom_style,
        addplot=add_plots,
        volume=True,
        ylabel="Price",
        ylabel_lower="Volume",
        figscale=1.3,
        returnfig=True,
    )

    current_price = ohlcv["Close"].iloc[-1]
    current_ma5   = ohlcv["MA5"].iloc[-1]
    title = (f"{code}  |  현재가: {current_price:,.0f}  |  MA5: {current_ma5:,.1f}"
             f"  |  변곡 필터: {min_pct}%  |  변곡 {len(inflection)}개")
    fig.suptitle(title, fontproperties=fontprop, fontsize=13)

    ax1 = axes[0]
    if not inflection.empty:
        ylim = ax1.get_ylim()
        for date, row in inflection.iterrows():
            if date not in ohlcv.index:
                continue
            x_pos = ohlcv.index.get_loc(date)
            color = "red" if row["type"] == "peak" else "royalblue"
            y_pos = ylim[1] * 0.98 if row["type"] == "peak" else ylim[0] * 1.02 + (ylim[1] - ylim[0]) * 0.03
            ax1.text(x_pos, y_pos, str(date)[:10],
                     color=color, fontsize=7, ha="center", va="top",
                     fontproperties=fontprop, alpha=0.85)

    os.makedirs(out_dir, exist_ok=True)
    out_path = os.path.join(out_dir, f"ma5_{code}.png")
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    plt.close(fig)
    print(f"[저장] {out_path}")
    return out_path


# ── 메인 ────────────────────────────────────────────────────
def main():
    parser = argparse.ArgumentParser(description="MA5 변곡점 분석")
    parser.add_argument("code", help="종목코드 (예: 005930) 또는 지수코드 (예: 0001)")
    parser.add_argument("--min-pct", type=float, default=1.5,
                        help="변곡 인정 최소 진폭 %% (기본: 1.5)")
    parser.add_argument("--days", type=int, default=60,
                        help="분석 기간 거래일 수 (기본: 60)")
    parser.add_argument("--index", action="store_true",
                        help="지수 코드로 조회 (SC.Sector_DailyIndex)")
    parser.add_argument("--chart", action="store_true",
                        help="차트 PNG 저장")
    args = parser.parse_args()

    with open_connection() as conn:
        if args.index:
            ohlcv = load_index_ohlcv(conn, args.code, args.days)
        else:
            ohlcv = load_stock_ohlcv(conn, args.code, args.days)

    if ohlcv.empty:
        print(f"[오류] 코드 '{args.code}' 데이터 없음")
        sys.exit(1)

    prices = ohlcv["Close"]
    inflection = detect_ma5_inflection(prices, min_pct=args.min_pct)
    print_result(inflection, args.code, prices, args.min_pct)

    if args.chart:
        draw_chart(ohlcv, inflection, args.code, args.min_pct)


if __name__ == "__main__":
    main()
