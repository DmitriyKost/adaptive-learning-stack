#!/usr/bin/env bash
set -euo pipefail
source scripts/diploma-metrics-lib.sh

make_out_dir progress-load
need curl
need jq
need docker
check_file scripts/measure-api-load.sh
check_gateway_ready

REQUESTS="${REQUESTS:-100}"
CONCURRENCY="${CONCURRENCY:-10}"

trap stop_sampler EXIT
log "API load test: /progress/me"
start_sampler progress-api
BASE="$BASE" REQUESTS="$REQUESTS" CONCURRENCY="$CONCURRENCY" \
  bash scripts/measure-api-load.sh | tee "$OUT_DIR/api-load.txt"
stop_sampler

write_summary_header "/progress/me load"
append_file_block "API load" "$OUT_DIR/api-load.txt"
echo "Raw docker stats samples: resources-progress-api.tsv" >> "$OUT_DIR/summary.md"

log "Done"
printf '\nResults directory: %s\nSummary: %s\n' "$OUT_DIR" "$OUT_DIR/summary.md"
