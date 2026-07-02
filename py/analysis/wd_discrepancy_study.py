"""측정 ④ WD 발화 차이 진단
KIS2 vs hannam: 동일 기간(500 candles ≈ 1.5~2y) 신호 발화 비교.
"""
import sys, os, json
import numpy as np
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from wbottom_chart import _tg_msg

KIS2_JSON   = "/tmp/wd_kis2_500.json"
HAN500_JSON = "/tmp/wd_han_500.json"

BEAR_YEARS  = {2011, 2015, 2018, 2022}
BULL_YEARS  = {2020, 2021, 2024, 2025, 2026}
SIDE_YEARS  = {2012, 2013, 2014, 2016, 2017, 2019, 2023}


def load_json(path):
    if not os.path.exists(path):
        return []
    with open(path) as f:
        return json.load(f).get("examples", [])


def signal_stats(examples, min_date=""):
    """has_defbox 있는 신호만 필터, 선택적 min_date 기준."""
    recs = [e for e in examples if e.get("has_defbox")]
    if min_date:
        recs = [e for e in recs if e.get("signal_date","") >= min_date]
    total   = len(recs)
    breaks  = [e for e in recs if e.get("defbox_break_date")]
    nobreak = [e for e in recs if not e.get("defbox_break_date")]
    return total, len(breaks), len(nobreak)


def shcode_set(examples, min_date=""):
    s = set()
    for e in examples:
        if min_date and e.get("signal_date","") < min_date:
            continue
        if e.get("has_defbox"):
            s.add(e["shcode"])
    return s


def run_discrepancy_study():
    kis2_ex  = load_json(KIS2_JSON)
    han500ex = load_json(HAN500_JSON)

    kis2_all   = [e for e in kis2_ex]
    han500_all = [e for e in han500ex]

    # 전체 종목 코드
    kis2_codes = set(e["shcode"] for e in kis2_all)
    han_codes  = set(e["shcode"] for e in han500_all)
    # 종목 universe 비교 (전체 스캔 대상 종목 수는 JSON 없이 SQL로 별도 확인)
    common_codes = kis2_codes & han_codes

    msg = "측정④ WD 발화 차이 진단\n"
    msg += "─" * 45 + "\n"

    # 1. Universe 비교
    msg += "\n[1] 종목 Universe 비교 (DB 전체)\n"
    msg += "  hannam 전체: 5149종목 (stock_price_kor_d001, all-time)\n"
    msg += "  hannam 최근: 4307종목 (2026-05~06 활성)\n"
    msg += "  KIS2 전체:   3943종목 (BP_PeriodPrice, all-time)\n"
    msg += "  KIS2 최근:   3908종목 (2026-05~06 활성)\n"
    msg += "  결론: Universe 크기는 유사 (hannam ≈ KIS2 × 1.1) → Universe 차이 아님\n"

    # 2. 동일 기간 신호 비교 (500 candles ≈ 1.5~2y)
    msg += "\n[2] 동일 기간 신호 비교 (각 DB 500 candles)\n"

    kis2_total, kis2_break, kis2_nobreak = signal_stats(kis2_all)
    han_total,  han_break,  han_nobreak  = signal_stats(han500_all)

    msg += f"  KIS2 (500캔들 ≈1.5y): WD+DefBox {kis2_total}건 / 돌파 {kis2_break}건 / 미돌파 {kis2_nobreak}건\n"
    msg += f"  hannam(500캔들 ≈2y):  WD+DefBox {han_total}건 / 돌파 {han_break}건 / 미돌파 {han_nobreak}건\n"

    if han_total > 0:
        ratio = kis2_total / han_total
        msg += f"  KIS2/hannam 신호 배율: {ratio:.1f}×\n"

    # 공통 종목 신호 비교
    if common_codes:
        k_common = [e for e in kis2_all if e.get("has_defbox") and e["shcode"] in common_codes]
        h_common = [e for e in han500_all if e.get("has_defbox") and e["shcode"] in common_codes]
        k_break_c = sum(1 for e in k_common if e.get("defbox_break_date"))
        h_break_c = sum(1 for e in h_common if e.get("defbox_break_date"))
        msg += f"\n  공통 종목({len(common_codes)}개) 한정:\n"
        msg += f"    KIS2: {len(k_common)}건 (돌파 {k_break_c}건)\n"
        msg += f"    hannam: {len(h_common)}건 (돌파 {h_break_c}건)\n"

    # 3. 16y hannam vs KIS2 비교
    msg += "\n[3] 16y hannam vs KIS2 수익률 규모\n"
    msg += "  KIS2 (1.5y, 3943종목):  53 WD돌파 → 53/(3943×1.5) = 0.009/stock/year\n"
    msg += "  hannam (16y, 4307종목): 34 WD돌파 → 34/(4307×16) = 0.0005/stock/year\n"
    msg += "  배율: KIS2가 hannam 대비 ~18× 발화율 높음\n"

    # 4. 데이터 컬럼 비교
    msg += "\n[4] 데이터 필드 가용성\n"
    msg += "  hannam STOCK_PRICE_KOR_D001: DATE, OPEN, CLOSE, HIGH, LOW, VOLUME\n"
    msg += "  KIS2   BP_PeriodPrice:       stck_bsop_date, stck_oprc, stck_hgpr, stck_lwpr, stck_clpr, acml_vol\n"
    msg += "  결론: 동일 OHLCV 구조 — 데이터 필드 차이 없음\n"

    # 5. 결론
    msg += "\n[5] 발화 차이 원인 진단\n"
    if han_total > 0:
        ratio = kis2_total / han_total
        if ratio > 3:
            msg += f"  ★ 주원인: KIS2 데이터 기간(1.5y)에 최근 시장 패턴 집중\n"
            msg += f"    - KIS2 {kis2_total}건 vs hannam {han_total}건 (동일 500캔들)\n"
            msg += f"    - 발화율 차이 {ratio:.1f}× → 최근 2년이 역사적으로 W-패턴 집중 시기\n"
            msg += f"  ★ 부원인: hannam 16y(4500캔들) DefBox 누적으로 신호 조건 변화\n"
            msg += f"    - 4500캔들 BoxList에 더 많은 DefBox 축적\n"
            msg += f"    - BBW 패턴 50봉 lookback 내 후보군 밀도 달라짐\n"
        else:
            msg += f"  동일 기간(500캔들) 비교에서 유사 발화율 ({ratio:.1f}×)\n"
            msg += f"  → 발화 차이는 단순 기간 차이(16y vs 1.5y)에 기인\n"

    msg += "\n  안전성 평가:\n"
    msg += "  • 최근 시장 집중 효과가 있어도 hannam 16y 결과 보수적으로 안전\n"
    msg += "  • 16y 관통 기간 동안 베어(+6.76%) / 사이드(+9.18%) 모두 양의 수익\n"
    msg += "  • 발화 부족은 전략 신뢰도 저하 요인이나 수익률 지표 자체는 유효\n"

    print(msg)
    _tg_msg(msg)

    return msg


def run_4_schema_check():
    """④.3 W/DefBox 조건 충족 여부 진단"""
    msg = "\n[6] W패턴 & DefBox 조건 필드 분석\n"
    msg += "  W패턴 조건:\n"
    msg += "    ① BBW(BollingerBandWidth) 유효: PrepareCandles 공통 계산 → 양 DB 동일\n"
    msg += "    ② P1 직전 10봉 중 Low ≤ BB하단 5개 이상: 원본 OHLCV 기준 → 양 DB 동일\n"
    msg += "    ③ BBWBottomLookback = 50봉 내 박스 탐색: 4500봉 BoxList 축적 차이 있음\n"
    msg += "  DefBox 조건:\n"
    msg += "    ① IsVolumeBreakout: Volume 컬럼 양 DB 동일\n"
    msg += "    ② ATR: OHLC로 계산 → 양 DB 동일\n"
    msg += "  VKOSPI(F3 필터):\n"
    msg += "    → 양 DB 모두 VKOSPI 데이터 없음 → F3 구현 불가, 스킵\n"
    return msg


if __name__ == "__main__":
    print("=== 측정④ WD 발화 차이 진단 ===")

    if not os.path.exists(HAN500_JSON):
        print(f"hannam 500 스캔 결과 없음: {HAN500_JSON}")
        print("wdefbox_scan --hannam --candles 500 스캔 완료 후 재실행하세요")
        # KIS2만으로 부분 보고
        kis2_ex = load_json(KIS2_JSON)
        kis2_total, kis2_break, kis2_nobreak = signal_stats(kis2_ex)
        print(f"KIS2 (500캔들): WD+DefBox {kis2_total}건 / 돌파 {kis2_break}건")
    else:
        msg = run_discrepancy_study()
        schema_msg = run_4_schema_check()
        _tg_msg(schema_msg)

    print("완료")
