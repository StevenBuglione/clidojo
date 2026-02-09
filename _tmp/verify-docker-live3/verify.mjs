import { chromium } from '@playwright/test';

const base = 'http://127.0.0.1:8765';
const dev = 'http://127.0.0.1:17365/__dev/ready';

async function ready(expectState, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await fetch(dev);
    const body = await res.json();
    last = body;
    if (!expectState || body.state === expectState) return body;
    await new Promise(r => setTimeout(r, 250));
  }
  throw new Error(`timeout waiting for state=${expectState}; last=${JSON.stringify(last)}`);
}

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });

await page.goto(base, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(1200);
await page.screenshot({ path: '_tmp/verify-docker-live3/01-main-menu.png' });
console.log('state:', await ready());

// Continue -> level start
await page.keyboard.press('Enter');
await ready('playing', 60000);
await page.waitForTimeout(2000);
await page.screenshot({ path: '_tmp/verify-docker-live3/02-playing.png' });

// run a command in terminal
await page.keyboard.type('echo CHECK123');
await page.keyboard.press('Enter');
await page.waitForTimeout(1200);
await page.screenshot({ path: '_tmp/verify-docker-live3/03-command.png' });

// open pause menu
await page.keyboard.press('F10');
await ready('pause_menu', 10000);
await page.waitForTimeout(600);
await page.screenshot({ path: '_tmp/verify-docker-live3/04-pause-menu.png' });

// select Main menu (Continue, Restart, Level select, Main menu)
await page.keyboard.press('ArrowDown');
await page.keyboard.press('ArrowDown');
await page.keyboard.press('ArrowDown');
await page.keyboard.press('Enter');
await ready('main_menu', 20000);
await page.waitForTimeout(800);
await page.screenshot({ path: '_tmp/verify-docker-live3/05-main-menu-return.png' });

console.log('final state:', await ready());
await browser.close();
