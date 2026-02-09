import { test, expect } from '@playwright/test';

const dockerEnabled = process.env.CLIDOJO_E2E_DOCKER === '1';

async function screenshotChanged(page, before: Buffer, waitMs = 700) {
  await page.waitForTimeout(waitMs);
  const after = await page.screenshot();
  expect(Buffer.compare(after, before)).not.toBe(0);
  return after;
}

test.describe('qa matrix', () => {
  test('keyboard-only, copy/paste, overlays, and stress-resize remain responsive', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded', timeout: 20_000 });
    await page.waitForTimeout(2_000);

    let shot = await page.screenshot();
    expect(shot.byteLength).toBeGreaterThan(10_000);

    // Keyboard-only sanity: terminal command path + Ctrl+C interrupt.
    await page.keyboard.type('sleep 5');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(900);
    await page.keyboard.press('Control+c');
    shot = await screenshotChanged(page, shot, 900);

    // Overlay open/close keys used in gameplay loop.
    await page.keyboard.press('F1');
    shot = await screenshotChanged(page, shot);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);

    await page.keyboard.press('F4');
    shot = await screenshotChanged(page, shot);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);

    await page.keyboard.press('F10');
    shot = await screenshotChanged(page, shot);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);

    // Copy/paste-heavy path (insertText maps to direct text input in webterm).
    await page.keyboard.insertText('echo PASTE_HEAVY_12345');
    await page.keyboard.press('Enter');
    shot = await screenshotChanged(page, shot, 900);

    // Check + result overlay path.
    await page.keyboard.press('F5');
    shot = await screenshotChanged(page, shot, 1300);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    // Reset confirmation must always be escapable.
    await page.keyboard.press('F6');
    shot = await screenshotChanged(page, shot, 700);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    // Stress loop: rapid resize + overlay churn.
    const sizes = [
      { width: 1200, height: 700 },
      { width: 1024, height: 640 },
      { width: 1366, height: 768 },
      { width: 1280, height: 720 },
    ];
    for (const size of sizes) {
      await page.setViewportSize(size);
      await page.waitForTimeout(250);
      await page.keyboard.press('F10');
      await page.waitForTimeout(250);
      await page.keyboard.press('Escape');
      await page.waitForTimeout(250);
    }
    shot = await screenshotChanged(page, shot, 900);

    // Keep terminal-app sanity in docker mode (optional commands by availability).
    if (dockerEnabled) {
      await page.keyboard.type("command -v less >/dev/null && less /levels/current/animals.txt || echo NO_LESS");
      await page.keyboard.press('Enter');
      await page.waitForTimeout(900);
      await page.keyboard.press('q');
      await page.waitForTimeout(400);

      await page.keyboard.type("command -v vim >/dev/null && vim -Nu NONE -n /tmp/dojo-qa-vim || echo NO_VIM");
      await page.keyboard.press('Enter');
      await page.waitForTimeout(900);
      await page.keyboard.press('Escape');
      await page.keyboard.type(':q!');
      await page.keyboard.press('Enter');
      await page.waitForTimeout(400);

      await page.keyboard.type("command -v top >/dev/null && top -b -n 1 | head -n 5 || echo NO_TOP");
      await page.keyboard.press('Enter');
      shot = await screenshotChanged(page, shot, 1200);
    }

    // Final liveness check: menu is always reachable.
    await page.keyboard.press('F10');
    shot = await screenshotChanged(page, shot, 700);
    await page.keyboard.press('Escape');
  });

  test('scoped mouse clicks do not freeze gameplay loop', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded', timeout: 20_000 });
    await page.waitForTimeout(2_000);

    let shot = await page.screenshot();

    // Random clicks in scoped mode should be safe and non-fatal during play.
    await page.mouse.click(640, 360);
    await page.mouse.click(980, 120);
    await page.mouse.click(220, 180);
    await page.waitForTimeout(700);

    // UI still responds to key overlays after mouse activity.
    await page.keyboard.press('F10');
    shot = await screenshotChanged(page, shot, 700);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);

    await page.keyboard.press('F1');
    shot = await screenshotChanged(page, shot, 700);
    await page.keyboard.press('Escape');
  });
});
