"""합성 전략 수익률 분석: WD(W+DefBox) + S1(DefBox 단독 돌파)"""
import sys, os, json, subprocess
import numpy as np
from concurrent.futures import ThreadPoolExecutor, as_completed
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import fetch_kr, fetch_foreign, _tg_msg

HORIZONS = [5, 10, 20]
MIN_DATE = "20240101"


def _go_combined_scan(mode_flag, out_json):
    project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    binary = os.path.join(project_root, "RESTGo")
    cmd = [c for c in [binary, "stock", "combined_scan", mode_flag, "--max", "0", "--out", out_json] if c]
    result = subprocess.run(cmd, cwd=project_root, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[go_scan] 오류: {result.stderr.strip()}")
        return []
    with open(out_json) as f:
        data = json.load(f)
    return data.get("examples", [])


def compute_returns(shcode, signals, fetch_fn, days=500):
    try:
        rows = fetch_fn(shcode, days)
        if len(rows) < 60:
            return []
        closes = np.array([float(r[4]) for r in rows])
        dates  = [str(r[0]) for r in rows]
        date_idx = {d: i for i, d in enumerate(dates)}
        max_h = max(HORIZONS)
        n = len(closes)
        results = []
        for sig in signals:
            if sig.get("type") == "WD":
                w_date = sig.get("w_signal_date", "")
                def_date = sig.get("defbox_break_date", "")
                if not w_date or w_date < MIN_DATE:
                    continue
                wp = date_idx.get(w_date)
                if wp is None or wp + max_h >= n:
                    continue
                rec = {
                    "type": "WD",
                    "shcode": shcode,
                    "w_signal_date": w_date,
                    "has_break": bool(def_date),
                }
                for h in HORIZONS:
                    rec[f"r_w{h}"] = (closes[wp + h] - closes[wp]) / closes[wp]
                if def_date:
                    dp = date_idx.get(def_date)
                    if dp is not None:
                        for h in HORIZONS:
                            end = dp + h
                            rec[f"r_def{h}"] = (closes[end] - closes[dp]) / closes[dp] if end < n else None
                results.append(rec)
            elif sig.get("type") == "S1":
                break_date = sig.get("defbox_break_date", "")
                if not break_date or break_date < MIN_DATE:
                    continue
                bp = date_idx.get(break_date)
                if bp is None or bp + max_h >= n:
                    continue
                rec = {"type": "S1", "shcode": shcode, "defbox_break_date": break_date}
                for h in HORIZONS:
                    end = bp + h
                    rec[f"r_s1_{h}"] = (closes[end] - closes[bp]) / closes[bp] if end < n else None
                results.append(rec)
        return results
    except Exception:
        return []


def stats(vals):
    if not vals:
        return 0.0, 0.0, 0.0
    arr = np.array(vals)
    return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean())


def run_study(market, mode_flag, fetch_fn):
    tmp_json = f"/tmp/combined_study_{market}.json"
    print(f"\n[{market}] Go 스캔...", flush=True)
    examples = _go_combined_scan(mode_flag, tmp_json)
    if not examples:
        print(f"  [{market}] 신호 없음")
        return []

    by_shcode = defaultdict(list)
    for ex in examples:
        shcode = ex.get("shcode", "")
        if shcode:
            by_shcode[shcode].append(ex)

    all_records = []
    done, total = 0, len(by_shcode)
    with ThreadPoolExecutor(max_workers=20) as ex:
        futs = {ex.submit(compute_returns, sh, sigs, fetch_fn): sh
                for sh, sigs in by_shcode.items()}
        for f in as_completed(futs):
            all_records.extend(f.result())
            done += 1
            if done % 200 == 0:
                print(f"  [{market}] {done}/{total} 완료, 신호:{len(all_records)}", flush=True)
    print(f"[{market}] 완료 — {len(all_records)}건")
    for r in all_records:
        r["market"] = market
    return all_records


def format_report(all_records):
    wd = [r for r in all_records if r["type"] == "WD"]
    wd_break = [r for r in wd if r.get("has_break")]
    wd_nobreak = [r for r in wd if not r.get("has_break")]
    s1 = [r for r in all_records if r["type"] == "S1"]

    msg = "합성 전략(WD+S1) 수익률 분석\n"
    msg += "기준: 4개국 전종목 / 2024-01-01~\n"
    msg += "─" * 45 + "\n"

    # WD 전체 (W 진입 기준)
    msg += f"\n[WD] W+DefBox 신호 전체 ({len(wd)}건) — W진입 기준\n"
    for h in HORIZONS:
        vals = [r[f"r_w{h}"] for r in wd if f"r_w{h}" in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD+돌파 (W 진입 기준)
    msg += f"\n[WD+돌파] W+DefBox+20일내돌파 ({len(wd_break)}건) — W진입 기준\n"
    for h in HORIZONS:
        vals = [r[f"r_w{h}"] for r in wd_break if f"r_w{h}" in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD+돌파 (돌파 진입 기준)
    msg += f"\n[WD+돌파] DefBox 돌파진입 기준 ({len(wd_break)}건)\n"
    for h in HORIZONS:
        vals = [r[f"r_def{h}"] for r in wd_break if r.get(f"r_def{h}") is not None]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD+돌파 블렌드 (50%W + 50%Def, h=20)
    blended = []
    for r in wd_break:
        rw = r.get("r_w20")
        rd = r.get("r_def20")
        if rw is not None and rd is not None:
            blended.append(0.5 * rw + 0.5 * rd)
    if blended:
        mn, med, wr = stats(blended)
        msg += f"\n[WD 블렌드] 50%W + 50%DefBox돌파 (h=20, n={len(blended)}건)\n"
        msg += f"  평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD 미돌파 (W 진입만, 50% 배치)
    msg += f"\n[WD 미돌파] 20일 내 미돌파 ({len(wd_nobreak)}건) — W진입만(50%)\n"
    for h in HORIZONS:
        vals = [r[f"r_w{h}"] for r in wd_nobreak if f"r_w{h}" in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # S1: DefBox 단독 돌파
    msg += f"\n[S1] DefBox 단독 돌파 ({len(s1)}건) — 돌파진입 기준\n"
    for h in HORIZONS:
        key = f"r_s1_{h}"
        vals = [r[key] for r in s1 if r.get(key) is not None]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # 비교 요약
    msg += f"\n─── 비교 요약 (h=20) ─────────────────\n"
    wdw20 = [r.get("r_w20") for r in wd_break if r.get("r_w20") is not None]
    msg += f"WD W진입만: {np.mean(wdw20)*100:+.2f}% ({len(wdw20)}건)\n" if wdw20 else ""
    if blended:
        msg += f"WD 블렌드:  {np.mean(blended)*100:+.2f}% ({len(blended)}건)\n"
    s1_20 = [r.get("r_s1_20") for r in s1 if r.get("r_s1_20") is not None]
    msg += f"S1 단독:    {np.mean(s1_20)*100:+.2f}% ({len(s1_20)}건)\n" if s1_20 else ""

    print(msg)
    _tg_msg(msg)
    return msg


if __name__ == "__main__":
    market_configs = [
        ("KR", "",             fetch_kr),
        ("JP", "--foreign-jp", fetch_foreign),
        ("CN", "--foreign-cn", fetch_foreign),
        ("HK", "--foreign-hk", fetch_foreign),
    ]

    all_records = []
    for market, flag, fetch_fn in market_configs:
        all_records.extend(run_study(market, flag, fetch_fn))

    format_report(all_records)
