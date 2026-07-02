"""T03_EMAPullback 매수/매도 시각화 차트 (Upbit 15분봉)

사용법:
  단일 차트:  python py/analysis/t03_chart.py <trades_json> [market=KRW-ETH] [out_png] [days=21]
  청크 분할:  python py/analysis/t03_chart.py --chunk <trades_json> [market=KRW-ETH] [out_dir=/tmp] [days=21] [chunk_bars=180]
"""
import sys
import json
import os
import pandas as pd
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.dates as mdates
import matplotlib.font_manager as fm
import pymssql

# 한글 폰트
import glob as _glob
for cand in (_glob.glob('/usr/share/fonts/**/NanumGothic.ttf', recursive=True) +
             _glob.glob('/home/feihong/code/REST/RESTGo/venv/**/NanumGothic.ttf', recursive=True)):
    if os.path.exists(cand):
        fm.fontManager.addfont(cand)
        break
plt.rcParams['font.family'] = 'NanumGothic'
plt.rcParams['axes.unicode_minus'] = False


for _root in ('/workspace', '/home/feihong/code/REST/RESTGo'):
    if os.path.isdir(_root):
        sys.path.insert(0, _root)
        break
from py.common.db import _load_config, _get_key, decrypt


def tuf_conn():
    cfg = _load_config()
    key = _get_key(cfg["FKEY"])
    return pymssql.connect(
        server='tuf.tail5b4272.ts.net',
        port=int(decrypt(cfg["MSSQL_PORT"], key)),
        user=decrypt(cfg["MSSQL_USER"], key),
        password=decrypt(cfg["MSSQL_PASSWORD"], key),
        database='Upbit', charset='utf8',
    )


def fetch_candles(market: str, date_from: str, date_to: str) -> pd.DataFrame:
    """candles_15m에서 [date_from, date_to] 범위 캔들 조회 (YYYYMMDD)."""
    df = date_from[:4] + '-' + date_from[4:6] + '-' + date_from[6:8] + ' 00:00:00'
    dt = date_to[:4] + '-' + date_to[4:6] + '-' + date_to[6:8] + ' 23:59:59'
    sql = f"""
        SELECT candle_date_time_kst, opening_price, high_price, low_price, trade_price, candle_acc_trade_volume
        FROM candles_15m
        WHERE market = '{market}'
          AND candle_date_time_kst >= '{df}'
          AND candle_date_time_kst <= '{dt}'
        ORDER BY candle_date_time_kst ASC
    """
    with tuf_conn() as conn:
        d = pd.read_sql(sql, conn)
    d['candle_date_time_kst'] = pd.to_datetime(d['candle_date_time_kst'])
    d.rename(columns={
        'opening_price': 'open', 'high_price': 'high',
        'low_price': 'low', 'trade_price': 'close',
        'candle_acc_trade_volume': 'volume',
    }, inplace=True)
    return d.reset_index(drop=True)


def _parse_time(date_yyyymmdd: str, time_str: str) -> pd.Timestamp:
    """YYYYMMDD + 'HH:MM:SS' or 'HHMMSS' → Timestamp. time 없으면 정오 fallback."""
    base = pd.to_datetime(date_yyyymmdd, format='%Y%m%d')
    if not time_str:
        return base + pd.Timedelta(hours=12)
    clean = time_str.replace(':', '')
    if len(clean) >= 4:
        try:
            return base + pd.Timedelta(hours=int(clean[:2]), minutes=int(clean[2:4]))
        except ValueError:
            pass
    return base + pd.Timedelta(hours=12)


def _draw_candles(ax, df_chunk: pd.DataFrame, body_minutes: int = 12):
    """OHLC 캔들 그리기 (15분봉이므로 width=12분)."""
    width = pd.Timedelta(minutes=body_minutes)
    for _, row in df_chunk.iterrows():
        t = row['candle_date_time_kst']
        o, h, l, c = row['open'], row['high'], row['low'], row['close']
        # 한국 관례: 양봉 빨강, 음봉 파랑
        bullish = c >= o
        color = '#d62728' if bullish else '#1f77b4'
        # 고저선
        ax.plot([t, t], [l, h], color=color, lw=0.6, zorder=1)
        # 몸통 (사각형)
        body_h = abs(c - o)
        bottom = min(o, c)
        rect = plt.Rectangle(
            (t - width/2, bottom), width, body_h if body_h > 0 else (h-l)*0.001,
            facecolor=color, edgecolor=color, linewidth=0.5, zorder=2,
        )
        ax.add_patch(rect)


def render_chart(df_chunk: pd.DataFrame, chunk_trades: list, r: dict, market: str,
                 out_png: str, chunk_idx: int = None, total_chunks: int = None):
    """단일 청크 차트 렌더링 (캔들차트 + EMA + 매수/매도)."""
    fig, (ax_price, ax_vol) = plt.subplots(
        2, 1, figsize=(14, 8), sharex=True,
        gridspec_kw={'height_ratios': [4, 1]},
    )
    _draw_candles(ax_price, df_chunk)
    ax_price.plot(df_chunk['candle_date_time_kst'], df_chunk['ema9'], color='#FFA500', lw=1.3, label='EMA9', zorder=3)
    ax_price.plot(df_chunk['candle_date_time_kst'], df_chunk['ema21'], color='#1565c0', lw=1.3, label='EMA21', zorder=3)
    ax_price.plot(df_chunk['candle_date_time_kst'], df_chunk['ema50'], color='#2ca02c', lw=1.3, label='EMA50', zorder=3)
    # y축 가격 범위 = 청크 내 최저/최고 (캔들 패치는 자동확장 안 됨)
    ax_price.set_ylim(df_chunk['low'].min() * 0.998, df_chunk['high'].max() * 1.002)
    ax_price.set_xlim(df_chunk['candle_date_time_kst'].iloc[0] - pd.Timedelta(minutes=15),
                      df_chunk['candle_date_time_kst'].iloc[-1] + pd.Timedelta(minutes=15))

    buy_added = sell_win = sell_loss = False
    for t in chunk_trades:
        buy_dt = _parse_time(t['buy_date'], t.get('buy_time', ''))
        buy_price = t['buy_price']
        ax_price.scatter([buy_dt], [buy_price], marker='^', s=180,
                         color='#0a8f0a', edgecolor='black', linewidth=0.9,
                         zorder=10, label=('매수' if not buy_added else None))
        buy_added = True
        if t.get('sell_date'):
            sell_dt = _parse_time(t['sell_date'], t.get('sell_time', ''))
            sell_price = t['buy_price'] * (1 + t['net_return_pct'] / 100)
            is_win = t['net_return_pct'] > 0
            color = '#1565c0' if is_win else '#c62828'
            label = None
            if is_win and not sell_win:
                label = '매도 (수익)'; sell_win = True
            elif not is_win and not sell_loss:
                label = '매도 (손실)'; sell_loss = True
            ax_price.scatter([sell_dt], [sell_price], marker='v', s=180,
                             color=color, edgecolor='black', linewidth=0.9,
                             zorder=10, label=label)
            ax_price.plot([buy_dt, sell_dt], [buy_price, sell_price],
                          color=color, alpha=0.5, lw=1.0, linestyle='--', zorder=9)

    # 거래량 (양봉 빨강·음봉 파랑)
    for _, row in df_chunk.iterrows():
        color = '#d62728' if row['close'] >= row['open'] else '#1f77b4'
        ax_vol.bar(row['candle_date_time_kst'], row['volume'],
                   width=pd.Timedelta(minutes=12), color=color, alpha=0.6)
    ax_vol.set_ylabel('Volume')

    if chunk_trades:
        wins = sum(1 for t in chunk_trades if t['net_return_pct'] > 0)
        wr = wins / len(chunk_trades) * 100
        avg = sum(t['net_return_pct'] for t in chunk_trades) / len(chunk_trades)
        stats = f' 거래 {len(chunk_trades)} (승률 {wr:.0f}%, 평균 {avg:+.2f}%)'
    else:
        stats = ' 거래 0'
    chunk_label = f' [{chunk_idx}/{total_chunks}]' if chunk_idx else ''
    period = f'{df_chunk["candle_date_time_kst"].iloc[0].strftime("%Y-%m-%d %H:%M")} ~ {df_chunk["candle_date_time_kst"].iloc[-1].strftime("%Y-%m-%d %H:%M")}'
    strat_name = r.get('strategy', 'Strategy')
    ax_price.set_title(
        f'{strat_name} × {market} (15m){chunk_label}\n'
        f'{period}{stats}',
        fontsize=11,
    )
    ax_price.set_ylabel('가격 (KRW)')
    ax_price.legend(loc='upper left', fontsize=9, ncol=4)
    ax_price.grid(alpha=0.3)
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
    if not args:
        print(__doc__)
        sys.exit(1)
    chunk_mode = False
    if args[0] == '--chunk':
        chunk_mode = True
        args = args[1:]
    trades_json = args[0]
    market = args[1] if len(args) > 1 else 'KRW-ETH'
    if chunk_mode:
        out_dir = args[2] if len(args) > 2 else '/tmp'
        days = int(args[3]) if len(args) > 3 else 21
        chunk_bars = int(args[4]) if len(args) > 4 else 180
    else:
        out_png = args[2] if len(args) > 2 else '/tmp/t03_chart.png'
        days = int(args[3]) if len(args) > 3 else 21

    with open(trades_json) as f:
        data = json.load(f)
    results = [r for r in data['results'] if r['market'] == market]
    if not results:
        print(f'No results for market {market}')
        sys.exit(1)
    r = results[0]
    trades = r['trades']
    if not trades:
        print('No trades')
        sys.exit(1)

    # 최근 N일 윈도우 (최근 거래 기준)
    latest_buy = max(t['buy_date'] for t in trades)
    end = pd.to_datetime(latest_buy, format='%Y%m%d')
    start = end - pd.Timedelta(days=days)
    date_from = start.strftime('%Y%m%d')
    date_to = end.strftime('%Y%m%d')
    print(f'Window: {date_from} ~ {date_to} ({days}일)')

    # 윈도우 내 거래 필터
    window_trades = [t for t in trades if date_from <= t['buy_date'] <= date_to]
    print(f'Window trades: {len(window_trades)} (총 {len(trades)} 중)')

    df = fetch_candles(market, date_from, date_to)
    print(f'Candles: {len(df)}')
    if len(df) < 60:
        print('캔들 부족 — 윈도우 확장')
        sys.exit(1)

    # EMA 계산
    df['ema9'] = df['close'].ewm(span=9, adjust=False).mean()
    df['ema21'] = df['close'].ewm(span=21, adjust=False).mean()
    df['ema50'] = df['close'].ewm(span=50, adjust=False).mean()

    # 청크 모드: chunk_bars 봉씩 잘라서 거래 있는 청크만 차트 생성
    if chunk_mode:
        chunks = []
        n = len(df)
        total = (n + chunk_bars - 1) // chunk_bars
        for idx, start in enumerate(range(0, n, chunk_bars), 1):
            sub = df.iloc[start:start+chunk_bars]
            if len(sub) < 10:
                continue
            t0 = sub['candle_date_time_kst'].iloc[0]
            t1 = sub['candle_date_time_kst'].iloc[-1]
            d0 = t0.strftime('%Y%m%d')
            d1 = t1.strftime('%Y%m%d')
            ctrades = [tr for tr in window_trades if d0 <= tr['buy_date'] <= d1]
            if not ctrades:
                continue
            png = f'{out_dir}/t03_{market.replace("-","_")}_chunk_{idx:02d}.png'
            render_chart(sub, ctrades, r, market, png, chunk_idx=idx, total_chunks=total)
            chunks.append(png)
            print(f'  청크 {idx}/{total}: {len(ctrades)} 거래 → {png}')
        print(f'총 청크 PNG: {len(chunks)}개')
        # paths를 stdout 마지막에 한 줄로 출력 (호출자가 파싱 가능)
        print('CHUNKS=' + ','.join(chunks))
        return

    # 단일 차트 (기존 동작)
    fig, (ax_price, ax_vol) = plt.subplots(
        2, 1, figsize=(16, 9), sharex=True,
        gridspec_kw={'height_ratios': [4, 1]},
    )

    # 가격
    ax_price.plot(df['candle_date_time_kst'], df['close'], color='black', lw=0.6, label='Close', alpha=0.7)
    ax_price.plot(df['candle_date_time_kst'], df['ema9'], color='#FFA500', lw=1.2, label='EMA9')
    ax_price.plot(df['candle_date_time_kst'], df['ema21'], color='#1f77b4', lw=1.2, label='EMA21')
    ax_price.plot(df['candle_date_time_kst'], df['ema50'], color='#d62728', lw=1.2, label='EMA50')

    # 매수/매도 마커 — 정확한 가격으로 표시 (Buy: 청산 위 ^, Sell: 청산 아래 v)
    buy_added = sell_win = sell_loss = False
    for t in window_trades:
        buy_dt = pd.to_datetime(t['buy_date'], format='%Y%m%d') + pd.Timedelta(hours=12)
        buy_price = t['buy_price']
        ax_price.scatter([buy_dt], [buy_price], marker='^', s=120,
                         color='green', edgecolor='black', linewidth=0.7,
                         zorder=5, label=('매수' if not buy_added else None))
        buy_added = True
        if t['sell_date']:
            sell_dt = pd.to_datetime(t['sell_date'], format='%Y%m%d') + pd.Timedelta(hours=12)
            sell_price = t['buy_price'] * (1 + t['net_return_pct'] / 100)
            is_win = t['net_return_pct'] > 0
            color = '#1565c0' if is_win else '#c62828'
            label = None
            if is_win and not sell_win:
                label = '매도 (수익)'; sell_win = True
            elif not is_win and not sell_loss:
                label = '매도 (손실)'; sell_loss = True
            ax_price.scatter([sell_dt], [sell_price], marker='v', s=120,
                             color=color, edgecolor='black', linewidth=0.7,
                             zorder=5, label=label)
            # 매수-매도 연결선
            ax_price.plot([buy_dt, sell_dt], [buy_price, sell_price],
                          color=color, alpha=0.4, lw=0.8, linestyle='--')

    # 거래량
    ax_vol.bar(df['candle_date_time_kst'], df['volume'], width=0.005, color='#888', alpha=0.7)
    ax_vol.set_ylabel('Volume')

    # 통계 박스 (윈도우 내 거래 + 전체 거래)
    win_trades = [t for t in window_trades if t['net_return_pct'] > 0]
    win_rate = len(win_trades) / len(window_trades) * 100 if window_trades else 0
    avg_ret = sum(t['net_return_pct'] for t in window_trades) / len(window_trades) if window_trades else 0
    title_window = f'{date_from[:4]}-{date_from[4:6]}-{date_from[6:8]} ~ {date_to[:4]}-{date_to[4:6]}-{date_to[6:8]}'
    ax_price.set_title(
        f'T03_EMAPullback × {market} (15m) — {title_window}\n'
        f'윈도우 거래: {len(window_trades)}건 (승률 {win_rate:.0f}% / 평균 {avg_ret:+.2f}%)  '
        f'|  전체(IS): {r["trade_count"]}건 (승률 {r["win_rate"]:.1f}% / avgNet {r["avg_net_return_pct"]:+.3f}% / PF {r["profit_factor"]:.2f})',
        fontsize=11,
    )
    ax_price.set_ylabel('가격 (KRW)')
    ax_price.legend(loc='upper left', fontsize=9, ncol=4)
    ax_price.grid(alpha=0.3)
    ax_vol.grid(alpha=0.3)
    ax_vol.set_xlabel('시각 (KST)')

    # x-axis 포맷
    ax_vol.xaxis.set_major_locator(mdates.AutoDateLocator())
    ax_vol.xaxis.set_major_formatter(mdates.DateFormatter('%m-%d'))
    plt.setp(ax_vol.xaxis.get_majorticklabels(), rotation=0)

    plt.tight_layout()
    os.makedirs(os.path.dirname(out_png) or '.', exist_ok=True)
    plt.savefig(out_png, dpi=120, bbox_inches='tight')
    print(f'저장: {out_png}')


if __name__ == '__main__':
    main()
