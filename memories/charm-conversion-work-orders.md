# Charm Conversion Work Orders

These are executable work orders for migration.

## Work Order 1: Bootstrap Charm runtime

1. Add Bubble Tea/Lip Gloss/Bubbles deps (v2 import paths).
2. Add `internal/charmui` package with `RootModel` skeleton.
3. Wire `cmd/clidojo/main.go` to launch Bubble Tea root path behind a feature flag.

Acceptance:
- `go test ./...` passes.
- `./bin/clidojo --ui=charm --sandbox=mock --demo=main_menu` renders deterministic menu.

## Work Order 2: Message bus and external dispatch

1. Define typed message structs in `internal/charmui/messages.go`.
2. Add adapter methods in app layer to send events with `Program.Send`.
3. Route `/__dev/demo` to message send only.

Acceptance:
- `/__dev/demo` causes state transitions without direct UI calls.
- no goroutine directly mutates model/view structs.

## Work Order 3: Main menu + level select parity

1. Implement menu and catalog models with Bubbles list/help/key.
2. Integrate existing packs catalog data from `internal/levels`.
3. Implement `Continue`, `StartLevel`, `Back` flows.

Acceptance:
- app launches to main menu.
- level select starts selected level.
- container does not start until level launch.

## Work Order 4: Playing layout shell

1. Build pure layout calculator shared by render + terminal resize.
2. Port HUD/status/header rendering with Lip Gloss styles.
3. Add overlay stack manager (pause, hints, goal, journal, result, reset confirm).

Acceptance:
- wide/medium/too-small breakpoints match spec.
- no layout rebuild thrash per frame.

## Work Order 5: Terminal adapter integration

1. Keep existing PTY/vt core but expose as command/message-driven adapter.
2. On PTY output, send `TerminalDataMsg`; on resize, send `TerminalResizeMsg`.
3. Encode non-global key events directly to terminal input.

Acceptance:
- no output lag waiting for extra keypress.
- no index panic on resize stress.
- vim/less/top usable in pane.

## Work Order 6: Gameplay actions parity

1. F6 must open reset confirm first; execute reset only on confirm.
2. Journal overlay reads and renders parsed `/work/.dojo_cmdlog` entries.
3. F5 result overlay supports reference-solution unlock and diff action paths.

Acceptance:
- reset is never immediate.
- journal is not static.
- results actions enable/disable correctly by state.

## Work Order 7: Deterministic dev/demo and tests

1. Implement deterministic demo state transformer in `Update` path.
2. Add/port teatest model tests for state transitions.
3. Refresh Playwright golden snapshots for target demo states.

Acceptance:
- `/__dev/ready` and `/__dev/demo` are stable and visually consistent.
- Playwright suite passes locally under ttyd flow.

## Work Order 8: Cleanup and cutover

1. Remove old tview/tcell UI glue once feature parity is met.
2. Keep sandbox/grader/state interfaces stable.
3. Update AGENTS.md with charm-specific run/debug/test instructions.

Acceptance:
- default runtime uses Charm path.
- no references to old UI runtime remain in production path.
