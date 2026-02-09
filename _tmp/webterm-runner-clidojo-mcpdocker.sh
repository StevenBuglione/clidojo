#!/usr/bin/env bash
set -euo pipefail
cd '/home/sbuglione/clidojo'
set +e
bash -lc './bin/clidojo --dev --sandbox=docker --ui-style=modern_arcade --motion-level=full --mouse-scope=scoped --dev-http=127.0.0.1:17321'
status=$?
set -e
printf '\n[webterm] command exited with status %s\n' "$status"
exec bash --noprofile --norc
