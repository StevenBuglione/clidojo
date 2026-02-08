import { test, expect } from '@playwright/test';
import { devRequest, setDemo, waitReady, DemoState } from './devctl';

const DEV_HTTP = process.env.DEV_HTTP ?? '127.0.0.1:17321';

async function snap(page, demo: DemoState, name: string) {
  await page.goto('/', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(400);

  const api = await devRequest(DEV_HTTP);
  await waitReady(api);
  await setDemo(api, demo);

  await page.waitForTimeout(800);

  expect(await page.screenshot()).toMatchSnapshot(name, { maxDiffPixels: 120 });
}

test('main_menu', async ({ page }) => {
  await snap(page, 'main_menu', '01-main-menu.png');
});

test('level_select', async ({ page }) => {
  await snap(page, 'level_select', '02-level-select.png');
});

test('playing', async ({ page }) => {
  await snap(page, 'playing', '03-playing.png');
});

test('pause_menu', async ({ page }) => {
  await snap(page, 'pause_menu', '04-pause-menu.png');
});

test('results_pass', async ({ page }) => {
  await snap(page, 'results_pass', '05-results-pass.png');
});

test('results_fail', async ({ page }) => {
  await snap(page, 'results_fail', '06-results-fail.png');
});

test('hints_open', async ({ page }) => {
  await snap(page, 'hints_open', '07-hints-open.png');
});

test('journal_open', async ({ page }) => {
  await snap(page, 'journal_open', '08-journal-open.png');
});
