#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/e2e-common.sh"

RUNS="${RUNS:-5}"
OUT_CSV="${OUT_CSV:-/tmp/next-task-wait-benchmark.csv}"

need_base_tools
need date
need awk

check_readiness
prewarm_intelligence

printf 'run,user_id,initial_task_id,next_task_id,execute_ms,next_wait_ms,total_until_next_ms,pending_count\n' > "$OUT_CSV"

log "Running cold-start next-task benchmark: RUNS=$RUNS"

for i in $(seq 1 "$RUNS"); do
  log "Run $i/$RUNS"

  register_student "student+next-bench-${i}"

  BODY_FILE="$(mktemp)"
  HTTP_CODE="$(get_next_task_http "$TOKEN" "$BODY_FILE")"

  if [[ "$HTTP_CODE" != "200" ]]; then
    cat "$BODY_FILE" >&2
    rm -f "$BODY_FILE"
    fail "cold-start /tasks/next should return 200, got $HTTP_CODE"
  fi

  INITIAL_NEXT_JSON="$(cat "$BODY_FILE")"
  rm -f "$BODY_FILE"

  assert_json_has_no_field "$INITIAL_NEXT_JSON" "reference_sql" "cold-start /tasks/next"

  INITIAL_TASK_ID="$(printf '%s\n' "$INITIAL_NEXT_JSON" | jq -r '.task.id')"
  USER_QUERY="$(solve_task_from_json "$INITIAL_NEXT_JSON")"

  EXEC_START_MS="$(now_ms)"

  EXEC_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc \
      --arg task_id "$INITIAL_TASK_ID" \
      --arg user_query "$USER_QUERY" \
      '{task_id:$task_id,user_query:$user_query}')")"

  EXEC_END_MS="$(now_ms)"
  EXECUTE_MS=$((EXEC_END_MS - EXEC_START_MS))

  EXECUTION_SUCCESS="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.execution_success')"
  IS_CORRECT="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.is_correct')"
  HAS_REFERENCE_RESULT="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r 'has("reference_result")')"

  [[ "$EXECUTION_SUCCESS" == "true" ]] || fail "run $i: execution_success=false"
  [[ "$IS_CORRECT" == "true" ]] || fail "run $i: is_correct=false"
  [[ "$HAS_REFERENCE_RESULT" == "false" ]] || fail "run $i: reference_result leaked"

  NEXT_START_MS="$(now_ms)"
  DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
  PENDING_COUNT=0
  NEXT_TASK_ID=""
  BODY_FILE="$(mktemp)"

  while (( SECONDS < DEADLINE )); do
    HTTP_CODE="$(get_next_task_http "$TOKEN" "$BODY_FILE")"

    if [[ "$HTTP_CODE" == "200" ]]; then
      NEXT_TASK_ID="$(jq -r '.task.id // empty' "$BODY_FILE")"
      if [[ -n "$NEXT_TASK_ID" && "$NEXT_TASK_ID" != "$INITIAL_TASK_ID" ]]; then
        break
      fi
    fi

    if [[ "$HTTP_CODE" == "409" ]]; then
      ERROR_CODE="$(jq -r '.error // .code // empty' "$BODY_FILE" 2>/dev/null || true)"
      if [[ "$ERROR_CODE" == "analysis_pending" ]]; then
        PENDING_COUNT=$((PENDING_COUNT + 1))
        sleep "$POLL_INTERVAL_SECONDS"
        continue
      fi
    fi

    if [[ "$HTTP_CODE" != "200" ]]; then
      printf 'run %s: unexpected /tasks/next response HTTP %s\n' "$i" "$HTTP_CODE" >&2
      cat "$BODY_FILE" >&2
      rm -f "$BODY_FILE"
      exit 1
    fi

    sleep "$POLL_INTERVAL_SECONDS"
  done

  rm -f "$BODY_FILE"

  [[ -n "$NEXT_TASK_ID" ]] || fail "run $i: next task did not become ready"

  NEXT_END_MS="$(now_ms)"
  NEXT_WAIT_MS=$((NEXT_END_MS - NEXT_START_MS))
  TOTAL_UNTIL_NEXT_MS=$((NEXT_END_MS - EXEC_START_MS))

  printf '%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$i" "$USER_ID" "$INITIAL_TASK_ID" "$NEXT_TASK_ID" "$EXECUTE_MS" "$NEXT_WAIT_MS" "$TOTAL_UNTIL_NEXT_MS" "$PENDING_COUNT" \
    >> "$OUT_CSV"

  printf 'run=%s user_id=%s initial=%s next=%s execute_ms=%s next_wait_ms=%s pending_count=%s\n' \
    "$i" "$USER_ID" "$INITIAL_TASK_ID" "$NEXT_TASK_ID" "$EXECUTE_MS" "$NEXT_WAIT_MS" "$PENDING_COUNT"
done

log "CSV results"
cat "$OUT_CSV"

log "Summary"

awk -F, '
NR == 1 { next }
{
  n += 1
  execute_sum += $5
  wait_sum += $6
  total_sum += $7
  pending_sum += $8

  if (n == 1 || $6 < wait_min) wait_min = $6
  if (n == 1 || $6 > wait_max) wait_max = $6
}
END {
  if (n == 0) {
    print "no rows"
    exit 1
  }

  printf "runs=%d\n", n
  printf "avg_execute_ms=%.2f\n", execute_sum / n
  printf "avg_next_wait_ms=%.2f\n", wait_sum / n
  printf "min_next_wait_ms=%d\n", wait_min
  printf "max_next_wait_ms=%d\n", wait_max
  printf "avg_total_until_next_ms=%.2f\n", total_sum / n
  printf "avg_pending_responses=%.2f\n", pending_sum / n
}
' "$OUT_CSV"

ok "next-task wait benchmark completed"
printf '\nCSV=%s\n' "$OUT_CSV"
