"""KR 실패 사례 5건 차트 생성 및 텔레그램 전송"""
import sys
import os
import json

# wbottom_chart.py가 있는 폴더를 경로에 추가
_this_dir = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _this_dir)
from wbottom_chart import fetch_kr, _draw_from_signal, _tg_send_files

def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "fail"  # fail or success
    n = int(sys.argv[2]) if len(sys.argv) > 2 else 5

    study_json  = "/home/feihong/code/REST/RESTGo/zpicture/miiib_scan/return_study.json"
    scan_kr_json = "/tmp/miiib_study_KR.json"

    with open(study_json) as f:
        records = json.load(f)

    # KR만, r20 존재하는 것만
    kr = [r for r in records if r.get("market") == "KR" and "r20" in r]

    if mode == "fail":
        # r20 < 0, 가장 나쁜 순
        candidates = sorted([r for r in kr if r["r20"] < 0], key=lambda r: r["r20"])
    else:
        # r20 > 0, 가장 좋은 순
        candidates = sorted([r for r in kr if r["r20"] > 0], key=lambda r: r["r20"], reverse=True)

    print(f"[{mode}] 후보: {len(candidates)}건")

    # scan_kr_json에서 p1_date, p2_date 매핑 (shcode+signal_date 기준)
    p1p2_map = {}
    if os.path.exists(scan_kr_json):
        with open(scan_kr_json) as f:
            scan_data = json.load(f)
        for ex in scan_data.get("examples", []):
            key = (ex["shcode"], ex["signal_date"])
            p1p2_map[key] = (ex.get("p1_date", ""), ex.get("p2_date", ""))

    out_dir = "/tmp/miiib_fail_charts" if mode == "fail" else "/tmp/miiib_success_charts"
    os.makedirs(out_dir, exist_ok=True)

    files = []
    seen_shcode = set()
    for rec in candidates:
        if len(files) >= n:
            break
        shcode = rec["shcode"]
        sig_date = rec["date"]
        if shcode in seen_shcode:
            continue
        seen_shcode.add(shcode)

        p1_date, p2_date = p1p2_map.get((shcode, sig_date), ("", ""))
        ex = {
            "market": "KR",
            "shcode": shcode,
            "signal_date": sig_date,
            "p1_date": p1_date,
            "p2_date": p2_date,
        }
        r20 = rec["r20"] * 100
        print(f"  [{mode}] {shcode} {sig_date}  r20={r20:+.1f}%  p1={p1_date} p2={p2_date}")
        path = _draw_from_signal(ex, fetch_kr, out_dir)
        if path:
            files.append(path)

    print(f"\n차트 {len(files)}개 생성 → 텔레그램 전송")
    _tg_send_files(files)

if __name__ == "__main__":
    main()
