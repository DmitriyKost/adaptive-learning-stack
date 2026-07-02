#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
INTEL="${INTEL:-http://localhost:8090}"
TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
QUERY="${QUERY:-SELECT id, name FROM task_data.employees ORDER BY id;}"
EMAIL="${EMAIL:-student-smoke-$(date +%s)@example.com}"
PASSWORD="${PASSWORD:-password123}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[FAIL] missing command: $1" >&2
    exit 1
  }
}

ok() {
  printf '[OK] %s\n' "$*"
}

fail() {
  printf '[FAIL] %s\n' "$*" >&2
  exit 1
}

short() {
  printf '%s' "$1" | cut -c1-8
}

ch_query() {
  docker exec adaptive-clickhouse clickhouse-client \
    --user analytics \
    --password analytics \
    --database analytics \
    --query "$1" 2>/dev/null | tr -d '\r'
}

need curl
need jq
need docker

GATEWAY_STATUS="$(curl -fsS "$BASE/ready" | jq -r '.status // "unknown"')"
INTEL_STATUS="$(curl -fsS "$INTEL/ready" | jq -r '.status // "unknown"')"
ok "ready gateway=$GATEWAY_STATUS external=$INTEL_STATUS"

REGISTER_RESPONSE="$(
  curl -fsS -X POST "$BASE/auth/register" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
)"

TOKEN="$(printf '%s' "$REGISTER_RESPONSE" | jq -r '.access_token // empty')"
USER_ID="$(printf '%s' "$REGISTER_RESPONSE" | jq -r '.user.id // empty')"

[[ -n "$TOKEN" && -n "$USER_ID" ]] || fail "registration failed"
ok "user email=$EMAIL id=$(short "$USER_ID")"

EXEC_PAYLOAD="$(
  jq -n \
    --arg task_id "$TASK_ID" \
    --arg user_query "$QUERY" \
    '{task_id: $task_id, user_query: $user_query}'
)"

EXEC_RESPONSE="$(
  curl -fsS -X POST "$BASE/playground/execute" \
    -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' \
    -d "$EXEC_PAYLOAD"
)"

EVENT_ID="$(printf '%s' "$EXEC_RESPONSE" | jq -r '.event_id // empty')"
ROWS="$(printf '%s' "$EXEC_RESPONSE" | jq -r '.user_result.rows_affected // (.user_result.rows | length) // 0')"
MS="$(printf '%s' "$EXEC_RESPONSE" | jq -r '.user_result.query_time_ms // "-"')"

[[ -n "$EVENT_ID" ]] || fail "execute failed"
ok "execute event=$(short "$EVENT_ID") rows=$ROWS ms=$MS"

PROGRESS_JSON=""
SELECT_LINE=""

for _ in $(seq 1 30); do
  PROGRESS_JSON="$(curl -fsS "$BASE/progress/me" -H "Authorization: Bearer $TOKEN")"
  SELECT_LINE="$(
    printf '%s' "$PROGRESS_JSON" |
      jq -r '.skills[]? | select(.skill_code=="select") |
        "\(.mastery_score)|\(.confidence)|\(.attempts_count)|\(.success_count)"' |
      head -n 1
  )"

  [[ -n "$SELECT_LINE" ]] && break
  sleep 1
done

[[ -n "$SELECT_LINE" ]] || fail "progress was not updated"

COMPLETED="$(printf '%s' "$PROGRESS_JSON" | jq -r '.completed_tasks // 0')"
IFS='|' read -r MASTERY CONFIDENCE ATTEMPTS SUCCESSES <<< "$SELECT_LINE"
ok "progress completed=$COMPLETED attempts=$ATTEMPTS select=${MASTERY}/${CONFIDENCE}"

ANALYSIS_LINE=""

for _ in $(seq 1 60); do
  ANALYSIS_LINE="$(
    ch_query "
      SELECT concat(toString(run_id), '|', status)
      FROM llm_analysis_runs
      WHERE user_id = '$USER_ID'
      ORDER BY completed_at DESC
      LIMIT 1
      FORMAT TSVRaw
    " || true
  )"

  [[ "$ANALYSIS_LINE" == *"|completed"* ]] && break
  sleep 2
done

[[ "$ANALYSIS_LINE" == *"|completed"* ]] || fail "external analysis was not completed"

RUN_ID="${ANALYSIS_LINE%%|*}"
ok "analysis run=$(short "$RUN_ID") status=completed"

LEARNER_LINE=""

for _ in $(seq 1 30); do
  LEARNER_LINE="$(
    ch_query "
      SELECT concat(skill_code, '|', toString(new_mastery_score), '|', toString(new_confidence))
      FROM learner_model_update_logs
      WHERE user_id = '$USER_ID'
        AND source = 'analytics-service'
      ORDER BY created_at DESC
      LIMIT 1
      FORMAT TSVRaw
    " || true
  )"

  [[ -n "$LEARNER_LINE" ]] && break
  sleep 1
done

[[ -n "$LEARNER_LINE" ]] || fail "external assessment was not applied"

IFS='|' read -r SKILL NEW_MASTERY NEW_CONFIDENCE <<< "$LEARNER_LINE"
ok "learner source=analytics $SKILL=${NEW_MASTERY}/${NEW_CONFIDENCE}"

NEXT_JSON=""

for _ in $(seq 1 15); do
  NEXT_JSON="$(curl -fsS "$BASE/tasks/next" -H "Authorization: Bearer $TOKEN")"

  NEXT_TITLE="$(
    printf '%s' "$NEXT_JSON" |
      jq -r '(.task // .next_task // .recommendation.task // .) as $t |
        ($t.title // $t.name // empty)'
  )"

  [[ -n "$NEXT_TITLE" ]] && break
  sleep 1
done

[[ -n "${NEXT_TITLE:-}" ]] || fail "next task was not returned"

NEXT_ID="$(
  printf '%s' "$NEXT_JSON" |
    jq -r '(.task // .next_task // .recommendation.task // .) as $t |
      ($t.id // $t.task_id // "-")'
)"

NEXT_DIFF="$(
  printf '%s' "$NEXT_JSON" |
    jq -r '(.task // .next_task // .recommendation.task // .) as $t |
      ($t.difficulty // "-")'
)"

ok "next task=$(short "$NEXT_ID") title=\"$NEXT_TITLE\" difficulty=$NEXT_DIFF"
ok "smoke passed"
