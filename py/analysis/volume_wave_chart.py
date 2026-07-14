#!/usr/bin/env python3
"""volume_wave_chart.py — VW2 첫 눌림 전략 매수·매도 샘플 차트 렌더러.

사용법:
  python3 volume_wave_chart.py <sample_json> <output_png>
  python3 volume_wave_chart.py zpicture/volume_wave_chart_samples.json zpicture/vw_chart.png

JSON 구조: {
  "strategy": "...",
  "config": {...},
  "samples": [
    {
      "label": "P90 winner",
      "shcode": "...", "source_date": "...", "pullback_date": "...",
      "fire_date": "...", "entry_date": "...", "exit_date": "...",
      "entry_price": ..., "exit_price": ..., "net_return_pct": ...,
      "candles": [{"date":..., "open":..., "high":..., "low":..., "close":...,
                   "volume":..., "ma20":..., "vol_ma20":...}]
    }, ...
  ]
}

각 샘플당 하나의 PNG를 생성한다.
출력 경로에 label 기반 접미사가 자동 삽입된다.
"""

import json
import sys
import os
import re
from datetime import datetime

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
import matplotlib.patches as mpatches
from matplotlib.ticker import FuncFormatter
import matplotlib.font_manager as fm
import numpy as np

# Korean font setup — use NanumGothic if available, otherwise fall back
_kor_font_prop = None
_kor_font_name = None
for f in fm.fontManager.ttflist:
    if "NanumGothic" in f.name or "Nanum" in f.name:
        _kor_font_prop = fm.FontProperties(fname=f.fname)
        _kor_font_name = f.name
        break
if _kor_font_prop is None:
    # Try file-based search
    import os as _os
    for _p in [_os.path.expanduser("~/.fonts/NanumGothic.ttf"), "/usr/share/fonts/truetype/nanum/NanumGothic.ttf"]:
        if _os.path.exists(_p):
            _kor_font_prop = fm.FontProperties(fname=_p)
            _kor_font_name = "NanumGothic"
            fm.fontManager.addfont(_p)
            break
if _kor_font_prop is not None:
    plt.rcParams["font.family"] = _kor_font_name
    print(f"[font] Using: {_kor_font_name}")
else:
    print("[font] No Korean font found, falling back to DejaVu Sans")


def load_json(path):
    with open(path) as f:
        return json.load(f)


def dstr_to_dt(s):
    """Convert YYYYMMDD string to datetime."""
    return datetime.strptime(s, "%Y%m%d")


def make_mpl_candles(candles):
    """Return arrays of dates, open, high, low, close, volume, ma20, vol_ma20."""
    dates = [dstr_to_dt(c["date"]) for c in candles]
    opens = np.array([c["open"] for c in candles])
    highs = np.array([c["high"] for c in candles])
    lows = np.array([c["low"] for c in candles])
    closes = np.array([c["close"] for c in candles])
    volumes = np.array([c["volume"] for c in candles])
    ma20s = np.array([c["ma20"] for c in candles])
    vol_ma20s = np.array([c["vol_ma20"] for c in candles])
    return dates, opens, highs, lows, closes, volumes, ma20s, vol_ma20s


def index_of_date(dates, date_str):
    """Return the index of a date string in the dates list."""
    target = dstr_to_dt(date_str)
    for i, d in enumerate(dates):
        if d == target:
            return i
    return -1


def volume_fmt(x, _):
    if x >= 1e6:
        return f"{x/1e6:.0f}M"
    if x >= 1e3:
        return f"{x/1e3:.0f}K"
    return f"{x:.0f}"


def render_sample(sample, strategy, config, out_path):
    """Render a single sample to a PNG file."""
    candles = sample["candles"]
    dates, opens, highs, lows, closes, volumes, ma20s, vol_ma20s = make_mpl_candles(candles)

    # Find marker indices
    idx_source = index_of_date(dates, sample["source_date"])
    idx_pullback = index_of_date(dates, sample["pullback_date"])
    idx_fire = index_of_date(dates, sample["fire_date"])
    idx_entry = index_of_date(dates, sample["entry_date"])
    idx_exit = index_of_date(dates, sample["exit_date"])

    # Determine up/down for candlestick colors
    ups = closes >= opens
    downs = closes < opens

    fig, (ax_price, ax_vol) = plt.subplots(
        2, 1, figsize=(18, 10),
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

    # ── PRICE PANEL with candlesticks ──
    wick_color = "#cccccc"
    body_width = 0.6
    for i in range(len(dates)):
        if i >= len(opens):
            break
        color = "#26a69a" if ups[i] else "#ef5350"
        # wick
        ax_price.plot(
            [mdates.date2num(dates[i]), mdates.date2num(dates[i])],
            [lows[i], highs[i]],
            color=wick_color, linewidth=0.8, alpha=0.7,
        )
        # body
        body_bottom = opens[i] if ups[i] else closes[i]
        body_height = abs(closes[i] - opens[i])
        ax_price.bar(
            mdates.date2num(dates[i]), body_height, body_width,
            bottom=body_bottom, color=color, edgecolor=color, linewidth=0.5,
        )

    # MA20 line
    valid_ma = ma20s > 0
    ax_price.plot(
        [mdates.date2num(d) for d, v in zip(dates, valid_ma) if v],
        ma20s[valid_ma],
        color="#ff9800", linewidth=1.2, alpha=0.8, label="MA20",
    )

    # ── Markers ──
    marker_y_offset_ratio = 0.02
    y_range = max(highs) - min(lows)

    def _offset_y(y, direction=1):
        return y + direction * y_range * marker_y_offset_ratio

    # VW2 source (breakout day)
    if idx_source >= 0:
        ax_price.axvline(
            mdates.date2num(dates[idx_source]), color="#ffeb3b",
            linestyle="--", linewidth=1.2, alpha=0.7,
        )
        ax_price.annotate(
            "VW2 돌파\n(추격 금지)",
            (mdates.date2num(dates[idx_source]), _offset_y(highs[idx_source], 1)),
            color="#ffeb3b", fontsize=7, ha="center", va="bottom",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#ffeb3b", alpha=0.8),
        )

    # Pullback start
    if idx_pullback >= 0:
        ax_price.axvline(
            mdates.date2num(dates[idx_pullback]), color="#2196f3",
            linestyle="-.", linewidth=1.0, alpha=0.6,
        )
        ax_price.annotate(
            "첫 눌림 시작",
            (mdates.date2num(dates[idx_pullback]), _offset_y(highs[idx_pullback], 1)),
            color="#2196f3", fontsize=7, ha="center", va="bottom",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#2196f3", alpha=0.8),
        )

    # Fire/confirmation bar
    if idx_fire >= 0:
        ax_price.axvline(
            mdates.date2num(dates[idx_fire]), color="#4caf50",
            linestyle=":", linewidth=1.5, alpha=0.8,
        )
        ax_price.annotate(
            "반등 확인봉\n(전일고가 돌파)",
            (mdates.date2num(dates[idx_fire]), _offset_y(highs[idx_fire], 1)),
            color="#4caf50", fontsize=7, ha="center", va="bottom",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#4caf50", alpha=0.8),
        )

    # Entry (next open)
    if idx_entry >= 0:
        ax_price.scatter(
            mdates.date2num(dates[idx_entry]), opens[idx_entry],
            marker="^", s=120, color="#00e5ff", edgecolors="#00e5ff",
            linewidths=1.5, zorder=5,
        )
        ax_price.annotate(
            f"매수 시가\n{opens[idx_entry]:.0f}",
            (mdates.date2num(dates[idx_entry]), _offset_y(opens[idx_entry], -1)),
            color="#00e5ff", fontsize=7, ha="center", va="top",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#00e5ff", alpha=0.9),
        )

    # Exit (+20 close)
    if idx_exit >= 0:
        ax_price.scatter(
            mdates.date2num(dates[idx_exit]), closes[idx_exit],
            marker="v", s=120, color="#ff5252", edgecolors="#ff5252",
            linewidths=1.5, zorder=5,
        )
        ax_price.annotate(
            f"매도 +20봉 종가\n{closes[idx_exit]:.0f}",
            (mdates.date2num(dates[idx_exit]), _offset_y(closes[idx_exit], 1)),
            color="#ff5252", fontsize=7, ha="center", va="bottom",
            bbox=dict(boxstyle="round,pad=0.2", facecolor="#1a1a2e", edgecolor="#ff5252", alpha=0.9),
        )

    ax_price.set_ylabel("Price", color="#cccccc", fontsize=10)
    ax_price.yaxis.set_major_formatter(FuncFormatter(lambda x, _: f"{x:.0f}"))
    ax_price.legend(loc="upper left", fontsize=8, facecolor="#1a1a2e", edgecolor="#333333", labelcolor="#cccccc")
    ax_price.grid(True, alpha=0.15, color="#666666")

    # ── VOLUME PANEL ──
    for i in range(len(dates)):
        if i >= len(volumes):
            break
        vol_color = "#26a69a" if ups[i] else "#ef5350"
        vol_alpha = 0.6
        ax_vol.bar(
            mdates.date2num(dates[i]), volumes[i], body_width,
            color=vol_color, alpha=vol_alpha, edgecolor=vol_color, linewidth=0.3,
        )

    # Volume MA20
    valid_vol_ma = vol_ma20s > 0
    ax_vol.plot(
        [mdates.date2num(d) for d, v in zip(dates, valid_vol_ma) if v],
        vol_ma20s[valid_vol_ma],
        color="#ff9800", linewidth=1.0, alpha=0.7, label="Vol MA20",
    )

    # VW2 volume highlight
    if idx_source >= 0:
        ax_vol.axvline(
            mdates.date2num(dates[idx_source]), color="#ffeb3b",
            linestyle="--", linewidth=1.2, alpha=0.7,
        )

    ax_vol.set_ylabel("Volume", color="#cccccc", fontsize=10)
    ax_vol.yaxis.set_major_formatter(FuncFormatter(volume_fmt))
    ax_vol.legend(loc="upper left", fontsize=8, facecolor="#1a1a2e", edgecolor="#333333", labelcolor="#cccccc")
    ax_vol.grid(True, alpha=0.15, color="#666666")

    # ── Date axis ──
    ax_vol.xaxis.set_major_formatter(mdates.DateFormatter("%m/%d"))
    ax_vol.xaxis.set_major_locator(mdates.AutoDateLocator())
    for label in ax_vol.get_xticklabels():
        label.set_color("#cccccc")
    plt.setp(ax_vol.xaxis.get_majorticklabels(), rotation=30, ha="right", fontsize=7)

    # ── Title block ──
    label_map = {
        "P90 winner": "P90 승자 표본",
        "P50 median": "P50 중앙 표본",
        "P10 loser": "P10 패자 표본",
    }
    label_kr = label_map.get(sample["label"], sample["label"])
    net_str = f"{sample['net_return_pct']:+.2f}%"
    net_color = "#4caf50" if sample["net_return_pct"] > 0 else "#ef5350"

    title_lines = [
        f"VW2 첫 눌림 — {label_kr}  |  {sample['shcode']}",
        f"매수 {sample['entry_date']} 시가 {sample['entry_price']:.0f}  →  매도 {sample['exit_date']} 종가 {sample['exit_price']:.0f}  |  비용후 수익률 {net_str}",
    ]
    fig.suptitle(
        "\n".join(title_lines),
        color="#ffffff", fontsize=11, y=0.985, fontweight="bold",
    )

    # ── Strategy description footer ──
    footer_lines = [
        "전략: VW2 이후 최대 15봉 안 첫 눌림 (깊이 1~8%, 진입봉·눌림평균 거래량 ≤ VW2의 50%)",
        "반등 확인: 양봉 + 전일 고가 돌파  →  매수: 다음 거래일 시가  →  매도: +20봉째 종가  →  비용: 0.30% 차감",
    ]
    fig.text(
        0.5, 0.005, "\n".join(footer_lines),
        ha="center", va="bottom", fontsize=7, color="#888888",
    )

    plt.tight_layout(rect=[0, 0.04, 1, 0.93])
    fig.savefig(out_path, dpi=150, facecolor=fig.get_facecolor(), bbox_inches="tight")
    plt.close(fig)
    print(f"  → saved {out_path}")


def main():
    if len(sys.argv) < 3:
        print(f"사용법: {sys.argv[0]} <sample_json> <output_base_png>")
        sys.exit(1)

    json_path = sys.argv[1]
    out_base = sys.argv[2]

    data = load_json(json_path)
    samples = data.get("samples", [])
    strategy = data.get("strategy", "")
    config = data.get("config", {})

    for sample in samples:
        label = sample["label"]
        suffix = label.replace(" ", "_")
        out_dir = os.path.dirname(out_base)
        out_name = os.path.basename(out_base)
        root, ext = os.path.splitext(out_name)
        out_path = os.path.join(out_dir, f"{root}_{suffix}{ext}") if out_dir else f"{root}_{suffix}{ext}"
        render_sample(sample, strategy, config, out_path)


if __name__ == "__main__":
    main()
