#!/usr/bin/env bash
# demo.sh — ~30-секундная демонстрация сквозного сценария.
# Показывает: регистрацию, текст задачи + SQL-запрос, выполнение,
# мгновенный локальный прогресс, граф навыков и его уточнение
# после асинхронной внешней оценки, выбор следующего задания по графу.
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
EMAIL="demo-$(date +%s)@example.com"
PASS="password123"
TASK_ID="00000000-0000-0000-0000-000000010001"
USER_QUERY="SELECT id, name FROM task_data.employees ORDER BY id;"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/graph-dump.sh
source "$SCRIPT_DIR/lib/graph-dump.sh"

# Если задан GRAPH_JSON — граф визуализируется в браузере (graph.html),
# demo лишь обновляет этот файл. Иначе печатается текстовый снимок.
GRAPH_JSON="${GRAPH_JSON:-}"
PRIORITY_CODE=""   # skill_code приоритетного навыка (заполняется из /tasks/next)
graph_show() {  # <phase-header>
  local phase="$1"
  if [[ -n "$GRAPH_JSON" ]]; then
    graph_dump "$USER_ID" "$GRAPH_JSON" "$phase" "$PRIORITY_CODE"
    printf '  \033[2m→ граф обновлён в браузере: %s\033[0m\n' "$phase"
  else
    # текстовый fallback (если запущено без HTML)
    source "$SCRIPT_DIR/lib/graph-snapshot.sh" 2>/dev/null || true
    if declare -F graph_snapshot >/dev/null; then graph_snapshot "$USER_ID" "$phase"; fi
  fi
}

step() { printf "\n\033[1;36m▶ %s\033[0m\n" "$1"; }
sql_block() { printf "\033[1;34m  SQL ┃\033[0m \033[0;37m%s\033[0m\n" "$1"; }

step "1. Регистрация пользователя"
TOKEN=$(curl -s -X POST "$BASE/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" \
  | jq -r '.access_token')
echo "  token: ${TOKEN:0:24}…"
USER_ID="$(jwt_sub "$TOKEN")"
# Для split-режима record-demo.sh (правая панель читает user_id отсюда).
[[ -n "${USER_ID_FILE:-}" ]] && printf '%s' "$USER_ID" > "$USER_ID_FILE"
# Стартовый снимок графа (нулевой) — чтобы браузер сразу что-то показал.
[[ -n "$GRAPH_JSON" ]] && { curl -s "$BASE/progress/me" -H "Authorization: Bearer $TOKEN" >/dev/null; graph_dump "$USER_ID" "$GRAPH_JSON" "Старт · до выполнения" || true; }
sleep 1.5

step "2. Задание и SQL-запрос пользователя"
# Текст задачи берём из API (как видит пользователь).
TASK_JSON=$(curl -s "$BASE/tasks/$TASK_ID" -H "Authorization: Bearer $TOKEN")
echo "$TASK_JSON" | jq -r '"  \u001b[1mЗадача:\u001b[0m " + .title + " [" + .difficulty + "]\n  " + .description'
sql_block "$USER_QUERY"
sleep 2.5

step "3. Выполнение запроса"
curl -s -X POST "$BASE/playground/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"task_id\":\"$TASK_ID\",\"user_query\":\"$USER_QUERY\"}" \
  | jq '{is_correct, rows: .user_result.rows_affected, ms: .user_result.query_time_ms}'
# Инициализируем профиль (создаётся при первом обращении к прогрессу).
curl -s "$BASE/progress/me" -H "Authorization: Bearer $TOKEN" >/dev/null
sleep 1.5

step "4. Граф навыков обновлён локально сразу"
graph_show "После выполнения · локальная оценка"
sleep 3

step "5. Ожидание асинхронной внешней оценки…"
# Поллим analysis_state: pending → done. Потолок 12с, чтобы ролик не растягивался.
DEADLINE=$(( $(date +%s) + 12 ))
while :; do
  STATE=$(curl -s "$BASE/progress/me" -H "Authorization: Bearer $TOKEN" \
    | jq -r '.analysis_state.status // "done"')
  [[ "$STATE" != "pending" ]] && break
  (( $(date +%s) >= DEADLINE )) && break
  printf '  \033[2m…оценка выполняется (%s)\033[0m\r' "$STATE"
  [[ -n "$GRAPH_JSON" ]] && graph_dump "$USER_ID" "$GRAPH_JSON" "Применяется внешняя оценка…" || true
  sleep 1
done
printf '  \033[0;32m✓ внешняя оценка применена\033[0m       \n'

step "6. Граф уточнён внешней оценкой"
graph_show "После внешней оценки · уточнено"
sleep 3

step "7. Следующее задание по графу навыков"
NEXT_JSON=$(curl -s "$BASE/tasks/next" -H "Authorization: Bearer $TOKEN")
echo "$NEXT_JSON" | jq '{title: .task.title, difficulty: .task.difficulty, score, reason, repeat_mode}'
# Приоритетный навык: сперва из reason ("…для навыка <code>:"), иначе из
# recommended_skills с максимальным priority.
PRIORITY_CODE=$(echo "$NEXT_JSON" | jq -r '
  (.reason // "" | capture("навыка (?<c>[a-z_]+)") .c) //
  (.recommended_skills // [] | sort_by(-.priority) | .[0].skill_code) //
  ""' 2>/dev/null || echo "")
if [[ -n "$GRAPH_JSON" && -n "$PRIORITY_CODE" ]]; then
  graph_dump "$USER_ID" "$GRAPH_JSON" "Приоритет: $PRIORITY_CODE" "$PRIORITY_CODE"
  printf '  \033[2m→ приоритетный навык подсвечен на графе: %s\033[0m\n' "$PRIORITY_CODE"
fi
sleep 3.5
