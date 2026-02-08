# CLI Dojo

Real-terminal Linux CLI training game with a polished TUI, deterministic level staging, and sandboxed execution.

## Quick Start

```bash
make build
./bin/clidojo --sandbox=mock --dev --demo=playable
```

## Runtime Modes

- `--sandbox=auto` detects Podman first, then Docker.
- `--sandbox=mock` avoids containers for deterministic UI/demo tests.
- `--demo=<scenario>` seeds deterministic UI states (`main_menu`, `level_select`, `playing`, `pause_menu`, `results_pass`, `results_fail`, `hints_open`, `journal_open`, `playable`).

## Dev Harness

```bash
scripts/dev-web.sh
# open http://127.0.0.1:7681
```

Stable browser-debug loop (ttyd + tmux):

```bash
make webterm
# or for a clean rebuild/restart while iterating:
make webterm-restart
# for Playwright MCP (public tunnel + password prompt):
make webterm-mcp
```

Environment overrides:

```bash
CLIDOJO_WEBTERM_PORT=7682 CLIDOJO_WEBTERM_SESSION=clidojo-review make webterm-restart
CLIDOJO_DATA_DIR=/tmp/clidojo-e2e-data CLIDOJO_RESET_DATA=1 scripts/dev-web.sh
```

Note for Playwright MCP in remote/sandboxed environments: if MCP cannot reach
`127.0.0.1`, expose the webterm on a reachable URL (tunnel/reverse proxy),
then point the browser tool at that public URL.

Playwright screenshot scaffolding lives in `e2e/playwright`.

Dev API endpoints (enabled with `--dev`):
- `GET /__dev/ready` returns `{ state, demo, rendered, pending, render_seq, error }`
- `POST /__dev/demo` with `{ "demo": "<scenario>" }` applies deterministic UI scenarios

Run screenshots:

```bash
scripts/dev-snapshots.sh
```

The built-in `builtin-core` pack ships LevelSpec v1 content:
- `level-001-pipes-101`
- `level-002-find-safe`
- `level-003-top-ips`

## Keybindings

- `F1` hints
- `F2` goal/check drawer
- `F4` journal
- `F5` check
- `F6` reset
- `F9` toggle scrollback mode
- `F10` menu

Terminal control keys (including `Ctrl+C`) are passed through.
