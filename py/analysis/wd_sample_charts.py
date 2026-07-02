"""WD+DefBox 대표 예시차트 5개 생성 + 텔레그램 전송
wd_hannam_16y_v2.json 돌파 신호 중 사이클별 대표 5건 선택
"""
import sys, os, json
import numpy as np

_here = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _here)
from wbottom_chart import fetch_kr, _tg_send_files
from draw_wdefbox_charts import draw_from_wdefbox

_PROJ_ROOT = os.path.dirname(os.path.dirname(_here))
SCAN_JSON  = os.path.join(_PROJ_ROOT, "zpicture", "wd_hannam_16y_v2.json")
OUT_DIR    = "/tmp/wd_sample_charts"

BEAR_YEARS = {2011, 2015, 2018, 2022}
BULL_YEARS = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS = {2012, 2013, 2014, 2016, 2017, 2019, 2023}

def run():
    with open(SCAN_JSON) as f:
        data = json.load(f)
    examples = data.get("examples", [])

    # 돌파 신호만 (post-2010)
    breaks = [e for e in examples
              if e.get("has_defbox") and e.get("defbox_break_date")
              and e.get("signal_date", "") >= "20100101"]
    print(f"돌파 신호: {len(breaks)}건")

    # 사이클별 그룹화 후 각 그룹 최고 수익률 1건 선택
    def pick_best(group):
        return max(group, key=lambda e: e.get("r_w20", -999))

    bear = [e for e in breaks if int(e["signal_date"][:4]) in BEAR_YEARS]
    bull = [e for e in breaks if int(e["signal_date"][:4]) in BULL_YEARS]
    side = [e for e in breaks if int(e["signal_date"][:4]) in SIDE_YEARS]

    selected = []
    for seg, grp in [("약세", bear), ("강세", bull), ("사이드1", side[:len(side)//2]), ("사이드2", side[len(side)//2:])]:
        if grp:
            best = pick_best(grp)
            selected.append((seg, best))
            print(f"  [{seg}] {best['shcode']}  진입:{best['signal_date']}  r_w20:{best.get('r_w20',0)*100:+.1f}%")

    # 5번째: 전체에서 가장 최근
    recent = sorted(breaks, key=lambda e: e["signal_date"], reverse=True)[0]
    selected.append(("최근", recent))
    print(f"  [최근] {recent['shcode']}  진입:{recent['signal_date']}  r_w20:{recent.get('r_w20',0)*100:+.1f}%")

    os.makedirs(OUT_DIR, exist_ok=True)
    files = []
    for seg, ex in selected:
        print(f"\n차트 생성: [{seg}] {ex['shcode']} {ex['signal_date']}")
        # out_dir 파일명에 세그먼트 포함
        out_dir_seg = OUT_DIR
        # draw_from_wdefbox가 {out_dir}/{market}_{shcode}.png 로 저장
        # 중복 방지를 위해 market 임시 변경
        ex_copy = dict(ex)
        ex_copy["market"] = f"KR_{seg}"
        path = draw_from_wdefbox(ex_copy, fetch_kr, out_dir_seg)
        if path:
            files.append(path)
            print(f"  → {path}")
        else:
            print(f"  → 실패")

    print(f"\n총 {len(files)}개 차트 생성, 텔레그램 전송 중...")
    _tg_send_files(files)
    print("전송 완료")

if __name__ == "__main__":
    run()
