#!/usr/bin/env bash

BASE="${BASE:-http://localhost:8080}"
INTELLIGENCE_BASE="${INTELLIGENCE_BASE:-http://localhost:8090}"
CLICKHOUSE_CONTAINER="${CLICKHOUSE_CONTAINER:-adaptive-clickhouse}"
TASK_PROGRESS_PG_CONTAINER="${TASK_PROGRESS_PG_CONTAINER:-adaptive-task-progress-postgres}"
PASSWORD="${PASSWORD:-password123}"
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

need_base_tools() {
  need curl
  need jq
  need docker
}

now_ms() {
  date +%s%3N
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

check_readiness() {
  log "Checking readiness"

  BASE_READY="$(curl -sS "$BASE/ready")"
  printf '%s\n' "$BASE_READY" | jq .
  printf '%s\n' "$BASE_READY" | jq -e '.status == "ready"' >/dev/null || fail "gateway is not ready"

  INT_READY="$(curl -sS "$INTELLIGENCE_BASE/ready")"
  printf '%s\n' "$INT_READY" | jq .
  printf '%s\n' "$INT_READY" | jq -e '.status == "ready"' >/dev/null || fail "intelligence is not ready"
}

prewarm_intelligence() {
  log "Prewarming intelligence model if needed"

  READY_JSON="$(curl -fsS "$INTELLIGENCE_BASE/ready")"
  LOADED="$(printf '%s\n' "$READY_JSON" | jq -r '.loaded // true')"

  if [[ "$LOADED" != "false" ]]; then
    ok "intelligence already loaded"
    return
  fi

  WARMUP_BODY="$(mktemp)"
  WARMUP_ARGS=(-sS -X POST "$INTELLIGENCE_BASE/warmup" -H 'Content-Type: application/json' -d '{}')

  if [[ -n "${INTELLIGENCE_API_KEY:-}" ]]; then
    WARMUP_ARGS+=(-H "X-API-Key: $INTELLIGENCE_API_KEY")
  fi

  WARMUP_HTTP_CODE="$(curl "${WARMUP_ARGS[@]}" -o "$WARMUP_BODY" -w '%{http_code}' || true)"
  cat "$WARMUP_BODY" | jq . || cat "$WARMUP_BODY"
  rm -f "$WARMUP_BODY"

  [[ "$WARMUP_HTTP_CODE" == "200" ]] || fail "warmup failed with HTTP $WARMUP_HTTP_CODE"
  ok "intelligence model loaded"
}

register_student() {
  local prefix="${1:-student+e2e}"
  EMAIL="${prefix}-$(date +%s)@example.com"

  REGISTER_RESPONSE="$(curl -fsS -X POST "$BASE/auth/register" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc \
      --arg email "$EMAIL" \
      --arg password "$PASSWORD" \
      '{email:$email,password:$password}')")"

  TOKEN="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.access_token')"
  USER_ID="$(printf '%s\n' "$REGISTER_RESPONSE" | jq -r '.user.id')"

  [[ -n "$TOKEN" && "$TOKEN" != "null" ]] || fail "no token"
  [[ -n "$USER_ID" && "$USER_ID" != "null" ]] || fail "no user id"

  ok "user_id=$USER_ID"
}

assert_json_has_no_field() {
  local json="$1"
  local field="$2"
  local label="$3"

  if printf '%s\n' "$json" | jq -e --arg field "$field" '.. | objects | select(has($field))' >/dev/null; then
    printf '%s\n' "$json" | jq --arg field "$field" '.. | objects | select(has($field))' >&2
    fail "$label leaks $field"
  fi

  ok "$label does not contain $field"
}

get_next_task_http() {
  local token="$1"
  local body_file="$2"

  curl -sS \
    -o "$body_file" \
    -w '%{http_code}' \
    -H "Authorization: Bearer $token" \
    "$BASE/tasks/next" || true
}

solve_task_from_json() {
  local task_json="$1"

  local skills
  skills="$(printf '%s\n' "$task_json" | jq -r '[.task.skills[]?.skill_code] | sort | join(",")')"

  case "$skills" in
    "select")
      printf '%s\n' 'SELECT id, name FROM task_data.employees ORDER BY id;'
      ;;
    "select,where")
      printf '%s\n' "SELECT id, name FROM task_data.employees WHERE department = 'Engineering' ORDER BY id;"
      ;;
    "order_by,select")
      printf '%s\n' 'SELECT id, name, salary FROM task_data.employees ORDER BY salary DESC;'
      ;;
    *)
      printf 'Unsupported task skills for helper: %s\n' "$skills" >&2
      return 1
      ;;
  esac
}

wait_for_post_submit_next_task() {
  local user_id="$1"
  local initial_task_id="${2:-}"

  DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
  RUN_ID=""
  NEXT_TASK_ID=""

  while (( SECONDS < DEADLINE )); do
    RUN_ID="$(ch_query "
SELECT run_id
FROM llm_analysis_runs
WHERE user_id = '$user_id'
  AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1
FORMAT TSV
" | tr -d '[:space:]')"

    VERSION_COUNT="$(pg_query "
SELECT count(*)
FROM user_skill_assessment_versions
WHERE user_id = '$user_id'::uuid
  AND model_version = 'sql-intelligence-v1';
" | tr -d '[:space:]')"

    ANALYTICS_UPDATE_COUNT="$(ch_query "
SELECT count()
FROM learner_model_update_logs
WHERE user_id = '$user_id'
  AND source = 'analytics-service'
FORMAT TSV
" | tr -d '[:space:]')"

    NEXT_ROW="$(pg_query "
SELECT status || '|' || COALESCE(task_id::text, '')
FROM user_next_task_recommendations
WHERE user_id = '$user_id'::uuid;
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
      if [[ -z "$initial_task_id" || "$NEXT_TASK_ID" != "$initial_task_id" ]]; then
        return 0
      fi
    fi

    sleep "$POLL_INTERVAL_SECONDS"
  done

  fail "post-submit next task did not become ready"
}
