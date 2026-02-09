# AGENTS Handoff

## Build and Run

```bash
make build
./bin/clidojo --sandbox=auto
```

Deterministic local mode:

```bash
./bin/clidojo --dev --sandbox=mock --demo=playable
```

## Webterm (ttyd, optional tmux)

Prerequisites:

- `ttyd`
- `tmux` (optional; only required when `CLIDOJO_WEBTERM_USE_TMUX=1`)

Primary workflows:

1. Deterministic UI debug (recommended for MCP + screenshots)

```bash
scripts/dev-web.sh
# default URL:   http://127.0.0.1:7681
# default dev API: http://127.0.0.1:17321
```

2. Stable tmux-backed webterm session

```bash
make webterm
```

3. Clean rebuild + restart (recommended while iterating)

```bash
make webterm-restart
```

4. MCP-accessible tunnel (WSL2-safe workflow)

```bash
make webterm-mcp
# prints a public https://*.loca.lt URL
# keep this running while using Playwright MCP
```

Environment overrides:

```bash
PORT=7682 DEV_HTTP=127.0.0.1:17322 scripts/dev-web.sh

# deterministic local state for screenshots:
CLIDOJO_DATA_DIR=/tmp/clidojo-e2e-data CLIDOJO_RESET_DATA=1 scripts/dev-web.sh

CLIDOJO_WEBTERM_PORT=7683 CLIDOJO_WEBTERM_SESSION=clidojo-review make webterm-restart

# real container run in webterm:
CLIDOJO_WEBTERM_CMD='./bin/clidojo --dev --sandbox=auto --dev-http=127.0.0.1:17321' make webterm-restart

# direct mode (recommended for input debugging / avoids tmux key interception):
CLIDOJO_WEBTERM_USE_TMUX=0 make webterm-restart
```

Quick health checks:

```bash
curl -sS http://127.0.0.1:7681/ >/dev/null && echo WEBTERM_OK
curl -sS http://127.0.0.1:17321/__dev/ready
tmux list-sessions | rg clidojo
```

Troubleshooting:

- Restart log: `_tmp/webterm-restart.log`
- If stale listener/session exists: run `make webterm-restart` again with a fresh `CLIDOJO_WEBTERM_SESSION`.

## Debugging With Playwright MCP

Use this order for reliable debugging:

1. Start deterministic webterm:

```bash
scripts/dev-web.sh
```

2. In Playwright MCP:

- `browser_navigate` to the public URL from `make webterm-mcp`
- `browser_snapshot` to get the `Terminal input` textbox ref
- `browser_type` into the textbox (`submit: true`) for terminal commands
- `browser_press_key` for global keys (`F1`, `F2`, `F4`, `F5`, `F6`, `F10`)
- `browser_take_screenshot` after state changes

3. If the tunnel shows “Tunnel website ahead!”:

- Enter the tunnel password printed by `make webterm-mcp`
- You can always fetch it with: `curl https://loca.lt/mytunnelpassword`

4. If function keys are not honored by host terminal path:

- Use app fallback: press `F10` to open pause menu, then choose `Main menu` or `Quit`
- Use deterministic demo endpoint to force states:
  - `GET http://127.0.0.1:17321/__dev/ready`
  - `POST http://127.0.0.1:17321/__dev/demo` with `{"demo":"main_menu|level_select|playing|pause_menu|results_pass|results_fail|hints_open|journal_open|playable"}`

If MCP cannot reach localhost from your environment:

- This is an environment/network boundary, not an app bug.
- Use `make webterm-mcp` to expose webterm on a reachable URL, then navigate MCP to that public URL.
- Continue using the local dev API (`/__dev/ready`, `/__dev/demo`) for deterministic state control.

## Playwright Test Harness

```bash
cd e2e/playwright
corepack pnpm install
corepack pnpm test
```

Snapshots are stored under:
- `e2e/playwright/tests/ui.snapshots.spec.ts-snapshots/`

## Logs and Dev State

- Structured logs: `--log <path>` (JSONL).
- Dev ready endpoint: `GET http://127.0.0.1:17321/__dev/ready`
- Ready payload fields: `state`, `demo`, `rendered`, `pending`, `render_seq`, `error`
- Dev scenario endpoint: `POST http://127.0.0.1:17321/__dev/demo` with `{ "demo": "..." }`
- Dev state cache file: `~/.cache/clidojo/dev_state.json`.

## Demo State Reproduction

Use `--demo=` values:

- `main_menu`
- `level_select`
- `playing`
- `pause_menu`
- `results_pass`
- `results_fail`
- `hints_open`
- `journal_open`
- `playable`

## Milestone Done Definitions

1. Terminal proof: PTY + VT pane runs interactive apps and survives resize.
2. Sandbox runner: secure container flags + cleanup + exec shell.
3. Polished split UI: wide/medium/too-small with status and overlays.
4. Level loader/reset: deterministic workdir staging and replayable reset.
5. Grader/results: checks run via F5 and display PASS/FAIL with details.
6. Persistence: sqlite tracks runs, resets, attempts.
7. Dev harness: mock sandbox, ttyd script, Playwright screenshot scaffold.

## Strict Rules

- Keep global controls on function keys.
- Do not hijack `Ctrl+C` unless explicitly enabled by config.
- Follow layout breakpoints and sandbox flags from spec; avoid ad-hoc shortcuts.
