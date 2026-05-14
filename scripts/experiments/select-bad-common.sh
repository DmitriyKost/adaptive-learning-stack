#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/e2e-common.sh"

TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
CORRECT_QUERY="${CORRECT_QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"

register_user() {
  local scenario="$1"
  register_student "student+select-bad-${scenario}"
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
    execution_success,
    is_correct,
    has_reference_result: has("reference_result"),
    user_rows: (.user_result.rows | length),
    user_columns: .user_result.columns,
    created_at
  }'

  if printf '%s\n' "$RESPONSE" | jq -e 'has("reference_result")' >/dev/null; then
    fail "/playground/execute leaked reference_result"
  fi
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

  BODY_FILE="$(mktemp)"
  DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
  HTTP_CODE=""

  while (( SECONDS < DEADLINE )); do
    HTTP_CODE="$(get_next_task_http "$TOKEN" "$BODY_FILE")"

    if [[ "$HTTP_CODE" == "200" ]]; then
      break
    fi

    if [[ "$HTTP_CODE" == "202" || "$HTTP_CODE" == "409" ]]; then
      ERROR_CODE="$(jq -r '.error // .code // empty' "$BODY_FILE" 2>/dev/null || true)"
      if [[ "$ERROR_CODE" == "analysis_pending" ]]; then
        jq . "$BODY_FILE"
        sleep "$POLL_INTERVAL_SECONDS"
        continue
      fi
    fi

    printf 'Unexpected /tasks/next HTTP %s\n' "$HTTP_CODE" >&2
    cat "$BODY_FILE" >&2
    rm -f "$BODY_FILE"
    return
  done

  if [[ "$HTTP_CODE" == "200" ]]; then
    NEXT_JSON="$(cat "$BODY_FILE")"
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
  fi

  rm -f "$BODY_FILE"

  printf '\nUSER_ID=%s\nEMAIL=%s\nRUN_ID=%s\n' "$USER_ID" "$EMAIL" "$RUN_ID"
}
