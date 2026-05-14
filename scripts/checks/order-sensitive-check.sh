#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/e2e-common.sh"

ORDER_TASK_ID="${ORDER_TASK_ID:-00000000-0000-0000-0000-000000010003}"

need_base_tools
check_readiness

log "Checking internal comparison policy if internal port is available"

if curl -fsS "http://localhost:8083/internal/tasks/$ORDER_TASK_ID/reference" >/tmp/order-policy.json 2>/dev/null; then
  cat /tmp/order-policy.json | jq .
  POLICY="$(cat /tmp/order-policy.json | jq -r '.comparison_policy.order_sensitive')"
  [[ "$POLICY" == "true" ]] || fail "ORDER task should have order_sensitive=true"
  ok "internal policy order_sensitive=true"
else
  warn "internal task-progress endpoint is not available from host; skipping direct policy check"
fi
rm -f /tmp/order-policy.json

register_student "student+order-sensitive"

log "Bad ORDER BY should be incorrect"

BAD_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$ORDER_TASK_ID" \
    --arg user_query "SELECT id, name, salary FROM task_data.employees ORDER BY id;" \
    '{task_id:$task_id,user_query:$user_query}')")"

printf '%s\n' "$BAD_RESPONSE" | jq '{
  execution_success,
  is_correct,
  has_reference_result: has("reference_result"),
  rows: .user_result.rows
}'

[[ "$(printf '%s\n' "$BAD_RESPONSE" | jq -r '.execution_success')" == "true" ]] || fail "bad order query did not execute"
[[ "$(printf '%s\n' "$BAD_RESPONSE" | jq -r '.is_correct')" == "false" ]] || fail "bad order query was accepted"
[[ "$(printf '%s\n' "$BAD_RESPONSE" | jq -r 'has("reference_result")')" == "false" ]] || fail "bad order response leaked reference_result"

ok "bad ORDER BY rejected"

log "Correct ORDER BY should be correct"

GOOD_RESPONSE="$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg task_id "$ORDER_TASK_ID" \
    --arg user_query "SELECT id, name, salary FROM task_data.employees ORDER BY salary DESC;" \
    '{task_id:$task_id,user_query:$user_query}')")"

printf '%s\n' "$GOOD_RESPONSE" | jq '{
  execution_success,
  is_correct,
  has_reference_result: has("reference_result"),
  rows: .user_result.rows
}'

[[ "$(printf '%s\n' "$GOOD_RESPONSE" | jq -r '.execution_success')" == "true" ]] || fail "good order query did not execute"
[[ "$(printf '%s\n' "$GOOD_RESPONSE" | jq -r '.is_correct')" == "true" ]] || fail "good order query was not accepted"
[[ "$(printf '%s\n' "$GOOD_RESPONSE" | jq -r 'has("reference_result")')" == "false" ]] || fail "good order response leaked reference_result"

ok "order-sensitive integration check passed"
