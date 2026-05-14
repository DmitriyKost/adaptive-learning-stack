#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
INTELLIGENCE_BASE="${INTELLIGENCE_BASE:-http://localhost:8090}"

CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-adaptive-clickhouse}"
TASK_PROGRESS_PG_CONTAINER="${TASK_PROGRESS_PG_CONTAINER:-adaptive-task-progress-postgres}"

TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
PASSWORD="${PASSWORD:-password123}"
MAX_WAIT_SECONDS="${MAX_WAIT_SECONDS:-600}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-2}"

USER_QUERY="${USER_QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"
FAKE_REFERENCE_QUERY="${FAKE_REFERENCE_QUERY:-SELECT 1 AS hacked_reference;}"

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
need date

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

  HTTP_CODE="$(curl "${WARMUP_ARGS[@]}" -o /tmp/e2e-warmup-body.json -w '%{http_code}' || true)"
  cat /tmp/e2e-warmup-body.json | jq . || cat /tmp/e2e-warmup-body.json

  [[ "$HTTP_CODE" == "200" ]] || fail "warmup failed with HTTP $HTTP_CODE"
fi

ok "intelligence ready"

log "Registering fresh user"

EMAIL="student+ref-integrity-$(date +%s)@example.com"

REGISTER_RESPONSE="$(curl -fsS -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg email "$EMAIL" \
    --arg password "$PASSWORD" \
    '{email:$email,password:$password}')")"

TOKEN="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.access_token')"
USER_ID="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.user.id')"

[[ -n "$TOKEN" && "$TOKEN" != "null" ]] || fail "no access token"
[[ -n "$USER_ID" && "$USER_ID" != "null" ]] || fail "no user id"

ok "user_id=$USER_ID"

log "Executing task with intentionally fake client-provided reference_query"

EXEC_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$TASK_ID" \
    --arg user_query "$USER_QUERY" \
    --arg reference_query "$FAKE_REFERENCE_QUERY" \
    '{task_id:$task_id,user_query:$user_query,reference_query:$reference_query}')")"

printf '%s\n' "$EXEC_RESPONSE" | jq .

EVENT_ID="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.event_id')"
REFERENCE_COLUMNS="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.reference_result.columns | join(",")')"

[[ -n "$EVENT_ID" && "$EVENT_ID" != "null" ]] || fail "no event_id"

if [[ "$REFERENCE_COLUMNS" == "hacked_reference" ]]; then
  fail "client-provided reference_query was used"
fi

if [[ "$REFERENCE_COLUMNS" != "id,name" ]]; then
  fail "unexpected reference columns: $REFERENCE_COLUMNS"
fi

ok "client-provided reference_query ignored; DB reference SQL was used"

log "Waiting for LLM run, learner model update and persisted next task"

DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
RUN_ID=""
NEXT_STATUS=""
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

  if [[ -n "$RUN_ID" && "${VERSION_COUNT:-0}" -gt 0 && "${ANALYTICS_UPDATE_COUNT:-0}" -gt 0 && "$NEXT_STATUS" == "ready" && -n "$NEXT_TASK_ID" ]]; then
    break
  fi

  sleep "$POLL_INTERVAL_SECONDS"
done

[[ -n "$RUN_ID" ]] || fail "LLM run was not completed"
[[ "${VERSION_COUNT:-0}" -gt 0 ]] || fail "learner model version was not updated"
[[ "${ANALYTICS_UPDATE_COUNT:-0}" -gt 0 ]] || fail "analytics learner update was not logged"
[[ "$NEXT_STATUS" == "ready" ]] || fail "persisted next task is not ready"
[[ -n "$NEXT_TASK_ID" ]] || fail "persisted next task id is empty"

ok "pipeline completed; run_id=$RUN_ID next_task_id=$NEXT_TASK_ID"

log "Calling /tasks/next; should return already persisted next task"

NEXT_RESPONSE="$(curl -fsS "$BASE/tasks/next" \
  -H "Authorization: Bearer $TOKEN")"

printf '%s\n' "$NEXT_RESPONSE" | jq '{
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

API_NEXT_TASK_ID="$(printf '%s\n' "$NEXT_RESPONSE" | jq -r '.task.id')"
[[ "$API_NEXT_TASK_ID" == "$NEXT_TASK_ID" ]] || fail "/tasks/next returned task_id=$API_NEXT_TASK_ID, expected persisted task_id=$NEXT_TASK_ID"

ok "/tasks/next returns persisted task"

log "Proof: llm_analysis_runs"
ch_query "
SELECT
  status,
  model_version,
  prompt_version,
  source_task_id,
  source_attempt_id,
  completed_at
FROM llm_analysis_runs
WHERE run_id = '$RUN_ID'
FORMAT PrettyCompact
"

log "Proof: user_skill_assessment_versions"
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

log "Proof: user_next_task_recommendations"
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

ok "E2E reference integrity check passed"

printf '\nUSER_ID=%s\nEMAIL=%s\nEVENT_ID=%s\nRUN_ID=%s\nNEXT_TASK_ID=%s\n' \
  "$USER_ID" "$EMAIL" "$EVENT_ID" "$RUN_ID" "$NEXT_TASK_ID"
