#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
QUERY="${QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"
EMAIL="student-e2e-$(date +%s)@example.com"
PASSWORD="password123"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

need curl
need jq
need docker

ms_now() {
  date +%s%3N
}

short() {
  printf "%s" "$1" | cut -c1-8
}

ch_query() {
  docker exec adaptive-clickhouse clickhouse-client \
    --user analytics \
    --password analytics \
    --database analytics \
    --query "$1" 2>/dev/null | tr -d '\r'
}

REGISTER_RESPONSE="$(
  curl -fsS -X POST "$BASE/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
)"

TOKEN="$(printf "%s" "$REGISTER_RESPONSE" | jq -r ".access_token")"
USER_ID="$(printf "%s" "$REGISTER_RESPONSE" | jq -r ".user.id")"

PAYLOAD="$(
  jq -n \
    --arg task_id "$TASK_ID" \
    --arg user_query "$QUERY" \
    '{task_id: $task_id, user_query: $user_query}'
)"

execute_start="$(ms_now)"

EXEC_RESPONSE="$(
  curl -fsS -X POST "$BASE/playground/execute" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD"
)"

execute_end="$(ms_now)"
execute_ms=$((execute_end - execute_start))

EVENT_ID="$(printf "%s" "$EXEC_RESPONSE" | jq -r ".event_id")"
ROWS="$(printf "%s" "$EXEC_RESPONSE" | jq -r ".user_result.rows_affected")"

progress_start="$(ms_now)"
progress_ms=""

for _ in $(seq 1 60); do
  PROGRESS_JSON="$(curl -fsS "$BASE/progress/me" -H "Authorization: Bearer $TOKEN")"

  attempts="$(
    printf "%s" "$PROGRESS_JSON" |
      jq -r '.skills[]? | select(.skill_code=="select") | .attempts_count' |
      head -n 1
  )"

  if [ "${attempts:-0}" != "0" ] && [ -n "${attempts:-}" ]; then
    progress_end="$(ms_now)"
    progress_ms=$((progress_end - progress_start))
    break
  fi

  sleep 1
done

if [ -z "$progress_ms" ]; then
  echo "progress update timeout" >&2
  exit 1
fi

analysis_start="$(ms_now)"
analysis_ms=""

for _ in $(seq 1 90); do
  status="$(
    ch_query "
      SELECT status
      FROM llm_analysis_runs
      WHERE user_id = '$USER_ID'
      ORDER BY completed_at DESC
      LIMIT 1
      FORMAT TSVRaw
    " || true
  )"

  if [ "$status" = "completed" ]; then
    analysis_end="$(ms_now)"
    analysis_ms=$((analysis_end - analysis_start))
    break
  fi

  sleep 2
done

if [ -z "$analysis_ms" ]; then
  echo "external analysis timeout" >&2
  exit 1
fi

learner_start="$(ms_now)"
learner_ms=""

for _ in $(seq 1 60); do
  learner_update="$(
    ch_query "
      SELECT count()
      FROM learner_model_update_logs
      WHERE user_id = '$USER_ID'
        AND source = 'analytics-service'
      FORMAT TSVRaw
    " || true
  )"

  if [ "${learner_update:-0}" != "0" ]; then
    learner_end="$(ms_now)"
    learner_ms=$((learner_end - learner_start))
    break
  fi

  sleep 1
done

if [ -z "$learner_ms" ]; then
  echo "learner model external update timeout" >&2
  exit 1
fi

echo "E2E timing"
echo "user_id=$USER_ID"
echo "event_id=$EVENT_ID"
echo
printf "metric                         value\n"
printf "execute_api_latency_ms         %s\n" "$execute_ms"
printf "execute_rows                   %s\n" "$ROWS"
printf "local_progress_update_ms       %s\n" "$progress_ms"
printf "external_analysis_complete_ms  %s\n" "$analysis_ms"
printf "external_model_apply_ms        %s\n" "$learner_ms"
