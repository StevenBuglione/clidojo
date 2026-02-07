#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

go build -o bin/clidojo ./cmd/clidojo
./bin/clidojo --dev --sandbox=mock --dev-http="${DEV_HTTP:-127.0.0.1:17321}" "$@"
