#!/usr/bin/env bash
set -euo pipefail
source scripts/diploma-metrics-lib.sh

make_out_dir warmup-intelligence
need curl
need jq

WARMUP_TIMEOUT="${WARMUP_TIMEOUT:-900}"
WARMUP_POLL_INTERVAL="${WARMUP_POLL_INTERVAL:-3}"
WARMUP_COOLDOWN="${WARMUP_COOLDOWN:-3}"

log "Checking external intelligence readiness"
ready_json="$(curl -fsS "$INTELLIGENCE_BASE/ready")"
printf '%s\n' "$ready_json" | tee "$OUT_DIR/intelligence-ready-before.json" | jq . >/dev/null

status="$(printf '%s\n' "$ready_json" | jq -r '.status // "unknown"')"
loaded="$(printf '%s\n' "$ready_json" | jq -r 'if has("loaded") then .loaded else false end')"

if [ "$status" != "ready" ]; then
  echo "intelligence service is not ready: status=$status" >&2
  exit 1
fi

if [ "$loaded" != "true" ]; then
  log "Calling /warmup"
  warmup_body="$OUT_DIR/intelligence-warmup.json"
  if [ -n "${INTELLIGENCE_API_KEY:-}" ]; then
    http_code="$(curl -sS -X POST "$INTELLIGENCE_BASE/warmup" \
      -H 'Content-Type: application/json' \
      -H "X-API-Key: $INTELLIGENCE_API_KEY" \
      -d '{}' \
      -o "$warmup_body" \
      -w '%{http_code}' || echo 'curl_error')"
  else
    http_code="$(curl -sS -X POST "$INTELLIGENCE_BASE/warmup" \
      -H 'Content-Type: application/json' \
      -d '{}' \
      -o "$warmup_body" \
      -w '%{http_code}' || echo 'curl_error')"
  fi
  cat "$warmup_body" | jq . >/dev/null 2>&1 || true
  if [ "$http_code" != "200" ]; then
    echo "warmup failed with HTTP $http_code" >&2
    cat "$warmup_body" >&2 || true
    exit 1
  fi
else
  log "Model is already loaded"
fi

log "Waiting until loaded=true"
start="$(ms_now)"
while true; do
  ready_json="$(curl -fsS "$INTELLIGENCE_BASE/ready")"
  status="$(printf '%s\n' "$ready_json" | jq -r '.status // "unknown"')"
  loaded="$(printf '%s\n' "$ready_json" | jq -r 'if has("loaded") then .loaded else false end')"
  printf 'status=%s loaded=%s\n' "$status" "$loaded" | tee -a "$OUT_DIR/warmup-poll.log"

  if [ "$status" = "ready" ] && [ "$loaded" = "true" ]; then
    printf '%s\n' "$ready_json" | tee "$OUT_DIR/intelligence-ready-after.json" | jq . >/dev/null
    break
  fi

  now="$(ms_now)"
  elapsed=$(( (now - start) / 1000 ))
  if [ "$elapsed" -ge "$WARMUP_TIMEOUT" ]; then
    echo "external model warmup timeout after ${WARMUP_TIMEOUT}s" >&2
    printf '%s\n' "$ready_json" >&2
    exit 1
  fi
  sleep "$WARMUP_POLL_INTERVAL"
done

sleep "$WARMUP_COOLDOWN"

write_summary_header "External model warmup"
append_file_block "Ready before" "$OUT_DIR/intelligence-ready-before.json"
append_file_block "Ready after" "$OUT_DIR/intelligence-ready-after.json"

log "Done"
printf '\nResults directory: %s\nSummary: %s\n' "$OUT_DIR" "$OUT_DIR/summary.md"
