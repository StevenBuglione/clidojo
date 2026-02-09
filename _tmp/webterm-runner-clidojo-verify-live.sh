#!/usr/bin/env bash
set -euo pipefail
cd '/home/sbuglione/clidojo'
set +e
bash -lc './bin/clidojo --dev --sandbox=docker --demo=playable --dev-http=127.0.0.1:17377 --data-dir=/tmp/clidojo-verify-live'
status=$?
set -e
printf '\n[webterm] command exited with status %s\n' "$status"
exec bash --noprofile --norc
