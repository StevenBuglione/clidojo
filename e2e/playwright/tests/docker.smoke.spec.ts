import { test, expect } from '@playwright/test';

const dockerEnabled = process.env.CLIDOJO_E2E_DOCKER === '1';

test.describe('docker smoke', () => {
  test.skip(!dockerEnabled, 'Set CLIDOJO_E2E_DOCKER=1 to run docker interaction checks.');

  test('interactive shell, overlays, and check flow stay responsive', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded', timeout: 20_000 });
    await page.waitForTimeout(2_000);
    const initial = await page.screenshot();
    expect(initial.byteLength).toBeGreaterThan(10_000);

    await page.keyboard.type('echo CHECK123');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(1_000);
    const afterCommand = await page.screenshot();
    expect(Buffer.compare(afterCommand, initial)).not.toBe(0);

    await page.keyboard.press('F10');
    await page.waitForTimeout(800);
    const afterMenu = await page.screenshot();
    expect(Buffer.compare(afterMenu, afterCommand)).not.toBe(0);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(700);

    await page.keyboard.press('F1');
    await page.waitForTimeout(800);
    const afterHints = await page.screenshot();
    expect(Buffer.compare(afterHints, afterMenu)).not.toBe(0);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(700);

    await page.keyboard.press('F4');
    await page.waitForTimeout(800);
    const afterJournal = await page.screenshot();
    expect(Buffer.compare(afterJournal, afterHints)).not.toBe(0);
    await page.keyboard.press('Enter');
    await page.waitForTimeout(800);
    const afterExplain = await page.screenshot();
    expect(Buffer.compare(afterExplain, afterJournal)).not.toBe(0);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(700);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(700);

    await page.keyboard.press('F5');
    await page.waitForTimeout(1_500);
    const afterResult = await page.screenshot();
    expect(Buffer.compare(afterResult, afterJournal)).not.toBe(0);
    await page.keyboard.press('Escape');
  });
});
