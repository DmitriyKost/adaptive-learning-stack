#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
REQUESTS="${REQUESTS:-100}"
CONCURRENCY="${CONCURRENCY:-10}"
EMAIL="student-load-$(date +%s)@example.com"
PASSWORD="password123"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

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

REGISTER_RESPONSE="$(
  curl -fsS -X POST "$BASE/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
)"

TOKEN="$(printf "%s" "$REGISTER_RESPONSE" | jq -r ".access_token")"

echo "API load test"
echo "base=$BASE"
echo "requests=$REQUESTS concurrency=$CONCURRENCY"
echo

start_ms="$(date +%s%3N)"

seq "$REQUESTS" | xargs -I{} -P "$CONCURRENCY" sh -c '
  curl -sS -o /dev/null \
    -w "%{http_code} %{time_total}\n" \
    -H "Authorization: Bearer '"$TOKEN"'" \
    "'"$BASE"'/progress/me"
' > "$tmp"

end_ms="$(date +%s%3N)"
duration_ms=$((end_ms - start_ms))

ok_count="$(awk '$1 == 200 {count++} END {print count + 0}' "$tmp")"
fail_count="$(awk '$1 != 200 {count++} END {print count + 0}' "$tmp")"

rps="$(awk -v n="$REQUESTS" -v ms="$duration_ms" 'BEGIN { printf "%.2f", n / (ms / 1000) }')"

awk '{print $2 * 1000}' "$tmp" | sort -n > "$tmp.sorted"

avg="$(awk '{sum += $1} END {printf "%.2f", sum / NR}' "$tmp.sorted")"
p50="$(awk 'BEGIN {p=0.50} {a[NR]=$1} END {idx=int(NR*p); if (idx < 1) idx=1; printf "%.2f", a[idx]}' "$tmp.sorted")"
p95="$(awk 'BEGIN {p=0.95} {a[NR]=$1} END {idx=int(NR*p); if (idx < 1) idx=1; printf "%.2f", a[idx]}' "$tmp.sorted")"
max="$(tail -n 1 "$tmp.sorted")"

printf "endpoint             requests concurrency ok fail rps   avg_ms p50_ms p95_ms max_ms\n"
printf "/progress/me         %-8s %-11s %-2s %-4s %-5s %-6s %-6s %-6s %-6s\n" \
  "$REQUESTS" "$CONCURRENCY" "$ok_count" "$fail_count" "$rps" "$avg" "$p50" "$p95" "$max"
