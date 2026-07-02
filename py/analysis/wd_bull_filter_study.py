"""측정 ⑤ 강세장 진입 차단 필터 분석
hannam 16y WD+돌파 위에서 F1/F2/F1+F2 필터 시나리오 비교.
(F3 VKOSPI는 데이터 없어 스킵)
"""
import sys, os, json
import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg

SCAN_JSON    = "/tmp/wd_hannam_16y_v2.json"
RECORDS_JSON = "/tmp/wd_hannam_16y_records_v2.json"

BEAR_YEARS  = {2011, 2015, 2018, 2022}
BULL_YEARS  = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS  = {2012, 2013, 2014, 2016, 2017, 2019, 2023}
MIN_DATE    = "20100101"


def stats(vals):
    if not vals:
        return 0.0, 0.0, 0.0
    arr = np.array(vals)
    return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean())


def sharpe(vals, annual_factor):
    """단순 Sharpe: annualized_return / annualized_std"""
    if not vals or len(vals) < 2:
        return 0.0
    arr = np.array(vals)
    if arr.std() == 0:
        return 0.0
    return (arr.mean() * annual_factor) / (arr.std() * np.sqrt(annual_factor))


def build_records(examples):
    recs = []
    for sig in examples:
        if not sig.get("has_defbox"):
            continue
        sig_date = sig.get("signal_date", "")
        if sig_date < MIN_DATE:
            continue
        r_w20 = sig.get("r_w20")
        if r_w20 is None:
            continue
        rec = {
            "shcode":         sig["shcode"],
            "signal_date":    sig_date,
            "year":           int(sig_date[:4]),
            "has_break":      bool(sig.get("defbox_break_date")),
            "r_w20":          r_w20,
            "ma200":          sig.get("ma200_at_signal", 0.0),
            "rsi14":          sig.get("rsi14_at_signal", 0.0),
            "close_origin":   sig.get("close_at_signal", 0.0),
        }
        recs.append(rec)
    return recs


def cycle(year):
    if year in BEAR_YEARS:
        return "약세"
    if year in BULL_YEARS:
        return "강세"
    return "사이드"


def analyze_filter(break_recs, f_name, f_func):
    """필터 적용 후 메트릭 계산"""
    passed = [r for r in break_recs if f_func(r)]
    filtered = len(break_recs) - len(passed)
    pct_kept = len(passed) / len(break_recs) * 100 if break_recs else 0

    vals_all  = [r["r_w20"] for r in passed]
    mn, med, wr = stats(vals_all)

    # 사이클별
    cycle_stats = {}
    for seg, years in [("약세", BEAR_YEARS), ("강세", BULL_YEARS), ("사이드", SIDE_YEARS)]:
        seg_vals = [r["r_w20"] for r in passed if r["year"] in years]
        cycle_stats[seg] = stats(seg_vals) + (len(seg_vals),)

    # 연환산 (h=20 = 20일 보유)
    annual_factor = 252 / 20
    ann_alpha = mn * annual_factor
    sh = sharpe(vals_all, annual_factor)

    bull_mn  = cycle_stats["강세"][0]
    bull_wr  = cycle_stats["강세"][2]
    bull_cnt = cycle_stats["강세"][3]

    return {
        "name": f_name,
        "total_kept":   len(passed),
        "total_filter": filtered,
        "pct_kept":     pct_kept,
        "mn": mn, "med": med, "wr": wr,
        "ann_alpha": ann_alpha,
        "sharpe": sh,
        "bull_mn": bull_mn, "bull_wr": bull_wr, "bull_cnt": bull_cnt,
        "bear_mn": cycle_stats["약세"][0], "bear_wr": cycle_stats["약세"][2], "bear_cnt": cycle_stats["약세"][3],
        "side_mn": cycle_stats["사이드"][0], "side_wr": cycle_stats["사이드"][2], "side_cnt": cycle_stats["사이드"][3],
    }


def format_filter_table(results, baseline):
    hdr  = f"{'필터':<12} {'건수':>5} {'유지%':>6} {'평균':>8} {'승률':>7} {'강세건':>6} {'강세avg':>8} {'강세승률':>8} {'연알파':>8} {'Sharpe':>7}\n"
    hdr += "─" * 80 + "\n"
    rows = ""

    def fmt(r):
        return (f"{r['name']:<12} {r['total_kept']:>5} {r['pct_kept']:>5.0f}% "
                f"{r['mn']*100:>+7.2f}% {r['wr']*100:>6.1f}% "
                f"{r['bull_cnt']:>6} {r['bull_mn']*100:>+7.2f}% {r['bull_wr']*100:>7.1f}% "
                f"{r['ann_alpha']*100:>+7.1f}% {r['sharpe']:>7.2f}\n")

    rows += fmt(baseline)
    for r in results:
        rows += fmt(r)
    return hdr + rows


def run_bull_filter_study(examples):
    all_recs   = build_records(examples)
    break_recs = [r for r in all_recs if r["has_break"]]

    total_break = len(break_recs)
    ma200_avail = sum(1 for r in break_recs if r["ma200"] > 0)
    rsi_avail   = sum(1 for r in break_recs if r["rsi14"] > 0)

    msg = "측정⑤ 강세장 차단 필터 (hannam 16y WD+돌파)\n"
    msg += "─" * 45 + "\n"
    msg += f"WD+돌파 {total_break}건 | MA200 데이터 {ma200_avail}건 | RSI14 데이터 {rsi_avail}건\n"

    # 베이스라인 (필터 없음)
    baseline = analyze_filter(break_recs, "기준(없음)", lambda r: True)

    # F1: Close < MA200 (장기 추세 아래서만 진입)
    f1_recs = [r for r in break_recs if r["ma200"] > 0]
    f1_total = len(f1_recs)
    f1 = analyze_filter(f1_recs, "F1(Close<MA200)", lambda r: r["close_origin"] < r["ma200"])

    # F2: RSI(14) < 70 (과매수 제외)
    f2_recs = [r for r in break_recs if r["rsi14"] > 0]
    f2 = analyze_filter(f2_recs, "F2(RSI14<70)", lambda r: r["rsi14"] < 70)

    # F1+F2
    f12_recs = [r for r in break_recs if r["ma200"] > 0 and r["rsi14"] > 0]
    f12 = analyze_filter(f12_recs, "F1+F2(복합)", lambda r: r["close_origin"] < r["ma200"] and r["rsi14"] < 70)

    # F3 스킵 (VKOSPI 데이터 없음)

    msg += "\n─── 필터 시나리오 비교 ─────────────────────────────────\n"
    msg += format_filter_table([f1, f2, f12], baseline)

    # 사이클별 상세
    msg += "\n─── 강세 segment 상세 ──────────────────────────────────\n"
    for r in [baseline, f1, f2, f12]:
        msg += (f"  {r['name']:<14}: 강세 {r['bull_cnt']}건 "
                f"평균{r['bull_mn']*100:+.2f}% 승률{r['bull_wr']*100:.1f}%\n")

    # 베어/사이드도 비교
    msg += "\n─── 약세·사이드 segment 검증 ───────────────────────────\n"
    for r in [baseline, f1, f2, f12]:
        msg += (f"  {r['name']:<14}: 약세 {r['bear_cnt']}건 {r['bear_mn']*100:+.2f}%/{r['bear_wr']*100:.0f}%  "
                f"사이드 {r['side_cnt']}건 {r['side_mn']*100:+.2f}%/{r['side_wr']*100:.0f}%\n")

    # 판정
    msg += "\n─── 판정 기준: 강세≥+6%, 유지건수≥70%, Sharpe≥0.30 ────\n"
    adopt = []
    for r in [f1, f2, f12]:
        ok_bull   = r["bull_mn"] >= 0.06
        ok_count  = r["pct_kept"] >= 70
        ok_sharpe = r["sharpe"] >= 0.30
        mark = "✅" if (ok_bull and ok_count and ok_sharpe) else "❌"
        reason = []
        if not ok_bull:   reason.append(f"강세 {r['bull_mn']*100:+.1f}%<+6%")
        if not ok_count:  reason.append(f"유지{r['pct_kept']:.0f}%<70%")
        if not ok_sharpe: reason.append(f"Sharpe{r['sharpe']:.2f}<0.30")
        msg += f"  {mark} {r['name']}: {', '.join(reason) if reason else '모두 통과'}\n"
        if ok_bull and ok_count and ok_sharpe:
            adopt.append(r["name"])

    msg += "\n  F3(VKOSPI): 양 DB 데이터 없음 → 스킵\n"

    if adopt:
        msg += f"\n권고: {', '.join(adopt)} 채택 후보\n"
    else:
        msg += "\n권고: 현행 필터 없음 유지 (강세 개선 효과 불충분 또는 건수 손실 과다)\n"

    print(msg)
    _tg_msg(msg)
    return msg


if __name__ == "__main__":
    print("=== 측정⑤ 강세장 차단 필터 ===")

    if not os.path.exists(SCAN_JSON):
        print(f"스캔 파일 없음: {SCAN_JSON}")
        print("wdefbox_scan --hannam --candles 4500 --out /tmp/wd_hannam_16y_v2.json 재실행 필요")
        sys.exit(1)

    with open(SCAN_JSON) as f:
        data = json.load(f)
    examples = data.get("examples", [])
    print(f"로드: {len(examples)}건 원시 신호")

    with open(RECORDS_JSON, "w") as f:
        json.dump([e for e in examples if e.get("has_defbox") and e.get("signal_date","") >= "20100101"], f)

    run_bull_filter_study(examples)
