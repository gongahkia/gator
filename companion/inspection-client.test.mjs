import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createCompanion } from './server.mjs';

const { chromium } = await import(process.env.GATOR_PLAYWRIGHT_MODULE || '../desktop/node_modules/playwright/index.mjs');
const fixture = (digit, chat) => {
  const date = '2026-10-31T03:00:00Z';
  return { target: { binding: { id: 'target-' + digit.repeat(32), backend: 'gator_work', owner: 'owner-' + digit, generation: digit.repeat(32), incarnation: digit.repeat(32) }, label: 'Work <img src=x onerror="window.injected=true"> ' + digit, capabilities: ['inspect'], available: true }, run_id: 'run-' + digit, conversation_id: chat, status: 'running', started_at: date, observed_at: date, updated_at: date, progress: ['<script>window.injected=true</script>'], specialists: [{ id: 'subagent-1', role: 'cli_codex', status: 'running', native_status: 'awaiting_input', pending_question: true }], pending_approvals: 1, limitations: ['No terminal transcript.'] };
};

test('phone work view renders untrusted progress as text, handles selection races and clears revoked/offline state', { timeout: 90000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-work-browser-')), tokenFile = join(root, 'owner.token'), token = 'a'.repeat(64);
  let browser, companion, release, held = false, hold = false, reads = 0;
  const a = fixture('a', 'chat-a'), b = fixture('b', 'chat-b');
  try {
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    companion = createCompanion({ binary: '', workspace: root, tokenFile,
      queryCatalog: async () => ({ items: [], next: '', incomplete: false }),
      spawnBackend() { throw new Error('Work inspection started a chat worker'); },
      queryWork: async ({ target }) => {
        reads++;
        if (!target) return { targets: [a, b] };
        if (target === a.target.binding.id && hold) { held = true; await new Promise(r => release = r); }
        return { targets: [target === a.target.binding.id ? a : b] };
      },
    });
    const address = await companion.listen();
    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 390, height: 844 } }), page = await context.newPage();
    await page.goto(`http://127.0.0.1:${address.port}`);
    await page.locator('#token').fill(token); await page.getByRole('button', { name: 'Connect', exact: true }).click();
    await page.locator('#work-title').click();
    await page.waitForFunction(() => document.querySelector('#work-select').options.length === 3);
    await page.locator('#work-select').selectOption(a.target.binding.id);
    await page.locator('#work-meta').filter({ hasText: 'chat-a' }).waitFor();
    assert.match(await page.locator('#work-progress').textContent(), /<script>/);
    assert.equal(await page.locator('#owned-work img, #owned-work script').count(), 0);
    assert.equal(await page.evaluate(() => window.injected), undefined);
    assert.match(await page.locator('#work-progress').textContent(), /question pending/);
    const before = reads; await page.waitForTimeout(3500); assert.ok(reads > before, 'visible view polls');
    hold = true;
    await page.locator('#work-refresh').click();
    await page.waitForFunction(() => document.querySelector('#work-status').textContent.includes('Refreshing'));
    for (let i = 0; i < 100 && !held; i++) await page.waitForTimeout(20);
    assert.equal(held, true);
    await page.locator('#work-select').selectOption(b.target.binding.id);
    hold = false; release();
    await page.locator('#work-meta').filter({ hasText: 'chat-b' }).waitFor();
    assert.doesNotMatch(await page.locator('#work-meta').textContent(), /chat-a/);
    await context.setOffline(true);
    await page.waitForFunction(() => document.querySelector('#work-status').textContent.includes('stale'));
    await context.setOffline(false);
    await page.locator('#work-meta').filter({ hasText: 'chat-b' }).waitFor();
    for (const width of [320, 390]) { await page.setViewportSize({ width, height: 844 }); assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true); }
    await writeFile(tokenFile, 'b'.repeat(64) + '\n', { mode: 0o600 });
    await page.waitForFunction(() => document.querySelector('#owned-work').hidden);
    assert.equal(await page.locator('#work-progress').textContent(), '');
  } finally { release?.(); await browser?.close(); await companion?.close(); await rm(root, { recursive: true, force: true }); }
});
