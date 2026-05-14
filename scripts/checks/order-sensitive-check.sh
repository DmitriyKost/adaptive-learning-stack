#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
PASSWORD="${PASSWORD:-password123}"

ORDER_TASK_ID="${ORDER_TASK_ID:-00000000-0000-0000-0000-000000010003}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

ok() {
  echo "OK: $*"
}

EMAIL="student+order-sensitive-$(date +%s)@example.com"

REGISTER_RESPONSE=$(curl -fsS -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")

TOKEN=$(printf '%s' "$REGISTER_RESPONSE" | jq -r '.access_token')
USER_ID=$(printf '%s' "$REGISTER_RESPONSE" | jq -r '.user.id')

echo "USER_ID=$USER_ID"

echo
echo "==> Bad ORDER BY should be incorrect"

BAD_RESPONSE=$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{
    \"task_id\": \"$ORDER_TASK_ID\",
    \"user_query\": \"SELECT id, name, salary FROM task_data.employees ORDER BY id;\"
  }")

printf '%s\n' "$BAD_RESPONSE" | jq '{
  execution_success,
  is_correct,
  has_reference_result: has("reference_result"),
  columns: .user_result.columns,
  rows: .user_result.rows
}'

BAD_EXECUTION_SUCCESS=$(printf '%s\n' "$BAD_RESPONSE" | jq -r '.execution_success')
BAD_IS_CORRECT=$(printf '%s\n' "$BAD_RESPONSE" | jq -r '.is_correct')
BAD_HAS_REFERENCE=$(printf '%s\n' "$BAD_RESPONSE" | jq -r 'has("reference_result")')

[[ "$BAD_EXECUTION_SUCCESS" == "true" ]] || fail "bad order query did not execute successfully"
[[ "$BAD_IS_CORRECT" == "false" ]] || fail "bad order query was incorrectly accepted"
[[ "$BAD_HAS_REFERENCE" == "false" ]] || fail "bad response leaked reference_result"

ok "bad ORDER BY rejected"

echo
echo "==> Correct ORDER BY should be correct"

GOOD_RESPONSE=$(curl -fsS -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{
    \"task_id\": \"$ORDER_TASK_ID\",
    \"user_query\": \"SELECT id, name, salary FROM task_data.employees ORDER BY salary DESC;\"
  }")

printf '%s\n' "$GOOD_RESPONSE" | jq '{
  execution_success,
  is_correct,
  has_reference_result: has("reference_result"),
  columns: .user_result.columns,
  rows: .user_result.rows
}'

GOOD_EXECUTION_SUCCESS=$(printf '%s\n' "$GOOD_RESPONSE" | jq -r '.execution_success')
GOOD_IS_CORRECT=$(printf '%s\n' "$GOOD_RESPONSE" | jq -r '.is_correct')
GOOD_HAS_REFERENCE=$(printf '%s\n' "$GOOD_RESPONSE" | jq -r 'has("reference_result")')

[[ "$GOOD_EXECUTION_SUCCESS" == "true" ]] || fail "good order query did not execute successfully"
[[ "$GOOD_IS_CORRECT" == "true" ]] || fail "good order query was not accepted"
[[ "$GOOD_HAS_REFERENCE" == "false" ]] || fail "good response leaked reference_result"

ok "correct ORDER BY accepted"

echo
ok "order-sensitive integration check passed"
