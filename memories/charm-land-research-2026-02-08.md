# Charm.land Research Notes (2026-02-08)

This document captures official-source findings for migrating CLI Dojo to Charm libraries.

## Source Set (primary)

- https://charm.land/libs/
- https://github.com/charmbracelet/bubbletea
- https://pkg.go.dev/charm.land/bubbletea/v2
- https://github.com/charmbracelet/bubbletea/releases
- https://charm.land/blog/commands-in-bubbletea/
- https://charm.land/blog/teatest/
- https://pkg.go.dev/github.com/charmbracelet/x/exp/teatest
- https://github.com/charmbracelet/bubbles
- https://pkg.go.dev/charm.land/bubbles/v2
- https://pkg.go.dev/charm.land/bubbles/v2/viewport
- https://pkg.go.dev/charm.land/bubbles/v2/help
- https://pkg.go.dev/charm.land/bubbles/v2/key
- https://github.com/charmbracelet/lipgloss
- https://pkg.go.dev/charm.land/lipgloss/v2
- https://github.com/charmbracelet/huh
- https://pkg.go.dev/charm.land/huh/v2
- https://github.com/charmbracelet/wish
- https://pkg.go.dev/charm.land/wish/v2
- https://pkg.go.dev/charm.land/wish/v2/bubbletea
- https://github.com/charmbracelet/glamour
- https://github.com/charmbracelet/harmonica
- https://pkg.go.dev/charm.land/log/v2
- https://github.com/charmbracelet/ultraviolet
- https://github.com/charmbracelet/vhs

## Core Findings

## 1) Bubble Tea v2 is the primary runtime

- Bubble Tea remains Elm/MVU (`Init`, `Update`, `View`) and command-driven async.
- `Program.Send(msg)` is the official external injection path (critical for `/__dev/demo` style control).
- Current import path is `charm.land/bubbletea/v2`.
- Release notes show major renderer/input improvements in v2 and ongoing fixes for resize/renderer edge cases.

Implication for CLI Dojo:
- External control paths (dev HTTP endpoint, async sandbox state updates) should send typed messages via `Program.Send`, not mutate UI directly from goroutines.

## 2) Bubbles provides most UI primitives needed

From docs/readme:
- Lists, tables, viewport, textinput/textarea, key maps/help, progress, spinners.
- `viewport` supports content scrolling and keymap-driven navigation.
- `help` + `key` package are designed for consistent key hint rendering.

Implication for CLI Dojo:
- Main menu/level select/status/help should be Bubbles + Lip Gloss composition.
- Avoid re-implementing list/help key systems.

## 3) Lip Gloss v2 remains styling/layout layer

- Styling and layout utilities (`JoinHorizontal`, `JoinVertical`, width/height/padding/border utilities).
- In Bubble Tea contexts, color/background detection should be driven by Bubble Tea messages (e.g. background color events), not standalone blocking calls.

Implication:
- Use Lip Gloss for all static layout composition and theme tokens.
- Keep style definitions centralized and immutable per render tick.

## 4) Huh v2 is suitable for modal forms/settings flows

- Works standalone and also as embedded `tea.Model`.
- Has accessible mode.

Implication:
- Use Huh for settings forms and confirmation flows where practical.
- Keep critical in-game overlays (results/journal) as custom views for tighter control.

## 5) Wish is for SSH-delivered apps (not required for local ttyd)

- Bubble Tea middleware in Wish handles PTY IO + resize over SSH sessions.
- Useful when shipping remote CLI Dojo sessions over SSH.

Implication:
- Keep as optional future transport; not required for local desktop/ttyd dev loop.

## 6) Teatest exists and is useful for deterministic model tests

- Supports final model assertions, final output golden checks, intermediate waits, and message injection.
- Uses golden diffs and supports update workflow.

Implication:
- Replace fragile screen-level UI tests with model-level + golden tests for deterministic states.

## 7) Charm log package can replace ad-hoc logging

- Structured logging + levels + formatter options.

Implication:
- Standardize logs with `charm.land/log/v2` or keep existing logger; either is fine. Use one consistently.

## 8) Ultraviolet is lower-level and currently cautionary

- UV powers Bubble Tea v2/Lip Gloss v2 internals.
- Project explicitly warns no API stability guarantees yet for external direct use.

Implication:
- Do not build CLI Dojo directly on UV now.
- Use Bubble Tea v2 abstractions and only depend on UV transitively.

## 9) Harmonica and Glamour are optional add-ons

- Harmonica: spring animations; useful for tasteful progress/transition effects.
- Glamour: markdown rendering; useful for level descriptions/hints docs.

Implication:
- Nice-to-have polish, not blockers for core migration.

## 10) VHS is ideal for deterministic terminal demos

- Scripted `.tape` terminal recordings and CI-friendly artifact generation.

Implication:
- Use VHS for demo artifacts and regression docs, while Playwright remains the browser snapshot harness.

## Migration-Relevant API Notes

- Bubble Tea v2 imports moved to `charm.land/*/v2` domain.
- v2 message types have become struct-based for extensibility in several areas.
- `tea.ExecProcess` exists for temporarily running external interactive programs in a Bubble Tea app context.
  - For CLI Dojo, this is not a replacement for embedded PTY pane; useful for occasional out-of-band flows only.

## Gap Notes For Current Repo

Current repo (`go.mod`) still uses:
- `github.com/gdamore/tcell/v2`
- `github.com/rivo/tview`
- `github.com/hinshun/vt10x`

This matches current pain points (manual key routing/layout/render coupling).
The Charm migration should preserve existing:
- `internal/sandbox`
- `internal/levels`
- `internal/grading`
- `internal/state`

and replace primarily:
- `internal/ui`
- parts of `internal/app` orchestration
- `internal/term` input/event wiring (renderer side), while keeping PTY/vt core where needed.
