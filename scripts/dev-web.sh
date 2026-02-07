#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-7681}"
DEV_HTTP="${DEV_HTTP:-127.0.0.1:17321}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

go build -o bin/clidojo ./cmd/clidojo

ttyd -p "$PORT" -W \
  -t "cursorBlink=false" \
  -t "fontSize=14" \
  -t "titleFixed=CLI Dojo Dev" \
  ./bin/clidojo --dev --sandbox=mock --dev-http="$DEV_HTTP"
