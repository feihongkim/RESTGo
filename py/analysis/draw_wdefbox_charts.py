"""W패턴+DefBox 결합 신호 차트 생성"""
import sys
import os
import json
import numpy as np

_this_dir = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _this_dir)
from wbottom_chart import (fetch_kr, fetch_foreign, find_boxes,
                            calc_bb, draw_chart, _tg_send_files)
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.patches as mpatches
from matplotlib.lines import Line2D

def draw_wdefbox_chart(dates, opens, highs, lows, closes,
                       upper, mid, lower, pct_b, ma5,
                       sup_boxes, res_boxes,
                       p1_idx, p2_idx, entry_idx,
                       defbox_price, defbox_idx, defbox_break_idx,
                       title, outpath):
    """W패턴 + DefBox 수평선 + 돌파 마킹 차트"""
    s = max(0, p1_idx - 20)
    e = min(len(closes), max(entry_idx + 35,
                              (defbox_break_idx + 10 if defbox_break_idx >= 0 else 0)))
    idx = list(range(s, e))

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(13, 7),
                                    gridspec_kw={'height_ratios': [3, 1]})
    fig.patch.set_facecolor('#1e1e1e')
    for ax in [ax1, ax2]:
        ax.set_facecolor('#1e1e1e')
        ax.tick_params(colors='#aaaaaa')
        for spine in ax.spines.values():
            spine.set_color('#444')

    local = {v: i for i, v in enumerate(idx)}

    # 캔들스틱
    for i, gi in enumerate(idx):
        o, h, l, c = opens[gi], highs[gi], lows[gi], closes[gi]
        color = '#ef5350' if c >= o else '#26a69a'
        ax1.plot([i, i], [l, h], color=color, linewidth=0.8)
        ax1.bar(i, abs(c - o), bottom=min(o, c), color=color, width=0.6)

    # Bollinger Bands
    for xv, yv, col, ls in [
        ([local[i] for i in idx if not np.isnan(upper[i])],
         [upper[i] for i in idx if not np.isnan(upper[i])], '#90caf9', '-'),
        ([local[i] for i in idx if not np.isnan(mid[i])],
         [mid[i] for i in idx if not np.isnan(mid[i])],   '#fff176', '--'),
        ([local[i] for i in idx if not np.isnan(lower[i])],
         [lower[i] for i in idx if not np.isnan(lower[i])], '#80cbc4', '-'),
    ]:
        ax1.plot(xv, yv, color=col, linewidth=1.1, linestyle=ls)

    # MA5
    m5x = [local[i] for i in idx if not np.isnan(ma5[i])]
    m5v = [ma5[i] for i in idx if not np.isnan(ma5[i])]
    ax1.plot(m5x, m5v, color='#ff9800', linewidth=1.1)

    # Box 마킹
    for (bp, _) in sup_boxes:
        if bp in local:
            ax1.plot(local[bp], lows[bp], marker='^', markersize=7,
                     color='#ff5252', markeredgewidth=0, zorder=5)
    for (bp, _) in res_boxes:
        if bp in local:
            ax1.plot(local[bp], highs[bp], marker='v', markersize=7,
                     color='#64b5f6', markeredgewidth=0, zorder=5)

    # P1, P2, Entry 마킹
    for pos, label, col, mult in [
        (p1_idx,    'P1\n(BB터치)',   '#ff5252', 0.975),
        (p2_idx,    'P2\n(내부저점)', '#ffab40', 0.975),
        (entry_idx, '진입\n(W감지)',  '#69f0ae', 1.025),
    ]:
        if pos in local:
            xi = local[pos]
            y = lows[pos] if mult < 1 else closes[pos]
            ax1.annotate(label, xy=(xi, y), xytext=(xi, y * mult),
                         arrowprops=dict(arrowstyle='->', color=col),
                         color=col, fontsize=8, ha='center')

    # DefBox 수평선
    if defbox_price is not None and defbox_price > 0:
        ax1.axhline(defbox_price, color='#f48fb1', linewidth=1.5,
                    linestyle='--', alpha=0.9, label=f'DefBox {defbox_price:.0f}')
        ax1.text(0, defbox_price * 1.003, f'DefBox {defbox_price:.0f}',
                 color='#f48fb1', fontsize=8, va='bottom')

    # DefBox 박스 위치 마킹
    if defbox_idx is not None and defbox_idx in local:
        xi = local[defbox_idx]
        ax1.plot(xi, highs[defbox_idx], marker='D', markersize=8,
                 color='#f48fb1', markeredgewidth=0, zorder=6)

    # DefBox 돌파 마킹
    if defbox_break_idx is not None and defbox_break_idx >= 0 and defbox_break_idx in local:
        xi = local[defbox_break_idx]
        ax1.annotate('DefBox\n돌파(+50%)', xy=(xi, closes[defbox_break_idx]),
                     xytext=(xi, closes[defbox_break_idx] * 1.04),
                     arrowprops=dict(arrowstyle='->', color='#f48fb1'),
                     color='#f48fb1', fontsize=8, ha='center')

    ax1.set_title(title, color='white', fontsize=10)
    ax1.legend(loc='upper left', fontsize=7, facecolor='#2e2e2e', labelcolor='white')
    ax1.yaxis.label.set_color('#aaaaaa')

    # %B 패널
    pbx = [local[i] for i in idx if not np.isnan(pct_b[i])]
    pbv = [pct_b[i] for i in idx if not np.isnan(pct_b[i])]
    ax2.plot(pbx, pbv, color='#ce93d8', linewidth=1.2)
    ax2.axhline(0.5, color='#fff176', linewidth=0.7, linestyle='--')
    ax2.axhline(0.0, color='#80cbc4', linewidth=0.7, linestyle='--')
    ax2.set_ylim(-0.5, 1.5)
    ax2.set_ylabel('%B', color='#aaaaaa', fontsize=8)

    step = max(1, len(idx) // 8)
    ticks = list(range(0, len(idx), step))
    ax2.set_xticks(ticks)
    ax2.set_xticklabels([str(dates[idx[t]]) for t in ticks],
                         rotation=30, ha='right', fontsize=7, color='#aaaaaa')
    ax1.set_xticks([])

    plt.tight_layout()
    plt.savefig(outpath, dpi=120, bbox_inches='tight', facecolor='#1e1e1e')
    plt.close()
    return outpath


def draw_from_wdefbox(ex, fetch_fn, out_dir):
    shcode = ex['shcode']
    market = ex['market']
    sig_date = ex['signal_date']
    p1_date = ex.get('p1_date', '')
    p2_date = ex.get('p2_date', '')
    defbox_price = ex.get('defbox_price', 0)
    defbox_date = ex.get('defbox_date', '')
    defbox_break_date = ex.get('defbox_break_date', '')

    try:
        rows = fetch_fn(shcode, 600)
        if len(rows) < 60:
            return None
        dates  = [str(r[0]) for r in rows]
        opens  = np.array([float(r[1]) for r in rows])
        highs  = np.array([float(r[2]) for r in rows])
        lows   = np.array([float(r[3]) for r in rows])
        closes = np.array([float(r[4]) for r in rows])
        ma5 = np.full(len(closes), np.nan)
        for i in range(4, len(closes)):
            ma5[i] = closes[i-4:i+1].mean()
        upper, mid, lower, pct_b, _ = calc_bb(closes)
        sup_boxes, res_boxes = find_boxes(opens, highs, lows, closes, ma5)

        def nearest(d):
            if d in dates:
                return dates.index(d)
            return min(range(len(dates)), key=lambda i: abs(int(dates[i]) - int(d)))

        p1_idx   = nearest(p1_date)   if p1_date   else -1
        p2_idx   = nearest(p2_date)   if p2_date   else -1
        entry_idx = nearest(sig_date) if sig_date  else -1
        defbox_idx = nearest(defbox_date) if defbox_date else None
        defbox_break_idx = nearest(defbox_break_date) if defbox_break_date else -1

        break_str = f"돌파:{defbox_break_date}" if defbox_break_date else "미돌파"
        title = f"W+DefBox [{market}] {shcode}  진입:{sig_date}  DefBox:{defbox_price:.0f}  {break_str}"
        outpath = f"{out_dir}/{market}_{shcode}.png"
        draw_wdefbox_chart(dates, opens, highs, lows, closes,
                           upper, mid, lower, pct_b, ma5,
                           sup_boxes, res_boxes,
                           p1_idx, p2_idx, entry_idx,
                           defbox_price, defbox_idx, defbox_break_idx,
                           title, outpath)
        return outpath
    except Exception as e:
        print(f"  [{market}] {shcode} 오류: {e}")
        return None


def main():
    n = int(sys.argv[1]) if len(sys.argv) > 1 else 5
    scan_json = '/tmp/wdefbox_kr.json'

    with open(scan_json) as f:
        data = json.load(f)
    examples = data.get('examples', [])

    # DefBox 있음 + 20일 내 돌파 필터
    hits = [e for e in examples if e.get('has_defbox') and e.get('defbox_break_date')]
    print(f"DefBox+돌파 후보: {len(hits)}건")

    out_dir = '/tmp/wdefbox_charts'
    os.makedirs(out_dir, exist_ok=True)

    files = []
    seen = set()
    for ex in hits:
        if len(files) >= n:
            break
        if ex['shcode'] in seen:
            continue
        seen.add(ex['shcode'])
        print(f"  {ex['shcode']} {ex['signal_date']}  DefBox:{ex.get('defbox_price',0):.0f}  돌파:{ex['defbox_break_date']}")
        path = draw_from_wdefbox(ex, fetch_kr, out_dir)
        if path:
            files.append(path)

    print(f"\n차트 {len(files)}개 생성 → 텔레그램 전송")
    _tg_send_files(files)


if __name__ == '__main__':
    main()
