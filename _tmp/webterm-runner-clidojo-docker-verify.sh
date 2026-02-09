#!/usr/bin/env bash
set -euo pipefail
cd '/home/sbuglione/clidojo'
set +e
bash -lc './bin/clidojo --sandbox=docker --dev --demo=playable --dev-http=127.0.0.1:17331'
status=$?
set -e
printf '\n[webterm] command exited with status %s\n' "$status"
exec bash --noprofile --norc
