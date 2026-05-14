#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
INTELLIGENCE_BASE="${INTELLIGENCE_BASE:-http://localhost:8090}"

RUNS="${RUNS:-5}"
MAX_WAIT_SECONDS="${MAX_WAIT_SECONDS:-600}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-1}"

PASSWORD="${PASSWORD:-password123}"
TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
USER_QUERY="${USER_QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"
FAKE_REFERENCE_QUERY="${FAKE_REFERENCE_QUERY:-SELECT 1 AS hacked_reference;}"

OUT_CSV="${OUT_CSV:-/tmp/next-task-wait-benchmark.csv}"

log() {
  printf '\n\033[1;34m==>\033[0m %s\n' "$*"
}

ok() {
  printf '\033[1;32mOK\033[0m %s\n' "$*"
}

fail() {
  printf '\033[1;31mFAIL\033[0m %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"
}

now_ms() {
  date +%s%3N
}

need curl
need jq
need docker
need date
need awk
need sort

log "Checking readiness"

curl -fsS "$BASE/ready" | jq .
curl -fsS "$INTELLIGENCE_BASE/ready" | jq .

log "Prewarming intelligence model if needed"

READY_JSON="$(curl -fsS "$INTELLIGENCE_BASE/ready")"
LOADED="$(printf '%s\n' "$READY_JSON" | jq -r '.loaded // true')"

if [[ "$LOADED" == "false" ]]; then
  WARMUP_ARGS=(-sS -X POST "$INTELLIGENCE_BASE/warmup" -H 'Content-Type: application/json' -d '{}')
  if [[ -n "${INTELLIGENCE_API_KEY:-}" ]]; then
    WARMUP_ARGS+=(-H "X-API-Key: $INTELLIGENCE_API_KEY")
  fi

  HTTP_CODE="$(curl "${WARMUP_ARGS[@]}" -o /tmp/benchmark-warmup-body.json -w '%{http_code}' || true)"
  cat /tmp/benchmark-warmup-body.json | jq . || cat /tmp/benchmark-warmup-body.json

  [[ "$HTTP_CODE" == "200" ]] || fail "warmup failed with HTTP $HTTP_CODE"
fi

ok "intelligence ready"

printf 'run,user_id,event_id,next_task_id,execute_ms,next_wait_ms,total_until_next_ms,pending_count\n' > "$OUT_CSV"

log "Running benchmark: RUNS=$RUNS"

for i in $(seq 1 "$RUNS"); do
  log "Run $i/$RUNS"

  EMAIL="student+next-bench-${i}-$(date +%s)@example.com"

  REGISTER_RESPONSE="$(curl -fsS -X POST "$BASE/auth/register" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc \
      --arg email "$EMAIL" \
      --arg password "$PASSWORD" \
      '{email:$email,password:$password}')")"

  TOKEN="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.access_token')"
  USER_ID="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.user.id')"

  [[ -n "$TOKEN" && "$TOKEN" != "null" ]] || fail "run $i: no token"
  [[ -n "$USER_ID" && "$USER_ID" != "null" ]] || fail "run $i: no user id"

  EXEC_START_MS="$(now_ms)"

  EXEC_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc \
      --arg task_id "$TASK_ID" \
      --arg user_query "$USER_QUERY" \
      --arg reference_query "$FAKE_REFERENCE_QUERY" \
      '{task_id:$task_id,user_query:$user_query,reference_query:$reference_query}')")"

  EXEC_END_MS="$(now_ms)"
  EXECUTE_MS=$((EXEC_END_MS - EXEC_START_MS))

  EVENT_ID="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.event_id')"
  REFERENCE_COLUMNS="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.reference_result.columns | join(",")')"

  [[ -n "$EVENT_ID" && "$EVENT_ID" != "null" ]] || fail "run $i: no event_id"

  if [[ "$REFERENCE_COLUMNS" == "hacked_reference" ]]; then
    fail "run $i: client-provided reference_query was used"
  fi

  NEXT_START_MS="$(now_ms)"
  DEADLINE_SECONDS=$((SECONDS + MAX_WAIT_SECONDS))
  PENDING_COUNT=0
  NEXT_TASK_ID=""
  BODY_FILE="$(mktemp)"

  while (( SECONDS < DEADLINE_SECONDS )); do
    HTTP_CODE="$(curl -sS \
      -o "$BODY_FILE" \
      -w '%{http_code}' \
      -H "Authorization: Bearer $TOKEN" \
      "$BASE/tasks/next" || true)"

    if [[ "$HTTP_CODE" == "200" ]]; then
      NEXT_TASK_ID="$(jq -r '.task.id // empty' "$BODY_FILE")"
      [[ -n "$NEXT_TASK_ID" ]] || fail "run $i: /tasks/next returned 200 but no task.id"
      break
    fi

    if [[ "$HTTP_CODE" == "409" ]]; then
      ERROR_CODE="$(jq -r '.error // .code // empty' "$BODY_FILE" 2>/dev/null || true)"

      if [[ "$ERROR_CODE" == "analysis_pending" ]]; then
        PENDING_COUNT=$((PENDING_COUNT + 1))
        sleep "$POLL_INTERVAL_SECONDS"
        continue
      fi
    fi

    printf 'run %s: unexpected /tasks/next response HTTP %s\n' "$i" "$HTTP_CODE" >&2
    cat "$BODY_FILE" >&2
    printf '\n' >&2
    rm -f "$BODY_FILE"
    exit 1
  done

  rm -f "$BODY_FILE"

  [[ -n "$NEXT_TASK_ID" ]] || fail "run $i: next task did not become ready within ${MAX_WAIT_SECONDS}s"

  NEXT_END_MS="$(now_ms)"
  NEXT_WAIT_MS=$((NEXT_END_MS - NEXT_START_MS))
  TOTAL_UNTIL_NEXT_MS=$((NEXT_END_MS - EXEC_START_MS))

  printf '%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$i" "$USER_ID" "$EVENT_ID" "$NEXT_TASK_ID" "$EXECUTE_MS" "$NEXT_WAIT_MS" "$TOTAL_UNTIL_NEXT_MS" "$PENDING_COUNT" \
    >> "$OUT_CSV"

  printf 'run=%s user_id=%s event_id=%s next_task_id=%s execute_ms=%s next_wait_ms=%s pending_count=%s\n' \
    "$i" "$USER_ID" "$EVENT_ID" "$NEXT_TASK_ID" "$EXECUTE_MS" "$NEXT_WAIT_MS" "$PENDING_COUNT"
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

  waits[n] = $6
}
END {
  if (n == 0) {
    print "no rows"
    exit 1
  }

  asort(waits)
  p50_idx = int((n + 1) * 0.50)
  p95_idx = int((n + 1) * 0.95)
  if (p50_idx < 1) p50_idx = 1
  if (p95_idx < 1) p95_idx = 1
  if (p50_idx > n) p50_idx = n
  if (p95_idx > n) p95_idx = n

  printf "runs=%d\n", n
  printf "avg_execute_ms=%.2f\n", execute_sum / n
  printf "avg_next_wait_ms=%.2f\n", wait_sum / n
  printf "min_next_wait_ms=%d\n", wait_min
  printf "max_next_wait_ms=%d\n", wait_max
  printf "p50_next_wait_ms=%d\n", waits[p50_idx]
  printf "p95_next_wait_ms=%d\n", waits[p95_idx]
  printf "avg_total_until_next_ms=%.2f\n", total_sum / n
  printf "avg_pending_responses=%.2f\n", pending_sum / n
}
' "$OUT_CSV"

ok "next-task wait benchmark completed"
printf '\nCSV=%s\n' "$OUT_CSV"
