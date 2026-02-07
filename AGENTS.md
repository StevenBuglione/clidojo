# AGENTS Handoff

## Build and Run

```bash
make build
./bin/clidojo --sandbox=auto
```

Development deterministic mode:

```bash
./bin/clidojo --dev --sandbox=mock --demo=playable
```

## Browser Preview via ttyd

```bash
scripts/dev-web.sh
# default: http://127.0.0.1:7681
# override: PORT=7682 DEV_HTTP=127.0.0.1:17322 scripts/dev-web.sh
```

## Playwright Screenshots

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
- Dev scenario endpoint: `POST http://127.0.0.1:17321/__dev/demo` with `{ "demo": "..." }`
- Dev state cache file: `~/.cache/clidojo/dev_state.json`.

## Demo State Reproduction

Use `--demo=` values:

- `menu`
- `playing`
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
