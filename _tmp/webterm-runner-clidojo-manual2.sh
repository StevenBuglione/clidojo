#!/usr/bin/env bash
set -euo pipefail
cd '/home/sbuglione/clidojo'
set +e
bash -lc './bin/clidojo --sandbox=docker --dev --dev-http=127.0.0.1:17322 --data-dir=/tmp/clidojo-manual2-data'
status=$?
set -e
printf '\n[webterm] command exited with status %s\n' "$status"
exec bash --noprofile --norc
