#!/usr/bin/env bash
set -euo pipefail
cd '/home/sbuglione/clidojo'
set +e
bash -lc './bin/clidojo --dev --sandbox=docker --dev-http=127.0.0.1:17393 --log /tmp/clidojo-docker-verify.jsonl --data-dir=/tmp/clidojo-e2e-docker'
status=$?
set -e
printf '\n[webterm] command exited with status %s\n' "$status"
exec bash --noprofile --norc
