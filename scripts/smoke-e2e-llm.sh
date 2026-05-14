#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
INTELLIGENCE_BASE="${INTELLIGENCE_BASE:-http://localhost:8090}"
PREWARM_INTELLIGENCE="${PREWARM_INTELLIGENCE:-true}"

CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-adaptive-clickhouse}"
TASK_PROGRESS_PG_CONTAINER="${TASK_PROGRESS_PG_CONTAINER:-adaptive-task-progress-postgres}"

TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
PASSWORD="${PASSWORD:-password123}"
EMAIL="${EMAIL:-student+lora-e2e-$(date +%s)@example.com}"

MAX_WAIT_SECONDS="${MAX_WAIT_SECONDS:-180}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-3}"

USER_QUERY="${USER_QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"
REFERENCE_QUERY="${REFERENCE_QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"

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

log "Checking gateway readiness"
READY_JSON="$(curl -fsS "$BASE/ready" || true)"
printf '%s\n' "$READY_JSON" | jq .

READY_STATUS="$(printf '%s\n' "$READY_JSON" | jq -r '.status // empty')"
[[ "$READY_STATUS" == "ready" ]] || fail "gateway is not ready"

ok "gateway ready"

log "Checking intelligence readiness"
INTELLIGENCE_READY_JSON="$(curl -fsS "$INTELLIGENCE_BASE/ready" || true)"
printf '%s\n' "$INTELLIGENCE_READY_JSON" | jq .

INTELLIGENCE_READY_STATUS="$(printf '%s\n' "$INTELLIGENCE_READY_JSON" | jq -r '.status // empty')"
[[ "$INTELLIGENCE_READY_STATUS" == "ready" ]] || fail "intelligence service is not ready"

if [[ "$PREWARM_INTELLIGENCE" == "true" ]]; then
  log "Prewarming local LoRA model inside intelligence container"
  curl -fsS -X POST "$INTELLIGENCE_BASE/warmup" \
    -H 'Content-Type: application/json' \
    -d '{}' \
    | jq .
  ok "intelligence model loaded"
else
  ok "intelligence ready; warmup skipped"
fi

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

log "Executing playground task: $TASK_ID"
EXEC_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$TASK_ID" \
    --arg user_query "$USER_QUERY" \
    '{task_id:$task_id,user_query:$user_query}')")"

printf '%s\n' "$EXEC_RESPONSE" | jq .

EVENT_ID="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.event_id')"
[[ "$EVENT_ID" != "null" && -n "$EVENT_ID" ]] || fail "no event_id returned"

ok "event_id=$EVENT_ID"

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

log "Waiting for LLM analysis and learner model update"

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

  ANALYTICS_UPDATE_COUNT="$(ch_query "
SELECT count()
FROM learner_model_update_logs
WHERE user_id = '$USER_ID'
  AND source = 'analytics-service'
FORMAT TSV
" | tr -d '[:space:]')"

  printf 'run_id=%s version_count=%s analytics_updates=%s\n' \
    "${RUN_ID:-none}" \
    "${VERSION_COUNT:-0}" \
    "${ANALYTICS_UPDATE_COUNT:-0}"

  if [[ -n "$RUN_ID" && "${VERSION_COUNT:-0}" -gt 0 && "${ANALYTICS_UPDATE_COUNT:-0}" -gt 0 ]]; then
    break
  fi

  sleep "$POLL_INTERVAL_SECONDS"
done

[[ -n "$RUN_ID" ]] || fail "LLM analysis did not complete within ${MAX_WAIT_SECONDS}s"
[[ "${VERSION_COUNT:-0}" -gt 0 ]] || fail "learner model version was not updated by LLM"
[[ "${ANALYTICS_UPDATE_COUNT:-0}" -gt 0 ]] || fail "no analytics-service learner model update log found"

ok "LLM run completed: $RUN_ID"

log "Proof 1: ClickHouse llm_analysis_runs"
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

log "Proof 2: ClickHouse skill_assessment_logs"
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

log "Proof 3: Postgres user_skills"
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

log "Proof 4: Postgres user_skill_assessment_versions"
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

log "Proof 5: ClickHouse learner_model_update_logs"
ch_query "
SELECT
  source,
  skill_code,
  old_mastery_score,
  new_mastery_score,
  old_confidence,
  new_confidence,
  analysis_run_id,
  created_at
FROM learner_model_update_logs
WHERE user_id = '$USER_ID'
ORDER BY created_at DESC
LIMIT 10
FORMAT PrettyCompact
"

log "Final API progress"
curl -fsS "$BASE/progress/me" \
  -H "Authorization: Bearer $TOKEN" \
  | jq '{
      completed_tasks,
      total_attempts,
      analysis_state,
      skills: [.skills[] | {
        skill_code,
        mastery_score,
        confidence,
        attempts_count,
        success_count,
        updated_at
      }],
      recommended_skills
    }'

log "Requesting next task from planner"

NEXT_BODY_FILE="$(mktemp)"
trap 'rm -f "$NEXT_BODY_FILE"' EXIT

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
    ERROR_CODE="$(jq -r '.error // empty' "$NEXT_BODY_FILE")"

    if [[ "$ERROR_CODE" == "analysis_pending" ]]; then
      RETRY_AFTER="$(jq -r '.retry_after_seconds // empty' "$NEXT_BODY_FILE")"
      [[ "$RETRY_AFTER" =~ ^[0-9]+$ ]] || RETRY_AFTER="$POLL_INTERVAL_SECONDS"

      printf 'planner says analysis_pending, retrying after %ss\n' "$RETRY_AFTER"
      jq . "$NEXT_BODY_FILE"
      sleep "$RETRY_AFTER"
      continue
    fi
  fi

  printf 'Unexpected /tasks/next response, http=%s\n' "$NEXT_HTTP_CODE" >&2
  cat "$NEXT_BODY_FILE" >&2
  exit 1
done

[[ "$NEXT_HTTP_CODE" == "200" ]] || fail "planner did not return next task within ${MAX_WAIT_SECONDS}s"

NEXT_TASK_JSON="$(cat "$NEXT_BODY_FILE")"
NEXT_TASK_ID="$(printf '%s\n' "$NEXT_TASK_JSON" | jq -r '.task.id // empty')"

[[ -n "$NEXT_TASK_ID" ]] || fail "planner returned no task.id"

ok "planner returned next_task_id=$NEXT_TASK_ID"

printf '%s\n' "$NEXT_TASK_JSON" | jq '{
  selected_task: {
    id: .task.id,
    title: .task.title,
    difficulty: .task.difficulty,
    description: .task.description,
    skills: [.task.skills[]? | {
      skill_code,
      weight
    }]
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

if [[ "$NEXT_TASK_ID" == "$TASK_ID" ]]; then
  REPEAT_MODE="$(printf '%s\n' "$NEXT_TASK_JSON" | jq -r '.repeat_mode')"
  printf '\033[1;33mWARN\033[0m planner returned the same task_id=%s repeat_mode=%s\n' "$NEXT_TASK_ID" "$REPEAT_MODE"
fi

log "Explaining why the selected task fits the current learner model"

PROGRESS_JSON="$(curl -fsS "$BASE/progress/me" \
  -H "Authorization: Bearer $TOKEN")"

jq -n \
  --argjson next "$NEXT_TASK_JSON" \
  --argjson progress "$PROGRESS_JSON" '
  {
    next_task: {
      id: $next.task.id,
      title: $next.task.title,
      difficulty: $next.task.difficulty,
      score: $next.score,
      reason: $next.reason,
      repeat_mode: $next.repeat_mode
    },
    task_skill_match: [
      $next.task.skills[]? as $ts |
      {
        skill_code: $ts.skill_code,
        task_weight: $ts.weight,
        learner_skill: (
          $progress.skills[]?
          | select(.skill_code == $ts.skill_code)
          | {
              mastery_score,
              confidence,
              effective_mastery,
              retention,
              mastery_threshold,
              graph_priority_weight,
              attempts_count,
              success_count
            }
        ),
        llm_recommendation: (
          $next.recommended_skills[]?
          | select(.skill_code == $ts.skill_code)
          | {
              priority,
              recommended_action,
              reason
            }
        )
      }
    ],
    all_current_skills: [
      $progress.skills[]? | {
        skill_code,
        mastery_score,
        confidence,
        effective_mastery,
        mastery_threshold,
        graph_priority_weight,
        attempts_count,
        success_count
      }
    ]
  }'

ok "E2E smoke test passed"
printf '\nUSER_ID=%s\nEMAIL=%s\nEVENT_ID=%s\nRUN_ID=%s\nNEXT_TASK_ID=%s\n' \
  "$USER_ID" "$EMAIL" "$EVENT_ID" "$RUN_ID" "$NEXT_TASK_ID"
