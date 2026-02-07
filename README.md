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
- `--demo=<scenario>` seeds deterministic UI states (`menu`, `playing`, `results_pass`, `results_fail`, `hints_open`, `journal_open`, `playable`).

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
```

Environment overrides:

```bash
CLIDOJO_WEBTERM_PORT=7682 CLIDOJO_WEBTERM_SESSION=clidojo-review make webterm-restart
```

Note for Playwright MCP in remote/sandboxed environments: if MCP cannot reach
`127.0.0.1`, expose the webterm on a reachable URL (tunnel/reverse proxy),
then point the browser tool at that public URL.

Playwright screenshot scaffolding lives in `e2e/playwright`.

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
