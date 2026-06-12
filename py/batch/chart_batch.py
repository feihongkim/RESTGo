"""
batch_signals.json을 읽어 매수 신호가 있는 종목의 차트를 일괄 생성
- zpicture/batch_signals.json 에서 신호 목록 읽기
- 각 종목 캔들 데이터 DB 조회 + Box 분석
- 매수 신호 날짜에 빨간 점선, MainBox 가격에 초록 점선 오버레이
- zpicture/batch_YYYYMMDD/box_{shcode}.png 로 저장
"""
import sys
import json
import os as _os
from datetime import datetime

# 프로젝트 루트 설정 (py/batch/ 기준 2단계 상위)
_PROJECT_ROOT = _os.path.dirname(_os.path.dirname(_os.path.dirname(_os.path.abspath(__file__))))
sys.path.insert(0, _PROJECT_ROOT)

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.font_manager as fm
from matplotlib.lines import Line2D
import glob as _glob
import numpy as np

# 한글 폰트 설정
_nanum_candidates = _glob.glob('/home/node/.local/lib/python3.12/site-packages/koreanize_matplotlib/fonts/NanumGothic.ttf') + \
                    _glob.glob('/usr/share/fonts/**/NanumGothic.ttf', recursive=True)
if _nanum_candidates:
    fm.fontManager.addfont(_nanum_candidates[0])
plt.rcParams['font.family'] = 'NanumGothic'
plt.rcParams['axes.unicode_minus'] = False

from py.common.db import open_connection
from py.analysis.box_chart import (
    prepare_candles, run_box_analysis,
    KIND_BOX, KIND_MAIN, KIND_DEF,
    BOXTYPE_SUPPORT, BOXTYPE_RESIST,
)


# ─────────────────────────────────────────────
# 1. DB 조회
# ─────────────────────────────────────────────

def fetch_candles(shcode: str, days: int = 250) -> 'pd.DataFrame':
    import pandas as pd
    sql = f"""
        SELECT TOP {days}
            stck_bsop_date AS date,
            CAST(stck_oprc AS FLOAT) AS oprc,
            CAST(stck_hgpr AS FLOAT) AS hgpr,
            CAST(stck_lwpr AS FLOAT) AS lwpr,
            CAST(stck_clpr AS FLOAT) AS clpr,
            CAST(acml_vol AS FLOAT) AS volume
        FROM DM.BP_PeriodPrice
        WHERE stck_shrn_iscd = '{shcode}' AND period_type = 'D'
        ORDER BY stck_bsop_date DESC
    """
    with open_connection(server='white', database='KIS2') as conn:
        df = pd.read_sql(sql, conn)
    df = df.rename(columns={'clpr': 'close', 'oprc': 'open', 'hgpr': 'high', 'lwpr': 'low'})
    df = df.sort_values('date').reset_index(drop=True)
    return df


# ─────────────────────────────────────────────
# 2. 어노테이션 차트 그리기
# ─────────────────────────────────────────────

def draw_annotated_chart(df, candles: list, box_list: list,
                         shcode: str, name: str,
                         signals: list, display_days: int = 180,
                         out_dir: str = '') -> str:
    """
    signals: list of dict with keys date, position, reason,
             defbox_price, mainbox_price, mainbox_date
    """
    n = len(candles)
    start_i = max(0, n - display_days)
    display_candles = candles[start_i:]
    import pandas as pd
    display_df = df.iloc[start_i:].reset_index(drop=True)

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(18, 10),
                                   gridspec_kw={'height_ratios': [3, 1]},
                                   facecolor='#1a1a2e')
    ax1.set_facecolor('#1a1a2e')
    ax2.set_facecolor('#1a1a2e')

    # ── 캔들스틱 ──
    for xi, c in enumerate(display_candles):
        color = '#e74c3c' if c['close'] >= c['open'] else '#3498db'
        body_low  = min(c['open'], c['close'])
        body_high = max(c['open'], c['close'])
        ax1.bar(xi, body_high - body_low, bottom=body_low, color=color, width=0.6, linewidth=0)
        ax1.plot([xi, xi], [c['low'],  body_low],  color=color, linewidth=0.8)
        ax1.plot([xi, xi], [body_high, c['high']], color=color, linewidth=0.8)

    # ── 이동평균선 ──
    ax1.plot(range(len(display_candles)), [c['ma5']  for c in display_candles],
             color='#f1c40f', linewidth=0.8, label='MA5',  alpha=0.9)
    ax1.plot(range(len(display_candles)), [c['ma20'] for c in display_candles],
             color='#2ecc71', linewidth=0.8, label='MA20', alpha=0.9)
    ax1.plot(range(len(display_candles)), [c['ma60'] for c in display_candles],
             color='#e67e22', linewidth=0.8, label='MA60', alpha=0.9)

    price_min = min(c['low']  for c in display_candles) * 0.98
    price_max = max(c['high'] for c in display_candles) * 1.02

    # ── Box 표시 ──
    added_labels = set()
    for box in box_list:
        box_xi = box['pos'] - start_i
        if box_xi < 0:
            box_xi = 0

        box_price = box['price']
        if box_price < price_min or box_price > price_max:
            continue

        if box['kind'] == KIND_DEF:
            color, lw, ls, label, zorder = '#ff6b6b', 1.8, '-', 'DefBox', 5
        elif box['kind'] == KIND_MAIN:
            color, lw, ls, label, zorder = '#ffd93d', 1.4, '--', 'MainBox', 4
        else:
            color = '#6bcb77' if box['boxtype'] == BOXTYPE_SUPPORT else '#4d96ff'
            lw, ls, zorder = 1.0, ':', 3
            label = 'SupportBox' if box['boxtype'] == BOXTYPE_SUPPORT else 'ResistBox'

        ax1.hlines(y=box_price, xmin=box_xi, xmax=len(display_candles) - 1,
                   colors=color, linewidths=lw, linestyles=ls, zorder=zorder,
                   label=label if label not in added_labels else '')
        added_labels.add(label)

        if 0 <= box_xi < len(display_candles):
            marker = 'v' if box['boxtype'] == BOXTYPE_RESIST else '^'
            ax1.scatter(box_xi, box_price, color=color, s=40, zorder=zorder+1, marker=marker)

        if box['kind'] == KIND_DEF:
            ax1.text(len(display_candles) - 1, box_price,
                     f' {box_price:,.0f}', color=color, fontsize=7,
                     va='center', ha='left', zorder=6)

    # ── 매수 신호 어노테이션 ──
    # 날짜→표시 구간 x좌표 매핑
    date_to_xi = {c['date']: xi for xi, c in enumerate(display_candles)}

    added_signal_label = False
    added_main_label   = False

    for sig in signals:
        sig_date = sig.get('date', '')
        xi = date_to_xi.get(sig_date)

        if xi is not None:
            # 매수 신호 빨간 수직 점선
            ax1.axvline(x=xi, color='#ff4444', linewidth=1.2, linestyle='--', alpha=0.85,
                        zorder=7, label='매수신호' if not added_signal_label else '')
            added_signal_label = True

            # 텍스트 어노테이션
            ax1.annotate(
                f"매수\n{sig.get('reason', '')}",
                xy=(xi, display_candles[xi]['high']),
                xytext=(xi + 0.5, display_candles[xi]['high'] * 1.005),
                color='#ff4444', fontsize=6, fontweight='bold',
                va='bottom', zorder=8,
            )

        # MainBox 가격 초록 수평 점선
        mainbox_price = sig.get('mainbox_price', 0)
        if mainbox_price and price_min <= mainbox_price <= price_max:
            ax1.axhline(y=mainbox_price, color='#00e676', linewidth=1.0, linestyle='--',
                        alpha=0.75, zorder=6,
                        label='매수기준MainBox' if not added_main_label else '')
            added_main_label = True
            ax1.text(len(display_candles) - 1, mainbox_price,
                     f' {mainbox_price:,.0f}', color='#00e676', fontsize=7,
                     va='bottom', ha='left', zorder=7)

    # ── 거래량 ──
    for xi, c in enumerate(display_candles):
        color = '#e74c3c' if c['close'] >= c['open'] else '#3498db'
        ax2.bar(xi, c['volume'], color=color, width=0.6, alpha=0.7, linewidth=0)

    # ── 축 스타일 ──
    ax1.set_xlim(-1, len(display_candles))
    ax1.set_ylim(price_min, price_max)
    ax1.yaxis.set_major_formatter(plt.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    tick_step = max(1, len(display_candles) // 10)
    ticks = list(range(0, len(display_candles), tick_step))
    ax1.set_xticks(ticks)
    ax1.set_xticklabels([display_candles[t]['date'] for t in ticks],
                        rotation=30, fontsize=7, color='#aaaaaa')
    ax2.set_xticks(ticks)
    ax2.set_xticklabels([display_candles[t]['date'] for t in ticks],
                        rotation=30, fontsize=7, color='#aaaaaa')

    ax1.tick_params(colors='#aaaaaa', labelsize=8)
    ax2.tick_params(colors='#aaaaaa', labelsize=7)
    ax1.spines['bottom'].set_visible(False)
    for spine in ax2.spines.values():
        spine.set_color('#333366')
    for spine in ax1.spines.values():
        spine.set_color('#333366')

    # ── 제목/범례 ──
    box_counts = {KIND_BOX: 0, KIND_MAIN: 0, KIND_DEF: 0}
    for b in box_list:
        box_counts[b['kind']] = box_counts.get(b['kind'], 0) + 1

    signal_dates = ', '.join(s.get('date', '') for s in signals)
    ax1.set_title(
        f'{shcode} {name}  |  최근 {display_days}일  |  '
        f'Box:{box_counts[KIND_BOX]}  MainBox:{box_counts[KIND_MAIN]}  DefBox:{box_counts[KIND_DEF]}  '
        f'매수신호: {signal_dates}',
        color='white', fontsize=11, fontweight='bold', pad=10
    )

    legend_elements = [
        Line2D([0], [0], color='#f1c40f', lw=1, label='MA5'),
        Line2D([0], [0], color='#2ecc71', lw=1, label='MA20'),
        Line2D([0], [0], color='#e67e22', lw=1, label='MA60'),
        Line2D([0], [0], color='#4d96ff', lw=1.2, ls=':', label='ResistBox'),
        Line2D([0], [0], color='#6bcb77', lw=1.2, ls=':', label='SupportBox'),
        Line2D([0], [0], color='#ffd93d', lw=1.4, ls='--', label='MainBox'),
        Line2D([0], [0], color='#ff6b6b', lw=1.8, ls='-', label='DefBox'),
        Line2D([0], [0], color='#ff4444', lw=1.2, ls='--', label='매수신호'),
        Line2D([0], [0], color='#00e676', lw=1.0, ls='--', label='매수기준MainBox'),
    ]
    ax1.legend(handles=legend_elements, loc='upper left', fontsize=7,
               facecolor='#1a1a2e', edgecolor='#333366', labelcolor='white')

    ax2.set_ylabel('Volume', color='#aaaaaa', fontsize=8)
    ax1.set_ylabel('Price (KRW)', color='#aaaaaa', fontsize=8)
    ax1.grid(axis='y', color='#2a2a4e', linewidth=0.5, alpha=0.5)
    ax2.grid(axis='y', color='#2a2a4e', linewidth=0.5, alpha=0.5)

    plt.tight_layout(pad=1.5)

    _os.makedirs(out_dir, exist_ok=True)
    out_path = _os.path.join(out_dir, f'box_{shcode}.png')
    plt.savefig(out_path, dpi=130, bbox_inches='tight', facecolor='#1a1a2e')
    plt.close()
    return out_path


# ─────────────────────────────────────────────
# 3. 메인
# ─────────────────────────────────────────────

def main():
    signals_path = _os.path.join(_PROJECT_ROOT, 'zpicture', 'batch_signals.json')

    if not _os.path.exists(signals_path):
        print(f'[오류] 신호 파일 없음: {signals_path}')
        print('먼저 ./RESTGo stock batch 를 실행하세요.')
        sys.exit(1)

    with open(signals_path, 'r', encoding='utf-8') as f:
        batch = json.load(f)

    display_days = batch.get('display_days', 180)
    stocks = batch.get('stocks', [])
    today_str = datetime.now().strftime('%Y%m%d')
    out_dir = _os.path.join(_PROJECT_ROOT, 'zpicture', f'batch_{today_str}')

    print(f'[batch_chart] 신호 파일: {signals_path}')
    print(f'[batch_chart] 대상 종목: {len(stocks)}개  출력 경로: {out_dir}')

    success, failed = 0, 0

    for idx, stock in enumerate(stocks, 1):
        shcode = stock['shcode']
        hname  = stock.get('hname', shcode)
        signals = stock.get('signals', [])

        print(f'[{idx}/{len(stocks)}] {shcode} {hname}  신호수: {len(signals)}', end=' ... ', flush=True)
        try:
            df = fetch_candles(shcode, days=250)
            if len(df) < 6:
                print('스킵 (캔들 부족)')
                failed += 1
                continue

            candles  = prepare_candles(df)
            box_list = run_box_analysis(candles)

            out_path = draw_annotated_chart(
                df, candles, box_list,
                shcode, hname, signals,
                display_days=display_days,
                out_dir=out_dir,
            )
            print(f'저장: {_os.path.basename(out_path)}')
            success += 1
        except Exception as e:
            print(f'오류: {e}')
            failed += 1

    print(f'\n[완료] 성공: {success}  실패: {failed}  저장 경로: {out_dir}')


if __name__ == '__main__':
    main()
