#!/usr/bin/env bash
# graph-dump.sh — дамп текущего состояния графа навыков пользователя в JSON.
# Узлы (skill + mastery + threshold + confidence + mastered) и рёбра
# (skill_dependencies) читаются напрямую из Postgres task-progress,
# т.к. публичного API для связей графа нет. JSON потребляет graph.html.
#
# Usage: graph_dump <USER_ID> <OUT_JSON> [PHASE]
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-adaptive-task-progress-postgres}"
PG_USER="${PG_USER:-task_progress}"
PG_DB="${PG_DB:-task_progress}"

jwt_sub() {
  local payload="${1#*.}"; payload="${payload%%.*}"
  local pad=$(( (4 - ${#payload} % 4) % 4 ))
  printf '%s' "$payload$(printf '=%.0s' $(seq 1 $pad))" \
    | tr '_-' '/+' | base64 -d 2>/dev/null | jq -r '.sub // .user_id // empty'
}

pg() { docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -At -F $'\t' -v ON_ERROR_STOP=1 "$@"; }

graph_dump() {
  local user_id="$1"
  local out="$2"
  local phase="${3:-}"
  local priority_code="${4:-}"

  local gid
  gid=$(pg -c "SELECT graph_id FROM user_learning_profiles WHERE user_id='${user_id}';" 2>/dev/null || true)
  if [[ -z "$gid" ]]; then
    printf '{"phase":%s,"ready":false,"nodes":[],"edges":[]}\n' "$(jq -Rn --arg p "$phase" '$p')" > "$out.tmp"
    mv "$out.tmp" "$out"
    return 0
  fi

  # Узлы как JSON-массив. jq -R/-s собирает строки psql в объекты.
  local nodes_tsv edges_tsv graph_name
  graph_name=$(pg -c "SELECT name FROM learning_graphs WHERE id='${gid}';" 2>/dev/null || echo "graph")

  nodes_tsv=$(pg <<SQL
SELECT
  gs.skill_id,
  s.code,
  s.name,
  gs.position,
  COALESCE(us.mastery_score,0)::float8,
  gs.mastery_threshold::float8,
  COALESCE(us.confidence,0)::float8,
  (COALESCE(us.mastery_score,0) >= gs.mastery_threshold)
FROM graph_skills gs
JOIN skills s ON s.id = gs.skill_id
LEFT JOIN user_skills us ON us.skill_id = gs.skill_id AND us.user_id = '${user_id}'
WHERE gs.graph_id = '${gid}'
ORDER BY gs.position;
SQL
)

  edges_tsv=$(pg <<SQL
SELECT sd.skill_id, sd.depends_on_skill_id, sd.strength::float8
FROM skill_dependencies sd
WHERE sd.graph_id = '${gid}';
SQL
)

  local nodes_json edges_json
  nodes_json=$(printf '%s\n' "$nodes_tsv" | jq -R -s '
    [ split("\n")[] | select(length>0) | split("\t")
      | {id:.[0], code:.[1], name:.[2], position:(.[3]|tonumber),
         mastery:(.[4]|tonumber), threshold:(.[5]|tonumber),
         confidence:(.[6]|tonumber), mastered:(.[7]=="t")} ]')

  edges_json=$(printf '%s\n' "$edges_tsv" | jq -R -s '
    [ split("\n")[] | select(length>0) | split("\t")
      | {source:.[0], target:.[1], strength:(.[2]|tonumber)} ]')

  jq -n \
    --arg phase "$phase" \
    --arg graph "$graph_name" \
    --arg priority "$priority_code" \
    --argjson nodes "$nodes_json" \
    --argjson edges "$edges_json" \
    '{phase:$phase, graph:$graph, priority_skill:$priority, ready:true, ts:(now|floor), nodes:$nodes, edges:$edges}' \
    > "$out.tmp"
  mv "$out.tmp" "$out"   # атомарная замена — фронт не прочитает половину файла
}

# Прямой вызов: graph-dump.sh <user_id|token> <out.json> [phase]
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  arg="${1:?user_id или token}"
  out="${2:?out.json}"
  phase="${3:-}"
  # если передан JWT (содержит точки) — извлечь sub
  if [[ "$arg" == *.*.* ]]; then arg="$(jwt_sub "$arg")"; fi
  graph_dump "$arg" "$out" "$phase"
fi
