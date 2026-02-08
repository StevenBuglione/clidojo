# Memories: Charm Migration Reference

Purpose: persistent research + implementation references for converting CLI Dojo from `tview/tcell` to the Charm ecosystem using Elm architecture.

## Files

- `memories/charm-land-research-2026-02-08.md`
  - Official-source research notes for Charm libraries and related tooling.
  - Version/migration facts, capabilities, caveats, and source links.

- `memories/charm-migration-master-plan.md`
  - Master conversion architecture for this repo.
  - Package selection matrix, target module layout, state model, event model, and phased migration plan.

- `memories/charm-conversion-work-orders.md`
  - Concrete implementation work orders with acceptance checks.
  - Designed for direct execution by coding agents.

- `memories/charm-code-snippets.md`
  - Practical code skeletons for key integration points:
    - Bubble Tea app shell
    - message bus + external `Program.Send`
    - keymap/help model
    - Huh modal embedding
    - deterministic test harness shape

## How To Use These Docs

1. Read `charm-migration-master-plan.md` first.
2. Execute work in the order from `charm-conversion-work-orders.md`.
3. Use `charm-land-research-2026-02-08.md` for API/migration details and source links.
4. Pull snippets from `charm-code-snippets.md` while implementing each phase.

## Scope Note

These docs intentionally keep sandbox/grading/state modules in Go and focus the migration on UI/input/render/event orchestration, which is where the current instability lives.
