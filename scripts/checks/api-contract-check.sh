#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/e2e-common.sh"

need_base_tools
check_readiness

TASK_ID="${TASK_ID:-00000000-0000-0000-0000-000000010001}"
EXPECTED_REF="${EXPECTED_REF:-SELECT id, name FROM task_data.employees ORDER BY id;}"

register_student "student+api-contract"

log "Public /tasks must not leak reference_sql"

TASKS_JSON="$(curl -fsS "$BASE/tasks" -H "Authorization: Bearer $TOKEN")"
assert_json_has_no_field "$TASKS_JSON" "reference_sql" "/tasks"

TASK_JSON="$(curl -fsS "$BASE/tasks/$TASK_ID" -H "Authorization: Bearer $TOKEN")"
assert_json_has_no_field "$TASK_JSON" "reference_sql" "/tasks/{id}"

log "Public /playground/execute must reject client-provided reference_query"

BODY_FILE="$(mktemp)"
HTTP_CODE="$(curl -sS -o "$BODY_FILE" -w '%{http_code}' \
  -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$TASK_ID" \
    --arg user_query "$EXPECTED_REF" \
    --arg reference_query "SELECT 1 AS hacked_reference;" \
    '{task_id:$task_id,user_query:$user_query,reference_query:$reference_query}')")"

cat "$BODY_FILE" | jq . || cat "$BODY_FILE"
rm -f "$BODY_FILE"

[[ "$HTTP_CODE" == "400" ]] || fail "reference_query should be rejected with 400, got $HTTP_CODE"
ok "reference_query rejected"

log "Legacy public /tasks/{id}/submit must be disabled"

BODY_FILE="$(mktemp)"
HTTP_CODE="$(curl -sS -o "$BODY_FILE" -w '%{http_code}' \
  -X POST "$BASE/tasks/$TASK_ID/submit" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "submitted_sql": "fake",
    "execution_success": true,
    "is_correct": true,
    "execution_time_ms": 1,
    "row_count": 1
  }')"

cat "$BODY_FILE" | jq . || cat "$BODY_FILE"
rm -f "$BODY_FILE"

[[ "$HTTP_CODE" == "410" || "$HTTP_CODE" == "403" || "$HTTP_CODE" == "404" ]] \
  || fail "legacy submit should be disabled, got HTTP $HTTP_CODE"

ok "legacy submit disabled"

log "Normal /playground/execute contract"

EXEC_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$TASK_ID" \
    --arg user_query "$EXPECTED_REF" \
    '{task_id:$task_id,user_query:$user_query}')")"

printf '%s\n' "$EXEC_RESPONSE" | jq '{
  event_id,
  execution_success,
  is_correct,
  has_user_result: has("user_result"),
  has_reference_result: has("reference_result")
}'

EXECUTION_SUCCESS="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.execution_success')"
IS_CORRECT="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r '.is_correct')"
HAS_REFERENCE_RESULT="$(printf '%s\n' "$EXEC_RESPONSE" | jq -r 'has("reference_result")')"

[[ "$EXECUTION_SUCCESS" == "true" ]] || fail "execution_success should be true"
[[ "$IS_CORRECT" == "true" ]] || fail "is_correct should be true"
[[ "$HAS_REFERENCE_RESULT" == "false" ]] || fail "reference_result leaked"

ok "execute response is frontend-safe"

log "Internal reference must still reach analytics"

DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
REF_SQL=""

while (( SECONDS < DEADLINE )); do
  REF_SQL="$(ch_query "
SELECT reference_sql
FROM task_attempt_logs
WHERE user_id = '$USER_ID'
ORDER BY created_at DESC
LIMIT 1
FORMAT TSV
" | sed 's/[[:space:]]*$//')"

  if [[ -n "$REF_SQL" ]]; then
    break
  fi

  sleep "$POLL_INTERVAL_SECONDS"
done

echo "analytics.reference_sql=[$REF_SQL]"

[[ "$REF_SQL" == "$EXPECTED_REF" ]] || fail "analytics did not receive expected DB reference SQL"

RAW_COUNTS="$(ch_query "
SELECT
  countIf(position(payload, 'hacked_reference') > 0),
  countIf(position(payload, 'reference_sql') > 0)
FROM raw_events
WHERE user_id = '$USER_ID'
FORMAT TSV
")"

HACKED_COUNT="$(printf '%s\n' "$RAW_COUNTS" | awk '{print $1}')"
REFERENCE_COUNT="$(printf '%s\n' "$RAW_COUNTS" | awk '{print $2}')"

[[ "$HACKED_COUNT" == "0" ]] || fail "fake reference found in raw_events"
[[ "$REFERENCE_COUNT" -gt 0 ]] || fail "reference_sql not found in raw_events"

ok "internal reference flow preserved"

log "Final /tasks/next must not leak reference_sql"

wait_for_post_submit_next_task "$USER_ID" "$TASK_ID"

FINAL_NEXT_JSON="$(curl -fsS "$BASE/tasks/next" -H "Authorization: Bearer $TOKEN")"
assert_json_has_no_field "$FINAL_NEXT_JSON" "reference_sql" "/tasks/next"

ok "API contract check passed"

printf '\nUSER_ID=%s\nEMAIL=%s\n' "$USER_ID" "$EMAIL"
