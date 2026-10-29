import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createCompanion } from './server.mjs';

// Uses the desktop's pinned Playwright. A path override supports an existing
// isolated installation without changing this checkout's dependency tree.
const { chromium, webkit } = await import(process.env.GATOR_PLAYWRIGHT_MODULE || '../desktop/node_modules/playwright/index.mjs');
for (const name of (process.env.GATOR_COMPANION_BROWSERS || 'chromium').split(',')) {
  test(`${name}: mobile reconnect, retained admission, exact approval and revocation`, { timeout: 90000 }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'gator-phone-'));
    const tokenFile = join(root, 'token'), token = 'a'.repeat(64), submitted = [], decisions = [];
    let backend, browser, companion;
    const approval = (id, target) => ({ role: 'assistant', text: `Send the reviewed note to ${target}.`, result: { kind: 'approval', title: 'Review requested action', fields: [{ label: 'Request ID', value: String(id) }], preview: { to: target, body: '<img src=x onerror="window.injected=true">', attachments: ['report.txt'] } } });
    const setApproval = (id, target) => {
      backend.state.messages.push(approval(id, target));
      backend.state.pending_approval = { id, token: String(id).padStart(64, 'a'), message_index: backend.state.messages.length - 1 };
    };
    try {
      await writeFile(tokenFile, token + '\n', { mode: 0o600 });
      companion = createCompanion({ tokenFile, spawnBackend: id => backend = {
        state: { conversation_id: id, title: 'Weekly account check', messages: [{ role: 'assistant', text: 'Saved answer from the host.' }], running: false, queue: [] },
        async ready() {}, async stop() { this.closed = true; },
        async submitUpload(frame) {
          const upload = frame.upload; submitted.push(structuredClone(upload));
          if (!this.state.messages.some(m => m.request_id === upload.request_id)) this.state.messages.push({ role: 'user', text: upload.text, request_id: upload.request_id });
        },
        async apply(frame) {
          const current = this.state.pending_approval;
          if (frame.interaction_id !== current?.id || frame.approval_token !== current.token) throw new Error('Stale approval.');
          decisions.push(structuredClone(frame)); delete this.state.pending_approval;
          this.state.messages.push({ role: 'assistant', text: frame.approved ? 'Action receipt: completed.' : 'Action denied.' });
        },
      } });
      const address = await companion.listen(), base = `http://127.0.0.1:${address.port}`;
      browser = await ({ chromium, webkit }[name]).launch({ headless: true, executablePath: process.env[`GATOR_${name.toUpperCase()}_EXECUTABLE`] });
      const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, colorScheme: 'dark' });
      const page = await context.newPage(), errors = [];
      page.on('pageerror', error => errors.push(error.message));
      await page.goto(base);
      const connect = async () => {
        await page.getByLabel('Host access token', { exact: true }).fill(token);
        await page.getByLabel('Conversation ID', { exact: true }).fill('work-phone');
        await page.getByRole('button', { name: 'Open conversation', exact: true }).click();
        await page.getByText('Saved answer from the host.', { exact: true }).waitFor();
      };
      await connect();
      assert.equal(await page.locator('#token').inputValue(), '');
      const message = 'Keep the exact draft after losing acknowledgement.';
      await page.getByLabel('Message', { exact: true }).fill(message);
      let drop = true;
      await page.route('**/actions', async route => {
        if (drop && route.request().postDataJSON().type === 'upload_submit') {
          drop = false; await route.fetch(); await route.abort('connectionfailed');
        } else await route.continue();
      });
      await page.getByRole('button', { name: 'Send message', exact: true }).click();
      await page.getByRole('button', { name: 'Retry same message', exact: true }).waitFor();
      await page.waitForFunction(() => document.querySelector('#error').textContent.length > 0);
      assert.equal(submitted.length, 1);
      assert.equal(await page.getByLabel('Message', { exact: true }).inputValue(), message);
      assert.equal(await page.locator('#message').getAttribute('readonly'), '');
      assert.equal(await page.evaluate(token => JSON.stringify({ ...sessionStorage, ...localStorage }).includes(token), token), false);
      await page.reload();
      assert.equal(await page.locator('#token').inputValue(), '');
      await connect();
      await page.getByRole('button', { name: 'Retry same message', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#message').value === '');
      assert.equal(submitted.length, 2);
      assert.deepEqual(submitted[1], submitted[0]);
      assert.equal(backend.state.messages.filter(m => m.text === message).length, 1);

      setApproval(7, 'first@example.test');
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.getByRole('button', { name: 'Review approval', exact: true }).click();
      assert.ok((await page.locator('#confirm-preview').textContent()).includes('first@example.test'));
      assert.ok((await page.locator('#confirm-preview').textContent()).includes('report.txt'));
      assert.equal(await page.evaluate(() => !!window.injected), false);
      assert.equal(await page.locator('#approval img').count(), 0);
      assert.equal(decisions.length, 0, 'opening a review must not approve');
      setApproval(8, 'second@example.test');
      await page.waitForFunction(() => !document.querySelector('#approval-dialog').open);
      assert.equal(decisions.length, 0, 'a changed preview cancels confirmation');
      await page.getByRole('button', { name: 'Review approval', exact: true }).click();
      await page.getByRole('button', { name: 'Approve action', exact: true }).click();
      await page.getByText('Action receipt: completed.', { exact: true }).waitFor();
      assert.deepEqual(decisions, [{ type: 'approval', interaction_id: 8, approval_token: '8'.padStart(64, 'a'), approved: true }]);
      setApproval(9, 'denied@example.test');
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.getByRole('button', { name: 'Deny', exact: true }).click();
      await page.getByText('Action denied.', { exact: true }).waitFor();
      assert.equal(decisions[1].approved, false);

      // The shell must remain usable at narrow phone widths.
      for (const width of [320, 390, 760]) {
        await page.setViewportSize({ width, height: 844 });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, `horizontal overflow at ${width}px`);
      }
      await page.setViewportSize({ width: 390, height: 844 });
      const screenshot = process.env.GATOR_COMPANION_SCREENSHOT;
      if (screenshot) {
        await page.evaluate(() => scrollTo(0, 0));
        await page.screenshot({ path: `${screenshot}-${name}.png` });
        await page.locator('#send-form').scrollIntoViewIfNeeded();
        await page.screenshot({ path: `${screenshot}-${name}-composer.png` });
        await page.emulateMedia({ colorScheme: 'light' });
        await page.screenshot({ path: `${screenshot}-${name}-light.png` });
      }
      await context.setOffline(true);
      await page.waitForFunction(() => document.querySelector('#send').disabled);
      await context.setOffline(false);
      await page.getByRole('button', { name: 'Send message', exact: true }).waitFor();
      await writeFile(tokenFile, 'b'.repeat(64) + '\n');
      await page.getByText('Access expired · enter the current host token', { exact: true }).waitFor();
      assert.equal(await page.locator('#chat').isVisible(), false);
      assert.equal(await page.locator('#messages').textContent(), '');
      assert.deepEqual(errors, []);
    } finally {
      await browser?.close(); await companion?.close(); await rm(root, { recursive: true, force: true });
    }
  });
}
