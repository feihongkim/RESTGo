"""
W바텀 (Bollinger Band Method III) 예시 차트 생성
나라별 2개 종목을 찾아 Telegram으로 전송
"""
import sys
import os
import random
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.patches as mpatches
from matplotlib.lines import Line2D
import glob as _glob
import pymssql
import yaml, base64
from contextlib import contextmanager
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

# ── 폰트 설정 ─────────────────────────────────────────────────────────────────
_NANUM_PATH = '/home/node/.local/lib/python3.12/site-packages/koreanize_matplotlib/fonts/NanumGothic.ttf'
if not os.path.exists(_NANUM_PATH):
    _candidates = _glob.glob('/home/feihong/code/**/NanumGothic.ttf', recursive=True) + \
                  _glob.glob('/usr/share/fonts/**/NanumGothic.ttf', recursive=True)
    _NANUM_PATH = _candidates[0] if _candidates else None

if _NANUM_PATH:
    from matplotlib import font_manager as _fm
    _fm.fontManager.addfont(_NANUM_PATH)
    _prop = _fm.FontProperties(fname=_NANUM_PATH)
    plt.rcParams['font.family'] = _prop.get_name()

plt.rcParams['axes.unicode_minus'] = False

# ── DB 연결 ───────────────────────────────────────────────────────────────────
def _get_key(fkey):
    return base64.b64decode(fkey[:-1] + "=")

def _decrypt(enc, key):
    ct = base64.b64decode(enc)
    nonce, body = ct[:12], ct[12:]
    plain = AESGCM(key).decrypt(nonce, body, None).decode()
    p1, p2, p3 = plain[:1], plain[6:7], plain[14:]
    s = p1 + p2 + p3
    return s[:-1] if s.endswith("_") else s

def _load_cfg():
    for p in ["/workspace/config.yaml",
              "/home/feihong/code/REST/RESTGo/config.yaml",
              "/home/feihong/code/MakeSQL/config.yaml"]:
        if os.path.exists(p):
            with open(p) as f:
                return yaml.safe_load(f)

@contextmanager
def open_db(database="KIS2", server="white"):
    cfg = _load_cfg()
    key = _get_key(cfg["FKEY"])
    addr_key = f"MSSQL_ADDR_{server}"
    host = _decrypt(cfg.get(addr_key, cfg["MSSQL_ADDR"]), key)
    port = int(_decrypt(cfg["MSSQL_PORT"], key))
    user = _decrypt(cfg["MSSQL_USER"], key)
    pw   = _decrypt(cfg["MSSQL_PASSWORD"], key)
    conn = pymssql.connect(server=host, port=port, user=user, password=pw,
                           database=database, charset="utf8")
    try:
        yield conn
    finally:
        conn.close()

# ── Bollinger Band 계산 ──────────────────────────────────────────────────────
def calc_bb(closes, period=20, mult=2.0):
    closes = np.array(closes, dtype=float)
    n = len(closes)
    upper = np.full(n, np.nan)
    lower = np.full(n, np.nan)
    mid   = np.full(n, np.nan)
    pct_b = np.full(n, np.nan)
    width = np.full(n, np.nan)
    for i in range(period - 1, n):
        window = closes[i - period + 1: i + 1]
        m = window.mean()
        s = window.std(ddof=0)
        u = m + mult * s
        lo = m - mult * s
        mid[i]   = m
        upper[i] = u
        lower[i] = lo
        width[i] = (u - lo) / m * 100 if m > 0 else np.nan
        band_range = u - lo
        pct_b[i]  = (closes[i] - lo) / band_range if band_range > 0 else 0.5
    return upper, mid, lower, pct_b, width

# ── W바텀 패턴 탐지 (Box 기반 — Go IsBBWBottomBoxPattern 동일 로직) ─────────
def find_wbottom_patterns_box(sup_boxes, res_boxes, lows, bb_lower, closes=None):
    """
    하단Box(BB이탈) → 상단Box → 하단Box 연속 시퀀스를 순방향으로 탐색.
    P1 이후 '바로 다음' 상단Box, 그 이후 '바로 다음' 하단Box 순간 = 매수.
    Returns list of (p1_box_pos, p2_box_pos, entry_curve_pos) — 발생 순서.
    """
    def p1_sufficient_breach(bp):
        """P1 조건: bp 직전 10봉 중 종가 <= BB하단 봉이 5개 이상"""
        if closes is None:
            return True  # closes 없으면 pass-through
        count = 0
        for i in range(max(0, bp - 10), bp):
            if not np.isnan(bb_lower[i]) and closes[i] <= bb_lower[i]:
                count += 1
        return count >= 5

    def p2_no_breach(bp, cp):
        """P2 조건: 저가 기준으로 BB하단 이탈 없어야 함"""
        for i in range(max(0, bp - 2), min(len(lows), cp + 3)):
            if not np.isnan(bb_lower[i]) and lows[i] <= bb_lower[i]:
                return False
        return True

    # box_pos 기준 정렬
    all_boxes = sorted(
        [('S', bp, cp) for bp, cp in sup_boxes] +
        [('R', bp, cp) for bp, cp in res_boxes],
        key=lambda x: x[1]
    )

    patterns = []
    n = len(all_boxes)
    i = 0
    while i < n - 2:
        # Step 1: P1 — bp 직전 10봉 중 종가<=BB하단 5개 이상인 하단Box
        if all_boxes[i][0] != 'S' or not p1_sufficient_breach(all_boxes[i][1]):
            i += 1
            continue

        # Step 2: P1 바로 다음 Box가 상단Box여야 함 (중간 박스 끼임 불가)
        j = i + 1
        if j >= n or all_boxes[j][0] != 'R':
            i += 1
            continue

        # Step 3: 상단Box 바로 다음 Box가 BB이탈 없는 하단Box여야 함 (중간 박스 끼임 불가)
        k = j + 1
        if k >= n or all_boxes[k][0] != 'S' or not p2_no_breach(all_boxes[k][1], all_boxes[k][2]):
            i += 1
            continue

        _, bp1, _ = all_boxes[i]
        _, bp3, cp3 = all_boxes[k]
        patterns.append((bp1, bp3, cp3))
        i = k + 1

    return patterns

# ── 캔들 데이터 조회 ─────────────────────────────────────────────────────────
def fetch_kr(shcode, days=300):
    with open_db("KIS2", "white") as conn:
        cur = conn.cursor()
        cur.execute(f"""
            SELECT stck_bsop_date, stck_oprc, stck_hgpr, stck_lwpr, stck_clpr,
                   CAST(acml_vol AS FLOAT)
            FROM (
                SELECT TOP {days} stck_bsop_date, stck_oprc, stck_hgpr, stck_lwpr,
                       stck_clpr, acml_vol
                FROM DM.BP_PeriodPrice
                WHERE stck_shrn_iscd = '{shcode}' AND period_type = 'D'
                ORDER BY stck_bsop_date DESC
            ) t ORDER BY stck_bsop_date ASC
        """)
        rows = cur.fetchall()
    return rows

def fetch_kr_hannam(shcode, days=500):
    with open_db("han", "white") as conn:
        cur = conn.cursor()
        cur.execute(f"""
            SELECT DATE, [OPEN], [HIGH], [LOW], [CLOSE], CAST(VOLUME AS FLOAT)
            FROM (
                SELECT TOP {days} DATE, [OPEN], [HIGH], [LOW], [CLOSE], VOLUME
                FROM stock_price_kor_d001
                WHERE SHCODE = '{shcode}' AND STICK_TYPE = 'D001'
                ORDER BY DATE DESC
            ) t ORDER BY DATE ASC
        """)
        return cur.fetchall()

def fetch_foreign(rsym, days=300):
    with open_db("KIS2", "white") as conn:
        cur = conn.cursor()
        cur.execute(f"""
            SELECT xymd,
                   CAST([open] AS VARCHAR(20)),
                   CAST(high  AS VARCHAR(20)),
                   CAST([low] AS VARCHAR(20)),
                   CAST(clos  AS VARCHAR(20)),
                   CAST(tvol  AS FLOAT)
            FROM (
                SELECT TOP {days} xymd, [open], high, [low], clos, tvol
                FROM FG.BP_PeriodPrice
                WHERE rsym = '{rsym}'
                ORDER BY xymd DESC
            ) t ORDER BY xymd ASC
        """)
        rows = cur.fetchall()
    result = []
    for r in rows:
        try:
            o = float(r[1]) if r[1] else 0
            h = float(r[2]) if r[2] else 0
            lo = float(r[3]) if r[3] else 0
            c = float(r[4]) if r[4] else 0
            v = float(r[5]) if r[5] else 0
            result.append((r[0], o, h, lo, c, v))
        except:
            pass
    return result

def get_kr_stocks(n=200):
    with open_db("han", "white") as conn:
        cur = conn.cursor()
        cur.execute("""
            SELECT DISTINCT SHCODE FROM stock_price_kor_d001
            WHERE STICK_TYPE='D001' AND DATE > '20240101'
            ORDER BY SHCODE
        """)
        all_codes = [r[0] for r in cur.fetchall()]
    random.shuffle(all_codes)
    return all_codes[:n]

def get_kr_stocks_kis2(n=500):
    with open_db("KIS2", "white") as conn:
        cur = conn.cursor()
        cur.execute("""
            SELECT DISTINCT stck_shrn_iscd FROM DM.BP_PeriodPrice
            WHERE period_type = 'D' AND stck_bsop_date > '20240101'
            ORDER BY stck_shrn_iscd
        """)
        all_codes = [r[0] for r in cur.fetchall()]
    random.shuffle(all_codes)
    return all_codes[:n]

def get_foreign_stocks(prefix, n=200):
    with open_db("KIS2", "white") as conn:
        cur = conn.cursor()
        if isinstance(prefix, list):
            cond = " OR ".join(f"LEFT(rsym,3)='{p}'" for p in prefix)
        else:
            cond = f"LEFT(rsym,3)='{prefix}'"
        cur.execute(f"""
            SELECT DISTINCT rsym FROM FG.BP_PeriodPrice
            WHERE ({cond}) AND xymd > '20240101'
            ORDER BY rsym
        """)
        all_codes = [r[0] for r in cur.fetchall()]
    random.shuffle(all_codes)
    return all_codes[:n]

# ── Box 탐지 (Go AnalyzeCurvature 포팅) ──────────────────────────────────────
def _compute_gradient(ma, i):
    if i < 1 or np.isnan(ma[i]) or np.isnan(ma[i-1]) or ma[i] == 0:
        return np.nan
    return ((ma[i] - ma[i-1]) / ma[i]) * 100.0

def _compute_ma(closes, period):
    n = len(closes)
    ma = np.full(n, np.nan)
    for i in range(period - 1, n):
        ma[i] = closes[i - period + 1 : i + 1].mean()
    return ma

def _should_reverse_to_bearish(g, g20, g60, close, ma5, i):
    if i < 2: return False
    g_i   = g[i];   g_i1 = g[i-1];  g_i2 = g[i-2]
    if np.isnan(g_i) or np.isnan(g_i1) or np.isnan(g_i2): return False
    # isAcceleratingDowntrend
    cur_slope  = abs(g_i)  - abs(g_i1)
    prev_slope = abs(g_i1) - abs(g_i2)
    accel = g_i < 0 and ((cur_slope > 0 and g_i1 < 0) or (prev_slope > 0 and g_i1 < 0))
    strong = g_i < -0.17 or g_i1 < -0.17
    if accel and strong:
        return True
    # isShortTermWeaknessWithLongTermUptrend
    if np.isnan(ma5[i]) or np.isnan(ma5[i-1]) or np.isnan(ma5[i-2]): return False
    all_below = close[i-2] < ma5[i-2] and close[i-1] < ma5[i-1] and close[i] < ma5[i]
    trend_rev = g[i-2] >= 0 and g[i-1] >= 0 and g_i < 0
    if np.isnan(g20[i]) or np.isnan(g60[i]): return False
    long_up = g20[i] > g60[i] and g60[i] > 0
    return all_below and trend_rev and long_up

def _should_reverse_to_bullish(g, i):
    if i < 2: return False
    g_i   = g[i];   g_i1 = g[i-1];  g_i2 = g[i-2]
    if np.isnan(g_i) or np.isnan(g_i1) or np.isnan(g_i2): return False
    cur_slope  = abs(g_i)  - abs(g_i1)
    prev_slope = abs(g_i1) - abs(g_i2)
    accel = g_i > 0 and ((cur_slope > 0 and g_i1 > 0) or (prev_slope > 0 and g_i1 > 0))
    strong = g_i > 0.17 or g_i1 > 0.17
    return accel and strong

def find_boxes(opens, highs, lows, closes, ma5):
    """Go AnalyzeCurvature 동일 로직으로 Box 탐지.
    returns (support_list, resistance_list) 각각 (box_pos, curve_pos) 튜플"""
    n = len(closes)
    ma20 = _compute_ma(closes, 20)
    ma60 = _compute_ma(closes, 60)
    g   = np.array([_compute_gradient(ma5, i) for i in range(n)])
    g20 = np.array([_compute_gradient(ma20, i) for i in range(n)])
    g60 = np.array([_compute_gradient(ma60, i) for i in range(n)])

    curvekey  = np.zeros(n, dtype=int)
    if n > 5:
        curvekey[5] = 1 if (not np.isnan(g[5]) and g[5] >= 0) else -1

    support, resistance = [], []  # each: (box_position, curve_position)
    exposition = 0
    boxes = []  # list of dicts {box_pos, curve_pos, kind}

    def _calc_exposition(last_box):
        bp = last_box['box_pos']; cp = last_box['curve_pos']
        return (bp + 1) if (bp + 1 >= cp - 3) else (cp - 2)

    for i in range(6, n):
        pk = curvekey[i-1]
        if pk > 0:
            if _should_reverse_to_bearish(g, g20, g60, closes, ma5, i):
                # Find highest High in [exposition, i)
                best_pos, best_h = exposition, -1e9
                for j in range(exposition, i):
                    if highs[j] > best_h:
                        best_h = highs[j]; best_pos = j
                box = {'box_pos': best_pos, 'curve_pos': i, 'kind': 'resistance'}
                boxes.append(box)
                resistance.append((best_pos, i))
                exposition = _calc_exposition(box)
                curvekey[i] = -1
            else:
                curvekey[i] = pk
        elif pk < 0:
            if _should_reverse_to_bullish(g, i):
                # Find lowest Low in [exposition, i)
                best_pos, best_l = exposition, 1e9
                for j in range(exposition, i):
                    if lows[j] < best_l:
                        best_l = lows[j]; best_pos = j
                box = {'box_pos': best_pos, 'curve_pos': i, 'kind': 'support'}
                boxes.append(box)
                support.append((best_pos, i))
                exposition = _calc_exposition(box)
                curvekey[i] = 1
            else:
                curvekey[i] = pk
        else:
            curvekey[i] = pk

    return support, resistance

def draw_chart(dates, opens, highs, lows, closes,
               upper, mid, lower, pct_b, ma5,
               sup_boxes, res_boxes,
               p1_idx, p2_idx, entry_idx,
               title, outpath):
    # 차트 범위: 패턴 전후 50봉
    s = max(0, p1_idx - 20)
    e = min(len(closes), entry_idx + 30)
    idx = list(range(s, e))

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(12, 7),
                                    gridspec_kw={'height_ratios': [3, 1]})
    fig.patch.set_facecolor('#1e1e1e')
    for ax in [ax1, ax2]:
        ax.set_facecolor('#1e1e1e')
        ax.tick_params(colors='#aaaaaa')
        ax.spines['bottom'].set_color('#444')
        ax.spines['top'].set_color('#444')
        ax.spines['left'].set_color('#444')
        ax.spines['right'].set_color('#444')

    x = list(range(len(idx)))
    local = {v: i for i, v in enumerate(idx)}

    # 캔들스틱
    for i, gi in enumerate(idx):
        o, h, l, c = opens[gi], highs[gi], lows[gi], closes[gi]
        color = '#ef5350' if c >= o else '#26a69a'
        ax1.plot([i, i], [l, h], color=color, linewidth=0.8)
        ax1.bar(i, abs(c - o), bottom=min(o, c), color=color, width=0.6)

    # Bollinger Bands
    ux = [local[i] for i in idx if not np.isnan(upper[i])]
    uv = [upper[i] for i in idx if not np.isnan(upper[i])]
    mx = [local[i] for i in idx if not np.isnan(mid[i])]
    mv = [mid[i] for i in idx if not np.isnan(mid[i])]
    lx = [local[i] for i in idx if not np.isnan(lower[i])]
    lv = [lower[i] for i in idx if not np.isnan(lower[i])]

    ax1.plot(ux, uv, color='#90caf9', linewidth=1.2, label='BB Upper')
    ax1.plot(mx, mv, color='#fff176', linewidth=1.0, linestyle='--', label='BB Mid')
    ax1.plot(lx, lv, color='#80cbc4', linewidth=1.2, label='BB Lower')
    ax1.fill_between(ux, uv, lv, alpha=0.05, color='#90caf9')

    # MA5
    m5x = [local[i] for i in idx if not np.isnan(ma5[i])]
    m5v = [ma5[i] for i in idx if not np.isnan(ma5[i])]
    ax1.plot(m5x, m5v, color='#ff9800', linewidth=1.2, label='MA5')

    # Box 마킹 (Go AnalyzeCurvature 동일 로직)
    for (bp, _cp) in sup_boxes:
        if bp in local:
            xi = local[bp]
            ax1.plot(xi, lows[bp], marker='^', markersize=7,
                     color='#ff5252', markeredgewidth=0, zorder=5)
    for (bp, _cp) in res_boxes:
        if bp in local:
            xi = local[bp]
            ax1.plot(xi, highs[bp], marker='v', markersize=7,
                     color='#64b5f6', markeredgewidth=0, zorder=5)

    # P1, P2, Entry 마킹
    if p1_idx in local:
        xi = local[p1_idx]
        ax1.annotate('P1\n(BB터치)', xy=(xi, lows[p1_idx]),
                     xytext=(xi, lows[p1_idx] * 0.975),
                     arrowprops=dict(arrowstyle='->', color='#ff5252'),
                     color='#ff5252', fontsize=8, ha='center')

    if p2_idx in local:
        xi = local[p2_idx]
        ax1.annotate('P2\n(내부저점)', xy=(xi, lows[p2_idx]),
                     xytext=(xi, lows[p2_idx] * 0.975),
                     arrowprops=dict(arrowstyle='->', color='#ffab40'),
                     color='#ffab40', fontsize=8, ha='center')

    if entry_idx in local:
        xi = local[entry_idx]
        ax1.annotate('진입\n(W감지)', xy=(xi, closes[entry_idx]),
                     xytext=(xi, closes[entry_idx] * 1.025),
                     arrowprops=dict(arrowstyle='->', color='#69f0ae'),
                     color='#69f0ae', fontsize=8, ha='center')

    ax1.set_title(title, color='white', fontsize=11)
    sup_patch = Line2D([0], [0], marker='^', color='w', markerfacecolor='#ff5252',
                       markersize=7, label='하단Box', linestyle='None')
    res_patch = Line2D([0], [0], marker='v', color='w', markerfacecolor='#64b5f6',
                       markersize=7, label='상단Box', linestyle='None')
    handles, labels = ax1.get_legend_handles_labels()
    ax1.legend(handles=handles + [sup_patch, res_patch],
               loc='upper left', fontsize=7, facecolor='#2e2e2e', labelcolor='white')
    ax1.yaxis.label.set_color('#aaaaaa')

    # %B 패널
    pbx = [local[i] for i in idx if not np.isnan(pct_b[i])]
    pbv = [pct_b[i] for i in idx if not np.isnan(pct_b[i])]
    ax2.plot(pbx, pbv, color='#ce93d8', linewidth=1.2, label='%B')
    ax2.axhline(0.5, color='#fff176', linewidth=0.7, linestyle='--')
    ax2.axhline(0.0, color='#80cbc4', linewidth=0.7, linestyle='--')
    ax2.set_ylim(-0.5, 1.5)
    ax2.set_ylabel('%B', color='#aaaaaa', fontsize=8)
    ax2.legend(loc='upper left', fontsize=7, facecolor='#2e2e2e', labelcolor='white')

    # X축 날짜 레이블
    step = max(1, len(idx) // 8)
    ticks = list(range(0, len(idx), step))
    labels = [str(dates[idx[t]]) for t in ticks]
    ax1.set_xticks([])
    ax2.set_xticks(ticks)
    ax2.set_xticklabels(labels, rotation=30, ha='right', fontsize=7, color='#aaaaaa')

    plt.tight_layout()
    plt.savefig(outpath, dpi=120, bbox_inches='tight', facecolor='#1e1e1e')
    plt.close()
    return outpath

# ── 동적 스캔 ────────────────────────────────────────────────────────────────
def scan_for_examples(market, stocks, fetch_fn, n=2, days=300, min_date="20240101"):
    """stocks를 순회하며 W-bottom 패턴 n개를 수집 (min_date 이후 패턴만)"""
    examples = []
    for code in stocks:
        if len(examples) >= n:
            break
        try:
            rows = fetch_fn(code, days)
            if len(rows) < 60:
                continue
            dates  = [str(r[0]) for r in rows]
            opens  = np.array([float(r[1]) for r in rows])
            highs  = np.array([float(r[2]) for r in rows])
            lows   = np.array([float(r[3]) for r in rows])
            closes = np.array([float(r[4]) for r in rows])
            ma5 = np.full(len(closes), np.nan)
            for i in range(4, len(closes)):
                ma5[i] = closes[i-4:i+1].mean()
            _, _, lower, _, _ = calc_bb(closes)
            sup_boxes, res_boxes = find_boxes(opens, highs, lows, closes, ma5)
            patterns = find_wbottom_patterns_box(sup_boxes, res_boxes, lows, lower, closes)
            for (p1, p2_bp, ent_cp) in reversed(patterns):
                if dates[ent_cp] >= min_date:
                    examples.append({
                        "market": market, "shcode": code,
                        "rows": rows, "dates": dates,
                        "opens": opens, "highs": highs,
                        "lows": lows, "closes": closes,
                        "ma5": ma5,
                        "sup_boxes": sup_boxes, "res_boxes": res_boxes,
                        "p1_idx": p1, "p2_idx": p2_bp, "entry_idx": ent_cp,
                    })
                    print(f"  [{market}] {code}: W-pattern @ {dates[ent_cp]}")
                    break
        except Exception:
            pass
    return examples

def _go_scan(mode_flag, max_n, out_json):
    """Go 바이너리로 W-bottom 신호 스캔, JSON 반환"""
    import subprocess, json as _json
    project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    binary = os.path.join(project_root, "RESTGo")
    cmd = [binary, "stock", "miiib_scan", mode_flag,
           "--max", str(max_n), "--out", out_json]
    cmd = [c for c in cmd if c]  # 빈 문자열 제거 (mode_flag가 '' 일 경우)
    result = subprocess.run(cmd, cwd=project_root, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[go_scan] 오류: {result.stderr.strip()}")
        return []
    with open(out_json) as f:
        data = _json.load(f)
    return data.get("examples", [])


def _draw_from_signal(ex, fetch_fn, out_dir):
    """Go 신호(ex)에서 캔들 조회 후 차트 생성, 저장 경로 반환"""
    shcode = ex["shcode"]
    market = ex["market"]
    signal_date = ex["signal_date"]
    p1_date = ex.get("p1_date", "")
    p2_date = ex.get("p2_date", "")
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
            return dates.index(d) if d in dates else min(range(len(dates)), key=lambda i: abs(dates[i].replace('-','') if '-' in dates[i] else dates[i]) if False else (0 if dates[i] == d else abs(int(dates[i]) - int(d))))

        p1_idx   = nearest(p1_date)     if p1_date     else -1
        p2_idx   = nearest(p2_date)     if p2_date     else -1
        entry_idx = nearest(signal_date) if signal_date in dates else -1

        title = f"MIIIb W바텀 [{market}] {shcode}  진입:{signal_date}"
        outpath = f"{out_dir}/{market}_{shcode}.png"
        draw_chart(dates, opens, highs, lows, closes,
                   upper, mid, lower, pct_b, ma5,
                   sup_boxes, res_boxes,
                   p1_idx, p2_idx, entry_idx, title, outpath)
        return outpath
    except Exception as e:
        print(f"  [{market}] {shcode} 오류: {e}")
        return None


def scan_main():
    """Go 바이너리로 신호 탐지 → Python은 차트만 생성"""
    import json as _json
    out_dir = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(
        os.path.abspath(__file__)))), "zpicture", "miiib_scan")
    os.makedirs(out_dir, exist_ok=True)

    market_configs = [
        ("KR", "",              fetch_kr),
        ("JP", "--foreign-jp",  fetch_foreign),
        ("CN", "--foreign-cn",  fetch_foreign),
        ("HK", "--foreign-hk",  fetch_foreign),
    ]

    all_files = []
    for market, flag, fetch_fn in market_configs:
        tmp_json = f"/tmp/miiib_scan_{market}.json"
        print(f"\n[{market}] 스캔 중...")
        examples = _go_scan(flag, 2, tmp_json)
        if not examples:
            print(f"  [{market}] 패턴 없음")
            continue
        for ex in examples:
            path = _draw_from_signal(ex, fetch_fn, out_dir)
            if path:
                all_files.append(path)
                print(f"  저장: {path}")

    if all_files:
        _tg_send_files(all_files)
    return all_files


def _tg_send_files(files):
    """Telethon으로 저장된 메시지("me")에 차트 전송"""
    try:
        cfg_path = next(
            (p for p in ["/home/feihong/code/REST/RESTGo/config.yaml", "/workspace/config.yaml"]
             if os.path.exists(p)), None)
        if not cfg_path:
            print("[tg] config.yaml 없음 — 전송 생략")
            return
        import yaml as _yaml
        cfg = _yaml.safe_load(open(cfg_path))["telegram"]
        from telethon.sync import TelegramClient
        with TelegramClient(cfg["session_file"], cfg["api_id"], cfg["api_hash"]) as client:
            for f in files:
                client.send_file("me", f, caption=os.path.basename(f))
                print(f"[tg] 전송: {os.path.basename(f)}")
    except Exception as e:
        print(f"[tg] 전송 실패: {e}")

# ── Go 스캔 결과 (hardcoded) ──────────────────────────────────────────────────
# ./RESTGo stock wbottom_scan 결과에서 추출
# MIIIb Box 시퀀스 W바텀 예시 (miiib_scan 결과)
KNOWN_EXAMPLES = [
    {"market": "KR", "shcode": "000020",    "entry": "20260219"},
    {"market": "KR", "shcode": "000080",    "entry": "20260410"},
    {"market": "JP", "shcode": "DTSE1419",  "entry": "20251110"},
    {"market": "JP", "shcode": "DTSE1308",  "entry": "20260105"},
    {"market": "CN", "shcode": "DSHI000006","entry": "20260129"},
    {"market": "CN", "shcode": "DSHI000007","entry": "20251225"},
    {"market": "HK", "shcode": "DHKS00010", "entry": "20260223"},
    {"market": "HK", "shcode": "DHKS00020", "entry": "20250704"},
]

def main():
    out_dir = "/tmp/miiib_charts"
    os.makedirs(out_dir, exist_ok=True)
    all_files = []

    for ex in KNOWN_EXAMPLES:
        market = ex["market"]
        code   = ex["shcode"]
        entry_date = ex["entry"]
        try:
            if market == "KR":
                rows = fetch_kr(code)
            else:
                rows = fetch_foreign(code)

            if len(rows) < 60:
                print(f"  [{market}] {code}: 캔들 부족 ({len(rows)})")
                continue

            dates  = [str(r[0]) for r in rows]
            opens  = np.array([float(r[1]) for r in rows])
            highs  = np.array([float(r[2]) for r in rows])
            lows   = np.array([float(r[3]) for r in rows])
            closes = np.array([float(r[4]) for r in rows])
            upper, mid, lower, pct_b, _ = calc_bb(closes)
            ma5 = np.full(len(closes), np.nan)
            for i in range(4, len(closes)):
                ma5[i] = closes[i-4:i+1].mean()

            # 날짜 → 인덱스
            date_map = {d: i for i, d in enumerate(dates)}

            def nearest_idx(target):
                if target in date_map:
                    return date_map[target]
                for i, d in enumerate(dates):
                    if d >= target:
                        return i
                return len(dates) - 1

            ref_idx = nearest_idx(entry_date)

            # Box 탐지 (Go AnalyzeCurvature 포팅)
            sup_boxes, res_boxes = find_boxes(opens, highs, lows, closes, ma5)

            # Box 기반 W바텀 패턴: 하단Box(BB이탈)→바로다음상단Box→바로다음하단Box
            patterns = find_wbottom_patterns_box(sup_boxes, res_boxes, lows, lower, closes)
            # Go 스캔 결과 이전에 발생한 패턴 중 가장 가까운 것 선택
            p1_idx, p2_idx, entry_idx = -1, -1, ref_idx
            best_dist = 9999
            for (p1, p2_bp, ent_cp) in patterns:
                if ent_cp <= ref_idx and (ref_idx - ent_cp) < best_dist:
                    best_dist = ref_idx - ent_cp
                    p1_idx, p2_idx = p1, p2_bp
                    entry_idx = ent_cp

            title = (f"MIIIb W바텀(Box) [{market}] {code}  진입:{dates[entry_idx]}")
            outpath = f"{out_dir}/{market}_{code}.png"
            draw_chart(dates, opens, highs, lows, closes,
                       upper, mid, lower, pct_b, ma5,
                       sup_boxes, res_boxes,
                       p1_idx, p2_idx, entry_idx, title, outpath)
            all_files.append((market, code, outpath))
            print(f"  [{market}] {code} → {outpath}")
        except Exception as e:
            print(f"  [{market}] {code} 오류: {e}")

    print("\n=== 생성된 차트 ===")
    for market, code, path in all_files:
        print(f"  {path}")

def count_scan_main(min_date="20240101"):
    """Go 전수 스캔 → 신호 수 집계"""
    market_configs = [
        ("KR", "",              fetch_kr),
        ("JP", "--foreign-jp",  fetch_foreign),
        ("CN", "--foreign-cn",  fetch_foreign),
        ("HK", "--foreign-hk",  fetch_foreign),
    ]
    summary = []
    for market, flag, fetch_fn in market_configs:
        tmp_json = f"/tmp/miiib_count_{market}.json"
        print(f"\n[{market}] Go 전수 스캔...", flush=True)
        examples = _go_scan(flag, 0, tmp_json)  # max=0 = 제한 없음
        hit_shcodes = len(set(e["shcode"] for e in examples))
        total_sigs = len(examples)
        print(f"[{market}] 완료 — 신호:{total_sigs} 종목:{hit_shcodes}")
        summary.append((market, hit_shcodes, total_sigs))

    msg = "MIIIb W바텀 전수 스캔 결과 (Go)\n" + "─" * 35 + "\n"
    for market, hit, sigs in summary:
        msg += f"[{market}] 히트종목:{hit} / 신호:{sigs}개\n"
    print("\n" + msg)
    _tg_msg(msg)


def return_study_main(days=500, min_date="20240101"):
    """Go 신호 기반 fixed-horizon 이벤트 스터디 — 수익률 계산만 Python"""
    from concurrent.futures import ThreadPoolExecutor, as_completed

    horizons = [5, 10, 20]
    market_configs = [
        ("KR", "",             fetch_kr),
        ("JP", "--foreign-jp", fetch_foreign),
        ("CN", "--foreign-cn", fetch_foreign),
        ("HK", "--foreign-hk", fetch_foreign),
    ]

    all_records = []
    for market, flag, fetch_fn in market_configs:
        tmp_json = f"/tmp/miiib_study_{market}.json"
        print(f"\n[{market}] Go 신호 수집...", flush=True)
        examples = _go_scan(flag, 0, tmp_json)
        if not examples:
            continue

        # 종목별 그룹화: 1번 fetch로 전체 신호 처리
        from collections import defaultdict
        by_shcode = defaultdict(list)
        for ex in examples:
            if ex.get("signal_date", "") >= min_date:
                by_shcode[ex["shcode"]].append(ex["signal_date"])

        def _compute_returns(shcode_dates):
            shcode, sig_dates = shcode_dates
            try:
                rows = fetch_fn(shcode, days)
                if len(rows) < 60:
                    return []
                closes = np.array([float(r[4]) for r in rows])
                dates  = [str(r[0]) for r in rows]
                date_idx = {d: i for i, d in enumerate(dates)}
                max_h = max(horizons)
                n = len(closes)
                results = []
                for sig_date in sig_dates:
                    cp = date_idx.get(sig_date)
                    if cp is None or cp + max_h >= n:
                        continue
                    rec = {"market": market, "shcode": shcode, "date": sig_date}
                    for h in horizons:
                        rec[f"r{h}"] = (closes[cp + h] - closes[cp]) / closes[cp]
                    # 랜덤 베이스라인
                    valid = [i for i in range(20, n - max_h) if dates[i] >= min_date]
                    if valid:
                        rng = np.random.default_rng(abs(hash(shcode)) % (2**32))
                        samp = rng.choice(valid, size=min(len(valid), max(10, len(sig_dates) * 3)), replace=False)
                        for h in horizons:
                            rec[f"rb{h}"] = float(np.mean([(closes[i+h]-closes[i])/closes[i] for i in samp]))
                    results.append(rec)
                return results
            except Exception:
                return []

        done = 0
        total = len(by_shcode)
        with ThreadPoolExecutor(max_workers=20) as ex:
            futs = {ex.submit(_compute_returns, item): item[0] for item in by_shcode.items()}
            for f in as_completed(futs):
                all_records.extend(f.result())
                done += 1
                if done % 500 == 0:
                    print(f"  [{market}] {done}/{total} 완료, 신호:{len(all_records)}", flush=True)
        mkt_recs = [r for r in all_records if r["market"] == market]
        print(f"[{market}] 완료 — 신호:{len(mkt_recs)}")

    out_path = "/home/feihong/code/REST/RESTGo/zpicture/miiib_scan/return_study.json"
    with open(out_path, "w") as f:
        import json as _j
        _j.dump(all_records, f, ensure_ascii=False)
    print(f"\n결과 저장: {out_path} ({len(all_records)}개 신호)")

    def stats(vals):
        if not vals:
            return 0, 0, 0
        arr = np.array(vals)
        return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean())

    market_names = [m for m, _, _ in market_configs]
    msg = "MIIIb W바텀 Fixed-Horizon 이벤트 스터디 (Go신호)\n"
    msg += f"기간: {min_date}~  총신호: {len(all_records)}개\n" + "─" * 40 + "\n"
    for scope, recs in [("전체", all_records)] + [(m, [r for r in all_records if r["market"] == m]) for m in market_names]:
        if not recs:
            continue
        msg += f"\n[{scope}] {len(recs)}신호\n"
        for h in horizons:
            sv = [r[f"r{h}"] for r in recs if f"r{h}" in r]
            bv = [r[f"rb{h}"] for r in recs if f"rb{h}" in r]
            mn, med, wr = stats(sv)
            bmn, _, _ = stats(bv)
            msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}% | 랜덤{bmn*100:+.2f}%\n"

    print("\n" + msg)
    _tg_msg(msg)


def _tg_msg(msg):
    try:
        cfg_path = next(p for p in ["/home/feihong/code/REST/RESTGo/config.yaml", "/workspace/config.yaml"] if os.path.exists(p))
        import yaml as _yaml
        cfg = _yaml.safe_load(open(cfg_path))["telegram"]
        from telethon.sync import TelegramClient
        with TelegramClient(cfg["session_file"], cfg["api_id"], cfg["api_hash"]) as client:
            client.send_message("me", msg)
    except Exception as e:
        print(f"[tg] 전송 실패: {e}")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "scan":
        scan_main()
    elif len(sys.argv) > 1 and sys.argv[1] == "count":
        count_scan_main()
    elif len(sys.argv) > 1 and sys.argv[1] == "study":
        return_study_main()
    else:
        main()
