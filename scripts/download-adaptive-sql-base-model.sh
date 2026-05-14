#!/usr/bin/env bash
set -euo pipefail

ADAPTIVE_SQL_DIPLOMA_ROOT="${ADAPTIVE_SQL_DIPLOMA_ROOT:-../adaptive_sql_diploma}"
MODEL_ID="${ADAPTIVE_SQL_DIPLOMA_BASE_MODEL_ID:-unsloth/meta-llama-3.1-8b-instruct-unsloth-bnb-4bit}"
TARGET_DIR="${ADAPTIVE_SQL_DIPLOMA_MODELS_HOST:-$ADAPTIVE_SQL_DIPLOMA_ROOT/models/base}"
VENV_DIR="${HF_DOWNLOAD_VENV:-.venv-hf-download}"

log() {
  printf '\n==> %s\n' "$*"
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

command -v python3 >/dev/null 2>&1 || fail "python3 not found"

log "Creating local Python venv: $VENV_DIR"

if [[ ! -d "$VENV_DIR" ]]; then
  python3 -m venv "$VENV_DIR" || {
    cat >&2 <<'EOF'

python3 -m venv failed.

On Void Linux install venv support with:

  sudo xbps-install -S python3-virtualenv

Then rerun this script.

EOF
    exit 1
  }
fi

PY="$VENV_DIR/bin/python"

log "Installing downloader dependencies into venv"
"$PY" -m ensurepip --upgrade >/dev/null 2>&1 || true
"$PY" -m pip install --upgrade pip >/dev/null
"$PY" -m pip install --upgrade "huggingface_hub>=0.24.0" >/dev/null

mkdir -p "$TARGET_DIR"

log "Downloading base model"
printf 'model:  %s\n' "$MODEL_ID"
printf 'target: %s\n' "$TARGET_DIR"

HF_TOKEN="${HF_TOKEN:-}" \
MODEL_ID="$MODEL_ID" \
TARGET_DIR="$TARGET_DIR" \
"$PY" <<'PY'
import os
from huggingface_hub import snapshot_download

model_id = os.environ["MODEL_ID"]
target_dir = os.environ["TARGET_DIR"]
token = os.environ.get("HF_TOKEN") or None

snapshot_download(
    repo_id=model_id,
    local_dir=target_dir,
    local_dir_use_symlinks=False,
    token=token,
    resume_download=True,
)
PY

log "Downloaded files"
find "$TARGET_DIR" -maxdepth 2 -type f | sort | sed 's#^#  #'

log "Done"
