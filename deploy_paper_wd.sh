#!/usr/bin/env bash
# deploy_paper_wd.sh — B슬리브 WD Paper 트레이딩 배포 스크립트
# host(white, 192.168.3.120)에서 실행
set -uo pipefail
HOST_DIR="/home/feihong/code/REST/RESTGo"

echo "=== B슬리브 WD Paper 배포 ==="
echo "대상: ${HOST_DIR}"

# 1. 바이너리
cp RESTGo "${HOST_DIR}/RESTGo" && echo "[OK] 바이너리"

# 2. 데일리 스캔 스크립트
cp paper_wd_daily.sh "${HOST_DIR}/" && chmod +x "${HOST_DIR}/paper_wd_daily.sh" && echo "[OK] paper_wd_daily.sh"

# 3. 원장 디렉토리
mkdir -p "${HOST_DIR}/zpicture/paper_wd" && echo "[OK] zpicture/paper_wd/"

echo ""
echo "=== 설치 완료 ==="
echo ""
echo "초기 실행 (첫 스캔):"
echo "  cd ${HOST_DIR} && ./paper_wd_daily.sh \$(date +%Y%m%d)"
echo ""
echo "월간 리포트:"
echo "  cd ${HOST_DIR} && ./RESTGo stock paper_wd_report [--month YYYYMM]"
echo ""
echo "cron 등록 (매일 장 마감 후):"
echo "  45 16 * * 1-5  cd ${HOST_DIR} && ./paper_wd_daily.sh >> zpicture/paper_wd/daily.log 2>&1"
echo ""
echo "상세 설정 (필요 시 수정):"
echo "  stock/paper_wd.go 상수: 자본, 종목당비중, 비용bp, FX환율"
