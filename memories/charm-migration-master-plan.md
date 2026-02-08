# CLI Dojo Charm Migration Master Plan

Target: migrate presentation/input runtime to Charm Elm architecture while preserving game logic, sandbox, grading, and persistence.

## 1) Package Selection Matrix

Mandatory:
- `charm.land/bubbletea/v2` (runtime, event loop, commands)
- `charm.land/bubbles/v2` (list/help/viewport/key/progress components)
- `charm.land/lipgloss/v2` (theme/layout rendering)

Recommended:
- `github.com/charmbracelet/x/exp/teatest` (deterministic model tests)
- `charm.land/log/v2` (structured logs)

Optional:
- `charm.land/huh/v2` (forms/settings)
- `github.com/charmbracelet/glamour` (markdown rendering in panels)
- `github.com/charmbracelet/harmonica` (small animation polish)
- `github.com/charmbracelet/vhs` (demo recording)

Not recommended for direct dependency in v1 migration:
- `github.com/charmbracelet/ultraviolet` directly (use through Bubble Tea/Lip Gloss only)

## 2) High-Level Architecture

Use a single top-level Bubble Tea model with typed submodels.

- `RootModel`
  - owns app lifecycle state (`MainMenu`, `LevelSelect`, `Playing`, overlays)
  - owns shared state snapshot (pack/level/run/check/hints)
  - delegates to submodels for rendering and local updates

Submodels:
- `MenuModel` (main menu)
- `CatalogModel` (pack + level selection)
- `PlayingModel`
  - `HUDModel`
  - `TerminalModel` (PTY/vt surface adapter)
  - `OverlayStackModel` (results/menu/hints/journal/confirm)

Boundary rule:
- Only `RootModel.Update` mutates global app state.
- External events enter via `Program.Send` as typed messages.

## 3) Module Mapping From Current Repo

Keep mostly unchanged:
- `internal/sandbox/*`
- `internal/levels/*`
- `internal/grading/*`
- `internal/state/*`
- `internal/devtools/*` (adapt message wiring)

Replace/refactor:
- `internal/ui/*` -> `internal/charmui/*`
- `internal/app/*` -> orchestration shell + message adapters
- `internal/term/*` -> split into:
  - PTY/vt engine adapter (retain)
  - Bubble Tea terminal render/input adapter (new)

Suggested new layout:

- `internal/charmui/model_root.go`
- `internal/charmui/model_menu.go`
- `internal/charmui/model_catalog.go`
- `internal/charmui/model_playing.go`
- `internal/charmui/model_overlay.go`
- `internal/charmui/view_theme.go`
- `internal/charmui/messages.go`
- `internal/charmui/commands.go`
- `internal/charmui/keymap.go`

## 4) Message and Command Design

Typed messages only. No direct UI mutation from goroutines.

Core message categories:
- input: key/mouse/resize (Bubble Tea native)
- app: screen transitions, level lifecycle
- sandbox: started/stopped/error, shell ready
- terminal: bytes arrived, pty exited, size changed
- grading: check started/completed
- journal: cmdlog refreshed
- devtools: demo state requested/applied

Commands:
- long-running work (sandbox/grading/file IO) via `tea.Cmd`
- async callbacks return typed completion messages
- UI transitions happen in `Update` only

## 5) Terminal Strategy (Critical)

Keep PTY + vt10x core for now, but embed it as a Bubble Tea-managed component.

Rules:
- Terminal draw must be frame-driven by Bubble Tea renderer.
- Incoming PTY bytes enqueue `TerminalDataMsg` via `Program.Send`.
- Resize emits one `TerminalResizeMsg` per actual dimension change.
- No manual out-of-band drawing from goroutines.

This removes current redraw-thrashing and out-of-sync resize paths.

## 6) Key Handling Policy

Global intercept only for function keys and explicit app controls:
- `F1`, `F2`, `F4`, `F5`, `F6`, `F9`, `F10`

Everything else in Playing mode goes to terminal input by default.

Explicitly:
- do not hijack `Tab`
- do not hijack plain `Esc` unless closing an open overlay
- no double-Esc quit flow

## 7) Layout Policy

One pure layout function:
- input: `cols`, `rows`, `hudWidth`, `mode`
- output: terminal inner rect, hud rect, header/status rects

Both rendering and PTY resize use the same computed rect.

## 8) Dev Control Reliability (`/__dev/demo`)

Current blocker is dispatch reliability. Fix pattern:

- HTTP handler only validates/parses request and sends `DevDemoRequestedMsg` through `Program.Send`.
- `Update` applies full transition synchronously in one place.
- Ready endpoint reports:
  - `state`
  - `demo`
  - `render_seq`
  - `pending`
  - `error`
- Increment `render_seq` only after model applied and view cycle completed.

No direct UI mutation from HTTP goroutines.

## 9) Testing Strategy

Unit/model tests (teatest):
- screen transitions
- key handling policy
- overlay stack behavior
- dev-demo transitions
- layout invariants

Integration tests (existing Go tests):
- sandbox runner and grader behavior
- level schema validation

Browser snapshots (Playwright + ttyd):
- deterministic demos only
- states: `main_menu`, `level_select`, `playing`, `pause_menu`, `results_pass`, `results_fail`, `hints_open`, `journal_open`

## 10) Phased Migration

Phase 1: introduce Bubble Tea shell alongside existing app
- boot a minimal Bubble Tea root model and draw static main menu

Phase 2: port non-terminal screens
- main menu + level select + overlays

Phase 3: port playing shell without PTY bytes
- static playing HUD + status + key routing

Phase 4: integrate terminal adapter
- PTY bytes, resize, input pass-through

Phase 5: hook sandbox/grader/journal
- level run lifecycle, F5 checks, F6 reset confirm, journal from cmdlog

Phase 6: dev/demo reliability + tests
- `/__dev/demo` message-driven transitions + Playwright snapshot refresh

Phase 7: remove tview/tcell UI code
- delete old `internal/ui` path after parity checks pass

## 11) Definition of Done for Migration

- App starts in main menu, not directly in playing state.
- Playing mode has responsive terminal (real-time output updates, no redraw stalls).
- Tab and Esc behavior matches terminal expectations.
- Reset uses confirm modal before destructive action.
- Journal overlay shows real parsed cmdlog entries.
- Dev demo endpoint transitions are deterministic and reflected visually.
- Playwright snapshots stable across demo states.
