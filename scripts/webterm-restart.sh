#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

host="${CLIDOJO_WEBTERM_HOST:-127.0.0.1}"
port="${CLIDOJO_WEBTERM_PORT:-7681}"
session="${CLIDOJO_WEBTERM_SESSION:-clidojo-debug}"
use_tmux="${CLIDOJO_WEBTERM_USE_TMUX:-0}"

echo "Rebuilding..." >&2
(cd "${repo_root}" && make build >/dev/null)

echo "Stopping existing webterm on ${host}:${port} (if any)..." >&2
pid="$(
  ss -ltnpH 2>/dev/null \
    | sed -n "s/.*${host}:${port} .*pid=\([0-9]\+\).*/\1/p" \
    | head -n1 \
    || true
)"
if [ -n "${pid}" ]; then
  kill "${pid}" >/dev/null 2>&1 || true
fi

if [ "${use_tmux}" = "1" ] && command -v tmux >/dev/null 2>&1; then
  tmux kill-session -t "${session}" >/dev/null 2>&1 || true
fi

echo "Starting fresh webterm at http://${host}:${port} (session ${session})" >&2
mkdir -p "${repo_root}/_tmp"
export CLIDOJO_WEBTERM_RESET_SESSION=1
setsid "${repo_root}/scripts/webterm.sh" >"${repo_root}/_tmp/webterm-restart.log" 2>&1 </dev/null &
sleep 0.7

ss -ltnp | sed -n "s/.*${host}:${port}.*/&/p" || true
