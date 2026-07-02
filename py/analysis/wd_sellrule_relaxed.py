"""측정 ⑨ CF 조건 완화 매도룰 재합성
CriticalFailure 변형 V1/V2/V3 시나리오 비교.
V1: CF 조건2(MA역배열) 제거
V2(N=5): CF 조건2를 진입 후 5일간 유예
V2(N=10): CF 조건2를 진입 후 10일간 유예
V3: CF 전체 제거

목표: Sharpe ≥ 1.0 AND 연알파 ≥ +30% 시나리오 채택.
     달성 불가 시 WD 단독(Sharpe 2.45) 권고.
"""
import sys, os, json
import numpy as np
from collections import Counter

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "common"))
from db import _load_config, _get_key, decrypt

_HERE      = os.path.dirname(os.path.abspath(__file__))
_PROJ_ROOT = os.path.dirname(os.path.dirname(_HERE))
SCAN_JSON  = os.path.join(_PROJ_ROOT, "zpicture", "wd_hannam_16y_v2.json")
OUT_JSON   = os.path.join(_PROJ_ROOT, "zpicture", "wd_v4_sellrule_relaxed.json")
MIN_DATE   = "20100101"
MH         = 25
BUY_COST   = 0.002
SELL_COST  = 0.002
BB_MIN_PCT  = 0.95
BB_MIN_PROF = 0.08
BEAR_YEARS = {2011, 2015, 2018, 2022}
BULL_YEARS = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS = {2012, 2013, 2014, 2016, 2017, 2019, 2023}


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
    cur = conn.cursor()
    cur.execute(f"""
        SELECT TOP {n_hist} DATE,[OPEN],[HIGH],[LOW],[CLOSE],CAST(VOLUME AS FLOAT)
        FROM stock_price_kor_d001
        WHERE SHCODE='{shcode}' AND STICK_TYPE='D001' AND DATE<='{date_le}'
        ORDER BY DATE DESC
    """)
    hist = list(reversed(cur.fetchall()))
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
             "volume": float(r[5] or 0)} for r in rows], len(hist) - 1


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
    ma5   = _sma(closes, 5)
    ma20  = _sma(closes, 20)
    ma60  = _sma(closes, 60)
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


def simulate(candles, break_pos, sig_close, defbox_price,
             no_cf=False, cf_ma_reverse_grace=0):
    """
    no_cf=True          → V3: CriticalFailure 전체 제거
    cf_ma_reverse_grace → V2: 진입 후 N일간 조건2(MA역배열) 유예 (0=항상 적용=V1 포함)
    조건2 자체 제거는 cf_ma_reverse_grace=999로 표현
    """
    ma5, ma20, ma60, bb_pct, closes, opens, volumes = _compute_indicators(candles)
    break_close = candles[break_pos]["close"] if break_pos < len(candles) else sig_close
    if break_close == 0:
        break_close = sig_close
    entry = (0.5 * sig_close + 0.5 * break_close) * (1 + BUY_COST)
    if entry == 0:
        return None

    remaining = 1.0
    exit_log  = []
    crash_pos = -1
    mdd_running_peak = entry / (1 + BUY_COST)
    max_mdd = 0.0

    for day in range(1, MH + 1):
        pos = break_pos + day
        if pos >= len(candles):
            last = candles[min(break_pos + day - 1, len(candles) - 1)]
            exit_log.append(("DataEnd", remaining, last["close"] * (1 - SELL_COST)))
            remaining = 0.0
            break

        cur_close  = closes[pos]
        cur_open   = opens[pos]
        cur_ma5    = ma5[pos]
        cur_ma20   = ma20[pos]
        cur_ma60   = ma60[pos]
        cur_bb_pct = bb_pct[pos]

        if cur_close > mdd_running_peak:
            mdd_running_peak = cur_close
        low_dd = (mdd_running_peak - candles[pos]["low"]) / mdd_running_peak if mdd_running_peak > 0 else 0
        if low_dd > max_mdd:
            max_mdd = low_dd

        # ── CriticalFailure ──────────────────────────────
        triggered_critical = False
        if not no_cf:
            # 조건1: 폭락 + 3일 미회복
            if cur_open > 0 and (cur_close - cur_open) / cur_open <= -0.10:
                crash_pos = pos
            if crash_pos >= 0 and pos - crash_pos >= 3:
                recovered = any(candles[i]["close"] >= defbox_price
                                for i in range(crash_pos + 1, pos + 1))
                if not recovered:
                    triggered_critical = True

            # 조건2: MA 역배열 — grace period 적용
            if not triggered_critical and cur_ma60 > 0 and day > cf_ma_reverse_grace:
                if cur_close < cur_ma5 < cur_ma20 < cur_ma60:
                    triggered_critical = True

            # 조건4: 5일 누적 -15%
            if not triggered_critical and pos >= 5 and candles[pos - 5]["close"] > 0:
                cum = (cur_close - candles[pos - 5]["close"]) / candles[pos - 5]["close"]
                if cum <= -0.15:
                    triggered_critical = True

            # 조건5: MA 역배열 3일 지속
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
            exit_log.append(("CriticalFailure", remaining, cur_close * (1 - SELL_COST)))
            remaining = 0.0
            break

        # ── BBUpperBreakoutProfit ────────────────────────
        if remaining > 0.5 and cur_bb_pct > BB_MIN_PCT and entry > 0:
            ret_now = (cur_close * (1 - SELL_COST)) / entry - 1
            if ret_now > BB_MIN_PROF:
                partial = min(0.5, remaining)
                exit_log.append(("BBUpperBreakoutProfit", partial, cur_close * (1 - SELL_COST)))
                remaining -= partial

        # ── PeriodExpiry ─────────────────────────────────
        if day == MH and remaining > 0:
            exit_log.append(("PeriodExpiry", remaining, cur_close * (1 - SELL_COST)))
            remaining = 0.0
            break

    if remaining > 0:
        last_pos = min(break_pos + MH, len(candles) - 1)
        exit_log.append(("ForcedExit", remaining, candles[last_pos]["close"] * (1 - SELL_COST)))

    total_exit = sum(w * ep for _, w, ep in exit_log)
    pnl = total_exit / entry - 1 if entry > 0 else 0.0
    primary_rule = exit_log[0][0] if exit_log else "None"
    return {"pnl": pnl, "mdd": max_mdd, "primary_rule": primary_rule}


def run_variant(signals, conn, label, no_cf=False, cf_ma_reverse_grace=0):
    trades = []
    skipped = 0
    for sig in signals:
        shcode     = sig["shcode"]
        break_date = sig["defbox_break_date"]
        sig_close  = sig["close_at_signal"]
        defbox_price = sig.get("defbox_price", 0)
        try:
            candles, break_pos = fetch_candles(conn, shcode, break_date)
        except Exception:
            skipped += 1
            continue
        if break_pos < 20 or break_pos >= len(candles):
            skipped += 1
            continue
        sim = simulate(candles, break_pos, sig_close, defbox_price,
                       no_cf=no_cf, cf_ma_reverse_grace=cf_ma_reverse_grace)
        if sim is None:
            skipped += 1
            continue
        year = int(sig["signal_date"][:4])
        trades.append({"year": year, "pnl": sim["pnl"], "mdd": sim["mdd"],
                       "primary_rule": sim["primary_rule"]})
    return trades, skipped


def aggregate(trades, label):
    if not trades:
        return {}
    pnls = [t["pnl"] for t in trades]
    mdds = [t["mdd"] for t in trades]
    arr  = np.array(pnls)
    af   = 252.0 / MH
    mn   = float(arr.mean())
    med  = float(np.median(arr))
    wr   = float((arr > 0).mean())
    sh   = float((arr.mean() * af) / (arr.std() * np.sqrt(af))) if arr.std() > 0 else 0.0
    ann  = mn * af
    avg_mdd = float(np.mean(mdds))
    rng  = np.random.default_rng(42)
    bs   = np.array([rng.choice(arr, len(arr), replace=True).mean() for _ in range(10000)])
    ci_lo = float(np.percentile(bs, 2.5))
    ci_hi = float(np.percentile(bs, 97.5))
    rule_cnt = Counter(t["primary_rule"] for t in trades)
    cf_pct = rule_cnt.get("CriticalFailure", 0) / len(trades) * 100
    return {
        "label": label, "n": len(trades),
        "mean": mn, "median": med, "win_rate": wr,
        "annual_alpha": ann, "sharpe": sh, "avg_mdd": avg_mdd,
        "ci_lo": ci_lo, "ci_hi": ci_hi,
        "rule_freq": dict(rule_cnt), "cf_pct": cf_pct,
    }


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

    variants = [
        ("V0_base",    False, 0,    "기준(⑥ 재현)"),
        ("V1",         False, 999,  "CF조건2(MA역배열) 제거"),
        ("V2_N5",      False, 5,    "CF조건2 유예 5일"),
        ("V2_N10",     False, 10,   "CF조건2 유예 10일"),
        ("V3",         True,  0,    "CF 전체 제거"),
    ]

    all_results = {}
    for vkey, no_cf, grace, desc in variants:
        print(f"[{vkey}] {desc} ...", flush=True)
        trades, sk = run_variant(signals, conn, vkey,
                                 no_cf=no_cf, cf_ma_reverse_grace=grace)
        agg = aggregate(trades, desc)
        agg["skipped"] = sk
        all_results[vkey] = agg
        print(f"  완료: {len(trades)}건, CF={agg.get('cf_pct',0):.0f}%, "
              f"Sharpe={agg.get('sharpe',0):.2f}, ann={agg.get('annual_alpha',0)*100:+.1f}%")

    conn.close()

    # ── 보고서 ─────────────────────────────────────────────
    msg = "측정⑨ CF 완화 매도룰 재합성\n"
    msg += "─" * 55 + "\n"
    msg += f"  표본: n={len(signals)}건 (post-{MIN_DATE[:4]} WD+돌파)\n"
    msg += f"  진입: 50%@W신호 + 50%@DefBox돌파 (비용0.4%R/T)\n"
    msg += f"  비교: V0=기준 / V1=조건2제거 / V2(5/10)=조건2유예 / V3=CF전체제거\n\n"

    hdr = f"{'변형':<12} {'n':>4} {'평균':>8} {'승률':>7} {'Sharpe':>7} {'연알파':>8} {'MDD':>7} {'CF%':>6} {'CI_lo':>8} {'CI_hi':>8}\n"
    msg += hdr
    msg += "─" * 90 + "\n"

    for vkey, _, _, desc in variants:
        r = all_results.get(vkey, {})
        if not r:
            continue
        mark = "⑥" if vkey == "V0_base" else vkey
        msg += (f"  {mark:<10} {r['n']:>4} {r['mean']*100:>+7.2f}% {r['win_rate']*100:>6.1f}% "
                f"{r['sharpe']:>7.2f} {r['annual_alpha']*100:>+7.1f}% "
                f"{r['avg_mdd']*100:>6.1f}% {r['cf_pct']:>5.0f}% "
                f"{r['ci_lo']*100:>+7.2f}% {r['ci_hi']*100:>+7.2f}%\n")

    # 매도룰별 빈도 상세
    msg += "\n─── 매도룰 빈도 상세 ─────────────────────────────\n"
    for vkey, _, _, desc in variants:
        r = all_results.get(vkey, {})
        if not r:
            continue
        rf = r.get("rule_freq", {})
        mark = "V0(⑥기준)" if vkey == "V0_base" else vkey
        freq_str = " / ".join(f"{k}:{v}" for k, v in sorted(rf.items(), key=lambda x: -x[1]))
        msg += f"  {mark}: {freq_str}\n"

    # 게이트 판정
    msg += "\n─── 게이트 판정 (Sharpe≥1.0 AND 연알파≥+30%) ────\n"
    adopted = []
    for vkey, _, _, desc in variants:
        if vkey == "V0_base":
            continue
        r = all_results.get(vkey, {})
        if not r:
            continue
        sh  = r.get("sharpe", 0)
        ann = r.get("annual_alpha", 0)
        ok  = sh >= 1.0 and ann >= 0.30
        mark = "✅" if ok else "❌"
        fails = []
        if sh < 1.0:   fails.append(f"Sharpe {sh:.2f}<1.0")
        if ann < 0.30: fails.append(f"연알파 {ann*100:+.1f}%<+30%")
        msg += f"  {mark} {vkey}({desc}): {', '.join(fails) if fails else '통과'}\n"
        if ok:
            adopted.append(vkey)

    msg += "\n─── 참고: WD 단독 (⑦) ──────────────────────────\n"
    msg += "  Sharpe 2.45 / 연알파 +165% / 승률 83.8% / 95% CI [+7.25%, +19.60%]\n"

    if adopted:
        best_v = max(adopted, key=lambda v: all_results[v]["sharpe"])
        msg += f"\n  ★ 채택: {best_v} — 매도룰 합성 유효\n"
    else:
        msg += "\n  ❌ V1/V2/V3 모두 게이트 미달 → WD 단독(Sharpe 2.45) 확정 권고\n"
        msg += "  근거: CF 완화해도 WD 고유 수익(Sharpe 2.45)을 매도룰이 넘기 어려움\n"

    print(msg)
    _tg_msg(msg)

    out = {"variants": all_results, "gate": {"sharpe_min": 1.0, "annual_alpha_min": 0.30},
           "adopted": adopted}
    with open(OUT_JSON, "w") as f:
        json.dump(out, f, indent=2, ensure_ascii=False)
    print(f"[⑨] 저장: {OUT_JSON}")
    return msg, adopted


if __name__ == "__main__":
    print("=== 측정⑨ CF 완화 변형 시뮬레이션 ===")
    run()
