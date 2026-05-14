#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/select-bad-common.sh"

register_user "missing-columns"

execute_attempt "bad-1: only name, missing id" \
  'SELECT name FROM task_data.employees ORDER BY id;'

execute_attempt "bad-2: only id, missing name" \
  'SELECT id FROM task_data.employees ORDER BY id;'

execute_attempt "bad-3: wrong column set and order" \
  'SELECT name, id FROM task_data.employees ORDER BY name;'

execute_attempt "final-correct: complete task to trigger analytics" \
  "$CORRECT_QUERY"

wait_for_llm
print_proof
