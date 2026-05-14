#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
INTELLIGENCE_BASE="${INTELLIGENCE_BASE:-http://localhost:8090}"

CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-adaptive-clickhouse}"
TASK_PROGRESS_PG_CONTAINER="${TASK_PROGRESS_PG_CONTAINER:-adaptive-task-progress-postgres}"

PASSWORD="${PASSWORD:-password123}"
EMAIL="${EMAIL:-student+smoke-coldstart-$(date +%s)@example.com}"

MAX_WAIT_SECONDS="${MAX_WAIT_SECONDS:-600}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-1}"

log() {
  printf '\n\033[1;34m==>\033[0m %s\n' "$*"
}

ok() {
  printf '\033[1;32mOK\033[0m %s\n' "$*"
}

warn() {
  printf '\033[1;33mWARN\033[0m %s\n' "$*"
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

get_next_task() {
  local token="$1"
  local body_file="$2"

  curl -sS \
    -o "$body_file" \
    -w '%{http_code}' \
    -H "Authorization: Bearer $token" \
    "$BASE/tasks/next" || true
}

solve_cold_start_task() {
  local task_json="$1"

  local task_id
  local title
  local skills

  task_id="$(printf '%s\n' "$task_json" | jq -r '.task.id')"
  title="$(printf '%s\n' "$task_json" | jq -r '.task.title')"
  skills="$(printf '%s\n' "$task_json" | jq -r '[.task.skills[]?.skill_code] | sort | join(",")')"

  case "$skills" in
    "select")
      printf '%s\n' 'SELECT id, name FROM task_data.employees ORDER BY id;'
      ;;
    *)
      cat >&2 <<MSG
Unsupported cold-start task for this smoke test.

task_id=$task_id
title=$title
skills=$skills

The smoke test intentionally solves the task returned by /tasks/next.
Currently it only knows how to solve the starter SELECT task.
MSG
      exit 1
      ;;
  esac
}

assert_no_reference_sql_in_public_json() {
  local json="$1"
  local label="$2"

  if printf '%s\n' "$json" | jq -e '.. | objects | select(has("reference_sql"))' >/dev/null; then
    printf '%s\n' "$json" | jq '.. | objects | select(has("reference_sql"))' >&2
    fail "$label leaks reference_sql"
  fi

  ok "$label does not leak reference_sql"
}

log "Checking readiness"

BASE_READY="$(curl -sS "$BASE/ready")"
printf '%s\n' "$BASE_READY" | jq .
printf '%s\n' "$BASE_READY" | jq -e '.status == "ready"' >/dev/null || fail "gateway is not ready"

INT_READY="$(curl -sS "$INTELLIGENCE_BASE/ready")"
printf '%s\n' "$INT_READY" | jq .
printf '%s\n' "$INT_READY" | jq -e '.status == "ready"' >/dev/null || fail "intelligence is not ready"

log "Prewarming intelligence model if needed"

LOADED="$(printf '%s\n' "$INT_READY" | jq -r '.loaded // true')"

if [[ "$LOADED" == "false" ]]; then
  WARMUP_BODY="$(mktemp)"
  WARMUP_ARGS=(-sS -X POST "$INTELLIGENCE_BASE/warmup" -H 'Content-Type: application/json' -d '{}')

  if [[ -n "${INTELLIGENCE_API_KEY:-}" ]]; then
    WARMUP_ARGS+=(-H "X-API-Key: $INTELLIGENCE_API_KEY")
  fi

  WARMUP_HTTP_CODE="$(curl "${WARMUP_ARGS[@]}" -o "$WARMUP_BODY" -w '%{http_code}' || true)"
  cat "$WARMUP_BODY" | jq . || cat "$WARMUP_BODY"
  rm -f "$WARMUP_BODY"

  [[ "$WARMUP_HTTP_CODE" == "200" ]] || fail "intelligence warmup failed with HTTP $WARMUP_HTTP_CODE"
fi

ok "intelligence model loaded"

log "Registering fresh user: $EMAIL"

REGISTER_RESPONSE="$(curl -fsS -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg email "$EMAIL" \
    --arg password "$PASSWORD" \
    '{email:$email,password:$password}')")"

TOKEN="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.access_token')"
USER_ID="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.user.id')"

[[ "$TOKEN" != "null" && -n "$TOKEN" ]] || fail "no access token returned"
[[ "$USER_ID" != "null" && -n "$USER_ID" ]] || fail "no user id returned"

ok "user_id=$USER_ID"

log "Requesting cold-start task from /tasks/next"

NEXT_BODY_FILE="$(mktemp)"
NEXT_HTTP_CODE="$(get_next_task "$TOKEN" "$NEXT_BODY_FILE")"

if [[ "$NEXT_HTTP_CODE" != "200" ]]; then
  printf 'Unexpected cold-start /tasks/next response, http=%s\n' "$NEXT_HTTP_CODE" >&2
  cat "$NEXT_BODY_FILE" >&2
  printf '\n' >&2
  rm -f "$NEXT_BODY_FILE"
  fail "new user should receive initial task synchronously"
fi

COLD_START_NEXT_JSON="$(cat "$NEXT_BODY_FILE")"
rm -f "$NEXT_BODY_FILE"

printf '%s\n' "$COLD_START_NEXT_JSON" | jq '{
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
  }
}'

assert_no_reference_sql_in_public_json "$COLD_START_NEXT_JSON" "/tasks/next cold-start response"

TASK_ID="$(printf '%s\n' "$COLD_START_NEXT_JSON" | jq -r '.task.id')"
TASK_TITLE="$(printf '%s\n' "$COLD_START_NEXT_JSON" | jq -r '.task.title')"
USER_QUERY="$(solve_cold_start_task "$COLD_START_NEXT_JSON")"

[[ "$TASK_ID" != "null" && -n "$TASK_ID" ]] || fail "cold-start task has no id"

ok "cold-start task_id=$TASK_ID title=$TASK_TITLE"

log "Verifying persisted cold-start recommendation"

docker exec "$TASK_PROGRESS_PG_CONTAINER" psql \
  -U task_progress \
  -d task_progress \
  -x \
  -c "
SELECT
  user_id,
  status,
  task_id,
  score,
  reason,
  repeat_mode,
  source_analysis_run_id,
  expires_at,
  updated_at
FROM user_next_task_recommendations
WHERE user_id = '$USER_ID'::uuid;
"

CACHED_TASK_ID="$(pg_query "
SELECT COALESCE(task_id::text, '')
FROM user_next_task_recommendations
WHERE user_id = '$USER_ID'::uuid
  AND status = 'ready';
" | tr -d '[:space:]')"

[[ "$CACHED_TASK_ID" == "$TASK_ID" ]] || fail "cold-start recommendation was not persisted correctly"

ok "cold-start recommendation persisted"

log "Checking public /tasks and /tasks/{id} do not leak reference_sql"

TASKS_JSON="$(curl -fsS "$BASE/tasks" \
  -H "Authorization: Bearer $TOKEN")"

assert_no_reference_sql_in_public_json "$TASKS_JSON" "/tasks response"

TASK_JSON="$(curl -fsS "$BASE/tasks/$TASK_ID" \
  -H "Authorization: Bearer $TOKEN")"

assert_no_reference_sql_in_public_json "$TASK_JSON" "/tasks/{id} response"

log "Executing returned cold-start task"

printf 'task_id=%s\n' "$TASK_ID"
printf 'user_query=%s\n' "$USER_QUERY"

EXEC_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$TASK_ID" \
    --arg user_query "$USER_QUERY" \
    '{task_id:$task_id,user_query:$user_query}')")"

printf '%s\n' "$EXEC_RESPONSE" | jq .

EVENT_ID="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.event_id')"
EXECUTION_SUCCESS="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.execution_success')"
IS_CORRECT="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.is_correct')"

[[ "$EVENT_ID" != "null" && -n "$EVENT_ID" ]] || fail "no event_id returned"
[[ "$EXECUTION_SUCCESS" == "true" ]] || fail "execution_success is not true"
[[ "$IS_CORRECT" == "true" ]] || fail "is_correct is not true"

if printf '%s\n' "$EXEC_RESPONSE" | jq -e 'has("reference_result")' >/dev/null; then
  fail "/playground/execute leaked reference_result"
fi

ok "playground execute returned correct verdict without reference_result"

log "Calling /tasks/next immediately after submit; pending is expected"

PENDING_BODY_FILE="$(mktemp)"
PENDING_HTTP_CODE="$(get_next_task "$TOKEN" "$PENDING_BODY_FILE")"

if [[ "$PENDING_HTTP_CODE" == "409" ]]; then
  PENDING_ERROR="$(jq -r '.error // empty' "$PENDING_BODY_FILE")"
  if [[ "$PENDING_ERROR" == "analysis_pending" ]]; then
    ok "/tasks/next returns analysis_pending while LLM update is running"
    jq . "$PENDING_BODY_FILE"
  else
    cat "$PENDING_BODY_FILE" >&2
    rm -f "$PENDING_BODY_FILE"
    fail "/tasks/next returned 409 but not analysis_pending"
  fi
elif [[ "$PENDING_HTTP_CODE" == "200" ]]; then
  warn "/tasks/next already returned ready; pipeline completed before first pending check"
  cat "$PENDING_BODY_FILE" | jq .
else
  printf 'Unexpected /tasks/next response after submit, http=%s\n' "$PENDING_HTTP_CODE" >&2
  cat "$PENDING_BODY_FILE" >&2
  rm -f "$PENDING_BODY_FILE"
  exit 1
fi

rm -f "$PENDING_BODY_FILE"

log "Waiting for LLM analysis, learner model update and persisted next task"

DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
RUN_ID=""
NEXT_TASK_ID=""

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

  ANALYTICS_UPDATE_COUNT="$(ch_query "
SELECT count()
FROM learner_model_update_logs
WHERE user_id = '$USER_ID'
  AND source = 'analytics-service'
FORMAT TSV
" | tr -d '[:space:]')"

  NEXT_ROW="$(pg_query "
SELECT status || '|' || COALESCE(task_id::text, '')
FROM user_next_task_recommendations
WHERE user_id = '$USER_ID'::uuid;
" | tr -d '[:space:]')"

  NEXT_STATUS="${NEXT_ROW%%|*}"
  NEXT_TASK_ID="${NEXT_ROW#*|}"

  printf 'run_id=%s version_count=%s analytics_updates=%s next_status=%s next_task=%s\n' \
    "${RUN_ID:-none}" \
    "${VERSION_COUNT:-0}" \
    "${ANALYTICS_UPDATE_COUNT:-0}" \
    "${NEXT_STATUS:-none}" \
    "${NEXT_TASK_ID:-none}"

  if [[ -n "$RUN_ID" && "${VERSION_COUNT:-0}" -gt 0 && "${ANALYTICS_UPDATE_COUNT:-0}" -gt 0 && "$NEXT_STATUS" == "ready" && -n "$NEXT_TASK_ID" && "$NEXT_TASK_ID" != "$TASK_ID" ]]; then
    break
  fi

  sleep "$POLL_INTERVAL_SECONDS"
done

[[ -n "$RUN_ID" ]] || fail "LLM analysis did not complete within ${MAX_WAIT_SECONDS}s"
[[ "${VERSION_COUNT:-0}" -gt 0 ]] || fail "learner model version was not updated by LLM"
[[ "${ANALYTICS_UPDATE_COUNT:-0}" -gt 0 ]] || fail "no analytics-service learner model update log found"
[[ "$NEXT_STATUS" == "ready" ]] || fail "persisted next task is not ready"
[[ -n "$NEXT_TASK_ID" ]] || fail "persisted next task id is empty"

ok "LLM run completed: $RUN_ID"
ok "persisted post-submit next_task_id=$NEXT_TASK_ID"

log "Fetching final /tasks/next; should return persisted post-submit recommendation"

FINAL_NEXT_JSON="$(curl -fsS "$BASE/tasks/next" \
  -H "Authorization: Bearer $TOKEN")"

printf '%s\n' "$FINAL_NEXT_JSON" | jq '{
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

assert_no_reference_sql_in_public_json "$FINAL_NEXT_JSON" "/tasks/next final response"

FINAL_TASK_ID="$(printf '%s\n' "$FINAL_NEXT_JSON" | jq -r '.task.id')"
[[ "$FINAL_TASK_ID" == "$NEXT_TASK_ID" ]] || fail "/tasks/next returned $FINAL_TASK_ID, expected persisted $NEXT_TASK_ID"

log "Proof: ClickHouse llm_analysis_runs"

ch_query "
SELECT
  status,
  model_version,
  prompt_version,
  source_task_id,
  source_attempt_id,
  started_at,
  completed_at,
  dateDiff('millisecond', started_at, completed_at) AS llm_duration_ms
FROM llm_analysis_runs
WHERE run_id = '$RUN_ID'
FORMAT PrettyCompact
"

log "Proof: ClickHouse task_attempt_logs reference SQL"

ch_query "
SELECT
  submitted_sql,
  reference_sql,
  is_correct,
  created_at
FROM task_attempt_logs
WHERE user_id = '$USER_ID'
ORDER BY created_at DESC
LIMIT 1
FORMAT PrettyCompact
"

log "Proof: Postgres user_skills"

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

log "Proof: Postgres user_skill_assessment_versions"

docker exec "$TASK_PROGRESS_PG_CONTAINER" psql \
  -U task_progress \
  -d task_progress \
  -x \
  -c "
SELECT
  s.code AS skill_code,
  v.last_analysis_run_id,
  v.model_version,
  v.prompt_version,
  v.updated_at
FROM user_skill_assessment_versions v
JOIN skills s ON s.id = v.skill_id
WHERE v.user_id = '$USER_ID'::uuid
ORDER BY v.updated_at DESC;
"

log "Proof: Postgres user_next_task_recommendations"

docker exec "$TASK_PROGRESS_PG_CONTAINER" psql \
  -U task_progress \
  -d task_progress \
  -x \
  -c "
SELECT
  user_id,
  status,
  task_id,
  score,
  reason,
  repeat_mode,
  source_analysis_run_id,
  expires_at,
  updated_at
FROM user_next_task_recommendations
WHERE user_id = '$USER_ID'::uuid;
"

log "Proof: reference SQL reached internal analytics events"

ch_query "
SELECT
  countIf(position(payload, 'reference_sql') > 0) AS reference_payloads
FROM raw_events
WHERE user_id = '$USER_ID'
FORMAT PrettyCompact
"

ok "Cold-start E2E smoke test passed"

printf '\nUSER_ID=%s\nEMAIL=%s\nEVENT_ID=%s\nRUN_ID=%s\nINITIAL_TASK_ID=%s\nNEXT_TASK_ID=%s\n' \
  "$USER_ID" "$EMAIL" "$EVENT_ID" "$RUN_ID" "$TASK_ID" "$NEXT_TASK_ID"
