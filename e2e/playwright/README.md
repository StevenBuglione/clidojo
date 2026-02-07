# CLI Dojo E2E (Playwright)

## Install
From repo root:
- `cd e2e/playwright`
- `corepack pnpm install`
- `corepack pnpm exec playwright install --with-deps`

## Run screenshot tests
- `corepack pnpm test`

## Update golden snapshots
- `corepack pnpm run update-snapshots`

## Notes
These tests require:
- `ttyd` installed
- The app supports `--dev --sandbox=mock --dev-http=...`
- Dev server endpoints:
  - GET  /__dev/ready
  - POST /__dev/demo  { demo: "menu" | ... }
