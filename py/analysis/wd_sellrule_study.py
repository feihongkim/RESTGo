"""측정 ⑥ WD + sell_strategy1_posOnly_mh25 매도룰 합성 시뮬레이터
WD 진입 + 4가지 매도룰 (CriticalFailure / BBUpperBreakoutProfit / PeriodExpiry) 시뮬레이션.
비용: round-trip 0.4% (매수 0.2% + 매도 0.2%).
"""
import sys, os, json
import numpy as np
from datetime import datetime, timedelta

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "common"))
from db import _load_config, _get_key, decrypt

_HERE      = os.path.dirname(os.path.abspath(__file__))
_PROJ_ROOT = os.path.dirname(os.path.dirname(_HERE))
SCAN_JSON  = os.path.join(_PROJ_ROOT, "zpicture", "wd_hannam_16y_v2.json")
OUT_JSON   = os.path.join(_PROJ_ROOT, "zpicture", "wd_v3_sellrule.json")
MIN_DATE  = "20100101"
MH        = 25       # max_holding_period
BUY_COST  = 0.002   # 0.2% per tranche
SELL_COST = 0.002   # 0.2%
BB_MIN_PCT   = 0.95  # BBUpperBreakoutProfit: BB%B 임계
BB_MIN_PROF  = 0.08  # BBUpperBreakoutProfit: 수익률 임계 (8%)
BEAR_YEARS = {2011, 2015, 2018, 2022}
BULL_YEARS = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS = {2012, 2013, 2014, 2016, 2017, 2019, 2023}


# ── DB 연결 ──────────────────────────────────────────────
def _make_conn():
    cfg = _load_config()
    key = _get_key(cfg["FKEY"])
    import pymssql
    user = decrypt(cfg["MSSQL_USER"], key)
    pw   = decrypt(cfg["MSSQL_PASSWORD"], key)
    return pymssql.connect(server="192.168.3.120", port=1433,
                           user=user, password=pw,
                           database="hannam", charset="utf8")


def fetch_candles(conn, shcode, date_le, n_hist=200, n_fwd=35):
    """break_date 전 n_hist봉 + 이후 n_fwd봉 조회."""
    cur = conn.cursor()
    # 과거 (역순 조회 후 반전)
    cur.execute(f"""
        SELECT TOP {n_hist} DATE,[OPEN],[HIGH],[LOW],[CLOSE],CAST(VOLUME AS FLOAT)
        FROM stock_price_kor_d001
        WHERE SHCODE='{shcode}' AND STICK_TYPE='D001' AND DATE<='{date_le}'
        ORDER BY DATE DESC
    """)
    hist = list(reversed(cur.fetchall()))
    # 미래 (순방향)
    cur.execute(f"""
        SELECT TOP {n_fwd} DATE,[OPEN],[HIGH],[LOW],[CLOSE],CAST(VOLUME AS FLOAT)
        FROM stock_price_kor_d001
        WHERE SHCODE='{shcode}' AND STICK_TYPE='D001' AND DATE>'{date_le}'
        ORDER BY DATE ASC
    """)
    fwd = cur.fetchall()
    rows = hist + list(fwd)
    return [{"date": r[0], "open": float(r[1] or 0), "high": float(r[2] or 0),
             "low": float(r[3] or 0), "close": float(r[4] or 0),
             "volume": float(r[5] or 0)} for r in rows], len(hist) - 1  # break_pos = last hist index


# ── 지표 계산 ─────────────────────────────────────────────
def _sma(vals, period):
    result = [0.0] * len(vals)
    s = 0.0
    for i, v in enumerate(vals):
        s += v
        if i >= period:
            s -= vals[i - period]
        if i >= period - 1:
            result[i] = s / period
    return result


def _compute_indicators(candles):
    closes  = [c["close"]  for c in candles]
    opens   = [c["open"]   for c in candles]
    volumes = [c["volume"] for c in candles]

    ma5  = _sma(closes, 5)
    ma20 = _sma(closes, 20)
    ma60 = _sma(closes, 60)

    # Bollinger (20, 2σ)
    bb_pct = [0.0] * len(closes)
    for i in range(19, len(closes)):
        w = closes[i - 19: i + 1]
        mu = sum(w) / 20
        sigma = (sum((x - mu) ** 2 for x in w) / 20) ** 0.5
        lo = mu - 2 * sigma
        hi = mu + 2 * sigma
        if hi > lo:
            bb_pct[i] = (closes[i] - lo) / (hi - lo)

    return ma5, ma20, ma60, bb_pct, closes, opens, volumes


# ── 매도 시뮬레이션 ───────────────────────────────────────
def simulate(candles, break_pos, sig_close, defbox_price):
    """break_pos 이후 MH일 매도룰 시뮬레이션.
    반환: {returns: [float], rule: str, exit_day: int, mdd: float}
    """
    ma5, ma20, ma60, bb_pct, closes, opens, volumes = _compute_indicators(candles)

    break_close = candles[break_pos]["close"] if break_pos < len(candles) else sig_close
    if break_close == 0:
        break_close = sig_close

    # 블렌딩 진입가 (비용 포함)
    entry = (0.5 * sig_close + 0.5 * break_close) * (1 + BUY_COST)
    if entry == 0:
        return None

    remaining = 1.0       # 잔여 포지션 비율
    exit_log  = []        # [(rule, weight, exit_price)]
    crash_pos = -1        # 폭락 발생 위치
    peak      = entry     # MDD 계산용 고점

    for day in range(1, MH + 1):
        pos = break_pos + day
        if pos >= len(candles):
            # 데이터 없으면 마지막 가용 종가로 청산
            last = candles[min(break_pos + day - 1, len(candles) - 1)]
            exit_price = last["close"] * (1 - SELL_COST)
            exit_log.append(("DataEnd", remaining, exit_price))
            remaining = 0.0
            break

        cur_close  = closes[pos]
        cur_open   = opens[pos]
        cur_ma5    = ma5[pos]
        cur_ma20   = ma20[pos]
        cur_ma60   = ma60[pos]
        cur_bb_pct = bb_pct[pos]

        # MDD 추적
        if cur_close > peak:
            peak = cur_close

        # ── CriticalFailure ──────────────────────────────
        triggered_critical = False

        # 조건1: 폭락(open→close ≤ -10%) 후 3일 이상 defbox_price 미회복
        if cur_open > 0:
            if (cur_close - cur_open) / cur_open <= -0.10:
                crash_pos = pos
        if crash_pos >= 0 and pos - crash_pos >= 3:
            recovered = any(candles[i]["close"] >= defbox_price
                            for i in range(crash_pos + 1, pos + 1))
            if not recovered:
                triggered_critical = True

        # 조건2: MA 역배열 (Close < MA5 < MA20 < MA60)
        if not triggered_critical and cur_ma60 > 0:
            if cur_close < cur_ma5 < cur_ma20 < cur_ma60:
                triggered_critical = True

        # 조건4: 5일 누적 하락 ≤ -15%
        if not triggered_critical and pos >= 5 and candles[pos - 5]["close"] > 0:
            cum = (cur_close - candles[pos - 5]["close"]) / candles[pos - 5]["close"]
            if cum <= -0.15:
                triggered_critical = True

        # 조건5: MA 역배열 3일 이상 지속
        if not triggered_critical and cur_ma60 > 0:
            consec = 0
            for back in range(pos, max(pos - 5, -1), -1):
                if (ma5[back] < ma20[back] < ma60[back]) and ma60[back] > 0:
                    consec += 1
                else:
                    break
            if consec >= 3:
                triggered_critical = True

        if triggered_critical and remaining > 0:
            exit_price = cur_close * (1 - SELL_COST)
            exit_log.append(("CriticalFailure", remaining, exit_price))
            remaining = 0.0
            break

        # ── BBUpperBreakoutProfit (weight=0.5, 1회) ──────
        if remaining > 0.5 and cur_bb_pct > BB_MIN_PCT and entry > 0:
            ret_now = (cur_close * (1 - SELL_COST)) / entry - 1
            if ret_now > BB_MIN_PROF:
                partial = min(0.5, remaining)
                exit_price = cur_close * (1 - SELL_COST)
                exit_log.append(("BBUpperBreakoutProfit", partial, exit_price))
                remaining -= partial

        # ── PeriodExpiry (day == MH) ──────────────────────
        if day == MH and remaining > 0:
            exit_price = cur_close * (1 - SELL_COST)
            exit_log.append(("PeriodExpiry", remaining, exit_price))
            remaining = 0.0
            break

    # 잔여 미청산 → 마지막 가용 종가로 강제 청산
    if remaining > 0:
        last_pos = min(break_pos + MH, len(candles) - 1)
        exit_price = candles[last_pos]["close"] * (1 - SELL_COST)
        exit_log.append(("ForcedExit", remaining, exit_price))
        remaining = 0.0

    # 총 수익률
    total_exit_value = sum(w * ep for _, w, ep in exit_log)
    pnl = total_exit_value / entry - 1 if entry > 0 else 0.0

    # MDD (진입 이후 최대 낙폭)
    mdd = 0.0
    running_peak = entry / (1 + BUY_COST)  # 원가 기준 가격 추적
    for day in range(1, MH + 1):
        pos = break_pos + day
        if pos >= len(candles):
            break
        c = candles[pos]["close"]
        if c > running_peak:
            running_peak = c
        dd = (running_peak - candles[pos]["low"]) / running_peak if running_peak > 0 else 0
        if dd > mdd:
            mdd = dd

    # 지배적 매도 룰
    primary_rule = exit_log[0][0] if exit_log else "None"

    return {
        "pnl":          pnl,
        "mdd":          mdd,
        "primary_rule": primary_rule,
        "exit_log":     [(r, w, ep) for r, w, ep in exit_log],
        "entry":        entry,
        "break_close":  break_close,
    }


# ── 분석 실행 ─────────────────────────────────────────────
def run():
    with open(SCAN_JSON) as f:
        data = json.load(f)
    examples = data.get("examples", [])

    signals = [e for e in examples
               if e.get("has_defbox") and e.get("defbox_break_date")
               and e.get("signal_date", "") >= MIN_DATE
               and e.get("close_at_signal", 0) > 0]
    print(f"시뮬레이션 대상: {len(signals)}건")

    conn = _make_conn()
    results = []
    skipped = 0

    for idx, sig in enumerate(signals):
        shcode     = sig["shcode"]
        break_date = sig["defbox_break_date"]
        sig_close  = sig["close_at_signal"]
        defbox_price = sig.get("defbox_price", 0)

        try:
            candles, break_pos = fetch_candles(conn, shcode, break_date)
        except Exception as e:
            print(f"  [{idx+1}] {shcode} {break_date} 데이터 오류: {e}")
            skipped += 1
            continue

        if break_pos < 20 or break_pos >= len(candles):
            skipped += 1
            continue

        sim = simulate(candles, break_pos, sig_close, defbox_price)
        if sim is None:
            skipped += 1
            continue

        year = int(sig["signal_date"][:4])
        results.append({
            "shcode":       shcode,
            "signal_date":  sig["signal_date"],
            "break_date":   break_date,
            "year":         year,
            "pnl":          sim["pnl"],
            "mdd":          sim["mdd"],
            "primary_rule": sim["primary_rule"],
            "r_w20_raw":    sig.get("r_w20"),  # 비교용 원래 WD-기준 20일 수익률
        })
        if (idx + 1) % 10 == 0:
            print(f"  {idx+1}/{len(signals)} 처리 완료")

    conn.close()
    print(f"완료: {len(results)}건 / 스킵: {skipped}건")

    pnls = [r["pnl"] for r in results]
    mdds = [r["mdd"] for r in results]

    if not pnls:
        print("결과 없음")
        return

    arr = np.array(pnls)
    af  = 252 / MH
    mn  = float(arr.mean())
    med = float(np.median(arr))
    wr  = float((arr > 0).mean())
    ann = mn * af
    sh  = float((arr.mean() * af) / (arr.std() * np.sqrt(af))) if arr.std() > 0 else 0.0
    avg_mdd = float(np.mean(mdds))

    # 매도 룰 빈도
    from collections import Counter
    rule_cnt = Counter(r["primary_rule"] for r in results)
    total = len(results)

    # 사이클별
    def seg_stats(yrs):
        v = [r["pnl"] for r in results if r["year"] in yrs]
        if not v:
            return 0, 0.0, 0.0
        a = np.array(v)
        return len(v), float(a.mean()), float((a > 0).mean())

    bear_n, bear_mn, bear_wr = seg_stats(BEAR_YEARS)
    bull_n, bull_mn, bull_wr = seg_stats(BULL_YEARS)
    side_n, side_mn, side_wr = seg_stats(SIDE_YEARS)

    # Bootstrap CI
    rng = np.random.default_rng(42)
    bs_means = np.array([rng.choice(arr, len(arr), replace=True).mean() for _ in range(10000)])
    ci_lo = float(np.percentile(bs_means, 2.5))
    ci_hi = float(np.percentile(bs_means, 97.5))

    msg = "측정⑥ WD+매도룰 합성 시뮬레이션\n"
    msg += "─" * 50 + "\n"
    msg += f"  진입: 50% @ W-신호종가 + 50% @ DefBox돌파종가\n"
    msg += f"  비용: 매수0.2%×2 + 매도0.2% (round-trip 0.4%+0.2%?)\n"
    msg += f"  매도룰: CriticalFailure / BBUpperBreakoutProfit / PeriodExpiry(mh={MH})\n"
    msg += f"  표본: n={len(results)}건 (post-{MIN_DATE[:4]} WD+돌파)\n"

    msg += "\n[1] 종합 성과\n"
    msg += f"  평균 수익률 : {mn*100:+.2f}%\n"
    msg += f"  중간 수익률 : {med*100:+.2f}%\n"
    msg += f"  승률        : {wr*100:.1f}%\n"
    msg += f"  연환산 알파 : {ann*100:+.1f}%  (hold={MH}일, ×{af:.1f})\n"
    msg += f"  Sharpe      : {sh:.2f}\n"
    msg += f"  평균 MDD    : {avg_mdd*100:.1f}%\n"
    msg += f"  95% CI      : [{ci_lo*100:+.2f}%, {ci_hi*100:+.2f}%]\n"

    msg += "\n[2] 매도룰 빈도\n"
    for rule, cnt in rule_cnt.most_common():
        msg += f"  {rule:<26}: {cnt:3d}건 ({cnt/total*100:.0f}%)\n"

    msg += "\n[3] 사이클별 세그먼트\n"
    msg += f"  약세({','.join(str(y) for y in sorted(BEAR_YEARS))}): {bear_n}건  평균{bear_mn*100:+.2f}%  승률{bear_wr*100:.1f}%\n"
    msg += f"  강세: {bull_n}건  평균{bull_mn*100:+.2f}%  승률{bull_wr*100:.1f}%\n"
    msg += f"  사이드: {side_n}건  평균{side_mn*100:+.2f}%  승률{side_wr*100:.1f}%\n"

    # 비교: WD 원시 r_w20 vs 시뮬 PnL
    raw_w20 = [r["r_w20_raw"] for r in results if r.get("r_w20_raw") is not None]
    if raw_w20:
        raw_arr = np.array(raw_w20)
        msg += f"\n[4] 매도룰 적용 효과 (WD 기준 h=20 원시 수익률 비교)\n"
        msg += f"  WD h=20 원시: 평균{raw_arr.mean()*100:+.2f}%  승률{(raw_arr>0).mean()*100:.1f}%\n"
        msg += f"  매도룰 적용: 평균{mn*100:+.2f}%  승률{wr*100:.1f}%\n"
        delta = mn - raw_arr.mean()
        msg += f"  차이: {delta*100:+.2f}% ({'개선' if delta >= 0 else '악화'})\n"

    # 게이트 판정
    g1 = "✅" if sh >= 0.50 else "❌"
    g2 = "✅" if ann >= 0.15 else "❌"
    msg += "\n[5] 게이트 판정 (Sharpe≥0.5 AND 연알파≥+15%)\n"
    msg += f"  {g1} Sharpe: {sh:.2f} (≥0.50)\n"
    msg += f"  {g2} 연알파: {ann*100:+.1f}% (≥+15%)\n"
    gate_pass = sh >= 0.50 and ann >= 0.15
    if gate_pass:
        msg += "\n  ★ 게이트 통과 → WD+매도룰 합성 전략 유효\n"
    else:
        msg += "\n  ❌ 게이트 미통과\n"
        if sh < 0.50:
            msg += f"    Sharpe {sh:.2f} < 0.50: 변동성 대비 수익 부족\n"
        if ann < 0.15:
            msg += f"    연알파 {ann*100:+.1f}% < +15%: 절대 수익 부족\n"

    print(msg)
    _tg_msg(msg)

    out = {
        "n": len(results), "skipped": skipped,
        "mean_pnl": mn, "median_pnl": med, "win_rate": wr,
        "annual_alpha": ann, "sharpe": sh, "avg_mdd": avg_mdd,
        "bootstrap_ci": {"lo": ci_lo, "hi": ci_hi},
        "rule_freq": dict(rule_cnt),
        "bear": {"n": bear_n, "mean": bear_mn, "wr": bear_wr},
        "bull": {"n": bull_n, "mean": bull_mn, "wr": bull_wr},
        "side": {"n": side_n, "mean": side_mn, "wr": side_wr},
        "gate_pass": gate_pass,
        "trades": results,
    }
    with open(OUT_JSON, "w") as f:
        json.dump(out, f, indent=2, ensure_ascii=False)
    print(f"[⑥] 저장 완료: {OUT_JSON}")
    return msg, gate_pass


if __name__ == "__main__":
    print("=== 측정⑥ WD+매도룰 시뮬레이션 ===")
    run()
