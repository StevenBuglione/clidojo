import { test, expect } from '@playwright/test';

async function snap(page, name: string, delay = 600) {
  await page.waitForTimeout(delay);
  await page.waitForTimeout(800);
  expect(await page.screenshot()).toMatchSnapshot(name, { maxDiffPixels: 4000 });
}

test('deterministic ui flow', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded', timeout: 15_000 });

  // Start in playable demo state.
  await snap(page, '03-playing.png', 1200);

  await page.keyboard.press('F10');
  await snap(page, '04-pause-menu.png');

  // Pause menu -> Main menu.
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await snap(page, '01-main-menu.png');

  // Main menu -> Level select.
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await snap(page, '02-level-select.png');

  // Level select -> start selected level.
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(1200);

  await page.keyboard.press('F1');
  await snap(page, '07-hints-open.png');
  await page.keyboard.press('Escape');

  await page.keyboard.press('F4');
  await snap(page, '08-journal-open.png');
  await page.keyboard.press('Escape');

  // In mock mode first check fails, second check passes deterministically.
  await page.keyboard.press('F5');
  await snap(page, '06-results-fail.png', 1200);
  await page.keyboard.press('Escape');

  await page.keyboard.press('F5');
  await snap(page, '05-results-pass.png', 1200);
});
