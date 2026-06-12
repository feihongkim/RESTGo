"""
시총 상위 10개 종목 + 주요 지수 Box 차트 일괄 생성 및 Telegram 전송
"""
import sys
import os
import time
import pandas as pd
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.lines import Line2D
import numpy as np

import os as _os
_PROJECT_ROOT = _os.path.dirname(_os.path.dirname(_os.path.dirname(_os.path.abspath(__file__))))
sys.path.insert(0, _PROJECT_ROOT)
from py.common.db import open_connection
from py.analysis.box_chart import (
    prepare_candles, run_box_analysis, draw_chart,
    KIND_BOX, KIND_MAIN, KIND_DEF
)

import matplotlib
import matplotlib.font_manager as fm
plt.rcParams['font.family'] = 'NanumGothic'
plt.rcParams['axes.unicode_minus'] = False

# ─────────────────────────────────────────────
# 대상 목록 정의
# ─────────────────────────────────────────────

# 시총 상위 10 (위에서 확인한 결과)
TOP10_STOCKS = [
    ('005930', '삼성전자'),
    ('000660', 'SK하이닉스'),
    ('402340', 'SK스퀘어'),
    ('005380', '현대차'),
    ('009150', '삼성전기'),
    ('373220', 'LG에너지솔루션'),
    ('032830', '삼성생명'),
    ('028260', '삼성물산'),
    ('329180', 'HD현대중공업'),
    ('012330', '현대모비스'),
]

# 주요 지수 (SC.Sector_DailyIndex 코드)
MAJOR_INDICES = [
    ('0001', 'KOSPI'),
    ('1001', 'KOSDAQ'),
    ('2001', 'KOSPI200'),
]


# ─────────────────────────────────────────────
# 데이터 조회
# ─────────────────────────────────────────────

def fetch_stock_candles(shcode: str, days: int = 250) -> pd.DataFrame:
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
    return df.sort_values('date').reset_index(drop=True)


def fetch_index_candles(idx_code: str, days: int = 250) -> pd.DataFrame:
    sql = f"""
        SELECT TOP {days}
            stck_bsop_date AS date,
            CAST(bstp_nmix_oprc AS FLOAT) AS oprc,
            CAST(bstp_nmix_hgpr AS FLOAT) AS hgpr,
            CAST(bstp_nmix_lwpr AS FLOAT) AS lwpr,
            CAST(bstp_nmix_prpr AS FLOAT) AS clpr,
            CAST(acml_vol AS FLOAT) AS volume
        FROM SC.Sector_DailyIndex
        WHERE bstp_cls_code = '{idx_code}'
        ORDER BY stck_bsop_date DESC
    """
    with open_connection(server='white', database='KIS2') as conn:
        df = pd.read_sql(sql, conn)
    df = df.rename(columns={'clpr': 'close', 'oprc': 'open', 'hgpr': 'high', 'lwpr': 'low'})
    df = df[df['close'] > 0].copy()  # 0값 제거
    return df.sort_values('date').reset_index(drop=True)


# ─────────────────────────────────────────────
# 단일 종목/지수 처리
# ─────────────────────────────────────────────

def process_target(code: str, name: str, is_index: bool = False, display_days: int = 180) -> dict:
    """한 종목/지수 처리 → 차트 이미지 경로 + 요약 반환"""
    try:
        if is_index:
            df = fetch_index_candles(code)
        else:
            df = fetch_stock_candles(code)

        if len(df) < 10:
            return {'code': code, 'name': name, 'error': '데이터 부족'}

        candles = prepare_candles(df)
        box_list = run_box_analysis(candles)

        img_path = _os.path.join(_PROJECT_ROOT, 'zpicture', f'box_{code}.png')
        draw_chart(df, candles, box_list, code, name, display_days)

        kinds = {KIND_BOX: 0, KIND_MAIN: 0, KIND_DEF: 0}
        for b in box_list:
            kinds[b['kind']] = kinds.get(b['kind'], 0) + 1

        def_boxes = [b for b in box_list if b['kind'] == KIND_DEF]
        latest_def = def_boxes[-1] if def_boxes else None

        return {
            'code': code,
            'name': name,
            'img_path': img_path,
            'n_box': kinds[KIND_BOX],
            'n_main': kinds[KIND_MAIN],
            'n_def': kinds[KIND_DEF],
            'latest_def': latest_def,
            'error': None,
        }
    except Exception as e:
        return {'code': code, 'name': name, 'error': str(e)}


# ─────────────────────────────────────────────
# Telegram 전송 (MCP plugin 사용 - 파일 경로 출력)
# ─────────────────────────────────────────────

def print_result(result: dict, rank: int = 0):
    """결과 요약 출력"""
    if result.get('error'):
        print(f'[ERROR] {result["code"]} {result["name"]}: {result["error"]}')
        return

    latest_def = result.get('latest_def')
    def_str = f'{latest_def["date"]} @ {latest_def["price"]:,.0f}' if latest_def else '없음'
    rank_str = f'#{rank} ' if rank else ''
    print(f'[OK] {rank_str}{result["code"]} {result["name"]} | '
          f'Box:{result["n_box"]} Main:{result["n_main"]} Def:{result["n_def"]} | '
          f'최신DefBox:{def_str} | {result["img_path"]}')


# ─────────────────────────────────────────────
# 메인
# ─────────────────────────────────────────────

def main():
    print('=' * 60)
    print('시총 상위 10 + 주요 지수 Box 분석 시작')
    print('=' * 60)

    all_results = []

    print('\n[1] 주요 지수 처리...')
    for code, name in MAJOR_INDICES:
        print(f'  → {code} {name}')
        r = process_target(code, name, is_index=True)
        all_results.append(('index', r))
        print_result(r)

    print('\n[2] 시총 상위 10 처리...')
    for i, (code, name) in enumerate(TOP10_STOCKS, 1):
        print(f'  → #{i} {code} {name}')
        r = process_target(code, name, is_index=False)
        all_results.append(('stock', i, r))
        print_result(r, rank=i)

    # 결과 경로 리스트 출력 (Claude가 Telegram으로 전송)
    print('\n' + '=' * 60)
    print('생성된 이미지 경로 목록:')
    for item in all_results:
        r = item[-1]
        if not r.get('error') and r.get('img_path'):
            print(r['img_path'])

    print('\n분석 완료!')
    return all_results


if __name__ == '__main__':
    results = main()
