"""
000020 (동화약품) 180일 Box 분석 차트 생성
- DB에서 데이터 조회
- MA 계산 (차트 표시용)
- Box/DefBox 분석: Go boxcalc 바이너리 위임 (단일 소스: RESTGo box/·stg/)
- 캔들스틱 차트에 Box 표시
- Telegram으로 이미지 전송
"""
import sys
import json
import subprocess
import pandas as pd
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.font_manager as fm
from matplotlib.lines import Line2D

# 한글 폰트 설정 (NanumGothic 자동 탐색)
import glob as _glob, os as _os
_nanum_candidates = _glob.glob('/home/node/.local/lib/python3.12/site-packages/koreanize_matplotlib/fonts/NanumGothic.ttf') + \
                    _glob.glob('/usr/share/fonts/**/NanumGothic.ttf', recursive=True)
if _nanum_candidates:
    fm.fontManager.addfont(_nanum_candidates[0])
plt.rcParams['font.family'] = 'NanumGothic'
plt.rcParams['axes.unicode_minus'] = False
import numpy as np

# 프로젝트 경로 (자동 감지: py/analysis/ 기준 2단계 상위)
import os as _os
_PROJECT_ROOT = _os.path.dirname(_os.path.dirname(_os.path.dirname(_os.path.abspath(__file__))))
sys.path.insert(0, _PROJECT_ROOT)
from py.common.db import open_connection

# ─────────────────────────────────────────────
# 1. DB 조회
# ─────────────────────────────────────────────

def fetch_candles(shcode: str, days: int = 210) -> pd.DataFrame:
    """BP_PeriodPrice에서 일봉 데이터 조회 (days일치, 앞에 여유분 포함)"""
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
# 2. 지표 계산 (CandleProcessor.cs 포팅)
# ─────────────────────────────────────────────

def calc_ma(close: np.ndarray, period: int) -> np.ndarray:
    """단순 이동평균"""
    ma = np.zeros(len(close))
    for i in range(len(close)):
        if i >= period - 1:
            ma[i] = close[i - period + 1:i + 1].mean()
    return ma


def prepare_candles(df: pd.DataFrame) -> list:
    """DataFrame을 캔들 딕셔너리 리스트로 변환 (스케일 없이 원본 가격 사용)"""
    close = df['close'].values
    high = df['high'].values
    low = df['low'].values
    open_ = df['open'].values

    ma5 = calc_ma(close, 5)
    ma20 = calc_ma(close, 20)
    ma60 = calc_ma(close, 60)

    candles = []
    for i in range(len(df)):
        candles.append({
            'date': df['date'].iloc[i],
            'open': open_[i],
            'high': high[i],
            'low': low[i],
            'close': close[i],
            'volume': df['volume'].iloc[i],
            'ma5': ma5[i],
            'ma20': ma20[i],
            'ma60': ma60[i],
        })
    return candles


# ─────────────────────────────────────────────
# 3. Box 분석 (curvature.go 로직 포팅)
# ─────────────────────────────────────────────

KIND_BOX = 0
KIND_MAIN = 1
KIND_DEF = 2

BOXTYPE_SUPPORT = 0    # 지지선 (저점)
BOXTYPE_RESIST  = 1    # 저항선 (고점)


def run_box_analysis(candles, damage_limit=0) -> list:
    """Box 분석 — Go boxcalc 바이너리에 위임한다.

    2026-07-30: Python 자체 구현(curvature/DefBox 포팅)을 폐기하고 Go box/·stg/를
    단일 소스로 통일했다. boxcalc는 RESTGo 저장소에서 정적 빌드된다:
        CGO_ENABLED=0 go build -o boxcalc ./cmd/boxcalc   (또는 deploy/build_boxcalc.sh)
    경로는 RESTGO_BOXCALC 환경변수 또는 프로젝트 루트의 ./boxcalc.

    damage_limit 인수는 하위 호환용으로만 남겨 두었다 — boxcalc는 임베드된
    strategy1 settings(DamOption)를 사용하므로 0이 아닌 값은 무시된다.
    """
    if damage_limit != 0:
        print(f'경고: damage_limit={damage_limit}은 무시됩니다 (boxcalc는 strategy1 settings 사용)')

    boxcalc = _os.environ.get('RESTGO_BOXCALC') or _os.path.join(_PROJECT_ROOT, 'boxcalc')
    if not _os.path.isfile(boxcalc):
        raise FileNotFoundError(
            f'boxcalc 바이너리가 없습니다: {boxcalc}\n'
            f'빌드: cd {_PROJECT_ROOT} && CGO_ENABLED=0 go build -o boxcalc ./cmd/boxcalc')

    payload = {
        'candles': [
            {
                'date': str(c['date']),
                'open': float(c['open']),
                'high': float(c['high']),
                'low': float(c['low']),
                'close': float(c['close']),
                'volume': float(c['volume']),
            }
            for c in candles
        ]
    }
    proc = subprocess.run(
        [boxcalc], input=json.dumps(payload).encode(),
        capture_output=True, timeout=60)
    if proc.returncode != 0:
        raise RuntimeError(f'boxcalc 실패: {proc.stderr.decode(errors="replace")}')
    return json.loads(proc.stdout)['boxes']

# ─────────────────────────────────────────────
# 4. 차트 그리기
# ─────────────────────────────────────────────

def draw_chart(df: pd.DataFrame, candles: list, box_list: list,
               shcode: str, name: str, display_days: int = 180) -> str:
    """캔들 차트 + Box 표시"""
    # 표시 구간: 마지막 display_days개
    n = len(candles)
    start_i = max(0, n - display_days)
    display_candles = candles[start_i:]
    display_df = df.iloc[start_i:].reset_index(drop=True)

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(18, 10),
                                   gridspec_kw={'height_ratios': [3, 1]},
                                   facecolor='#1a1a2e')
    ax1.set_facecolor('#1a1a2e')
    ax2.set_facecolor('#1a1a2e')

    x_range = range(len(display_candles))

    # ── 캔들스틱 ──
    for xi, (c, row) in enumerate(zip(display_candles, display_df.itertuples())):
        color = '#e74c3c' if c['close'] >= c['open'] else '#3498db'
        # 몸통
        body_low = min(c['open'], c['close'])
        body_high = max(c['open'], c['close'])
        ax1.bar(xi, body_high - body_low, bottom=body_low, color=color, width=0.6, linewidth=0)
        # 꼬리
        ax1.plot([xi, xi], [c['low'], body_low], color=color, linewidth=0.8)
        ax1.plot([xi, xi], [body_high, c['high']], color=color, linewidth=0.8)

    # ── 이동평균선 ──
    ma5_vals  = [c['ma5']  for c in display_candles]
    ma20_vals = [c['ma20'] for c in display_candles]
    ma60_vals = [c['ma60'] for c in display_candles]
    ax1.plot(x_range, ma5_vals,  color='#f1c40f', linewidth=0.8, label='MA5',  alpha=0.9)
    ax1.plot(x_range, ma20_vals, color='#2ecc71', linewidth=0.8, label='MA20', alpha=0.9)
    ax1.plot(x_range, ma60_vals, color='#e67e22', linewidth=0.8, label='MA60', alpha=0.9)

    # ── Box 표시 ──
    price_min = min(c['low'] for c in display_candles) * 0.98
    price_max = max(c['high'] for c in display_candles) * 1.02

    added_labels = set()

    for box in box_list:
        box_xi = box['pos'] - start_i  # 표시 구간 기준 x좌표
        if box_xi < 0:
            box_xi = 0  # 왼쪽 경계로 클리핑

        box_price = box['price']
        if box_price < price_min or box_price > price_max:
            continue

        if box['kind'] == KIND_DEF:
            color = '#ff6b6b'
            lw = 1.8
            ls = '-'
            label = 'DefBox'
            zorder = 5
        elif box['kind'] == KIND_MAIN:
            color = '#ffd93d'
            lw = 1.4
            ls = '--'
            label = 'MainBox'
            zorder = 4
        else:
            color = '#6bcb77' if box['boxtype'] == BOXTYPE_SUPPORT else '#4d96ff'
            lw = 1.0
            ls = ':'
            label = 'SupportBox' if box['boxtype'] == BOXTYPE_SUPPORT else 'ResistBox'
            zorder = 3

        # 수평선 (box_xi 이후 끝까지)
        ax1.hlines(y=box_price, xmin=box_xi, xmax=len(display_candles) - 1,
                   colors=color, linewidths=lw, linestyles=ls, zorder=zorder,
                   label=label if label not in added_labels else '')
        added_labels.add(label)

        # 박스 위치에 마커
        if 0 <= box_xi < len(display_candles):
            marker = 'v' if box['boxtype'] == BOXTYPE_RESIST else '^'
            ax1.scatter(box_xi, box_price, color=color, s=40, zorder=zorder+1, marker=marker)

        # DefBox는 가격 라벨 표시
        if box['kind'] == KIND_DEF:
            ax1.text(len(display_candles) - 1, box_price,
                     f' {box_price:,.0f}', color=color, fontsize=7,
                     va='center', ha='left', zorder=6)

    # ── 거래량 ──
    for xi, c in enumerate(display_candles):
        color = '#e74c3c' if c['close'] >= c['open'] else '#3498db'
        ax2.bar(xi, c['volume'], color=color, width=0.6, alpha=0.7, linewidth=0)

    # ── 축 스타일 ──
    ax1.set_xlim(-1, len(display_candles))
    ax1.set_ylim(price_min, price_max)
    ax1.yaxis.set_major_formatter(plt.FuncFormatter(lambda x, _: f'{x:,.0f}'))

    # X축 날짜 라벨 (20일 간격)
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

    total_def = box_counts[KIND_DEF]
    total_main = box_counts[KIND_MAIN]
    total_box = box_counts[KIND_BOX]

    ax1.set_title(
        f'{shcode} {name}  |  최근 {display_days}일  |  '
        f'Box:{total_box}  MainBox:{total_main}  DefBox:{total_def}',
        color='white', fontsize=12, fontweight='bold', pad=10
    )

    legend_elements = [
        Line2D([0], [0], color='#f1c40f', lw=1, label='MA5'),
        Line2D([0], [0], color='#2ecc71', lw=1, label='MA20'),
        Line2D([0], [0], color='#e67e22', lw=1, label='MA60'),
        Line2D([0], [0], color='#4d96ff', lw=1.2, ls=':', label='ResistBox'),
        Line2D([0], [0], color='#6bcb77', lw=1.2, ls=':', label='SupportBox'),
        Line2D([0], [0], color='#ffd93d', lw=1.4, ls='--', label='MainBox'),
        Line2D([0], [0], color='#ff6b6b', lw=1.8, ls='-', label='DefBox'),
    ]
    ax1.legend(handles=legend_elements, loc='upper left', fontsize=7,
               facecolor='#1a1a2e', edgecolor='#333366', labelcolor='white')

    ax2.set_ylabel('Volume', color='#aaaaaa', fontsize=8)
    ax1.set_ylabel('Price (KRW)', color='#aaaaaa', fontsize=8)
    ax1.grid(axis='y', color='#2a2a4e', linewidth=0.5, alpha=0.5)
    ax2.grid(axis='y', color='#2a2a4e', linewidth=0.5, alpha=0.5)

    plt.tight_layout(pad=1.5)

    out_path = _os.path.join(_PROJECT_ROOT, 'zpicture', f'box_{shcode}.png')
    plt.savefig(out_path, dpi=130, bbox_inches='tight', facecolor='#1a1a2e')
    plt.close()
    return out_path


# ─────────────────────────────────────────────
# 5. 메인
# ─────────────────────────────────────────────

def main():
    shcode = sys.argv[1] if len(sys.argv) > 1 else '000020'
    name = sys.argv[2] if len(sys.argv) > 2 else shcode
    print(f'[1] {shcode} 데이터 조회 중...')
    df = fetch_candles(shcode, days=250)  # 여유분 포함해서 250개 조회
    print(f'    조회된 캔들: {len(df)}개')

    print('[2] 지표 계산...')
    candles = prepare_candles(df)

    print('[3] Box 분석...')
    box_list = run_box_analysis(candles)

    # 마지막 180일만 표시
    display_days = 180

    # 요약
    kinds = {KIND_BOX: 0, KIND_MAIN: 0, KIND_DEF: 0}
    for b in box_list:
        kinds[b['kind']] = kinds.get(b['kind'], 0) + 1
    print(f'    전체 Box: {kinds[KIND_BOX]}, MainBox: {kinds[KIND_MAIN]}, DefBox: {kinds[KIND_DEF]}')

    # 최근 DefBox/MainBox 정보
    def_boxes = [b for b in box_list if b['kind'] == KIND_DEF]
    if def_boxes:
        last_def = def_boxes[-1]
        print(f'    최신 DefBox: {last_def["date"]} @ {last_def["price"]:,.0f}원')

    print('[4] 차트 생성...')
    img_path = draw_chart(df, candles, box_list, shcode, name, display_days)
    print(f'    저장: {img_path}')

    print(f'완료! 이미지 경로: {img_path}')
    # 요약 출력
    lines = [f'{shcode} {name} Box 분석 (최근 {display_days}일)']
    lines.append(f'일반Box: {kinds[KIND_BOX]}개  MainBox: {kinds[KIND_MAIN]}개  DefBox: {kinds[KIND_DEF]}개  합계: {len(box_list)}')
    print('\n'.join(lines))

    # 전체 박스 목록 (비교용)
    kind_label = {KIND_BOX: 'Box    ', KIND_MAIN: 'MainBox', KIND_DEF: 'DefBox '}
    type_label = {BOXTYPE_SUPPORT: 'Support', BOXTYPE_RESIST: 'Resist '}
    print('\n[전체 Box 목록]')
    print(f"  {'Idx':<4}  {'Date':<10}  {'Pos':<5}  {'원본가격':<12}  {'Kind':<9}  Type")
    print('  ' + '-' * 60)
    for i, b in enumerate(box_list):
        k = kind_label.get(b['kind'], 'Unknown')
        t = type_label.get(b['boxtype'], 'Unknown')
        print(f"  {i:<4}  {b['date']:<10}  {b['pos']:<5}  {b['price']:<12.0f}  {k:<9}  {t}")


if __name__ == '__main__':
    main()
