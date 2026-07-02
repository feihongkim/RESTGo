"""측정 ⑧ 미돌파 cutoff 재측정 (⑦ 표준 정의 위)
hannam 16y WD+DefBox 전체(n=374) = 돌파(n=37) + 미돌파(n=337).
시나리오 A/B/C = 미돌파 거래 cutoff 20/10/5일.
"""
import sys, os, json
import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg

_HERE      = os.path.dirname(os.path.abspath(__file__))
_PROJ_ROOT = os.path.dirname(os.path.dirname(_HERE))
SCAN_JSON  = os.path.join(_PROJ_ROOT, "zpicture", "wd_hannam_16y_v2.json")
OUT_JSON   = os.path.join(_PROJ_ROOT, "zpicture", "wd_v4_cutoff.json")
MIN_DATE   = "20100101"

BEAR_YEARS = {2011, 2015, 2018, 2022}
BULL_YEARS = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS = {2012, 2013, 2014, 2016, 2017, 2019, 2023}

SCENARIOS = [
    ("A", 20, "r_w20"),
    ("B", 10, "r_w10"),
    ("C",  5, "r_w5"),
]


def stats(vals):
    if not vals:
        return 0.0, 0.0, 0.0
    arr = np.array(vals)
    return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean())


def sharpe(vals, annual_factor):
    if len(vals) < 2:
        return 0.0
    arr = np.array(vals)
    if arr.std() == 0:
        return 0.0
    return float((arr.mean() * annual_factor) / (arr.std() * np.sqrt(annual_factor)))


def bootstrap_ci(vals, n_iter=10000, ci=0.95):
    arr = np.array(vals)
    rng = np.random.default_rng(42)
    means = np.array([rng.choice(arr, len(arr), replace=True).mean() for _ in range(n_iter)])
    lo = (1 - ci) / 2
    return float(np.percentile(means, lo * 100)), float(np.percentile(means, (1 - lo) * 100))


def capital_efficiency(cutoff_days):
    """미돌파 거래의 자본 효율: 1 - (묶임일수 / 252)"""
    return 1.0 - cutoff_days / 252.0


def run():
    with open(SCAN_JSON) as f:
        data = json.load(f)
    examples = data.get("examples", [])

    defbox_all = [e for e in examples
                  if e.get("has_defbox") and e.get("signal_date", "") >= MIN_DATE]
    breaks  = [e for e in defbox_all if e.get("defbox_break_date")]
    nobreak = [e for e in defbox_all if not e.get("defbox_break_date")]

    # 돌파 거래 기준 수익률 (h=20 W-신호 기준, ⑦과 동일)
    break_r20 = [e["r_w20"] for e in breaks if e.get("r_w20") is not None]

    msg = "측정⑧ 미돌파 cutoff 재측정 (⑦ 표준 위)\n"
    msg += "─" * 55 + "\n"
    msg += f"  전체 WD+DefBox(post-{MIN_DATE[:4]}): {len(defbox_all)}건\n"
    msg += f"  돌파(break):  {len(breaks)}건 (이미 측정, 평균+13.13% Sharpe 2.45)\n"
    msg += f"  미돌파:       {len(nobreak)}건 → cutoff 시나리오 비교\n\n"

    result = {
        "total_defbox": len(defbox_all),
        "n_break": len(breaks),
        "n_nobreak": len(nobreak),
        "scenarios": {},
    }

    # ── 미돌파 단독 섹션 ───────────────────────────────────
    msg += "─── [1] 미돌파 단독 성과 ───────────────────────\n"
    msg += f"{'시나리오':<5} {'cutoff':>6} {'n':>5} {'평균':>9} {'중간':>9} {'승률':>7} {'연알파':>9} {'Sharpe':>7} {'자본효율':>8}\n"
    msg += "─" * 75 + "\n"

    for label, h, rkey in SCENARIOS:
        vals = [e[rkey] for e in nobreak if e.get(rkey) is not None]
        mn, med, wr = stats(vals)
        af = 252.0 / h
        sh = sharpe(vals, af)
        ann = mn * af
        ce = capital_efficiency(h)
        result["scenarios"][label] = {
            "cutoff_days": h,
            "nobreak": {"n": len(vals), "mean": mn, "median": med,
                        "win_rate": wr, "annual_alpha": ann, "sharpe": sh},
            "capital_efficiency": ce,
        }
        msg += (f"  {label}    {h:>5}일  {len(vals):>4}  {mn*100:>+7.2f}%  "
                f"{med*100:>+7.2f}%  {wr*100:>5.1f}%  {ann*100:>+7.1f}%  "
                f"{sh:>6.2f}  {ce*100:>6.1f}%\n")

    # ── 통합(break + nobreak) 섹션 ────────────────────────
    msg += "\n─── [2] 통합 포트폴리오 (돌파+미돌파) ──────────\n"
    msg += f"{'시나리오':<5} {'break_r':>8} {'nobreak_r':>9} {'총n':>5} {'평균':>9} {'승률':>7} {'연알파':>9} {'Sharpe':>7} {'CI_lo':>8} {'CI_hi':>8}\n"
    msg += "─" * 90 + "\n"

    for label, h, rkey in SCENARIOS:
        nb_vals = [e[rkey] for e in nobreak if e.get(rkey) is not None]
        # 돌파 거래는 항상 r_w20 (W신호 20일 기준, ⑦ 표준)
        combined = break_r20 + nb_vals
        if not combined:
            continue
        af = 252.0 / h  # 지배적 horizon 기준 (미돌파 cutoff)
        mn, med, wr = stats(combined)
        sh = sharpe(combined, af)
        ann = mn * af
        ci_lo, ci_hi = bootstrap_ci(combined)
        result["scenarios"][label]["combined"] = {
            "n": len(combined), "mean": mn, "median": med,
            "win_rate": wr, "annual_alpha": ann, "sharpe": sh,
            "ci_lo": ci_lo, "ci_hi": ci_hi,
        }
        br_mn = float(np.mean(break_r20)) if break_r20 else 0
        nb_mn = float(np.mean(nb_vals)) if nb_vals else 0
        msg += (f"  {label}    {br_mn*100:>+6.2f}%  {nb_mn*100:>+7.2f}%  "
                f"{len(combined):>4}  {mn*100:>+7.2f}%  {wr*100:>5.1f}%  "
                f"{ann*100:>+7.1f}%  {sh:>6.2f}  "
                f"{ci_lo*100:>+7.2f}%  {ci_hi*100:>+7.2f}%\n")

    # ── 사이클별 미돌파 손익 (시나리오 B 기준) ──────────────
    msg += "\n─── [3] 사이클별 미돌파 손익 (시나리오B=10일 기준) ─\n"
    for seg, years in [("약세", BEAR_YEARS), ("강세", BULL_YEARS), ("사이드", SIDE_YEARS)]:
        seg_nb = [e for e in nobreak if int(e["signal_date"][:4]) in years]
        vals = [e["r_w10"] for e in seg_nb if e.get("r_w10") is not None]
        mn, med, wr = stats(vals)
        msg += f"  {seg}: {len(vals):3d}건  평균{mn*100:+.2f}%  중간{med*100:+.2f}%  승률{wr*100:.1f}%\n"

    # ── 자본효율 요약 ──────────────────────────────────────
    msg += "\n─── [4] 자본 효율 요약 ────────────────────────\n"
    msg += "  (미돌파 거래 기준, break 거래는 별도)\n"
    for label, h, _ in SCENARIOS:
        ce = capital_efficiency(h)
        msg += f"  시나리오{label} (cutoff={h}일): 자본효율 {ce*100:.1f}% (묶임 {h}일/252)\n"

    # ── 게이트 판정 ────────────────────────────────────────
    msg += "\n─── [5] 게이트 판정 (자본효율 최고 + Sharpe≥1.0) ──\n"
    best = None
    best_score = -999.0
    for label, h, rkey in SCENARIOS:
        info = result["scenarios"].get(label, {})
        combined_info = info.get("combined", {})
        sh = combined_info.get("sharpe", 0.0)
        ce = info.get("capital_efficiency", 0.0)
        score = ce + sh / 10  # 자본효율 우선, sharpe 보조
        gate_ok = sh >= 1.0
        mark = "✅" if gate_ok else "❌"
        msg += f"  {mark} 시나리오{label}: 통합 Sharpe={sh:.2f}, 자본효율={ce*100:.1f}%\n"
        if gate_ok and score > best_score:
            best = label
            best_score = score

    if best:
        msg += f"\n  ★ 채택: 시나리오{best} — Sharpe≥1.0 + 자본효율 최고\n"
    else:
        msg += "\n  ❌ 모든 시나리오 Sharpe<1.0 → 미돌파 편입 불리\n"
        # 그래도 최선 추천
        best_sh = -999.0
        best_label = None
        for label, h, rkey in SCENARIOS:
            sh = result["scenarios"].get(label, {}).get("combined", {}).get("sharpe", 0.0)
            if sh > best_sh:
                best_sh = sh
                best_label = label
        if best_label:
            msg += f"  권고: Sharpe 최고인 시나리오{best_label} 또는 돌파 단독(Sharpe 2.45) 사용\n"

    print(msg)
    _tg_msg(msg)

    with open(OUT_JSON, "w") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)
    print(f"[⑧] 저장: {OUT_JSON}")
    return msg


if __name__ == "__main__":
    print("=== 측정⑧ cutoff 재측정 ===")
    run()
