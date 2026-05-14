#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/select-bad-common.sh"

register_user "wrong-projection"

execute_attempt "bad-1: constant id" \
  "SELECT 1 AS id, name FROM task_data.employees ORDER BY id;"

execute_attempt "bad-2: department returned as name" \
  "SELECT id, department AS name FROM task_data.employees ORDER BY id;"

execute_attempt "bad-3: same columns but intentionally truncated rows" \
  "SELECT id, name FROM task_data.employees ORDER BY id LIMIT 2;"

execute_attempt "final-correct: complete task to trigger analytics" \
  "$CORRECT_QUERY"

wait_for_llm
print_proof
