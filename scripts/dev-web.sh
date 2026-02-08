#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-7681}"
DEV_HTTP="${DEV_HTTP:-127.0.0.1:17321}"
CLIDOJO_DATA_DIR="${CLIDOJO_DATA_DIR:-}"
CLIDOJO_RESET_DATA="${CLIDOJO_RESET_DATA:-0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

go build -o bin/clidojo ./cmd/clidojo

if [ "$CLIDOJO_RESET_DATA" = "1" ] && [ -n "$CLIDOJO_DATA_DIR" ]; then
  rm -rf "$CLIDOJO_DATA_DIR"
fi

export CLIDOJO_WEBTERM_HOST="${CLIDOJO_WEBTERM_HOST:-127.0.0.1}"
export CLIDOJO_WEBTERM_PORT="${PORT}"
export CLIDOJO_WEBTERM_SESSION="${CLIDOJO_WEBTERM_SESSION:-clidojo-dev-web}"
export CLIDOJO_WEBTERM_RESET_SESSION="${CLIDOJO_WEBTERM_RESET_SESSION:-1}"
default_cmd="./bin/clidojo --dev --sandbox=mock --demo=playable --dev-http=${DEV_HTTP}"
if [ -n "$CLIDOJO_DATA_DIR" ]; then
  default_cmd="$default_cmd --data-dir=${CLIDOJO_DATA_DIR}"
fi
export CLIDOJO_WEBTERM_CMD="${CLIDOJO_WEBTERM_CMD:-$default_cmd}"
export DEV_HTTP="${DEV_HTTP}"

exec ./scripts/webterm.sh
