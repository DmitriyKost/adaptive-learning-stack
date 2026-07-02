#!/usr/bin/env bash
set -euo pipefail
source scripts/diploma-metrics-lib.sh

make_out_dir storage-snapshot
need docker

log "ClickHouse table sizes"
docker exec adaptive-clickhouse clickhouse-client \
  --user analytics \
  --password analytics \
  --database analytics \
  --query "
SELECT
  table,
  total_rows,
  formatReadableSize(total_bytes) AS bytes_on_disk
FROM
(
  SELECT
    table,
    sum(toUInt64OrZero(toString(rows))) AS total_rows,
    sum(toUInt64OrZero(toString(bytes_on_disk))) AS total_bytes
  FROM system.parts
  WHERE database = 'analytics' AND active = 1
  GROUP BY table
)
ORDER BY total_bytes DESC
FORMAT PrettyCompact
" | tee "$OUT_DIR/clickhouse-sizes.txt"

log "PostgreSQL task-progress row counts"
docker exec adaptive-task-progress-postgres psql \
  -U task_progress \
  -d task_progress \
  -tAc "
SELECT 'user_skills=' || count(*) FROM user_skills
UNION ALL SELECT 'user_task_status=' || count(*) FROM user_task_status
UNION ALL SELECT 'user_task_status_attempts_total=' || COALESCE(sum(attempts_count), 0) FROM user_task_status
UNION ALL SELECT 'processed_events=' || count(*) FROM processed_events
UNION ALL SELECT 'user_analysis_state=' || count(*) FROM user_analysis_state
UNION ALL SELECT 'user_next_task_recommendations=' || count(*) FROM user_next_task_recommendations;
" | tee "$OUT_DIR/postgres-counts.txt"

write_summary_header "Storage snapshot"
append_file_block "ClickHouse sizes" "$OUT_DIR/clickhouse-sizes.txt"
append_file_block "PostgreSQL counts" "$OUT_DIR/postgres-counts.txt"

log "Done"
printf '\nResults directory: %s\nSummary: %s\n' "$OUT_DIR" "$OUT_DIR/summary.md"
