#!/usr/bin/env bash
set -euo pipefail
source scripts/diploma-metrics-lib.sh

make_out_dir wait-analytics-quiet
need docker
wait_analysis_quiet
write_summary_header "Analytics quiet check"
append_file_block "Analytics quiet check" "$OUT_DIR/analytics-quiet.txt"

log "Done"
printf '\nResults directory: %s\nSummary: %s\n' "$OUT_DIR" "$OUT_DIR/summary.md"
