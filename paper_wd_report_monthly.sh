#!/usr/bin/env bash
# paper_wd_report_monthly.sh — B슬리브 WD Paper 월간 리포트 (매달 1일 실행)
#
# 스케줄러(scheduler/tasks.go)가 매달 1일 09:00에 호출한다.
# 수동 실행: ./paper_wd_report_monthly.sh [YYYYMM]
#
# 대상 월은 "직전 달"이다. `stock paper_wd_report`의 --month 기본값은 실행 시점의
# 당월이라, 1일에 그대로 부르면 갓 시작한 이번 달(체결 0건)을 집계한다.
# 그래서 여기서 직전 달을 계산해 명시적으로 넘긴다.
#
# 출력: zpicture/paper_wd/report_YYYYMM.json + 콘솔 요약
set -uo pipefail
export RESTGO_DEGRADE_KIS2=true
cd "$(dirname "$0")"

MONTH="${1:-$(date -d "$(date +%Y-%m-01) -1 day" +%Y%m)}"

echo "===== paper_wd_report ${MONTH} (실행 $(date +%Y%m%d_%H%M%S)) ====="
./RESTGo stock paper_wd_report --month "$MONTH"
RC=$?
echo "완료: $(date)  (exit ${RC})"
exit $RC
