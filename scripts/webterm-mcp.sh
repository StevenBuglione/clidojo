#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

port="${CLIDOJO_WEBTERM_PORT:-7681}"
dev_http="${DEV_HTTP:-127.0.0.1:17321}"

if ! command -v npx >/dev/null 2>&1; then
  echo "npx is required to run localtunnel (Node.js)." >&2
  exit 1
fi

echo "Restarting local webterm on port ${port}..." >&2
(
  cd "${repo_root}"
  DEV_HTTP="${dev_http}" CLIDOJO_WEBTERM_PORT="${port}" make webterm-restart >/dev/null
)

echo >&2
echo "Local webterm: http://127.0.0.1:${port}" >&2
echo "Dev API:       http://${dev_http}/__dev/ready" >&2
echo >&2
echo "Tunnel password (required once per public IP):" >&2
curl -fsSL https://loca.lt/mytunnelpassword || true
echo >&2
echo >&2
echo "Starting public tunnel for Playwright MCP..." >&2
echo "Keep this process running while debugging." >&2
echo >&2

exec npx --yes localtunnel --port "${port}"
