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

  expect(await page.screenshot()).toMatchSnapshot(name);
}

test('menu', async ({ page }) => {
  await snap(page, 'menu', '01-menu.png');
});

test('playing', async ({ page }) => {
  await snap(page, 'playing', '02-playing.png');
});

test('results_pass', async ({ page }) => {
  await snap(page, 'results_pass', '03-results-pass.png');
});

test('results_fail', async ({ page }) => {
  await snap(page, 'results_fail', '04-results-fail.png');
});

test('hints_open', async ({ page }) => {
  await snap(page, 'hints_open', '05-hints-open.png');
});

test('journal_open', async ({ page }) => {
  await snap(page, 'journal_open', '06-journal-open.png');
});
