#!/usr/bin/env python3
"""descending_trendline_chart.py — 하락추세선 돌파 전략 샘플 차트 렌더러.

사용법:
  python3 descending_trendline_chart.py <sample_json> <output_dir>
  python3 descending_trendline_chart.py zpicture/descending_trendline_chart_samples.json zpicture/
"""

import json
import sys
import os
from datetime import datetime

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
import matplotlib.patches as mpatches
from matplotlib.ticker import FuncFormatter
import matplotlib.font_manager as fm
import numpy as np

# ── 한글 폰트 설정 ──
_kor_font_prop = None
_kor_font_name = None
for f in fm.fontManager.ttflist:
    if "NanumGothic" in f.name or "Nanum" in f.name:
        _kor_font_prop = fm.FontProperties(fname=f.fname)
        _kor_font_name = f.name
        break
if _kor_font_prop is None:
    for _p in [os.path.expanduser("~/.fonts/NanumGothic.ttf"),
               "/usr/share/fonts/truetype/nanum/NanumGothic.ttf"]:
        if os.path.exists(_p):
            _kor_font_prop = fm.FontProperties(fname=_p)
            _kor_font_name = "NanumGothic"
            fm.fontManager.addfont(_p)
            break
if _kor_font_prop is not None:
    plt.rcParams["font.family"] = _kor_font_name
    print(f"[font] Using: {_kor_font_name}")
else:
    print("[font] No Korean font found, falling back to DejaVu Sans")


def dstr_to_dt(s):
    return datetime.strptime(s, "%Y%m%d")


def make_mpl_candles(candles):
    dates = [dstr_to_dt(c["date"]) for c in candles]
    opens = np.array([c["open"] for c in candles])
    highs = np.array([c["high"] for c in candles])
    lows = np.array([c["low"] for c in candles])
    closes = np.array([c["close"] for c in candles])
    volumes = np.array([c["volume"] for c in candles])
    ma20s = np.array([c["ma20"] for c in candles])
    ma60s = np.array([c["ma60"] for c in candles])
    return dates, opens, highs, lows, closes, volumes, ma20s, ma60s


def volume_fmt(x, _):
    if x >= 1e6:
        return f"{x/1e6:.0f}M"
    if x >= 1e3:
        return f"{x/1e3:.0f}K"
    return f"{x:.0f}"


def render_sample(sample, out_path):
    candles = sample["candles"]
    dates, opens, highs, lows, closes, volumes, ma20s, ma60s = make_mpl_candles(candles)

    ups = closes >= opens
    downs = closes < opens

    # ── figure ──
    fig, (ax_price, ax_vol) = plt.subplots(
        2, 1, figsize=(20, 11),
        gridspec_kw={"height_ratios": [3, 1]},
        sharex=True,
    )
    fig.patch.set_facecolor("#1a1a2e")

    for ax in (ax_price, ax_vol):
        ax.set_facecolor("#1a1a2e")
        ax.tick_params(colors="#cccccc")
        ax.spines["top"].set_visible(False)
        ax.spines["right"].set_visible(False)
        ax.spines["left"].set_color("#333333")
        ax.spines["bottom"].set_color("#333333")

    body_width = 0.6
    for i in range(len(dates)):
        color = "#26a69a" if ups[i] else "#ef5350"
        ax_price.plot(
            [mdates.date2num(dates[i]), mdates.date2num(dates[i])],
            [lows[i], highs[i]],
            color="#cccccc", linewidth=0.8, alpha=0.7,
        )
        body_bottom = opens[i] if ups[i] else closes[i]
        body_height = abs(closes[i] - opens[i])
        ax_price.bar(
            mdates.date2num(dates[i]), body_height, body_width,
            bottom=body_bottom, color=color, edgecolor=color, linewidth=0.5,
        )

    # MA20 / MA60
    valid_ma20 = ma20s > 0
    ax_price.plot(
        [mdates.date2num(d) for d, v in zip(dates, valid_ma20) if v],
        ma20s[valid_ma20],
        color="#ff9800", linewidth=1.2, alpha=0.8, label="MA20",
    )
    valid_ma60 = ma60s > 0
    ax_price.plot(
        [mdates.date2num(d) for d, v in zip(dates, valid_ma60) if v],
        ma60s[valid_ma60],
        color="#9c27b0", linewidth=1.2, alpha=0.8, label="MA60",
    )

    y_range = max(highs) - min(lows)
    def _oy(y, d=1):
        return y + d * y_range * 0.025

    # ── Pivot markers: R1, S1, R2, S2 ──
    pivots = [
        (sample["pivot_r1_idx"], sample["r1_price"], "R1", "#ff5252"),
        (sample["pivot_s1_idx"], sample["s1_price"], "S1", "#4caf50"),
        (sample["pivot_r2_idx"], sample["r2_price"], "R2", "#ff5252"),
        (sample["pivot_s2_idx"], sample["s2_price"], "S2", "#4caf50"),
    ]
    for idx, price, label, color in pivots:
        if 0 <= idx < len(dates):
            ax_price.scatter(
                mdates.date2num(dates[idx]), price,
                marker="o", s=100, color=color, edgecolors="white",
                linewidths=1.5, zorder=6,
            )
            ax_price.annotate(
                f"{label}\n{price:.0f}",
                (mdates.date2num(dates[idx]), _oy(price, 1)),
                color=color, fontsize=7, ha="center", va="bottom", fontweight="bold",
                bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor=color, alpha=0.85),
            )

    # ── Descending trendline: R1 → R2, extended through chart ──
    r1_price = sample["r1_price"]
    slope = sample["slope"]
    r1_pos_in_candles = sample["pivot_r1_idx"]
    # trendline: price = r1_price + slope * (i - r1_idx)
    n = len(dates)
    tl_start_idx = 0
    tl_end_idx = n - 1
    tl_dates = [mdates.date2num(dates[i]) for i in range(tl_start_idx, tl_end_idx + 1)]
    tl_prices = [r1_price + slope * (i - r1_pos_in_candles) for i in range(tl_start_idx, tl_end_idx + 1)]
    ax_price.plot(tl_dates, tl_prices, color="#ffeb3b", linewidth=1.5, linestyle="--", alpha=0.9, label="R1-R2 Trendline")

    # ── Horizontal floor: (S1+S2)/2 ──
    floor = sample["floor_price"]
    ax_price.axhline(y=floor, color="#00bcd4", linewidth=1.2, linestyle=":", alpha=0.8, label="Floor (S1/S2 avg)")

    # ── Breakout confirmation bar ──
    bo_idx = sample["breakout_idx"]
    if 0 <= bo_idx < len(dates):
        ax_price.axvline(
            mdates.date2num(dates[bo_idx]), color="#4caf50",
            linestyle=":", linewidth=1.8, alpha=0.9,
        )
        bo_high = highs[bo_idx]
        ax_price.annotate(
            f"돌파 확인\n{closes[bo_idx]:.0f}",
            (mdates.date2num(dates[bo_idx]), _oy(bo_high, 1)),
            color="#4caf50", fontsize=7, ha="center", va="bottom", fontweight="bold",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#4caf50", alpha=0.9),
        )

    # ── BUY: next bar open ──
    entry_idx = sample["entry_idx"]
    entry_price = sample["entry_price"]
    if 0 <= entry_idx < len(dates):
        ax_price.scatter(
            mdates.date2num(dates[entry_idx]), entry_price,
            marker="^", s=150, color="#00e5ff", edgecolors="#00e5ff",
            linewidths=2, zorder=7,
        )
        ax_price.annotate(
            f"BUY {entry_price:.0f}",
            (mdates.date2num(dates[entry_idx]), _oy(entry_price, -1)),
            color="#00e5ff", fontsize=7, ha="center", va="top", fontweight="bold",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#00e5ff", alpha=0.9),
        )

    # ── SELL: +20 close ──
    exit_idx = sample["exit_idx"]
    exit_price = sample["exit_price"]
    if 0 <= exit_idx < len(dates):
        ax_price.scatter(
            mdates.date2num(dates[exit_idx]), exit_price,
            marker="v", s=150, color="#ff5252", edgecolors="#ff5252",
            linewidths=2, zorder=7,
        )
        ax_price.annotate(
            f"SELL {exit_price:.0f}",
            (mdates.date2num(dates[exit_idx]), _oy(exit_price, 1)),
            color="#ff5252", fontsize=7, ha="center", va="bottom", fontweight="bold",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#ff5252", alpha=0.9),
        )

    ax_price.set_ylabel("Price", color="#cccccc", fontsize=10)
    ax_price.yaxis.set_major_formatter(FuncFormatter(lambda x, _: f"{x:.0f}"))
    ax_price.legend(loc="upper left", fontsize=7, facecolor="#1a1a2e", edgecolor="#333333", labelcolor="#cccccc")
    ax_price.grid(True, alpha=0.15, color="#666666")

    # ── VOLUME PANEL ──
    for i in range(len(dates)):
        vol_color = "#26a69a" if ups[i] else "#ef5350"
        ax_vol.bar(
            mdates.date2num(dates[i]), volumes[i], body_width,
            color=vol_color, alpha=0.6, edgecolor=vol_color, linewidth=0.3,
        )

    # Breakout volume highlight
    if 0 <= bo_idx < len(dates):
        ax_vol.axvline(
            mdates.date2num(dates[bo_idx]), color="#4caf50",
            linestyle=":", linewidth=1.8, alpha=0.9,
        )

    ax_vol.set_ylabel("Volume", color="#cccccc", fontsize=10)
    ax_vol.yaxis.set_major_formatter(FuncFormatter(volume_fmt))
    ax_vol.grid(True, alpha=0.15, color="#666666")

    # ── Date axis ──
    ax_vol.xaxis.set_major_formatter(mdates.DateFormatter("%m/%d"))
    ax_vol.xaxis.set_major_locator(mdates.AutoDateLocator())
    for label in ax_vol.get_xticklabels():
        label.set_color("#cccccc")
    plt.setp(ax_vol.xaxis.get_majorticklabels(), rotation=30, ha="right", fontsize=7)

    # ── Title ──
    pct_label = sample["percentile"]
    net_str = f"{sample['net_return_pct']:+.2f}%"
    net_color = "#4caf50" if sample["net_return_pct"] > 0 else "#ef5350"
    title = (
        f"하락추세선 돌파 — {pct_label} 표본  |  {sample['shcode']}\n"
        f"R1:{sample['r1_date']} S1:{sample['s1_date']} R2:{sample['r2_date']} S2:{sample['s2_date']}  "
        f"|  패턴 {sample['pattern_bars']}봉  |  "
        f"BUY {sample['entry_date']}({sample['entry_price']:.0f}) → SELL {sample['exit_date']}({sample['exit_price']:.0f})  "
        f"|  비용후 h20 {net_str}"
    )
    fig.suptitle(title, color="#ffffff", fontsize=10, y=0.985, fontweight="bold")

    # ── Footer ──
    footer = (
        "S08_D03_B60_W20 STRUCTURE  |  support tol=8%  R1→R2 drop≥3%  "
        "pattern 60~180봉  breakout wait≤20봉  |  buy=확인봉+1시가  sell=+20종가  cost=0.30%"
    )
    fig.text(0.5, 0.005, footer, ha="center", va="bottom", fontsize=7, color="#888888")

    plt.tight_layout(rect=[0, 0.04, 1, 0.93])
    fig.savefig(out_path, dpi=150, facecolor=fig.get_facecolor(), bbox_inches="tight")
    plt.close(fig)
    print(f"  → saved {out_path}")


def main():
    if len(sys.argv) < 3:
        print(f"사용법: {sys.argv[0]} <sample_json> <output_dir>")
        sys.exit(1)

    json_path = sys.argv[1]
    out_dir = sys.argv[2]
    os.makedirs(out_dir, exist_ok=True)

    with open(json_path) as f:
        data = json.load(f)

    samples = data.get("samples", [])
    print(f"총 {len(samples)}개 샘플 차트 생성 중...")

    for sample in samples:
        pct = sample["percentile"]
        out_path = os.path.join(out_dir, f"descending_trendline_{pct}.png")
        render_sample(sample, out_path)


if __name__ == "__main__":
    main()
