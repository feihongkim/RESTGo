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
#       ④ 이벤트 건별 적재(StrategyTradeLog: 오늘 매수/매도, 멱등) ⑤ 밀도 게이트 판정
#       ⑥ 요약(zpicture/daily_summary_YYYYMMDD.txt — 매수/매도/게이트)
#
# 주의: 매도 알림은 "분석 창(250일) 신호대로 매수했다면"의 시뮬레이션 포지션 기준 —
#       실계좌 보유와 다를 수 있음 (부분 진입·게이트 스킵 등). 실보유 대사는 StrategyTradeLog로.
#
# 캔들 소스: hannam (2026-07-09 전환 — KIS2 일봉 적재 지연). 종목명만 KIS2 KospiCode 보조.
# cron 예 (host, hannam 일봉 적재 완료 시각에 맞춰 조정):
#   30 16 * * 1-5  cd /home/feihong/code/REST/RESTGo && ./daily_batch.sh >> zpicture/daily_batch.log 2>&1
set -uo pipefail
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

# ③④ 신호 수·이벤트 적재 SQL 생성 (오늘 이벤트 추출)
python3 - "$TODAY" <<'PY'
import json, sys
today = sys.argv[1]
def esc(x): return str(x).replace("'", "''")
strat = {'daily_s03s23': 'S1_S03S23', 'daily_wdefbox': 'W_DefBoxGravity'}
counts, rows = {}, []
for f, name in strat.items():
    d = json.load(open('zpicture/%s.json' % f))
    n = 0
    for s in d['stocks']:
        for g in s['signals']:
            if g['date'] == today:
                n += 1
                rows.append("('%s','%s',N'%s','BUY','%s',N'%s',1.0,NULL,NULL)" % (
                    name, esc(s['shcode']), esc(s['hname']), today, esc(g['reason'])))
        for e in s.get('sells') or []:
            if e['sell_date'] == today:
                rows.append("('%s','%s',N'%s','SELL','%s',N'%s',%.4f,%.4f,'%s')" % (
                    name, esc(s['shcode']), esc(s['hname']), today, esc(e['reason']),
                    e['weight'], e['net_return_pct'], esc(e['buy_date'])))
    counts[name] = n
sql = []
for name, n in counts.items():
    sql.append("DELETE FROM StrategySignalDaily WHERE strategy='%s' AND trade_date='%s'" % (name, today))
    if n > 0:
        sql.append("INSERT INTO StrategySignalDaily (strategy, trade_date, signal_count) VALUES ('%s','%s',%d)" % (name, today, n))
# EOD(확정 재계산)만 교체 — LIVE(실시간 실체결 근거)는 절대 건드리지 않음 (2026-07-09 규약)
sql.append("DELETE FROM StrategyTradeLog WHERE trade_date='%s' AND source='EOD'" % today)
if rows:
    sql.append("INSERT INTO StrategyTradeLog (strategy, shcode, hname, event_type, trade_date, reason, weight, net_return_pct, buy_date, source) VALUES " + ",".join(r[:-1] + ",'EOD')" for r in rows))
open('/tmp/daily_batch_load.sql', 'w').write('\n'.join(sql) + '\n')  # 개행 필수 — while read는 개행 없는 마지막 줄을 버림
print('[적재 준비] 오늘 매수 신호: %s, 이벤트 행: %d' % (counts, len(rows)))
PY
while IFS= read -r Q; do
  [ -n "$Q" ] && ./RESTGo sqlquery -db han "$Q" >/dev/null
done < /tmp/daily_batch_load.sql
echo "[적재] StrategySignalDaily + StrategyTradeLog 완료"

# ⑤ 밀도 게이트 판정
GATE=$(./RESTGo stock densitygate "$TODAY" 2>&1 | grep '\[densitygate\]' || true)

# ⑥ 요약 (매수 + 매도 + 게이트)
python3 - "$TODAY" "$SUM" <<'PY'
import json, sys
today, out = sys.argv[1], sys.argv[2]
def load(path):
    d = json.load(open(path))
    buys = [(s['shcode'], s['hname'], g['reason']) for s in d['stocks'] for g in s['signals'] if g['date'] == today]
    sells = [(s['shcode'], s['hname'], e) for s in d['stocks'] for e in (s.get('sells') or []) if e['sell_date'] == today]
    return buys, sells
a_b, a_s = load('zpicture/daily_s03s23.json')
b_b, b_s = load('zpicture/daily_wdefbox.json')
L = [f"===== 일일 신호 요약 {today} =====", ""]
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
