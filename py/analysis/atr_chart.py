"""ATR 채널 시각화 — 15분 비트코인 차트 + ATR 손절/타겟 라인

사용법:
  python py/analysis/atr_chart.py [market=KRW-BTC] [out_dir=/tmp] [total_bars=900] [chunk_bars=180]

각 봉의 종가 기준:
  • 타겟 라인 = Close + ATRTargetMultiplier × ATR (기본 1.5)  → 파란 점선
  • 손절 라인 = Close − ATRStopMultiplier × ATR   (기본 3.0)  → 빨간 점선
"""
import sys
import os
import pandas as pd
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
import matplotlib.font_manager as fm
import pymssql
import glob as _glob

# 한글 폰트
for cand in (_glob.glob('/usr/share/fonts/**/NanumGothic.ttf', recursive=True) +
             _glob.glob('/home/node/.local/**/NanumGothic.ttf', recursive=True)):
    if os.path.exists(cand) and os.path.getsize(cand) > 1000:
        try:
            fm.fontManager.addfont(cand)
            break
        except Exception:
            continue
plt.rcParams['font.family'] = 'NanumGothic'
plt.rcParams['axes.unicode_minus'] = False

for _root in ('/workspace', '/home/feihong/code/REST/RESTGo'):
    if os.path.isdir(_root):
        sys.path.insert(0, _root)
        break
from py.common.db import _load_config, _get_key, decrypt


# Strategy3.yaml 의 settings 블록 값과 일치
ATR_PERIOD = 14
ATR_STOP_MULT = 3.0
ATR_TARGET_MULT = 1.5


def tuf_conn():
    cfg = _load_config()
    key = _get_key(cfg['FKEY'])
    return pymssql.connect(
        server='tuf.tail5b4272.ts.net',
        port=int(decrypt(cfg['MSSQL_PORT'], key)),
        user=decrypt(cfg['MSSQL_USER'], key),
        password=decrypt(cfg['MSSQL_PASSWORD'], key),
        database='Upbit', charset='utf8',
    )


def fetch_recent(market: str, n: int) -> pd.DataFrame:
    """최근 n봉을 가져온 뒤 오래된 봉부터 정렬."""
    sql = f"""
        SELECT TOP {n}
            candle_date_time_kst, opening_price, high_price, low_price, trade_price, candle_acc_trade_volume
        FROM candles_15m
        WHERE market = '{market}'
        ORDER BY candle_date_time_kst DESC
    """
    with tuf_conn() as conn:
        df = pd.read_sql(sql, conn)
    df['candle_date_time_kst'] = pd.to_datetime(df['candle_date_time_kst'])
    df = df.sort_values('candle_date_time_kst').reset_index(drop=True)
    df.rename(columns={
        'opening_price': 'open', 'high_price': 'high',
        'low_price': 'low', 'trade_price': 'close',
        'candle_acc_trade_volume': 'volume',
    }, inplace=True)
    return df


def calc_atr_wilder(df: pd.DataFrame, period: int = 14) -> pd.Series:
    """Wilder 방식 ATR — RESTGo indicator/candle_processor.go 와 동일 공식."""
    high = df['high'].values
    low = df['low'].values
    close = df['close'].values
    n = len(df)
    tr = np.zeros(n)
    for i in range(1, n):
        hl = high[i] - low[i]
        hc = abs(high[i] - close[i-1])
        lc = abs(low[i] - close[i-1])
        tr[i] = max(hl, hc, lc)
    atr = np.zeros(n)
    if n > period:
        # 초기 ATR = 단순평균 TR[1..period]
        atr[period] = tr[1:period+1].mean()
        # 이후 Wilder smoothing (rolling sum 방식 — Go 코드와 동일)
        ts = tr[1:period+1].sum()
        for i in range(period+1, n):
            ts += tr[i] - tr[i-period]
            atr[i] = ts / period
    return pd.Series(atr, index=df.index)


def _draw_candles(ax, df_chunk, body_minutes: int = 12):
    width = pd.Timedelta(minutes=body_minutes)
    for _, row in df_chunk.iterrows():
        t = row['candle_date_time_kst']
        o, h, l, c = row['open'], row['high'], row['low'], row['close']
        bullish = c >= o
        color = '#d62728' if bullish else '#1f77b4'  # 한국식: 양봉 빨강, 음봉 파랑
        ax.plot([t, t], [l, h], color=color, lw=0.6, zorder=1)
        body_h = abs(c - o)
        bottom = min(o, c)
        rect = plt.Rectangle(
            (t - width/2, bottom), width, body_h if body_h > 0 else (h-l)*0.001,
            facecolor=color, edgecolor=color, linewidth=0.5, zorder=2,
        )
        ax.add_patch(rect)


def render(df: pd.DataFrame, market: str, out_png: str, chunk_idx: int, total: int):
    fig, (ax_price, ax_atr, ax_vol) = plt.subplots(
        3, 1, figsize=(15, 9), sharex=True,
        gridspec_kw={'height_ratios': [4, 1.2, 1]},
    )

    # ── 가격 패널 ──────────────────────────────────────────────
    _draw_candles(ax_price, df)

    # ATR 채널 — 봉별 종가 기준 가상 매수 시 손절/타겟 라인
    valid = df['atr'] > 0
    target = df['close'] + ATR_TARGET_MULT * df['atr']  # +1.5×ATR
    stop   = df['close'] - ATR_STOP_MULT   * df['atr']  # -3.0×ATR

    ax_price.plot(df['candle_date_time_kst'][valid], target[valid],
                  color='#1565c0', lw=1.2, linestyle='--', alpha=0.85,
                  label=f'타겟 (Close + {ATR_TARGET_MULT}×ATR)')
    ax_price.plot(df['candle_date_time_kst'][valid], stop[valid],
                  color='#c62828', lw=1.2, linestyle='--', alpha=0.85,
                  label=f'손절 (Close − {ATR_STOP_MULT}×ATR)')

    # 채널 영역 음영
    ax_price.fill_between(
        df['candle_date_time_kst'][valid], stop[valid], target[valid],
        color='#888888', alpha=0.06, zorder=0,
    )

    ax_price.set_xlim(
        df['candle_date_time_kst'].iloc[0] - pd.Timedelta(minutes=15),
        df['candle_date_time_kst'].iloc[-1] + pd.Timedelta(minutes=15),
    )
    # y 범위 = 채널선 포함 마진
    lo_y = min(df['low'].min(), stop[valid].min()) * 0.998
    hi_y = max(df['high'].max(), target[valid].max()) * 1.002
    ax_price.set_ylim(lo_y, hi_y)
    ax_price.set_ylabel('가격 (KRW)')
    ax_price.legend(loc='upper left', fontsize=9, ncol=3)
    ax_price.grid(alpha=0.3)
    period = f'{df["candle_date_time_kst"].iloc[0].strftime("%Y-%m-%d %H:%M")} ~ {df["candle_date_time_kst"].iloc[-1].strftime("%Y-%m-%d %H:%M")}'
    avg_atr_pct = (df['atr'][valid] / df['close'][valid] * 100).mean()
    ax_price.set_title(
        f'{market} 15분봉 ATR 채널 [{chunk_idx}/{total}]\n'
        f'{period}  |  ATR 평균 비율 {avg_atr_pct:.2f}%  '
        f'|  Stop×{ATR_STOP_MULT} / Target×{ATR_TARGET_MULT} (strategy3.yaml)',
        fontsize=11,
    )

    # ── ATR 자체 패널 ────────────────────────────────────────
    ax_atr.plot(df['candle_date_time_kst'][valid], df['atr'][valid],
                color='#8e24aa', lw=1.0, label='ATR (Wilder 14봉)')
    ax_atr.set_ylabel('ATR')
    ax_atr.legend(loc='upper left', fontsize=8)
    ax_atr.grid(alpha=0.3)

    # ── 거래량 패널 ──────────────────────────────────────────
    for _, row in df.iterrows():
        color = '#d62728' if row['close'] >= row['open'] else '#1f77b4'
        ax_vol.bar(row['candle_date_time_kst'], row['volume'],
                   width=pd.Timedelta(minutes=12), color=color, alpha=0.6)
    ax_vol.set_ylabel('Volume')
    ax_vol.grid(alpha=0.3)
    ax_vol.set_xlabel('시각 (KST)')
    ax_vol.xaxis.set_major_locator(mdates.AutoDateLocator())
    ax_vol.xaxis.set_major_formatter(mdates.DateFormatter('%m-%d %H:%M'))
    plt.setp(ax_vol.xaxis.get_majorticklabels(), rotation=15)

    plt.tight_layout()
    os.makedirs(os.path.dirname(out_png) or '.', exist_ok=True)
    plt.savefig(out_png, dpi=120, bbox_inches='tight')
    plt.close(fig)


def main():
    args = sys.argv[1:]
    market = args[0] if len(args) >= 1 else 'KRW-BTC'
    out_dir = args[1] if len(args) >= 2 else '/tmp'
    total_bars = int(args[2]) if len(args) >= 3 else 900
    chunk_bars = int(args[3]) if len(args) >= 4 else 180

    # ATR 워밍업 100봉 추가 확보 후 잘라냄
    fetch_n = total_bars + 100
    print(f'fetching {fetch_n} bars for {market}...')
    df = fetch_recent(market, fetch_n)
    print(f'got {len(df)} bars')

    # ATR 계산
    df['atr'] = calc_atr_wilder(df, ATR_PERIOD)

    # 워밍업 잘라내고 최근 total_bars만 사용
    df = df.iloc[-total_bars:].reset_index(drop=True)

    # 청크 분할
    total = (len(df) + chunk_bars - 1) // chunk_bars
    print(f'rendering {total} chunks ({chunk_bars} bars each)...')
    pngs = []
    for idx, start in enumerate(range(0, len(df), chunk_bars), 1):
        sub = df.iloc[start:start+chunk_bars].copy()
        if len(sub) < 30:
            continue
        png = f'{out_dir}/atr_{market.replace("-", "_")}_chunk_{idx:02d}.png'
        render(sub, market, png, idx, total)
        pngs.append(png)
        print(f'  chunk {idx}/{total}: {len(sub)} bars → {png}')
    print(f'done: {len(pngs)} PNGs')
    print('CHUNKS=' + ','.join(pngs))


if __name__ == '__main__':
    main()
