"""측정 ② WD 16년 확장 — hannam DB (2010~2026, bear/bull cycle 검증)
Go 스캔이 forward return을 JSON에 직접 포함 (DB 재접속 불필요).
"""
import sys, os, json, subprocess
import numpy as np
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BINARY = os.path.join(PROJECT_ROOT, "RESTGo")
HORIZONS = [5, 10, 20]
MIN_DATE = "20100101"

BEAR_YEARS  = {2011, 2015, 2018, 2022}
BULL_YEARS  = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS  = {2012, 2013, 2014, 2016, 2017, 2019, 2023}


def _go_scan_hannam(out_json, candles=4500):
    cmd = [BINARY, "stock", "wdefbox_scan", "--hannam",
           "--candles", str(candles), "--max", "0", "--out", out_json]
    print(f"[Go 스캔] {' '.join(cmd)}", flush=True)
    result = subprocess.run(cmd, cwd=PROJECT_ROOT, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[Go] 오류: {result.stderr[-400:]}")
        return []
    with open(out_json) as f:
        data = json.load(f)
    return data.get("examples", [])


def stats(vals):
    if not vals:
        return 0.0, 0.0, 0.0
    arr = np.array(vals)
    return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean())


def build_records(examples):
    """Go JSON에서 직접 레코드 빌드 (DB 재접속 없음)"""
    records = []
    for sig in examples:
        if not sig.get("has_defbox"):
            continue
        sig_date = sig.get("signal_date", "")
        if sig_date < MIN_DATE:
            continue
        rec = {
            "shcode": sig["shcode"],
            "signal_date": sig_date,
            "year": int(sig_date[:4]),
            "has_break": bool(sig.get("defbox_break_date")),
            "break_days": sig.get("defbox_break_days", 0),
        }
        for key in ["r_w5", "r_w10", "r_w20", "r_def5", "r_def10", "r_def20"]:
            v = sig.get(key)
            if v is not None:
                rec[key] = v
        records.append(rec)
    return records


def format_report(all_records):
    break_recs   = [r for r in all_records if r.get("has_break")]
    nobreak_recs = [r for r in all_records if not r.get("has_break")]

    msg = "측정② WD 16년 확장 (hannam DB 2010~2026)\n"
    msg += "─" * 45 + "\n"

    msg += f"\n[전체] WD+DefBox 신호 ({len(all_records)}건)\n"
    msg += f"  돌파: {len(break_recs)}건 / 미돌파: {len(nobreak_recs)}건 ({len(nobreak_recs)/(len(all_records) or 1)*100:.0f}%)\n"

    msg += f"\n[WD+돌파] ({len(break_recs)}건) — W진입 기준\n"
    for h in HORIZONS:
        vals = [r[f"r_w{h}"] for r in break_recs if f"r_w{h}" in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    msg += f"\n[미돌파] ({len(nobreak_recs)}건) — W진입 기준\n"
    for h in HORIZONS:
        vals = [r[f"r_w{h}"] for r in nobreak_recs if f"r_w{h}" in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # 사이클별 세그먼트 (h=20, WD+돌파)
    msg += f"\n─── 사이클별 세그먼트 (h=20, WD+돌파) ─────\n"
    bear_result = None
    for seg_name, years in [("약세", BEAR_YEARS), ("강세", BULL_YEARS), ("사이드", SIDE_YEARS)]:
        seg = [r for r in break_recs if r["year"] in years]
        vals = [r["r_w20"] for r in seg if "r_w20" in r]
        mn, med, wr = stats(vals)
        if seg_name == "약세":
            bear_result = (mn, wr)
        msg += f"  {seg_name}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}% ({len(seg)}건)\n"

    # 연도별 분포
    msg += f"\n─── 연도별 WD+돌파 건수 & 수익률 (h=20) ────\n"
    by_year = defaultdict(list)
    for r in break_recs:
        by_year[r["year"]].append(r.get("r_w20"))
    for yr in sorted(by_year):
        vals = [v for v in by_year[yr] if v is not None]
        mn, _, wr = stats(vals)
        cycle = "🔴" if yr in BEAR_YEARS else ("🟢" if yr in BULL_YEARS else "⚪")
        msg += f"  {yr} {cycle}: {len(vals):3d}건 평균{mn*100:+.2f}% 승률{wr*100:.1f}%\n"

    # 게이트 판정
    overall_vals = [r["r_w20"] for r in break_recs if "r_w20" in r]
    _, _, overall_wr = stats(overall_vals)
    bear_mn, bear_wr = bear_result if bear_result else (0, 0)

    msg += f"\n─── 게이트 판정 ────────────────────────\n"
    gate1 = "✅ PASS" if bear_mn >= 0.05 else "❌ FAIL"
    gate2 = "✅ PASS" if overall_wr >= 0.60 else "❌ FAIL"
    msg += f"  베어사이클 h=20 평균: {bear_mn*100:+.2f}% (≥+5%) → {gate1}\n"
    msg += f"  전체 승률: {overall_wr*100:.1f}% (≥60%) → {gate2}\n"

    gate_pass = bear_mn >= 0.05 and overall_wr >= 0.60
    if gate_pass:
        msg += "  → 두 게이트 통과 → ③ cutoff 분석 자동 진행\n"
    else:
        msg += "  → 게이트 미통과\n"

    msg += f"\n참고: KIS2 2.5년 +13.79% 승률72.3% (347건)\n"

    print(msg)
    _tg_msg(msg)
    return msg, gate_pass


if __name__ == "__main__":
    scan_json = "/tmp/wd_hannam_16y.json"

    print("=== 측정② WD 16년 확장 ===")
    examples = _go_scan_hannam(scan_json, candles=4500)
    print(f"Go 스캔 완료: {len(examples)}건 원시 신호")

    all_records = build_records(examples)
    print(f"유효 레코드: {len(all_records)}건 (has_defbox + 2010~)")

    msg, gate_pass = format_report(all_records)

    with open("/tmp/wd_hannam_16y_records.json", "w") as f:
        json.dump(all_records, f)
    print("raw 저장: /tmp/wd_hannam_16y_records.json")

    if gate_pass:
        print("\n=== 측정③ 자동 진행 ===")
        import importlib.util
        spec = importlib.util.spec_from_file_location(
            "wd_cutoff_study",
            os.path.join(os.path.dirname(__file__), "wd_cutoff_study.py")
        )
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        mod.run_cutoff_study(all_records)
    else:
        print("게이트 미통과 — ③ 스킵")
