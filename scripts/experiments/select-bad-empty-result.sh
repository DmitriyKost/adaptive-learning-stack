#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/select-bad-common.sh"

register_user "empty-result"

execute_attempt "bad-1: empty result" \
  "SELECT id, name FROM task_data.employees WHERE 1 = 0 ORDER BY id;"

execute_attempt "bad-2: one arbitrary row only" \
  "SELECT id, name FROM task_data.employees ORDER BY id LIMIT 1;"

execute_attempt "bad-3: wrong ordering" \
  "SELECT id, name FROM task_data.employees ORDER BY name DESC;"

execute_attempt "final-correct: complete task to trigger analytics" \
  "$CORRECT_QUERY"

wait_for_llm
print_proof
