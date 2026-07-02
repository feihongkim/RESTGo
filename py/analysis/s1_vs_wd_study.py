"""Strategy1.yaml (전체 룰 엔진) vs W+DefBox 수익률 비교"""
import sys, os, json, subprocess
import numpy as np
from concurrent.futures import ThreadPoolExecutor, as_completed
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import fetch_kr, fetch_foreign, _tg_msg
from wdefbox_study import run_study as run_wd_study, stats

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BINARY = os.path.join(PROJECT_ROOT, "RESTGo")
HORIZONS = [5, 10, 20]
MIN_DATE = "20240101"


def run_strategy1_scan(mode_flag, out_json):
    """strategy1.yaml 전체 룰 엔진으로 이벤트 스터디 실행"""
    cmd = [c for c in [BINARY, "stock", "strategy_study", mode_flag,
                        "rules/strategy1.yaml", out_json] if c]
    print(f"  [S1 Go] {' '.join(cmd)}", flush=True)
    result = subprocess.run(cmd, cwd=PROJECT_ROOT, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"  [S1] 오류: {result.stderr[-500:]}")
        return []
    if not os.path.exists(out_json):
        print(f"  [S1] 출력 파일 없음: {out_json}")
        print(f"  [S1] stderr: {result.stderr[-300:]}")
        return []
    with open(out_json) as f:
        data = json.load(f)
    return data.get("trades", [])


def strategy1_stats(trades):
    """strategy_study의 trades JSON에서 h5/h10/h20 수익률 통계 산출"""
    results = {}
    for h in HORIZONS:
        key = f"net_h{h}"
        vals = [t[key] / 100 for t in trades if key in t and t.get("buy_date", "") >= MIN_DATE]
        results[h] = stats(vals)
    return results, len([t for t in trades if t.get("buy_date", "") >= MIN_DATE])


def format_report(s1_all_trades, wd_all_records):
    """비교 리포트 생성"""
    s1_stats, s1_count = strategy1_stats(s1_all_trades)

    # WD 분류
    wd_break = [r for r in wd_all_records if r.get("has_break")]
    wd_nobreak = [r for r in wd_all_records if r.get("has_defbox") and not r.get("has_break")]
    all_wd = [r for r in wd_all_records if r.get("has_defbox")]

    msg = "Strategy1(전체조건) vs W+DefBox 수익률 비교\n"
    msg += "기준: 4개국 전종목 / 2024-01-01~\n"
    msg += "─" * 45 + "\n"

    # Strategy1 전체
    msg += f"\n[Strategy1] 전체 룰 엔진 ({s1_count}건) — buy진입 기준\n"
    for h in HORIZONS:
        mn, med, wr = s1_stats[h]
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD 전체 (DefBox 있는 것만)
    msg += f"\n[WD] W+DefBox 신호 전체 ({len(all_wd)}건) — W진입 기준\n"
    for h in HORIZONS:
        key = f"r_w{h}"
        vals = [r[key] for r in all_wd if key in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD+돌파 (W진입 기준)
    msg += f"\n[WD+돌파] 20일내 DefBox돌파 ({len(wd_break)}건) — W진입 기준\n"
    for h in HORIZONS:
        key = f"r_w{h}"
        vals = [r[key] for r in wd_break if key in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD 블렌드 h=20
    blended = []
    for r in wd_break:
        rw = r.get("r_w20")
        rd = r.get("r_def20")
        if rw is not None and rd is not None:
            blended.append(0.5 * rw + 0.5 * rd)
    if blended:
        mn, med, wr = stats(blended)
        msg += f"\n[WD 블렌드] 50%W+50%DefBox돌파 (h=20, n={len(blended)}건)\n"
        msg += f"  평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # WD 미돌파
    msg += f"\n[WD 미돌파] 20일내 미돌파 ({len(wd_nobreak)}건) — W진입만\n"
    for h in HORIZONS:
        key = f"r_w{h}"
        vals = [r[key] for r in wd_nobreak if key in r]
        mn, med, wr = stats(vals)
        msg += f"  h={h:2d}: 평균{mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}%\n"

    # 핵심 비교 요약
    msg += f"\n─── 비교 요약 (h=20) ─────────────────\n"
    s1_mn, s1_med, s1_wr = s1_stats[20]
    msg += f"Strategy1(전조건): {s1_mn*100:+.2f}% 중간{s1_med*100:+.2f}% 승률{s1_wr*100:.1f}% ({s1_count}건)\n"

    if wd_break:
        wd_vals = [r["r_w20"] for r in wd_break if "r_w20" in r]
        wd_mn, wd_med, wd_wr = stats(wd_vals)
        msg += f"WD+돌파(W진입): {wd_mn*100:+.2f}% 중간{wd_med*100:+.2f}% 승률{wd_wr*100:.1f}% ({len(wd_break)}건)\n"
    if blended:
        mn, med, wr = stats(blended)
        msg += f"WD 블렌드:      {mn*100:+.2f}% 중간{med*100:+.2f}% 승률{wr*100:.1f}% ({len(blended)}건)\n"

    print(msg)
    _tg_msg(msg)
    return msg


if __name__ == "__main__":
    # (market, wd_flag, s1_flag, fetch_fn)
    # strategy_study: KR=기본(hannam), 나머지=--foreign-*
    # wdefbox_scan: KR=기본, 나머지=--foreign-*
    market_configs = [
        ("KR", "",             "",             fetch_kr),
        ("JP", "--foreign-jp", "--foreign-jp", fetch_foreign),
        ("CN", "--foreign-cn", "--foreign-cn", fetch_foreign),
        ("HK", "--foreign-hk", "--foreign-hk", fetch_foreign),
    ]

    # Strategy1: 각 시장 이벤트 스터디 실행
    s1_all_trades = []
    for market, _, s1_flag, _ in market_configs:
        out_json = f"/tmp/s1_study_{market}.json"
        print(f"\n[{market}] Strategy1 스캔...", flush=True)
        trades = run_strategy1_scan(s1_flag, out_json)
        print(f"[{market}] Strategy1 완료 — {len(trades)}건 trade")
        s1_all_trades.extend(trades)

    # WD: 각 시장 스캔 + forward return 계산
    print("\n--- WD 스캔 시작 ---")
    wd_all_records = []
    for market, wd_flag, _, fetch_fn in market_configs:
        records = run_wd_study(market, wd_flag, fetch_fn)
        wd_all_records.extend(records)

    format_report(s1_all_trades, wd_all_records)
