import { defineConfig } from '@playwright/test';
import path from 'path';

const PORT = process.env.PORT ?? '7681';
const DEV_HTTP = process.env.DEV_HTTP ?? '127.0.0.1:17321';

export default defineConfig({
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
  webServer: {
    command: `bash ../../scripts/dev-web.sh`,
    url: `http://127.0.0.1:${PORT}`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    env: {
      ...process.env,
      PORT,
      DEV_HTTP
    }
  }
});
