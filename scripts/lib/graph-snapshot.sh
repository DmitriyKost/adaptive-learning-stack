#!/usr/bin/env bash
# graph-snapshot.sh — рендер графа навыков пользователя прямо из БД task-progress.
# Узлы графа со связями (skill_dependencies) не отдаются публичным API,
# поэтому для наглядной демонстрации читаем их напрямую из Postgres.
#
# Usage: graph_snapshot <USER_ID> [HEADER]
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-adaptive-task-progress-postgres}"
PG_USER="${PG_USER:-task_progress}"
PG_DB="${PG_DB:-task_progress}"

# Декод user_id из JWT (sub) — payload это 2-й сегмент base64url.
jwt_sub() {
  local payload="${1#*.}"; payload="${payload%%.*}"
  local pad=$(( (4 - ${#payload} % 4) % 4 ))
  printf '%s' "$payload$(printf '=%.0s' $(seq 1 $pad))" \
    | tr '_-' '/+' | base64 -d 2>/dev/null | jq -r '.sub // .user_id // empty'
}

# psql-helper
pg() { docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -At -F '|' -v ON_ERROR_STOP=1 "$@"; }

graph_snapshot() {
  local user_id="$1"
  local header="${2:-Граф навыков}"

  # graph_id пользователя
  local gid
  gid=$(pg -c "SELECT graph_id FROM user_learning_profiles WHERE user_id='${user_id}';" || true)
  if [[ -z "$gid" ]]; then
    printf '\033[2m(профиль ещё не создан)\033[0m\n'
    return 0
  fi

  local out=""
  out+=$(printf '\033[1;35m╔══ %s ══\033[0m' "$header")$'\n'

  # Узлы: position, code, mastery, threshold, mastered. mastery из user_skills (LEFT JOIN).
  # Бар рисуем из mastery_score, порог отмечаем символом ┊.
  local rows
  rows=$(pg <<SQL
SELECT
  gs.position,
  s.name,
  COALESCE(us.mastery_score, 0)::float8,
  gs.mastery_threshold::float8,
  (COALESCE(us.mastery_score,0) >= gs.mastery_threshold) AS mastered,
  COALESCE(us.confidence, 0)::float8
FROM graph_skills gs
JOIN skills s ON s.id = gs.skill_id
LEFT JOIN user_skills us ON us.skill_id = gs.skill_id AND us.user_id = '${user_id}'
WHERE gs.graph_id = '${gid}'
ORDER BY gs.position;
SQL
)

  # Рёбра графа (зависимости): code -> depends_on_code
  local edges
  edges=$(pg <<SQL
SELECT s.name || ' → ' || d.name
FROM skill_dependencies sd
JOIN skills s ON s.id = sd.skill_id
JOIN skills d ON d.id = sd.depends_on_skill_id
WHERE sd.graph_id = '${gid}'
ORDER BY s.name;
SQL
)

  # Рендер узлов с mastery-баром (20 ячеек).
  while IFS='|' read -r pos name mastery threshold mastered confidence; do
    [[ -z "$pos" ]] && continue
    local filled bar pct cpct thr_cell mark color
    pct=$(awk -v m="$mastery" 'BEGIN{printf "%d", m*100}')
    cpct=$(awk -v c="$confidence" 'BEGIN{printf "%d", c*100}')
    filled=$(awk -v m="$mastery" 'BEGIN{printf "%d", m*20+0.5}')
    thr_cell=$(awk -v t="$threshold" 'BEGIN{printf "%d", t*20+0.5}')
    bar=""
    for ((i=1;i<=20;i++)); do
      if (( i == thr_cell )); then bar+="┊"
      elif (( i <= filled )); then bar+="█"
      else bar+="░"; fi
    done
    if [[ "$mastered" == "t" ]]; then color="1;32"; mark="✔"; else color="1;33"; mark="·"; fi
    out+=$(printf '  \033[%sm%s\033[0m \033[%sm[%s]\033[0m m:%3d%% c:%3d%%  %s' \
      "$color" "$mark" "$color" "$bar" "$pct" "$cpct" "$name")$'\n'
  done <<< "$rows"

  # Рёбра
  local edge first=1 edge_line="  связи: "
  while IFS= read -r edge; do
    [[ -z "$edge" ]] && continue
    if (( first )); then first=0; else edge_line+="; "; fi
    edge_line+="$edge"
  done <<< "$edges"
  out+=$(printf '\033[2m%s\033[0m' "$edge_line")$'\n'
  out+=$(printf '\033[2m  m = mastery; c = confidence; ┊ = порог; █ = уровень; ✔ = освоен\033[0m')$'\n'

  # Один write — без посимвольного мерцания.
  printf '%s' "$out"
}

# graph_watch — для split-панели: перерисовка курсором-домой (без clear → нет мерцания).
graph_watch() {
  local user_id="$1" header="${2:-Граф навыков (живой)}" interval="${3:-2}"
  printf '\033[2J\033[H'   # один раз очищаем
  while :; do
    local frame; frame="$(graph_snapshot "$user_id" "$header")"
    printf '\033[H%s\033[J' "$frame"   # домой, кадр, стереть остаток вниз
    sleep "$interval"
  done
}
