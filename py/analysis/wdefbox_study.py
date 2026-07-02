"""W+DefBox 결합 전략 수익률 비교 분석"""
import sys, os, json, subprocess
import numpy as np
from concurrent.futures import ThreadPoolExecutor, as_completed
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import fetch_kr, fetch_foreign, _tg_msg

HORIZONS = [5, 10, 20]

def _go_wdefbox_scan(mode_flag, out_json):
    project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    binary = os.path.join(project_root, "RESTGo")
    cmd = [c for c in [binary, "stock", "wdefbox_scan", mode_flag, "--max", "0", "--out", out_json] if c]
    result = subprocess.run(cmd, cwd=project_root, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[go_scan] 오류: {result.stderr.strip()}")
        return []
    with open(out_json) as f:
        data = json.load(f)
    return data.get("examples", [])


def compute_returns(shcode, signals, fetch_fn, days=500, min_date="20240101"):
    """한 종목의 모든 신호에 대해 forward return 계산"""
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
            if sig.get("signal_date", "") < min_date:
                continue
            cp = date_idx.get(sig["signal_date"])
            if cp is None or cp + max_h >= n:
                continue

            rec = {
                "shcode": shcode,
                "signal_date": sig["signal_date"],
                "has_defbox": sig.get("has_defbox", False),
                "has_break": bool(sig.get("defbox_break_date")),
            }

            # W-신호 진입 수익률 (h 후)
            for h in HORIZONS:
                rec[f"r_w{h}"] = (closes[cp + h] - closes[cp]) / closes[cp]

            # DefBox 돌파 진입 수익률 (돌파 캔들 → h 후)
            break_date = sig.get("defbox_break_date", "")
            if break_date:
                bp = date_idx.get(break_date)
                if bp is not None:
                    for h in HORIZONS:
                        end = bp + h
                        if end < n:
                            rec[f"r_def{h}"] = (closes[end] - closes[bp]) / closes[bp]
                        else:
                            rec[f"r_def{h}"] = None

            results.append(rec)
        return results
    except Exception:
        return []


def stats(vals):
    if not vals:
        return 0.0, 0.0, 0.0
    arr = np.array(vals)
    return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean())


def run_study(market, mode_flag, fetch_fn, min_date="20240101"):
    tmp_json = f"/tmp/wdefbox_study_{market}.json"
    print(f"\n[{market}] Go 스캔...", flush=True)
    examples = _go_wdefbox_scan(mode_flag, tmp_json)
    if not examples:
        print(f"  [{market}] 신호 없음")
        return []

    by_shcode = defaultdict(list)
    for ex in examples:
        by_shcode[ex["shcode"]].append(ex)

    all_records = []
    done = 0
    total = len(by_shcode)
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


def print_and_format(all_records):
    msg = "W+DefBox 결합 전략 수익률 비교\n"
    msg += "기준: KR 전종목 / 2024-01-01~\n"
    msg += "─" * 45 + "\n"

    groups = [
        ("① 전체 W신호 (DefBox 무관)",
         all_records),
        ("② W신호 + DefBox 존재",
         [r for r in all_records if r.get("has_defbox")]),
        ("③ W+DefBox+20일내돌파 [W진입 수익률]",
         [r for r in all_records if r.get("has_break")]),
        ("④ W+DefBox+20일내 미돌파",
         [r for r in all_records if r.get("has_defbox") and not r.get("has_break")]),
    ]

    for name, recs in groups:
        if not recs:
            continue
        msg += f"\n{name} ({len(recs)}건)\n"
        for h in HORIZONS:
            key = f"r_w{h}"
            vals = [r[key] for r in recs if key in r]
            mn, med, wr = stats(vals)
            msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # DefBox 돌파 기준 수익률 (돌파 캔들 기준 forward return)
    break_recs = [r for r in all_records if r.get("has_break")]
    if break_recs:
        msg += f"\n⑤ DefBox돌파 진입 기준 수익률 ({len(break_recs)}건)\n"
        for h in HORIZONS:
            key = f"r_def{h}"
            vals = [r[key] for r in break_recs if r.get(key) is not None]
            mn, med, wr = stats(vals)
            msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

        # 결합 수익률 (50% W진입 + 50% DefBox돌파진입, h=20 기준)
        msg += f"\n⑥ 결합 수익률 [50%W+50%DefBox돌파] (h=20)\n"
        blended = []
        for r in break_recs:
            rw = r.get("r_w20")
            rd = r.get("r_def20")
            if rw is not None and rd is not None:
                blended.append(0.5 * rw + 0.5 * rd)
        if blended:
            mn, med, wr = stats(blended)
            msg += f"  평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"
            msg += f"  (vs ③ W단독 h=20: {np.mean([r['r_w20'] for r in break_recs if 'r_w20' in r])*100:+.2f}%)\n"

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

    print_and_format(all_records)
