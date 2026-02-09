import { defineConfig } from '@playwright/test';
import path from 'path';

const PORT = process.env.PORT ?? '8761';
const DEV_HTTP = process.env.DEV_HTTP ?? '127.0.0.1:17331';
const REPO_ROOT = path.resolve(__dirname, '..', '..');
const EXTERNAL_WEBTERM = process.env.CLIDOJO_WEBTERM_EXTERNAL === '1';
const E2E_DOCKER = process.env.CLIDOJO_E2E_DOCKER === '1';
const DEFAULT_DEMO = process.env.CLIDOJO_DEMO ?? 'playable';
const DEFAULT_SANDBOX = E2E_DOCKER ? 'docker' : 'mock';
const DEFAULT_WEBTERM_CMD = `./bin/clidojo --dev --sandbox=${DEFAULT_SANDBOX} --demo=${DEFAULT_DEMO} --dev-http=${DEV_HTTP} --data-dir=/tmp/clidojo-e2e-data`;
const WEBTERM_CMD = process.env.CLIDOJO_WEBTERM_CMD ?? DEFAULT_WEBTERM_CMD;

const config = defineConfig({
  testDir: path.join(__dirname, 'tests'),
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    viewport: { width: 1280, height: 720 },
    screenshot: 'only-on-failure'
  },
  webServer: EXTERNAL_WEBTERM
    ? undefined
    : {
        command: 'bash scripts/dev-web.sh',
        cwd: REPO_ROOT,
        url: `http://127.0.0.1:${PORT}`,
        reuseExistingServer: process.env.CI !== 'true',
        timeout: 60_000,
        stdout: 'pipe',
        stderr: 'pipe',
        env: {
          ...process.env,
          PORT,
          DEV_HTTP,
          CLIDOJO_DATA_DIR: '/tmp/clidojo-e2e-data',
          CLIDOJO_RESET_DATA: '1',
          CLIDOJO_WEBTERM_USE_TMUX: '0',
          CLIDOJO_WEBTERM_CMD: WEBTERM_CMD
        }
      }
});

export default config;
