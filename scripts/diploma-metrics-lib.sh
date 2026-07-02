#!/usr/bin/env bash
# Common helpers for diploma metric scripts. Source this file from repository root.

set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
INTELLIGENCE_BASE="${INTELLIGENCE_BASE:-http://localhost:8090}"
METRICS_ROOT="${METRICS_ROOT:-./diploma-metrics}"
SAMPLE_INTERVAL="${SAMPLE_INTERVAL:-2}"

mkdir -p "$METRICS_ROOT"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

ts() { date -Iseconds; }
ms_now() { date +%s%3N; }

log() { printf '\n==> %s\n' "$*" | tee -a "$OUT_DIR/run.log"; }

check_file() {
  [ -f "$1" ] || {
    echo "required file not found: $1" >&2
    echo "run this script from adaptive-learning-stack repository root" >&2
    exit 1
  }
}

make_out_dir() {
  local name="$1"
  OUT_DIR="${OUT_DIR:-$METRICS_ROOT/${name}-$(date +%Y%m%d-%H%M%S)}"
  mkdir -p "$OUT_DIR"
  export OUT_DIR
  {
    echo "generated=$(ts)"
    echo "base=$BASE"
    echo "intelligence_base=$INTELLIGENCE_BASE"
  } > "$OUT_DIR/context.env"
}

check_gateway_ready() {
  need curl
  need jq
  log "Checking gateway readiness"
  curl -fsS "$BASE/ready" | tee "$OUT_DIR/gateway-ready.json" | jq . >/dev/null
}

check_intelligence_loaded() {
  need curl
  need jq
  local ready_json loaded status
  ready_json="$(curl -fsS "$INTELLIGENCE_BASE/ready")"
  printf '%s\n' "$ready_json" | tee "$OUT_DIR/intelligence-ready.json" | jq . >/dev/null
  status="$(printf '%s\n' "$ready_json" | jq -r '.status // "unknown"')"
  loaded="$(printf '%s\n' "$ready_json" | jq -r 'if has("loaded") then .loaded else false end')"
  if [ "$status" != "ready" ] || [ "$loaded" != "true" ]; then
    echo "external intelligence is not warmed up: status=$status loaded=$loaded" >&2
    echo "run: scripts/diploma-00-warmup-intelligence.sh" >&2
    exit 1
  fi
}

sample_resources() {
  while true; do
    printf 'timestamp=%s\n' "$(ts)"
    docker stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}'
    sleep "$SAMPLE_INTERVAL"
  done
}

start_sampler() {
  need docker
  local name="$1"
  sample_resources > "$OUT_DIR/resources-${name}.tsv" 2>/dev/null &
  SAMPLER_PID=$!
}

stop_sampler() {
  if [ -n "${SAMPLER_PID:-}" ]; then
    kill "$SAMPLER_PID" 2>/dev/null || true
    wait "$SAMPLER_PID" 2>/dev/null || true
    SAMPLER_PID=""
  fi
}

ch_query() {
  docker exec adaptive-clickhouse clickhouse-client \
    --user analytics \
    --password analytics \
    --database analytics \
    --query "$1" 2>/dev/null | tr -d '\r'
}

wait_analysis_quiet() {
  need docker
  local quiet_seconds="${QUIET_SECONDS:-20}"
  local max_wait="${QUIET_MAX_WAIT:-300}"
  local start now elapsed before after

  log "Waiting for analytics output to become quiet (${quiet_seconds}s stable window)"
  start="$(ms_now)"
  while true; do
    before="$(ch_query "SELECT count() FROM llm_analysis_runs FORMAT TSVRaw" || echo 0)"
    sleep "$quiet_seconds"
    after="$(ch_query "SELECT count() FROM llm_analysis_runs FORMAT TSVRaw" || echo 0)"
    if [ "$before" = "$after" ]; then
      echo "llm_analysis_runs_count=$after" | tee "$OUT_DIR/analytics-quiet.txt"
      break
    fi
    now="$(ms_now)"
    elapsed=$(( (now - start) / 1000 ))
    if [ "$elapsed" -ge "$max_wait" ]; then
      echo "analytics did not become quiet after ${max_wait}s: before=$before after=$after" >&2
      exit 1
    fi
  done
}

write_summary_header() {
  {
    echo "# $1"
    echo
    echo "Generated: $(ts)"
    echo "Base URL: $BASE"
    echo "Intelligence URL: $INTELLIGENCE_BASE"
    echo
  } > "$OUT_DIR/summary.md"
}

append_file_block() {
  local title="$1"
  local file="$2"
  {
    echo "## $title"
    echo '```'
    cat "$file"
    echo '```'
    echo
  } >> "$OUT_DIR/summary.md"
}
