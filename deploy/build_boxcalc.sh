#!/usr/bin/env bash
# build_boxcalc.sh — boxcalc 정적 바이너리 빌드 + 소비자 배포
#
# boxcalc는 Box 로직 단일 소스(box/·stg/)의 외부 소비자용 래퍼다:
#   - py/analysis/box_chart.py (프로젝트 루트 ./boxcalc 사용)
#   - MakeSQL/chart (makesql_chart 컨테이너, bin/boxcalc COPY)
#
# box/·stg/ 로직 변경 시 이 스크립트를 다시 실행하고,
# makesql_chart는 이미지 재빌드해야 반영된다:
#   docker compose -f docker-compose.yml build && docker compose up -d   (MakeSQL/chart에서)
set -euo pipefail
cd "$(dirname "$0")/.."

echo "[1/2] boxcalc 정적 빌드"
CGO_ENABLED=0 go build -ldflags="-s -w" -o boxcalc ./cmd/boxcalc
ls -la boxcalc

MAKESQL_CHART_BIN="/home/feihong/code/MakeSQL/chart/bin"
if [ -d "$(dirname "$MAKESQL_CHART_BIN")" ]; then
    echo "[2/2] MakeSQL/chart/bin 배포"
    mkdir -p "$MAKESQL_CHART_BIN"
    cp boxcalc "$MAKESQL_CHART_BIN/boxcalc"
    echo "  → $MAKESQL_CHART_BIN/boxcalc (이미지 재빌드 필요)"
else
    echo "[2/2] MakeSQL/chart 미존재 — 배포 생략"
fi
