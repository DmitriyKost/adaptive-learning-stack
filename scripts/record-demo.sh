#!/usr/bin/env bash
# record-demo.sh — записывает экран через ffmpeg пока идёт demo.sh.
#
# MODE=html  (по умолчанию) — граф рендерится в браузере (D3 DAG, lib/graph.html),
#                             demo.sh обновляет lib/graph-state.json вживую.
#                             Раскладку «браузер + терминал» расставь в ОС/WM.
# MODE=plain                — текстовый граф прямо в терминале (без браузера).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/lib"
DEMO="$SCRIPT_DIR/demo.sh"
OUT="${OUT:-demo.mp4}"
MODE="${MODE:-html}"

# --- запись ---
RES="${RES:-2560x1440}"
FPS="${FPS:-30}"
DISPLAY_ADDR="${DISPLAY:-:0}"
OFFSET="${OFFSET:-+0,0}"
HTTP_PORT="${HTTP_PORT:-8099}"
BROWSER="${BROWSER:-xdg-open}"

FFMPEG_PID=""; HTTP_PID=""; DEMO_PID=""; CLEANED=0
cleanup() {
  (( CLEANED )) && return; CLEANED=1
  # ffmpeg — SIGINT (дописывает корректный mp4), остальное — TERM по группе
  [[ -n "$FFMPEG_PID" ]] && kill -INT "$FFMPEG_PID" 2>/dev/null || true
  for pid in "$DEMO_PID" "$HTTP_PID"; do
    [[ -n "$pid" ]] && kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  done
  [[ -n "$FFMPEG_PID" ]] && wait "$FFMPEG_PID" 2>/dev/null || true
}
on_int() { cleanup; trap - INT; exit 130; }
trap on_int INT
trap cleanup EXIT TERM

start_record() {
  ffmpeg -y \
    -f x11grab -framerate "$FPS" -video_size "$RES" -i "${DISPLAY_ADDR}${OFFSET}" \
    -c:v libx264 -preset ultrafast -pix_fmt yuv420p -crf 18 \
    "$OUT" >/tmp/ffmpeg-demo.log 2>&1 &
  FFMPEG_PID=$!
  sleep 1.5   # инициализация ffmpeg
}

if [[ "$MODE" == "html" ]]; then
  command -v python3 >/dev/null || { echo "нужен python3 для http.server" >&2; exit 1; }
  JSON="$LIB/graph-state.json"
  # начальное состояние, чтобы страница не висела на 404
  printf '{"ready":false,"nodes":[],"edges":[]}\n' > "$JSON"

  # http-сервер раздаёт lib/ (graph.html + graph-state.json). setsid → своя
  # группа процессов, чтобы cleanup гарантированно её прибил по Ctrl+C.
  setsid python3 -m http.server "$HTTP_PORT" --directory "$LIB" \
    >/tmp/graph-http.log 2>&1 &
  HTTP_PID=$!
  sleep 0.5

  URL="http://localhost:${HTTP_PORT}/graph.html"
  echo "Граф: $URL"
  "$BROWSER" "$URL" >/dev/null 2>&1 || echo "открой вручную: $URL"
  sleep 1.5   # дать браузеру отрисоваться до старта записи

  start_record
  GRAPH_JSON="$JSON" setsid bash "$DEMO" &
  DEMO_PID=$!
  wait "$DEMO_PID"
else
  start_record
  setsid bash "$DEMO" &   # MODE=plain: текстовый граф (GRAPH_JSON не задан)
  DEMO_PID=$!
  wait "$DEMO_PID"
fi
