"""측정 ③ 미돌파 cutoff 자본 효율 — 시나리오 A(20d)/B(10d)/C(5d)"""
import sys, os, json
import numpy as np
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg


def stats(vals):
    if not vals:
        return 0.0, 0.0, 0.0, 0.0
    arr = np.array(vals)
    return float(arr.mean()), float(np.median(arr)), float((arr > 0).mean()), float(arr.std())


def max_drawdown(vals):
    if not vals:
        return 0.0
    return float(min(vals))  # 단일 거래 기준 MDD = 최소 수익률


def sharpe(vals):
    if len(vals) < 2:
        return 0.0
    arr = np.array(vals)
    return float(arr.mean() / arr.std()) if arr.std() > 0 else 0.0


def run_cutoff_study(all_records):
    """
    all_records: wd_16y_study.py에서 생성된 per-trade 레코드
    각 레코드: signal_date, has_break, break_days, r_w5/10/20, r_def5/10/20(optional)
    """
    break_recs = [r for r in all_records if r.get("has_break")]
    nobreak_recs = [r for r in all_records if not r.get("has_break")]
    all_wd = break_recs + nobreak_recs

    if not all_wd:
        print("[cutoff] 데이터 없음")
        return

    msg = "측정③ 미돌파 cutoff 자본 효율\n"
    msg += "기준: hannam DB 2010~2026 / WD+DefBox\n"
    msg += "─" * 45 + "\n"

    cutoffs = [
        ("A", 20, "현행"),
        ("B", 10, "10일"),
        ("C",  5, "5일"),
    ]

    scenario_results = []

    for label, cutoff_days, desc in cutoffs:
        # WD+돌파 재분류: break_days <= cutoff → 돌파, 그 외 → 미돌파
        break_in_cutoff = [r for r in all_wd if r.get("has_break") and r.get("break_days", 999) <= cutoff_days]
        nobreak_in_cutoff = [r for r in all_wd if not r.get("has_break") or r.get("break_days", 999) > cutoff_days]

        # 돌파 거래: blended (50%W + 50%DefBox@break, h=20 기준)
        blended = []
        for r in break_in_cutoff:
            rw = r.get("r_w20")
            rd = r.get("r_def20")
            if rw is not None and rd is not None:
                blended.append(0.5 * rw + 0.5 * rd)

        # 미돌파 거래: cutoff 시점 종료 (r_w{cutoff_days})
        nobreak_key = f"r_w{min(cutoff_days, 20)}"
        nobreak_returns = [r[nobreak_key] for r in nobreak_in_cutoff if nobreak_key in r]

        # 전체 합산 (50%W 비중 기준 → 실효 수익률)
        # 돌파: 100% 실제 배치 (blended)
        # 미돌파: 50% 배치, r_w{cutoff} 으로 마감
        all_trade_returns = blended + [r * 0.5 for r in nobreak_returns]

        # 자본 활용: 돌파 거래 → 최대 h=20일 유지, 미돌파 → cutoff일
        avg_holding = (len(break_in_cutoff) * 20 + len(nobreak_in_cutoff) * cutoff_days) / max(len(all_wd), 1)
        capital_util = 1 - (len(nobreak_in_cutoff) * (20 - cutoff_days)) / max(len(all_wd) * 20, 1)

        mn_blended, med_blended, wr_blended, std_blended = stats(blended)
        mn_nb, _, wr_nb, _ = stats(nobreak_returns)
        mn_all, _, _, _ = stats(all_trade_returns)
        mdd = max_drawdown(all_trade_returns)
        sp = sharpe(all_trade_returns)

        # 연환산: 252거래일 / 평균 보유일 * 평균 수익률 (단순 추정)
        annual_alpha = mn_all * (252 / max(avg_holding, 1)) if avg_holding > 0 else 0

        res = {
            "label": label,
            "desc": desc,
            "cutoff": cutoff_days,
            "break_count": len(break_in_cutoff),
            "nobreak_count": len(nobreak_in_cutoff),
            "blended_mean": mn_blended,
            "blended_wr": wr_blended,
            "nobreak_mean_at_cutoff": mn_nb,
            "nobreak_wr": wr_nb,
            "avg_holding_days": avg_holding,
            "capital_util": capital_util,
            "annual_alpha": annual_alpha,
            "mdd": mdd,
            "sharpe": sp,
        }
        scenario_results.append(res)

        msg += f"\n[시나리오 {label}] cutoff={cutoff_days}일 ({desc})\n"
        msg += f"  돌파({len(break_in_cutoff)}건) 블렌드수익: 평균{mn_blended*100:+.2f}% 승률{wr_blended*100:.1f}%\n"
        msg += f"  미돌파({len(nobreak_in_cutoff)}건) cutoff청산: 평균{mn_nb*100:+.2f}% 승률{wr_nb*100:.1f}%\n"
        msg += f"  평균 보유일: {avg_holding:.1f}d  자본활용률: {capital_util*100:.1f}%\n"
        msg += f"  연환산 알파 추정: {annual_alpha*100:+.1f}%  MDD: {mdd*100:+.2f}%  Sharpe: {sp:.2f}\n"

    # 비교 요약
    msg += f"\n─── 비교 요약 ─────────────────────────\n"
    msg += f"{'시나리오':<8} {'연환산':>8} {'자본활용률':>10} {'Sharpe':>8} {'돌파건수':>8}\n"
    for r in scenario_results:
        msg += f"  {r['label']}({r['desc']:<5}) {r['annual_alpha']*100:+7.1f}% {r['capital_util']*100:8.1f}%  {r['sharpe']:7.2f}  {r['break_count']:>6}건\n"

    # 판정
    best = max(scenario_results, key=lambda x: x["annual_alpha"])
    msg += f"\n권고: 시나리오 {best['label']} ({best['desc']}) — 연환산 {best['annual_alpha']*100:+.1f}%\n"

    print(msg)
    _tg_msg(msg)

    # raw 저장
    with open("/tmp/wd_cutoff_grid.json", "w") as f:
        json.dump(scenario_results, f, indent=2)
    print("raw 저장: /tmp/wd_cutoff_grid.json")
    return scenario_results


if __name__ == "__main__":
    # 단독 실행 시 ②의 cached 데이터 사용
    cache = "/tmp/wd_hannam_16y_records.json"
    if not os.path.exists(cache):
        print(f"캐시 없음: {cache} — 먼저 wd_16y_study.py 실행 필요")
        sys.exit(1)
    with open(cache) as f:
        all_records = json.load(f)
    print(f"캐시 로드: {len(all_records)}건")
    run_cutoff_study(all_records)
