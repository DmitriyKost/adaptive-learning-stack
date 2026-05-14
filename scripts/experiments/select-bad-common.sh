#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-adaptive-clickhouse}"
TASK_PROGRESS_PG_CONTAINER="${TASK_PROGRESS_PG_CONTAINER:-adaptive-task-progress-postgres}"

TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
PASSWORD="${PASSWORD:-password123}"
MAX_WAIT_SECONDS="${MAX_WAIT_SECONDS:-600}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-3}"

REFERENCE_QUERY='SELECT id, name FROM task_data.employees ORDER BY id;'
CORRECT_QUERY='SELECT id, name FROM task_data.employees ORDER BY id;'

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

need curl
need jq
need docker

register_user() {
  local scenario="$1"
  EMAIL="student+select-bad-${scenario}-$(date +%s)@example.com"

  log "Registering fresh user: $EMAIL"

  REGISTER_RESPONSE="$(curl -fsS -X POST "$BASE/auth/register" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc \
      --arg email "$EMAIL" \
      --arg password "$PASSWORD" \
      '{email:$email,password:$password}')")"

  TOKEN="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.access_token')"
  USER_ID="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.user.id')"

  [[ "$TOKEN" != "null" && -n "$TOKEN" ]] || fail "no token"
  [[ "$USER_ID" != "null" && -n "$USER_ID" ]] || fail "no user id"

  ok "user_id=$USER_ID"
}

execute_attempt() {
  local label="$1"
  local query="$2"

  log "Executing attempt: $label"
  printf '%s\n' "$query"

  RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc \
      --arg task_id "$TASK_ID" \
      --arg user_query "$query" \
      '{task_id:$task_id,user_query:$user_query}')")"

  printf '%s\n' "$RESPONSE" | jq '{
    event_id,
    task_id,
    user_rows: (.user_result.rows | length),
    reference_rows: (.reference_result.rows | length),
    user_columns: .user_result.columns,
    reference_columns: .reference_result.columns,
    created_at
  }'
}

ch_query() {
  docker exec "$CLICKHOUSE_CONTAINER" clickhouse-client \
    --user analytics \
    --password analytics \
    --database analytics \
    --query "$1"
}

pg_query() {
  docker exec "$TASK_PROGRESS_PG_CONTAINER" psql \
    -U task_progress \
    -d task_progress \
    -tAc "$1"
}

wait_for_llm() {
  log "Waiting for completed LLM analysis"

  DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
  RUN_ID=""

  while (( SECONDS < DEADLINE )); do
    RUN_ID="$(ch_query "
SELECT run_id
FROM llm_analysis_runs
WHERE user_id = '$USER_ID'
  AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1
FORMAT TSV
" | tr -d '[:space:]')"

    VERSION_COUNT="$(pg_query "
SELECT count(*)
FROM user_skill_assessment_versions
WHERE user_id = '$USER_ID'::uuid
  AND model_version = 'sql-intelligence-v1';
" | tr -d '[:space:]')"

    printf 'run_id=%s version_count=%s\n' "${RUN_ID:-none}" "${VERSION_COUNT:-0}"

    if [[ -n "$RUN_ID" && "${VERSION_COUNT:-0}" -gt 0 ]]; then
      break
    fi

    sleep "$POLL_INTERVAL_SECONDS"
  done

  [[ -n "$RUN_ID" ]] || fail "LLM analysis did not complete"
  ok "run_id=$RUN_ID"
}

print_proof() {
  log "LLM skill assessment"
  ch_query "
SELECT
  skill_code,
  mastery_score,
  confidence,
  reason,
  created_at
FROM skill_assessment_logs
WHERE analysis_run_id = '$RUN_ID'
ORDER BY created_at DESC
FORMAT PrettyCompact
"

  log "Raw LLM payload"
  ch_query "
SELECT response_payload
FROM llm_analysis_runs
WHERE run_id = '$RUN_ID'
FORMAT TSVRaw
" | jq '.skill_assessment | {
    skill_scores,
    recommended_skills,
    user_graph_overlay
  }'

  log "Learner model after LLM"
  docker exec "$TASK_PROGRESS_PG_CONTAINER" psql \
    -U task_progress \
    -d task_progress \
    -x \
    -c "
SELECT
  s.code AS skill_code,
  us.mastery_score,
  us.confidence,
  us.attempts_count,
  us.success_count,
  us.updated_at
FROM user_skills us
JOIN skills s ON s.id = us.skill_id
WHERE us.user_id = '$USER_ID'::uuid
ORDER BY s.code;
"

  log "Planner next task"

NEXT_BODY_FILE="$(mktemp)"
NEXT_HTTP_CODE=""
DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))

while (( SECONDS < DEADLINE )); do
  NEXT_HTTP_CODE="$(curl -sS \
    -o "$NEXT_BODY_FILE" \
    -w '%{http_code}' \
    -H "Authorization: Bearer $TOKEN" \
    "$BASE/tasks/next" || true)"

  if [[ "$NEXT_HTTP_CODE" == "200" ]]; then
    break
  fi

  if [[ "$NEXT_HTTP_CODE" == "409" ]]; then
    ERROR_CODE="$(jq -r '.error // .code // empty' "$NEXT_BODY_FILE" 2>/dev/null || true)"

    if [[ "$ERROR_CODE" == "analysis_pending" ]]; then
      RETRY_AFTER="$(jq -r '.retry_after_seconds // empty' "$NEXT_BODY_FILE" 2>/dev/null || true)"
      [[ "$RETRY_AFTER" =~ ^[0-9]+$ ]] || RETRY_AFTER="$POLL_INTERVAL_SECONDS"

      printf 'planner says analysis_pending, retrying after %ss\n' "$RETRY_AFTER"
      jq . "$NEXT_BODY_FILE" || cat "$NEXT_BODY_FILE"
      sleep "$RETRY_AFTER"
      continue
    fi
  fi

  printf 'Planner returned non-200 response, http=%s\n' "$NEXT_HTTP_CODE" >&2
  cat "$NEXT_BODY_FILE" >&2
  printf '\n' >&2
  break
done

if [[ "$NEXT_HTTP_CODE" == "200" ]]; then
  NEXT_JSON="$(cat "$NEXT_BODY_FILE")"

  printf '%s\n' "$NEXT_JSON" | jq '{
    selected_task: {
      id: .task.id,
      title: .task.title,
      difficulty: .task.difficulty,
      skills: [.task.skills[]? | {skill_code, weight}]
    },
    planner_decision: {
      score,
      reason,
      repeat_mode,
      graph_code,
      professional_track
    },
    llm_recommended_skills_seen_by_planner: [
      .recommended_skills[]? | {
        skill_code,
        priority,
        recommended_action,
        reason
      }
    ]
  }'
else
  printf '\033[1;33mWARN\033[0m planner did not return a task; learner-model part of the test still completed\n'
fi

rm -f "$NEXT_BODY_FILE"
  printf '\nUSER_ID=%s\nEMAIL=%s\nRUN_ID=%s\n' "$USER_ID" "$EMAIL" "$RUN_ID"
}
