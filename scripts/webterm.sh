#!/usr/bin/env bash
set -euo pipefail

# Prefer user-local installs.
export PATH="${HOME}/.local/bin:${PATH}"

# Local browser terminal for driving CLI Dojo with Playwright/manual checks.
#
# Environment:
#   CLIDOJO_WEBTERM_HOST          (default: 127.0.0.1)
#   CLIDOJO_WEBTERM_PORT          (default: 7681)
#   CLIDOJO_WEBTERM_CLIENTS       (default: 4)
#   CLIDOJO_WEBTERM_SESSION       (default: clidojo-debug)
#   CLIDOJO_WEBTERM_RESET_SESSION (default: 0)
#   CLIDOJO_WEBTERM_CMD           (default: ./bin/clidojo --dev --sandbox=mock ...)
#   CLIDOJO_WEBTERM_USE_TMUX      (default: 0; set 1 to run in tmux mode)
#   CLIDOJO_WEBTERM_TMUX_CONF     (default: tools/webterm/tmux.conf)
#   CLIDOJO_TTYD_EXTRA_ARGS       (default: empty)
#   DEV_HTTP                      (default: 127.0.0.1:17321)

host="${CLIDOJO_WEBTERM_HOST:-127.0.0.1}"
port="${CLIDOJO_WEBTERM_PORT:-7681}"
clients="${CLIDOJO_WEBTERM_CLIENTS:-4}"
session="${CLIDOJO_WEBTERM_SESSION:-clidojo-debug}"
reset_session="${CLIDOJO_WEBTERM_RESET_SESSION:-0}"
use_tmux="${CLIDOJO_WEBTERM_USE_TMUX:-0}"
tmux_conf="${CLIDOJO_WEBTERM_TMUX_CONF:-tools/webterm/tmux.conf}"
dev_http="${DEV_HTTP:-127.0.0.1:17321}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
cmd="${CLIDOJO_WEBTERM_CMD:-./bin/clidojo --dev --sandbox=mock --dev-http=${dev_http}}"
extra_args="${CLIDOJO_TTYD_EXTRA_ARGS:-}"

if [ "${use_tmux}" = "1" ]; then
  if ! command -v tmux >/dev/null 2>&1; then
    echo "tmux not found. Install tmux first." >&2
    exit 1
  fi
fi

if ! command -v ttyd >/dev/null 2>&1; then
  cat >&2 <<'EOF'
ttyd not found.

Install options:
  - snap:   sudo snap install ttyd --classic
  - brew:   brew install ttyd
  - binary: https://github.com/tsl0922/ttyd/releases
EOF
  exit 1
fi

if [ ! -x "${repo_root}/bin/clidojo" ]; then
  echo "${repo_root}/bin/clidojo not found; building..." >&2
  (cd "${repo_root}" && make build >/dev/null)
fi

echo "Starting web terminal at http://${host}:${port}" >&2
if [ "${use_tmux}" = "1" ]; then
  echo "tmux session: ${session}" >&2
else
  echo "mode: direct (no tmux)" >&2
fi
echo "command: ${cmd}" >&2

if [ "${use_tmux}" = "1" ] && [ "${reset_session}" = "1" ]; then
  tmux kill-session -t "${session}" >/dev/null 2>&1 || true
fi

set +e
ttyd -h 2>&1 | grep -q -- "-W"
has_writable=$?
set -e

ttyd_args=("-i" "${host}" "-p" "${port}" "-m" "${clients}")
if [ "${has_writable}" -eq 0 ]; then
  ttyd_args+=("-W")
fi
ttyd_args+=("-t" "fontSize=14")
ttyd_args+=("-t" "cursorBlink=false")
ttyd_args+=("-t" "titleFixed=CLI Dojo Dev")

# shellcheck disable=SC2206
if [ -n "${extra_args}" ]; then
  ttyd_args+=(${extra_args})
fi

mkdir -p "${repo_root}/_tmp"
runner_script="${repo_root}/_tmp/webterm-runner-${session}.sh"
cat >"${runner_script}" <<EOF
#!/usr/bin/env bash
set -euo pipefail
cd ${repo_root@Q}
set +e
bash -lc ${cmd@Q}
status=\$?
set -e
printf '\n[webterm] command exited with status %s\n' "\$status"
exec bash --noprofile --norc
EOF
chmod +x "${runner_script}"

if [ "${use_tmux}" != "1" ]; then
  exec ttyd "${ttyd_args[@]}" "${runner_script}"
fi

tmux_args=()
if [ -n "${tmux_conf}" ] && [ -f "${tmux_conf}" ]; then
  tmux_args+=("-f" "${tmux_conf}")
fi

if ! tmux "${tmux_args[@]}" has-session -t "${session}" >/dev/null 2>&1; then
  tmux "${tmux_args[@]}" new-session -d -s "${session}" "${runner_script}"
fi

exec ttyd "${ttyd_args[@]}" tmux "${tmux_args[@]}" attach -t "${session}"
