#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-7681}"
DEV_HTTP="${DEV_HTTP:-127.0.0.1:17321}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

go build -o bin/clidojo ./cmd/clidojo

export CLIDOJO_WEBTERM_HOST="${CLIDOJO_WEBTERM_HOST:-127.0.0.1}"
export CLIDOJO_WEBTERM_PORT="${PORT}"
export CLIDOJO_WEBTERM_SESSION="${CLIDOJO_WEBTERM_SESSION:-clidojo-dev-web}"
export CLIDOJO_WEBTERM_RESET_SESSION="${CLIDOJO_WEBTERM_RESET_SESSION:-1}"
export CLIDOJO_WEBTERM_CMD="${CLIDOJO_WEBTERM_CMD:-./bin/clidojo --dev --sandbox=mock --demo=playable --dev-http=${DEV_HTTP}}"
export DEV_HTTP="${DEV_HTTP}"

exec ./scripts/webterm.sh
