#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
REQUESTS="${REQUESTS:-300}"
CONCURRENCY="${CONCURRENCY:-20}"
USERS="${USERS:-10}"
TASK_LIMIT="${TASK_LIMIT:-5}"
MODE="${MODE:-mixed}" # mixed | correct | wrong
WRONG_QUERY="${WRONG_QUERY:-SELECT 1 AS wrong_answer;}"
PASSWORD="password123"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

need curl
need jq
need awk
need sort
need docker

load_tasks() {
  docker exec adaptive-task-progress-postgres psql \
    -U task_progress \
    -d task_progress \
    -A -F $'\t' -t \
    -c "
SELECT
  id,
  regexp_replace(reference_sql, E'[\\n\\r\\t]+', ' ', 'g') AS reference_sql
FROM tasks
WHERE is_active = true
  AND NULLIF(btrim(reference_sql), '') IS NOT NULL
ORDER BY
  CASE difficulty
    WHEN 'easy' THEN 1
    WHEN 'medium' THEN 2
    WHEN 'hard' THEN 3
    ELSE 9
  END,
  id
LIMIT ${TASK_LIMIT};
"
}

echo "loading tasks from task-progress database..."
load_tasks > "$tmp/tasks.tsv"

TASK_COUNT="$(wc -l < "$tmp/tasks.tsv" | tr -d ' ')"

if [ "$TASK_COUNT" = "0" ]; then
  echo "no tasks with reference_query found" >&2
  exit 1
fi

: > "$tmp/cases.tsv"

while IFS=$'\t' read -r task_id reference_sql; do
  case "$MODE" in
    mixed)
      printf "%s\tcorrect\t%s\n" "$task_id" "$reference_sql" >> "$tmp/cases.tsv"
      printf "%s\twrong\t%s\n" "$task_id" "$WRONG_QUERY" >> "$tmp/cases.tsv"
      ;;
    correct)
      printf "%s\tcorrect\t%s\n" "$task_id" "$reference_sql" >> "$tmp/cases.tsv"
      ;;
    wrong)
      printf "%s\twrong\t%s\n" "$task_id" "$WRONG_QUERY" >> "$tmp/cases.tsv"
      ;;
    *)
      echo "unknown MODE=$MODE, use mixed|correct|wrong" >&2
      exit 1
      ;;
  esac
done < "$tmp/tasks.tsv"

CASE_COUNT="$(wc -l < "$tmp/cases.tsv" | tr -d ' ')"

echo "registering users..."
: > "$tmp/users.tsv"

for i in $(seq 1 "$USERS"); do
  email="student-execute-load-${i}-$(date +%s%N)@example.com"

  register_response="$(
    curl -fsS -X POST "$BASE/auth/register" \
      -H "Content-Type: application/json" \
      -d "{\"email\":\"$email\",\"password\":\"$PASSWORD\"}"
  )"

  token="$(printf "%s" "$register_response" | jq -r ".access_token")"
  user_id="$(printf "%s" "$register_response" | jq -r ".user.id")"

  printf "%s\t%s\t%s\n" "$i" "$user_id" "$token" >> "$tmp/users.tsv"
done

echo "warming up user workspaces..."

first_task_id="$(awk -F'\t' 'NR == 1 {print $1}' "$tmp/tasks.tsv")"

warmup_payload="$(
  jq -n \
    --arg task_id "$first_task_id" \
    --arg user_query "$WRONG_QUERY" \
    '{task_id: $task_id, user_query: $user_query}'
)"

while IFS=$'\t' read -r user_idx user_id token; do
  curl -fsS -o /dev/null \
    -X POST "$BASE/playground/execute" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -d "$warmup_payload"
done < "$tmp/users.tsv"

sleep 1

worker() {
  i="$1"

  user_idx=$(( (i - 1) % USERS + 1 ))
  case_idx=$(( (i - 1) % CASE_COUNT + 1 ))

  user_line="$(awk -F'\t' -v idx="$user_idx" 'NR == idx {print}' "$tmp/users.tsv")"
  case_line="$(awk -F'\t' -v idx="$case_idx" 'NR == idx {print}' "$tmp/cases.tsv")"

  user_id="$(printf "%s" "$user_line" | awk -F'\t' '{print $2}')"
  token="$(printf "%s" "$user_line" | awk -F'\t' '{print $3}')"

  task_id="$(printf "%s" "$case_line" | awk -F'\t' '{print $1}')"
  kind="$(printf "%s" "$case_line" | awk -F'\t' '{print $2}')"
  query="$(printf "%s" "$case_line" | cut -f3-)"

  payload="$(
    jq -n \
      --arg task_id "$task_id" \
      --arg user_query "$query" \
      '{task_id: $task_id, user_query: $user_query}'
  )"

  body="$tmp/body_$i.json"

  started="$(date +%s%3N)"

  http_code="$(
    curl -sS -o "$body" \
      -w "%{http_code}" \
      -X POST "$BASE/playground/execute" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      -d "$payload" || echo "curl_error"
  )"

  finished="$(date +%s%3N)"
  latency_ms=$((finished - started))

  event_id="$(jq -r '.event_id // "-"' "$body" 2>/dev/null || echo "-")"
  rows="$(jq -r '.user_result.rows_affected // 0' "$body" 2>/dev/null || echo "0")"
  columns="$(jq -r '(.user_result.columns // []) | join(",")' "$body" 2>/dev/null || echo "-")"

  printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
    "$i" "$user_idx" "$user_id" "$kind" "$task_id" "$http_code" "$latency_ms" "$rows" "$columns" "$event_id" \
    > "$tmp/result_$i.tsv"
}

export -f worker
export BASE tmp USERS CASE_COUNT

echo
echo "execute mixed API load test"
echo "base=$BASE"
echo "endpoint=/playground/execute"
echo "requests=$REQUESTS concurrency=$CONCURRENCY users=$USERS"
echo "tasks=$TASK_COUNT cases=$CASE_COUNT mode=$MODE"
echo "wrong_query=$WRONG_QUERY"
echo

start_total="$(date +%s%3N)"

seq "$REQUESTS" | xargs -I{} -P "$CONCURRENCY" bash -c 'worker "$@"' _ {}

end_total="$(date +%s%3N)"
duration_ms=$((end_total - start_total))

cat "$tmp"/result_*.tsv > "$tmp/results.tsv"

ok_count="$(awk -F'\t' '$6 == 200 {c++} END {print c + 0}' "$tmp/results.tsv")"
fail_count="$(awk -F'\t' '$6 != 200 {c++} END {print c + 0}' "$tmp/results.tsv")"

awk -F'\t' '$6 == 200 {print $7}' "$tmp/results.tsv" | sort -n > "$tmp/latencies_all.txt"

rps="$(awk -v n="$REQUESTS" -v ms="$duration_ms" 'BEGIN {printf "%.2f", n / (ms / 1000)}')"
avg="$(awk '{s+=$1} END {if (NR > 0) printf "%.2f", s / NR; else print "-"}' "$tmp/latencies_all.txt")"
p50="$(awk '{a[NR]=$1} END {i=int(NR*0.50 + 0.999); if (i<1) i=1; printf "%.0f", a[i]}' "$tmp/latencies_all.txt")"
p95="$(awk '{a[NR]=$1} END {i=int(NR*0.95 + 0.999); if (i<1) i=1; printf "%.0f", a[i]}' "$tmp/latencies_all.txt")"
max="$(tail -n 1 "$tmp/latencies_all.txt")"

echo "HTTP code distribution:"
awk -F'\t' '{print $6}' "$tmp/results.tsv" | sort | uniq -c
echo

printf "metric                         value\n"
printf "requests                       %s\n" "$REQUESTS"
printf "concurrency                    %s\n" "$CONCURRENCY"
printf "users                          %s\n" "$USERS"
printf "tasks                          %s\n" "$TASK_COUNT"
printf "cases                          %s\n" "$CASE_COUNT"
printf "ok                             %s\n" "$ok_count"
printf "fail                           %s\n" "$fail_count"
printf "duration_ms                    %s\n" "$duration_ms"
printf "rps                            %s\n" "$rps"
printf "avg_latency_ms                 %s\n" "$avg"
printf "p50_latency_ms                 %s\n" "$p50"
printf "p95_latency_ms                 %s\n" "$p95"
printf "max_latency_ms                 %s\n" "$max"

echo
echo "by query kind:"
printf "%-10s %8s %12s %10s %10s %10s\n" "kind" "count" "avg_ms" "p50_ms" "p95_ms" "max_ms"

for kind in correct wrong; do
  awk -F'\t' -v k="$kind" '$4 == k && $6 == 200 {print $7}' "$tmp/results.tsv" | sort -n > "$tmp/latencies_$kind.txt"
  n="$(wc -l < "$tmp/latencies_$kind.txt" | tr -d ' ')"

  if [ "$n" = "0" ]; then
    printf "%-10s %8s %12s %10s %10s %10s\n" "$kind" 0 "-" "-" "-" "-"
    continue
  fi

  k_avg="$(awk '{s+=$1} END {printf "%.2f", s / NR}' "$tmp/latencies_$kind.txt")"
  k_p50="$(awk '{a[NR]=$1} END {i=int(NR*0.50 + 0.999); if (i<1) i=1; printf "%.0f", a[i]}' "$tmp/latencies_$kind.txt")"
  k_p95="$(awk '{a[NR]=$1} END {i=int(NR*0.95 + 0.999); if (i<1) i=1; printf "%.0f", a[i]}' "$tmp/latencies_$kind.txt")"
  k_max="$(tail -n 1 "$tmp/latencies_$kind.txt")"

  printf "%-10s %8s %12s %10s %10s %10s\n" "$kind" "$n" "$k_avg" "$k_p50" "$k_p95" "$k_max"
done

if [ "$fail_count" != "0" ]; then
  echo
  echo "failed requests:"
  awk -F'\t' '$6 != 200 {print "request=" $1, "user_idx=" $2, "kind=" $4, "task=" $5, "http=" $6, "latency_ms=" $7}' "$tmp/results.tsv" | head -n 20

  first_fail="$(awk -F'\t' '$6 != 200 {print $1; exit}' "$tmp/results.tsv")"
  if [ -n "${first_fail:-}" ]; then
    echo
    echo "first failed body:"
    cat "$tmp/body_$first_fail.json"
    echo
  fi
fi
