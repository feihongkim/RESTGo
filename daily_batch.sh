#!/usr/bin/env bash
# daily_batch.sh — 일일 운용 배치 (2026-07-07 확정 운용 4종 YAML + 매도 알림·DB 적재 v2)
#
# 운용 전략 2쌍 (사용자 확정 2026-07-07):
#   A. strategy1 축소판: rules/strategy1_s03s23.yaml + rules/sell_s03s23.yaml
#      (S03+S23 + 엔진 REST2 후속, 익절+만료 매도 — A/B: +3.15%/체결 PF 2.02)
#   B. W중력:            rules/buy_wdefbox.yaml     + rules/sell_wdefbox.yaml
#      (진입은 밀도 게이트 PASS 시에만 유효)
#
# 흐름: ① A배치 ② B배치 ③ 신호 수 적재(StrategySignalDaily: W_DefBoxGravity + S1_S03S23)
#       ④ 이벤트 건별 적재(StrategyTradeLog: 창 전체 매수/매도, 멱등 MERGE) ⑤ 밀도 게이트 판정
#       ⑥ 요약(zpicture/daily_summary_YYYYMMDD.txt — 매수/매도/게이트)
#
# 기준일 규약 (2026-07-30 개정):
#   DATA_DATE = 분석 창의 마지막 봉 날짜 (batch JSON의 data_date). 달력 오늘이 아니다.
#   일봉 적재가 지연되면 달력 오늘 신호는 구조적으로 0건이 되므로, "오늘 신호"는
#   반드시 DATA_DATE 기준으로 판정한다. (구버전은 달력 today로 걸러 신호를 통째로 버렸다)
#
# 적재 규약 (2026-07-30 개정 — 창 전체 기록):
#   StrategyTradeLog — 분석 창(250일) 안의 모든 매수/매도를 기록한다. 신호는 실행 간
#     불변이 실측 검증됨(35종목×3절단=105건 비교, 불일치 0)이라 재실행해도 값이 흔들리지 않는다.
#     source: EOD  = trade_date == DATA_DATE (그 배치가 확정한 당일분, 기존 규약 유지)
#             HIST = trade_date <  DATA_DATE (회고 재계산분)
#             LIVE = listen 모드 실시간분 — 이 스크립트는 절대 건드리지 않는다
#     as_of_date = 최초 관측일. MERGE 시 MIN(기존, 신규)로 유지되므로
#       "언제 처음 이 신호를 볼 수 있었나"가 보존된다 (as_of_date - trade_date = 적재 지연).
#   StrategySignalDaily — 밀도 게이트(stg/overlay_density.go)의 입력이다. 과거 날짜를
#     소급 덮어쓰면 분위수 임계값이 움직여 과거 게이트 판정이 바뀌므로, 회고분은
#     비어 있는 날짜만 채운다(WHEN NOT MATCHED). DATA_DATE 행만 확정 교체한다.
#     신호 0건인 거래일도 count=0 행으로 남겨 "0건"과 "미기록"을 구분한다
#     (0행은 밀도·임계 계산에 기여하지 않아 판정은 불변). as_of_date = 배치 실행일.
#
# 주의: 매도 알림은 "분석 창(250일) 신호대로 매수했다면"의 시뮬레이션 포지션 기준 —
#       실계좌 보유와 다를 수 있음 (부분 진입·게이트 스킵 등). 실보유 대사는 source='LIVE' 행으로.
#
# 캔들 소스: hannam (2026-07-09 전환 — KIS2 일봉 적재 지연). 종목명만 KIS2 KospiCode 보조.
# cron 예 (host, hannam 일봉 적재 완료 시각에 맞춰 조정):
#   30 16 * * 1-5  cd /home/feihong/code/RESTGo && ./daily_batch.sh >> zpicture/daily_batch.log 2>&1
set -uo pipefail
export RESTGO_DEGRADE_KIS2=true
cd "$(dirname "$0")"
TODAY=$(date +%Y%m%d)
DAYS=${1:-250}
SUM=zpicture/daily_summary_${TODAY}.txt

echo "===== daily_batch ${TODAY} (일수 ${DAYS}) ====="

# ① strategy1 축소판 배치
RESTGO_BUY_RULES=rules/strategy1_s03s23.yaml RESTGO_SELL_RULES=rules/sell_s03s23.yaml \
  ./RESTGo stock batch "$DAYS" zpicture/daily_s03s23.json || { echo "[오류] s03s23 배치 실패"; exit 1; }

# ② W중력 배치
RESTGO_BUY_RULES=rules/buy_wdefbox.yaml RESTGO_SELL_RULES=rules/sell_wdefbox.yaml \
  ./RESTGo stock batch "$DAYS" zpicture/daily_wdefbox.json || { echo "[오류] wdefbox 배치 실패"; exit 1; }

# ③④ 신호 수·이벤트 적재 SQL 생성 (창 전체 — 위 "적재 규약" 참조)
python3 - "$TODAY" <<'PY'
import json, sys
run_date = sys.argv[1]          # 배치 실행일 (as_of_date)
def esc(x): return str(x).replace("'", "''")
strat = {'daily_s03s23': 'S1_S03S23', 'daily_wdefbox': 'W_DefBoxGravity'}

data_date = ''
calendar = set()                # 거래일 달력 (batch JSON의 trading_dates 합집합)
counts = {}                     # (strategy, trade_date) -> 매수 신호 수
events = {}                     # 멱등 키 -> 행 튜플 (MERGE는 중복 소스행에서 실패하므로 선-중복제거)
for f, name in strat.items():
    d = json.load(open('zpicture/%s.json' % f))
    dd = d.get('data_date') or ''
    if dd > data_date:
        data_date = dd
    calendar.update(d.get('trading_dates') or [])
    for s in d['stocks']:
        for g in s['signals']:
            counts[(name, g['date'])] = counts.get((name, g['date']), 0) + 1
            events[(name, s['shcode'], g['date'], 'BUY', esc(g['reason']), '')] = (
                name, s['shcode'], s['hname'], 'BUY', g['date'], g['reason'], 1.0, None, None)
        for e in s.get('sells') or []:
            events[(name, s['shcode'], e['sell_date'], 'SELL', esc(e['reason']), e['buy_date'])] = (
                name, s['shcode'], s['hname'], 'SELL', e['sell_date'], e['reason'],
                e['weight'], e['net_return_pct'], e['buy_date'])

if not data_date:
    print('[오류] batch JSON에 data_date가 없습니다 — RESTGo 재빌드 필요', file=sys.stderr)
    sys.exit(1)

# DATA_DATE 이후 이벤트는 버린다. 적재가 진행 중일 때 선두 소수 종목만 최신 봉을 가지므로,
# 그 구간 신호는 "23종목만의 시장"이라 대표성이 없다. 게다가 HIST로 넣어두면 나중에
# 데이터가 다 찬 뒤 같은 신호가 EOD로 다시 들어와 중복된다 (MERGE 키에 source가 있음).
dropped = [k for k in events if k[2] > data_date]
for k in dropped:
    del events[k]
counts = {k: v for k, v in counts.items() if k[1] <= data_date}

def lit(v, quote=True, n=False):
    if v is None: return 'NULL'
    if isinstance(v, float): return '%.4f' % v
    return ("N'%s'" if n else "'%s'") % esc(v)

sql = []

# ③ StrategySignalDaily — DATA_DATE 행만 확정 교체, 과거는 빈 날짜만 채움(게이트 입력 보호)
#
# 신호가 0건인 거래일에도 count=0 행을 남긴다. 이 테이블은 "0건인 날"과
# "배치가 안 돈 날"을 구분하지 못해(둘 다 행 없음) 게이트가 미기록을 신호 가뭄으로
# 오독한다 — 2026-07-30 실제 오판정의 원인. 0행은 밀도 합산에도, 임계 표본(신호 수
# 가중)에도 기여하지 않으므로 기존 판정을 바꾸지 않는다.
for name in strat.values():
    sql.append("DELETE FROM StrategySignalDaily WHERE strategy='%s' AND trade_date='%s'" % (name, data_date))
    sql.append("INSERT INTO StrategySignalDaily (strategy, trade_date, signal_count, as_of_date) "
               "VALUES ('%s','%s',%d,'%s')" % (name, data_date, counts.get((name, data_date), 0), run_date))

cal_days = sorted(d for d in calendar if d < data_date)
hist_counts = [(name, day, counts.get((name, day), 0))
               for name in strat.values() for day in cal_days]
for i in range(0, len(hist_counts), 400):
    vals = ",".join("('%s','%s',%d,'%s')" % (c[0], c[1], c[2], run_date) for c in hist_counts[i:i+400])
    sql.append(
        "MERGE StrategySignalDaily AS t USING (VALUES %s) AS s(strategy, trade_date, signal_count, as_of_date) "
        "ON t.strategy = s.strategy AND t.trade_date = s.trade_date "
        "WHEN NOT MATCHED THEN INSERT (strategy, trade_date, signal_count, as_of_date) "
        "VALUES (s.strategy, s.trade_date, s.signal_count, s.as_of_date);" % vals)

# ④ StrategyTradeLog — 창 전체 멱등 MERGE. LIVE 행은 source가 달라 매칭되지 않으므로 안전.
rows = sorted(events.values(), key=lambda r: (r[0], r[4], r[1]))
for i in range(0, len(rows), 400):
    vals = ",".join(
        "(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)" % (
            lit(r[0]), lit(r[1]), lit(r[2], n=True), lit(r[3]), lit(r[4]), lit(r[5], n=True),
            lit(r[6]), lit(r[7]), lit(r[8]),
            lit('EOD' if r[4] == data_date else 'HIST'), lit(run_date))
        for r in rows[i:i+400])
    sql.append(
        "MERGE StrategyTradeLog AS t USING (VALUES %s) AS s(strategy, shcode, hname, event_type, "
        "trade_date, reason, weight, net_return_pct, buy_date, source, as_of_date) "
        "ON t.strategy = s.strategy AND t.shcode = s.shcode AND t.trade_date = s.trade_date "
        "AND t.event_type = s.event_type AND t.source = s.source AND t.reason = s.reason "
        "AND ISNULL(t.buy_date,'') = ISNULL(s.buy_date,'') "
        # as_of_date는 최초 관측일을 유지한다 (재실행이 첫 관측 시점을 밀어내지 않도록)
        "WHEN MATCHED THEN UPDATE SET hname = s.hname, weight = s.weight, "
        "net_return_pct = s.net_return_pct, "
        "as_of_date = CASE WHEN t.as_of_date IS NULL OR s.as_of_date < t.as_of_date "
        "THEN s.as_of_date ELSE t.as_of_date END "
        "WHEN NOT MATCHED THEN INSERT (strategy, shcode, hname, event_type, trade_date, reason, "
        "weight, net_return_pct, buy_date, source, as_of_date) VALUES (s.strategy, s.shcode, s.hname, "
        "s.event_type, s.trade_date, s.reason, s.weight, s.net_return_pct, s.buy_date, s.source, "
        "s.as_of_date);" % vals)

open('/tmp/daily_batch_load.sql', 'w').write('\n'.join(sql) + '\n')  # 개행 필수 — while read는 개행 없는 마지막 줄을 버림
open('/tmp/daily_batch_data_date', 'w').write(data_date + '\n')
n_eod = sum(1 for r in rows if r[4] == data_date)
lag = (int(run_date) - int(data_date))
print('[적재 준비] 기준일(DATA_DATE) %s (실행일 %s)  창 전체 이벤트 %d행 (당일분 EOD %d행)'
      % (data_date, run_date, len(rows), n_eod))
if dropped:
    print('[적재 준비] 기준일 이후 이벤트 %d건 제외 (부분 적재 구간 — 데이터가 다 차면 다음 실행에 포함)'
          % len(dropped))
if data_date != run_date:
    print('[경고] 일봉 적재 지연 — 분석 기준일이 실행일보다 과거입니다 (%s < %s). '
          '당일 신호는 %s 기준으로 판정됩니다.' % (data_date, run_date, data_date))
PY
[ ${PIPESTATUS[0]:-0} -eq 0 ] || { echo "[오류] 적재 SQL 생성 실패"; exit 1; }
DATA_DATE=$(cat /tmp/daily_batch_data_date)

LOAD_FAIL=0
while IFS= read -r Q; do
  [ -n "$Q" ] || continue
  if ! ./RESTGo sqlquery -db han "$Q" >/dev/null; then
    LOAD_FAIL=$((LOAD_FAIL + 1))
    echo "[오류] 적재 SQL 실패: ${Q:0:120}..."
  fi
done < /tmp/daily_batch_load.sql
if [ "$LOAD_FAIL" -gt 0 ]; then
  echo "[적재] ★ 실패 ${LOAD_FAIL}건 — StrategySignalDaily/StrategyTradeLog 부분 적재 상태"
else
  echo "[적재] StrategySignalDaily + StrategyTradeLog 완료 (기준일 ${DATA_DATE})"
fi

# ⑤ 밀도 게이트 판정 — 기준일은 DATA_DATE.
# 달력 오늘로 판정하면 적재 지연 구간이 "신호 0일"로 계산돼 밀도가 인위적으로 낮아진다.
GATE=$(./RESTGo stock densitygate "$DATA_DATE" 2>&1 | grep '\[densitygate\]' || true)

# ⑥ 요약 (매수 + 매도 + 게이트) — DATA_DATE 기준 당일분
python3 - "$DATA_DATE" "$SUM" "$TODAY" <<'PY'
import json, sys
today, out = sys.argv[1], sys.argv[2]   # today = DATA_DATE (분석 기준일)
run_date = sys.argv[3]
def load(path):
    d = json.load(open(path))
    buys = [(s['shcode'], s['hname'], g['reason']) for s in d['stocks'] for g in s['signals'] if g['date'] == today]
    sells = [(s['shcode'], s['hname'], e) for s in d['stocks'] for e in (s.get('sells') or []) if e['sell_date'] == today]
    return buys, sells
a_b, a_s = load('zpicture/daily_s03s23.json')
b_b, b_s = load('zpicture/daily_wdefbox.json')
L = [f"===== 일일 신호 요약 {today} =====", ""]
if today != run_date:
    L.append(f"※ 일봉 적재 지연 — 분석 기준일 {today}, 배치 실행일 {run_date}")
    L.append("")
def sect(title, buys, sells, note=""):
    L.append(title)
    L.append(f"  매수 {len(buys)}건" + (":" if buys else ""))
    L.extend(f"    {c} {n} ({r})" for c, n, r in buys)
    L.append(f"  매도 {len(sells)}건 (시뮬레이션 포지션 기준)" + (":" if sells else ""))
    L.extend(f"    {c} {n} — {e['reason']} w={e['weight']:.2f} ({e['buy_date']} 매수분, {e['net_return_pct']:+.2f}%)" for c, n, e in sells)
    if note: L.append(f"  {note}")
    L.append("")
sect("[A] strategy1 축소판 (s03s23+REST2)", a_b, a_s)
sect("[B] W중력 (wdefbox)", b_b, b_s, "※ 신규 진입은 밀도 게이트 PASS 시에만")
open(out, 'w').write('\n'.join(L) + '\n')
print('\n'.join(L))
PY
echo "$GATE" | tee -a "$SUM"
echo "요약 저장: $SUM"
