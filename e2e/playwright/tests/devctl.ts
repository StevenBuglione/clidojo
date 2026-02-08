import { request as pwRequest, APIRequestContext } from '@playwright/test';

export type DemoState =
  | 'main_menu'
  | 'level_select'
  | 'playing'
  | 'pause_menu'
  | 'results_pass'
  | 'results_fail'
  | 'hints_open'
  | 'journal_open';

export async function devRequest(DEV_HTTP: string): Promise<APIRequestContext> {
  return await pwRequest.newContext({
    baseURL: `http://${DEV_HTTP}`
  });
}

export async function setDemo(api: APIRequestContext, demo: DemoState) {
  const res = await api.post('/__dev/demo', {
    data: { demo }
  });
  if (!res.ok()) throw new Error(`Failed to set demo=${demo}: ${res.status()}`);
  for (let i = 0; i < 40; i++) {
    const ready = await api.get('/__dev/ready');
    if (ready.ok()) {
      const body = await ready.json();
      if (body?.state === demo && body?.rendered === true && body?.pending !== true) return;
    }
    await new Promise(r => setTimeout(r, 250));
  }
  throw new Error(`Timed out waiting for demo=${demo}`);
}

export async function waitReady(api: APIRequestContext) {
  for (let i = 0; i < 40; i++) {
    const res = await api.get('/__dev/ready');
    if (res.ok()) {
      const body = await res.json();
      if (body?.ok) return body;
    }
    await new Promise(r => setTimeout(r, 250));
  }
  throw new Error('Dev server never became ready');
}
