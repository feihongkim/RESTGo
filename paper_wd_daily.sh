#!/usr/bin/env bash
# paper_wd_daily.sh — B슬리브 WD Paper 트레이딩 일일 스캔 (2026-07-11)
#
# 매일 장 마감 후 실행하여:
#   ① 3시장(KR/CN/HK) WD 신호 스캔 → 신규 진입 + Telegram 알림
#   ② 기존 포지션 DefBox 돌파 체크 → stage2 진입 + 알림
#   ③ 만기 도래 포지션 청산 → 실현손익 + 알림
#   ④ 원장(ledger.json) 갱신
#
# cron 예 (host, hannam+KIS2 일봉 적재 완료 시각에 맞춰):
#   45 16 * * 1-5  cd /home/feihong/code/REST/RESTGo && ./paper_wd_daily.sh >> zpicture/paper_wd/daily.log 2>&1
#
# 월간 리포트:
#   ./RESTGo stock paper_wd_report [--month YYYYMM]
#
# KIS2 장애 시 KR(hannam) 단독 degrade 모드 (CN/HK 자동 스킵)
export RESTGO_DEGRADE_KIS2=true
set -uo pipefail
cd "$(dirname "$0")"

TODAY="${1:-$(date +%Y%m%d)}"
mkdir -p zpicture/paper_wd

echo "===== paper_wd_daily ${TODAY} ====="
./RESTGo stock paper_wd --date "$TODAY" 2>&1
echo ""
echo "완료: $(date)"
