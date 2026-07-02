#!/usr/bin/env bash
set -euo pipefail
source scripts/diploma-metrics-lib.sh

make_out_dir execute-load
need curl
need jq
need docker
check_file scripts/measure-execute-load.sh
check_gateway_ready

REQUESTS="${REQUESTS:-300}"
CONCURRENCY="${CONCURRENCY:-20}"
USERS="${USERS:-10}"
TASK_LIMIT="${TASK_LIMIT:-5}"
MODE="${MODE:-wrong}"

if [ "$MODE" != "wrong" ]; then
  cat >&2 <<MSG
warning: MODE=$MODE will include successful task completions and may create LLM backlog.
For isolated API latency/RPS measurements use the default MODE=wrong.
MSG
fi

trap stop_sampler EXIT
log "Execute load test: /playground/execute"
start_sampler execute-api
BASE="$BASE" REQUESTS="$REQUESTS" CONCURRENCY="$CONCURRENCY" USERS="$USERS" TASK_LIMIT="$TASK_LIMIT" MODE="$MODE" \
  bash scripts/measure-execute-load.sh | tee "$OUT_DIR/execute-load.txt"
stop_sampler

write_summary_header "/playground/execute load"
append_file_block "Execute load" "$OUT_DIR/execute-load.txt"
echo "Raw docker stats samples: resources-execute-api.tsv" >> "$OUT_DIR/summary.md"

log "Done"
printf '\nResults directory: %s\nSummary: %s\n' "$OUT_DIR" "$OUT_DIR/summary.md"
