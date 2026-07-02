#!/usr/bin/env bash
set -euo pipefail
source scripts/diploma-metrics-lib.sh

make_out_dir single-e2e
need curl
need jq
need docker
check_file scripts/measure-e2e-timing.sh
check_gateway_ready
check_intelligence_loaded

if [ "${WAIT_ANALYTICS_QUIET:-1}" = "1" ]; then
  wait_analysis_quiet
fi

trap stop_sampler EXIT
log "Single E2E timing"
start_sampler e2e
BASE="$BASE" bash scripts/measure-e2e-timing.sh | tee "$OUT_DIR/e2e-timing.txt"
stop_sampler

write_summary_header "Single E2E timing"
append_file_block "E2E timing" "$OUT_DIR/e2e-timing.txt"
if [ -f "$OUT_DIR/analytics-quiet.txt" ]; then
  append_file_block "Analytics quiet check" "$OUT_DIR/analytics-quiet.txt"
fi

echo "Raw docker stats samples: resources-e2e.tsv" >> "$OUT_DIR/summary.md"

log "Done"
printf '\nResults directory: %s\nSummary: %s\n' "$OUT_DIR" "$OUT_DIR/summary.md"
