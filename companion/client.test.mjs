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
  test(`${name}: follow-up status changes survive reconnect and reject stale decisions`, { timeout: 90000 }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'gator-phone-updates-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64);
    const actions = [], updates = [
      { id: 'inbox-new', title: 'Check needs attention <img src=x onerror="window.injected=true">', summary: 'Could not read the selected account.', status: 'failed', next_step: 'Reconnect on the host.', evidence: ['Account check: <script>not executable</script>'], created_at: '2026-10-29T01:00:00Z', read: false, resolved: false, version: 0 },
      { id: 'inbox-old', title: 'Earlier update', summary: 'An earlier retained result.', status: 'completed', created_at: '2026-10-28T01:00:00Z', read: false, resolved: false, version: 0 },
    ];
    let browser, companion, backend, old = false, stale = true, drop = false;
    const sync = () => Object.assign(backend.state, { chat_updates: [updates[old ? 1 : 0]], chat_updates_total: 2, updates_have_older: !old, updates_have_newer: old });
    try {
      await writeFile(tokenFile, token + '\n', { mode: 0o600 });
      companion = createCompanion({ tokenFile, spawnBackend: id => {
        backend = { state: { conversation_id: id, title: 'Follow-up chat', messages: [], queue: [], running: false }, async ready() {}, async stop() { this.closed = true; }, async apply(frame) {
          const action = frame.update; actions.push(structuredClone(action)); this.state.chat_updates_issue = '';
          assert.equal(action.conversation_id, 'work-updates');
          if (action.action === 'older' || action.action === 'newer') { old = action.action === 'older'; sync(); return; }
          const update = updates.find(update => update.id === action.id); assert.ok(update);
          if (stale && action.action === 'resolve') { stale = false; update.version = 2; }
          if (action.action === 'read') update.read = true;
          else if (action.version !== update.version) this.state.chat_updates_issue = 'This update changed; refresh it before changing its status.';
          else { update.resolved = action.action === 'resolve'; update.version++; }
          sync();
        } }; sync(); return backend;
      } });
      const address = await companion.listen(), base = `http://127.0.0.1:${address.port}`;
      browser = await ({ chromium, webkit }[name]).launch({ headless: true, executablePath: process.env[`GATOR_${name.toUpperCase()}_EXECUTABLE`] });
      const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, colorScheme: 'dark' }), page = await context.newPage(), errors = [];
      page.on('pageerror', error => errors.push(error.message));
      await page.goto(base);
      await page.getByLabel('Host access token', { exact: true }).fill(token);
      await page.getByText('Open a known conversation', { exact: true }).click();
      await page.getByLabel('Conversation ID', { exact: true }).fill('work-updates');
      await page.getByRole('button', { name: 'Connect', exact: true }).click();
      await page.locator('[data-update="inbox-new"]').waitFor();
      await page.locator('#message').fill('Keep this independent draft.');
      await page.getByText('Update details', { exact: true }).click();
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.waitForFunction(() => !document.querySelector('#refresh').disabled);
      assert.equal(await page.locator('#updates details').getAttribute('open'), '');
      assert.equal(actions.length, 0, 'reading/polling must not silently mark read or done');
      await page.getByRole('button', { name: 'Mark read', exact: true }).focus();
      await page.keyboard.press('Enter');
      await page.waitForFunction(() => document.querySelector('#updates').textContent.includes('Read · Open'));
      assert.equal(await page.evaluate(() => document.activeElement?.dataset.update), 'inbox-new');
      await page.getByRole('button', { name: 'Done', exact: true }).click();
      await page.locator('#updates-issue').filter({ hasText: 'This update changed' }).waitFor();
      assert.equal(updates[0].resolved, false);
      await page.getByRole('button', { name: 'Done', exact: true }).click();
      await page.getByRole('button', { name: 'Reopen', exact: true }).waitFor();
      assert.equal(updates[0].resolved, true);
      assert.equal(await page.locator('#updates').textContent().then(text => text.includes('Check result: failed')), true);
      assert.equal(await page.getByText('This check did not complete successfully. Earlier results do not confirm that nothing changed.', { exact: true }).isVisible(), true);
      await page.route('**/actions', async route => {
        if (drop) { drop = false; await route.fetch(); await route.abort('connectionfailed'); }
        else await route.continue();
      });
      drop = true;
      await page.getByRole('button', { name: 'Reopen', exact: true }).click();
      await page.locator('#updates-issue').filter({ hasText: 'unconfirmed' }).waitFor();
      const afterDrop = actions.length;
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.getByRole('button', { name: 'Done', exact: true }).waitFor();
      assert.equal(updates[0].resolved, false);
      assert.equal(actions.length, afterDrop, 'refresh must read status without replaying a lost mutation');
      await page.getByRole('button', { name: 'Earlier updates', exact: true }).click();
      await page.getByRole('heading', { name: 'Earlier update', exact: true }).waitFor();
      assert.equal(await page.locator('#updates-title').evaluate(node => node === document.activeElement), true);
      await page.getByRole('button', { name: 'Newer updates', exact: true }).click();
      await page.locator('[data-update="inbox-new"]').waitFor();
      assert.equal(await page.locator('#message').inputValue(), 'Keep this independent draft.');
      assert.equal(await page.evaluate(() => window.injected), undefined);
      Object.assign(backend.state, { chat_updates: [], chat_updates_issue: 'Follow-up storage unavailable.', updates_have_older: false, updates_have_newer: false });
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.getByText('Could not load follow-up updates.', { exact: true }).waitFor();
      assert.equal(await page.locator('#updates-issue').textContent(), 'Follow-up storage unavailable.');
      backend.state.chat_updates_issue = '';
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#followups').hidden);
      sync();
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.locator('[data-update="inbox-new"]').waitFor();
      for (const width of [320, 390, 760]) {
        await page.setViewportSize({ width, height: 844 });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
        for (const button of await page.locator('#updates button').all()) assert.ok((await button.boundingBox()).height >= 44);
      }
      await page.setViewportSize({ width: 390, height: 844 });
      for (const colorScheme of ['dark', 'light']) {
        await page.emulateMedia({ colorScheme });
        if (process.env.GATOR_COMPANION_SCREENSHOT) await page.screenshot({ path: `${process.env.GATOR_COMPANION_SCREENSHOT}-${name}-updates-${colorScheme}.png`, fullPage: true });
      }
      await context.setOffline(true);
      await page.waitForFunction(() => document.querySelector('#updates button').disabled);
      assert.equal(actions.length, afterDrop + 2);
      await context.setOffline(false);
      await page.waitForFunction(() => !document.querySelector('#updates button').disabled);
      await writeFile(tokenFile, 'b'.repeat(64) + '\n', { mode: 0o600 });
      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await page.getByText('Access expired · enter the current host token', { exact: true }).waitFor();
      assert.equal(await page.locator('#updates').textContent(), '');
      assert.deepEqual(errors, []);
    } finally { await browser?.close(); await companion?.close(); await rm(root, { recursive: true, force: true }); }
  });
  test(`${name}: new-chat retries survive reload without duplicates or premature work`, { timeout: 90000 }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'gator-phone-new-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64);
    const creations = new Map(), calls = [], submissions = [];
    let browser, companion, failOpen = true;
    try {
      await writeFile(tokenFile, token + '\n', { mode: 0o600 });
      companion = createCompanion({ tokenFile, workspace: '/host/configured-workspace',
        queryCatalog: async () => ({ items: [{ id: 'work-parent', title: 'Original chat', source: '/host/original' }] }),
        createChat: async requestID => {
          calls.push(requestID);
          if (!creations.has(requestID)) creations.set(requestID, { request_id: requestID, conversation_id: `work-new-${creations.size + 1}`, source: '/host/configured-workspace' });
          return creations.get(requestID);
        },
        spawnBackend: id => ({ state: { conversation_id: id, title: id === 'work-parent' ? 'Original chat' : 'New chat', messages: [], running: false, queue: [] },
          async ready() { if (id.startsWith('work-new') && failOpen) { failOpen = false; throw new Error('Four chats have pending work. Try opening this saved chat later.'); } },
          async stop() { this.closed = true; }, async submitUpload(frame) { submissions.push(structuredClone(frame.upload)); this.state.messages.push({ role: 'user', text: frame.upload.text }); },
        }),
      });
      const address = await companion.listen(), base = `http://127.0.0.1:${address.port}`;
      browser = await ({ chromium, webkit }[name]).launch({ headless: true, executablePath: process.env[`GATOR_${name.toUpperCase()}_EXECUTABLE`] });
      const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true }), page = await context.newPage(), errors = [];
      page.on('pageerror', error => errors.push(error.message));
      const connect = async () => {
        await page.getByLabel('Host access token', { exact: true }).fill(token);
        await page.getByRole('button', { name: 'Connect', exact: true }).click();
        await page.locator('.chat-choice').waitFor();
      };
      await page.goto(base); await connect();
      await page.locator('.chat-choice').click();
      await page.locator('#message').fill('Keep the original draft.');
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.getByText('New chats use this host workspace: /host/configured-workspace', { exact: true }).waitFor();
      assert.equal(calls.length, 0);
      let drop = true;
      await page.route('**/v1/chats', async route => {
        if (drop && route.request().method() === 'POST') { drop = false; await route.fetch(); await route.abort('connectionfailed'); }
        else await route.continue();
      });
      await page.getByRole('button', { name: 'New chat', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#error').textContent.length > 0);
      assert.equal(creations.size, 1);
      assert.equal(submissions.length, 0);
      await page.reload(); await connect();
      assert.equal(calls.length, 1, 'reload must not automatically retry creation');
      await page.getByRole('button', { name: 'Retry new chat', exact: true }).click();
      await page.getByText('Four chats have pending work. Try opening this saved chat later.', { exact: true }).waitFor();
      assert.equal(creations.size, 1, 'an opening failure must retain the created identity');
      await page.getByRole('button', { name: 'Retry new chat', exact: true }).click();
      await page.locator('#chat-title').filter({ hasText: 'New chat' }).waitFor();
      assert.equal(await page.locator('#message').inputValue(), '');
      assert.equal(await page.locator('#message').evaluate(node => document.activeElement === node), true);
      assert.deepEqual(calls, [calls[0], calls[0], calls[0]]);
      assert.equal(creations.size, 1);
      assert.equal(submissions.length, 0, 'opening must not start model work');
      assert.equal(await page.evaluate(() => sessionStorage.getItem('gator.companion.creation.v1')), null);
      await page.locator('#message').fill('Start only after this send.');
      await page.getByRole('button', { name: 'Send message', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#message').value === '');
      assert.equal(submissions.length, 1);
      assert.equal(submissions[0].context, 'conversation:work-new-1');
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.locator('.chat-choice').click();
      await page.locator('#chat-title').filter({ hasText: 'Original chat' }).waitFor();
      assert.equal(await page.locator('#message').inputValue(), 'Keep the original draft.');
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.locator('.chat-choice').waitFor();
      if (process.env.GATOR_COMPANION_SCREENSHOT) await page.screenshot({ path: `${process.env.GATOR_COMPANION_SCREENSHOT}-${name}-new-chat.png`, fullPage: true });
      for (const width of [320, 390]) {
        await page.setViewportSize({ width, height: 844 });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
        assert.ok((await page.locator('#new-chat').boundingBox()).height >= 44);
      }
      drop = true;
      await page.getByRole('button', { name: 'New chat', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#error').textContent.length > 0);
      const beforeDismiss = calls.length;
      await page.route('**/v1/chats?*', async route => {
        const response = await route.fetch(), pageData = await response.json();
        await route.fulfill({ response, json: { ...pageData, new_chat_source: '/host/changed-workspace' } });
      });
      await page.getByRole('button', { name: 'Refresh list', exact: true }).click();
      await page.getByText('The host workspace changed. Dismiss this retry before starting another chat.', { exact: true }).waitFor();
      assert.equal(await page.getByRole('button', { name: 'Retry new chat', exact: true }).isDisabled(), true);
      await page.getByRole('button', { name: 'Dismiss retry', exact: true }).click();
      assert.equal(await page.evaluate(() => sessionStorage.getItem('gator.companion.creation.v1')), null);
      assert.equal(calls.length, beforeDismiss, 'dismissing must not send or retry');
      assert.equal(creations.size, 2, 'dismissing must not delete the already-created chat');
      assert.equal(await page.evaluate(token => JSON.stringify({ ...sessionStorage, ...localStorage }).includes(token), token), false);
      assert.deepEqual(errors, []);
    } finally { await browser?.close(); await companion?.close(); await rm(root, { recursive: true, force: true }); }
  });
  test(`${name}: browse, search and switch chats with independent retained drafts`, { timeout: 90000 }, async () => {
    const root = await mkdtemp(join(tmpdir(), 'gator-phone-catalog-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64);
    const queries = [], submissions = [], workers = new Map();
    let browser, companion, failCatalog = false;
    try {
      await writeFile(tokenFile, token + '\n', { mode: 0o600 });
      companion = createCompanion({ tokenFile, queryCatalog: async query => {
        queries.push(query);
        if (failCatalog) throw new Error('Catalog is temporarily unavailable.');
        if (query.query === 'missing') return { items: [] };
        return { items: query.after ? [{ id: 'work-second', title: 'Second chat', source: '/host/second', archived: query.scope === 'archived' }] : [{ id: 'work-first', title: 'First chat <img src=x onerror="window.injected=true">', source: '/host/first', pinned: true }], next: query.after ? '' : 'second-page', incomplete: query.scope === 'archived' };
      }, spawnBackend: id => {
        const worker = { state: { conversation_id: id, title: id === 'work-first' ? 'First chat' : 'Second chat', messages: [{ role: 'assistant', text: `Answer belonging to ${id}` }], queue: [], running: false }, async ready() {}, async stop() { this.closed = true; }, async submitUpload(frame) {
          submissions.push(structuredClone(frame.upload));
          if (!this.state.messages.some(m => m.request_id === frame.upload.request_id)) this.state.messages.push({ role: 'user', text: frame.upload.text, request_id: frame.upload.request_id });
        } };
        workers.set(id, worker); return worker;
      } });
      const address = await companion.listen(), base = `http://127.0.0.1:${address.port}`;
      browser = await ({ chromium, webkit }[name]).launch({ headless: true, executablePath: process.env[`GATOR_${name.toUpperCase()}_EXECUTABLE`] });
      const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, colorScheme: 'dark' }), page = await context.newPage(), errors = [];
      page.on('pageerror', error => errors.push(error.message));
      await page.goto(base);
      await page.getByLabel('Host access token', { exact: true }).fill(token);
      await page.getByRole('button', { name: 'Connect', exact: true }).click();
      await page.locator('.chat-choice').waitFor();
      assert.equal(workers.size, 0, 'listing must not open any chat');
      assert.equal(await page.locator('#catalog img').count(), 0);
      assert.equal(await page.evaluate(() => !!window.injected), false);
      assert.equal(await page.locator('#token').inputValue(), '');
      for (const selector of ['#scope', '#search', '#search-submit', '.chat-choice']) assert.ok((await page.locator(selector).boundingBox()).height >= 44, `${selector} must have a touch-sized control`);
      for (const width of [320, 390, 760]) {
        await page.setViewportSize({ width, height: 844 });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
      }
      await page.setViewportSize({ width: 390, height: 844 });
      if (process.env.GATOR_COMPANION_SCREENSHOT) {
        await page.screenshot({ path: `${process.env.GATOR_COMPANION_SCREENSHOT}-${name}-catalog.png`, fullPage: true });
        await page.emulateMedia({ colorScheme: 'light' });
        await page.screenshot({ path: `${process.env.GATOR_COMPANION_SCREENSHOT}-${name}-catalog-light.png`, fullPage: true });
        await page.emulateMedia({ colorScheme: 'dark' });
      }
      await page.getByLabel('Search titles and folders', { exact: true }).fill('missing');
      failCatalog = true;
      await page.getByRole('button', { name: 'Search', exact: true }).click();
      await page.getByText('Catalog is temporarily unavailable.', { exact: true }).waitFor();
      assert.equal(await page.locator('#next').isDisabled(), true);
      failCatalog = false;
      await page.getByRole('button', { name: 'Refresh list', exact: true }).click();
      await page.getByText('No conversations match.', { exact: true }).waitFor();
      assert.equal(queries.at(-1).query, 'missing', 'refresh must retry the failed search');
      assert.equal(await page.locator('#next').isDisabled(), true);
      await page.getByLabel('Search titles and folders', { exact: true }).fill('');
      await page.getByLabel('Show', { exact: true }).selectOption('archived');
      await page.getByRole('button', { name: 'Search', exact: true }).click();
      await page.getByText(/Some retained conversations could not be checked/).waitFor();
      await page.getByRole('button', { name: 'Next page', exact: true }).click();
      await page.locator('.chat-choice').filter({ hasText: 'Second chat' }).waitFor();
      assert.deepEqual(queries.at(-1), { query: '', scope: 'archived', after: 'second-page' });
      await page.getByRole('button', { name: 'Previous page', exact: true }).click();
      const firstChoice = page.locator('.chat-choice').filter({ hasText: 'First chat' });
      await firstChoice.waitFor(); await firstChoice.focus(); await page.keyboard.press('Enter');
      await page.getByText('Answer belonging to work-first', { exact: true }).waitFor();
      assert.equal(await page.locator('#chat-title').evaluate(node => document.activeElement === node), true);
      await page.getByLabel('Message', { exact: true }).fill('Unsent first draft');
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.getByRole('button', { name: 'Next page', exact: true }).click();
      await page.locator('.chat-choice').filter({ hasText: 'Second chat' }).click();
      await page.getByText('Answer belonging to work-second', { exact: true }).waitFor();
      assert.equal(await page.getByLabel('Message', { exact: true }).inputValue(), '');
      await page.getByLabel('Message', { exact: true }).fill('Separate second draft');
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.locator('.chat-choice').filter({ hasText: 'First chat' }).click();
      await page.getByText('Answer belonging to work-first', { exact: true }).waitFor();
      assert.equal(await page.getByLabel('Message', { exact: true }).inputValue(), 'Unsent first draft');
      assert.equal(submissions.length, 0);
      let drop = true;
      await page.route('**/actions', async route => {
        if (drop) { drop = false; await route.fetch(); await route.abort('connectionfailed'); }
        else await route.continue();
      });
      await page.getByRole('button', { name: 'Send message', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#error').textContent.length > 0);
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.getByRole('button', { name: 'Next page', exact: true }).click();
      await page.locator('.chat-choice').filter({ hasText: 'Second chat' }).click();
      await page.getByText('Answer belonging to work-second', { exact: true }).waitFor();
      assert.equal(await page.getByLabel('Message', { exact: true }).inputValue(), 'Separate second draft');
      assert.equal(await page.locator('#message').getAttribute('readonly'), null);
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.locator('.chat-choice').filter({ hasText: 'First chat' }).click();
      await page.getByRole('button', { name: 'Retry same message', exact: true }).click();
      await page.waitForFunction(() => document.querySelector('#message').value === '');
      assert.deepEqual(submissions[1], submissions[0]);
      assert.equal(workers.get('work-first').state.messages.filter(m => m.role === 'user').length, 1);
      await page.getByRole('button', { name: 'Conversations', exact: true }).click();
      await page.locator('.chat-choice').waitFor();
      await writeFile(tokenFile, 'b'.repeat(64) + '\n');
      await page.getByRole('button', { name: 'Refresh list', exact: true }).click();
      await page.getByText('Access expired · enter the current host token', { exact: true }).waitFor();
      assert.equal(await page.locator('#catalog').isVisible(), false);
      assert.equal(await page.locator('#catalog-items').textContent(), '');
      assert.equal(await page.evaluate(token => JSON.stringify({ ...sessionStorage, ...localStorage }).includes(token), token), false);
      assert.deepEqual(errors, []);
    } finally { await browser?.close(); await companion?.close(); await rm(root, { recursive: true, force: true }); }
  });
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
        await page.getByText('Open a known conversation', { exact: true }).click();
        await page.getByLabel('Conversation ID', { exact: true }).fill('work-phone');
        await page.getByRole('button', { name: 'Connect', exact: true }).click();
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
