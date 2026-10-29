import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer, get as httpGet } from 'node:http';
import { execFileSync, execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, mkdir, writeFile, rm, readdir, readFile, chmod, symlink, realpath } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createCompanion, readToken } from './server.mjs';

test('real Go work survives a client disconnect, deduplicates admission and revokes old access', { timeout: 120000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-companion-'));
  const workspace = join(root, 'workspace'), config = join(root, 'config'), state = join(root, 'state'), binary = join(root, process.platform === 'win32' ? 'runtime.exe' : 'runtime');
  const tokenFile = join(root, 'token'), token = 'a'.repeat(64);
  let releaseModel, startedModel;
  const started = new Promise(r => startedModel = r), gate = new Promise(r => releaseModel = r);
  let requests = 0;
  const provider = createServer(async (req, res) => {
    let body = ''; for await (const chunk of req) body += chunk;
    const payload = JSON.parse(body);
    const classify = payload.messages.some(m => m.role === 'system' && m.content?.includes("Classify the user's immediate request"));
    requests++;
    if (!classify && payload.messages.some(m => m.role === 'user' && m.content?.includes('remote test question'))) { startedModel(); await gate; }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: classify ? '{"mode":"inspect"}' : 'Completed on the companion.' }, finish_reason: 'stop' }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  });
  let companion;
  try {
    await mkdir(workspace); await mkdir(join(config, 'gator'), { recursive: true });
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    await new Promise(r => provider.listen(0, '127.0.0.1', r));
    await writeFile(join(config, 'gator/config.json'), JSON.stringify({ version: 4, defaults: { provider: 'fixture', model: 'fixture' }, custom_providers: [{ id: 'fixture', base_url: `http://127.0.0.1:${provider.address().port}/chat`, models: ['fixture'], default_model: 'fixture' }] }));
    const env = { ...process.env, GATOR_CONFIG_DIR: config, GATOR_STATE_DIR: state, GATOR_INTERNAL_COMMANDS: '1' };
    execFileSync('go', ['build', '-o', binary, './cmd/gator'], { cwd: resolve('.'), timeout: 90000 });
    // Create a real retained conversation through the supported internal service.
    await promisify(execFile)(binary, ['work', '--source', workspace, '--mode', 'inspect', 'Create a test conversation'], { env, timeout: 30000 });
    const id = (await readdir(join(state, 'gator/conversations'))).find(name => name.startsWith('work-'));
    assert.ok(id);
    companion = createCompanion({ binary, workspace, env, tokenFile, origin: 'https://companion.example' });
    const address = await companion.listen();
    const url = `http://127.0.0.1:${address.port}/v1/chats/${id}`;
    const get = async (key = token, extra = {}) => {
      const options = { headers: { authorization: `Bearer ${key}`, ...extra } };
      try { return await fetch(url, options); }
      catch (error) {
        // The deliberate socket drop can leave one stale pooled connection.
        // Reconnect only this read; never invent a new mutation identity.
        if (error.cause?.code !== 'ECONNRESET' && error.cause?.code !== 'UND_ERR_SOCKET') throw error;
        return fetch(url, options);
      }
    };
    const send = frame => fetch(url + '/actions', { method: 'POST', headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' }, body: JSON.stringify(frame) });
    const catalog = `http://127.0.0.1:${address.port}/v1/chats`;
    const beforeCatalog = requests;
    assert.equal((await fetch(catalog)).status, 401);
    const listed = await fetch(catalog, { headers: { authorization: `Bearer ${token}` } });
    assert.equal(listed.status, 200, 'real Go catalog must be available without opening a chat');
    const page = await listed.json();
    assert.deepEqual(page.items.map(item => item.id), [id]);
    assert.equal(requests, beforeCatalog, 'browsing must not invoke a model');
    assert.equal((await readdir(join(state, 'gator/conversations'))).filter(name => name.startsWith('work-')).length, 1);
    assert.equal(page.new_chat_source, workspace);
    const creation = { request_id: 'stable-new-chat-request' };
    const create = (base = catalog, key = token) => fetch(base, { method: 'POST', headers: { authorization: `Bearer ${key}`, 'content-type': 'application/json' }, body: JSON.stringify(creation) });
    const created = await create(); assert.equal(created.status, 200, 'new-chat creation must succeed');
    const newChat = await created.json();
    assert.equal(newChat.request_id, creation.request_id);
    assert.equal(newChat.source, await realpath(workspace));
    assert.notEqual(newChat.conversation_id, id);
    assert.deepEqual(await (await create()).json(), newChat, 'retry must return the same empty chat');
    const emptyChat = await (await fetch(catalog + '/' + newChat.conversation_id, { headers: { authorization: `Bearer ${token}` } })).json();
    assert.equal(emptyChat.state.conversation_id, newChat.conversation_id);
    assert.equal(emptyChat.state.running, false);
    assert.equal(requests, beforeCatalog, 'creating and opening an empty chat must not invoke a model');
    assert.equal((await readdir(join(state, 'gator/conversations'))).filter(name => name.startsWith('work-')).length, 2);
    assert.equal((await get('bad')).status, 401);
    assert.equal((await get(token, { origin: 'https://attacker.example' })).status, 403);
    await mkdir(join(state, 'gator/inbox'), { recursive: true });
    for (let index = 0; index < 55; index++) {
      const entry = { version: 1, id: `inbox-remote-${String(index).padStart(3, '0')}`, kind: 'job', title: `Retained follow-up ${index}`, summary: 'Retained host update.', status: index === 54 ? 'failed' : 'completed', origin_conversation_id: id, created_at: new Date(Date.UTC(2026, 9, 29, 0, index)).toISOString() };
      await writeFile(join(state, 'gator/inbox', entry.id + '.json'), JSON.stringify(entry), { mode: 0o600 });
    }
    const first = await (await get()).json(); assert.equal(first.state.conversation_id, id);
    assert.equal(first.state.chat_updates.length, 50);
    assert.equal(first.state.chat_updates_total, 55);
    const updateID = first.state.chat_updates.at(-1).id;
    const changeUpdate = async (action, version = 0, update = updateID, conversation = id) => {
      const response = await send({ type: 'update', update: { conversation_id: conversation, id: update, action, version } });
      return { status: response.status, ...await response.json() };
    };
    assert.equal((await changeUpdate('read', 0, updateID, newChat.conversation_id)).status, 400);
    const read = await changeUpdate('read');
    assert.equal(read.state.chat_updates.at(-1).read, true);
    const done = await changeUpdate('resolve');
    assert.equal(done.state.chat_updates.at(-1).resolved, true);
    assert.equal(done.state.chat_updates.at(-1).status, 'failed', 'marking done must not rewrite a failed check');
    const stale = await changeUpdate('reopen');
    assert.match(stale.state.chat_updates_issue, /changed/);
    assert.equal(stale.state.chat_updates.at(-1).resolved, true);
    const reopenedUpdate = await changeUpdate('reopen', 1);
    assert.equal(reopenedUpdate.state.chat_updates.at(-1).resolved, false);
    assert.equal((await changeUpdate('resolve', 2)).state.chat_updates.at(-1).version, 3);
    const older = await changeUpdate('older', 0, first.state.chat_updates[0].id);
    assert.equal(older.state.chat_updates.length, 5);
    assert.equal(older.state.updates_have_newer, true);
    const newer = await changeUpdate('newer', 0, older.state.chat_updates.at(-1).id);
    assert.equal(newer.state.chat_updates.length, 50);
    assert.equal(requests, beforeCatalog, 'update receipts and paging must not call a model');
    assert.equal((await send({ type: 'submit', text: '/data' })).status, 400);
    const frame = { type: 'upload_submit', upload: { context: `conversation:${id}`, request_id: 'stable-remote-request', text: 'Answer this remote test question.', files: [] } };
    assert.equal((await send({ ...frame, upload: { ...frame.upload, context: 'conversation:other' } })).status, 400);
    const accepted = await send(frame); assert.equal(accepted.status, 200, await accepted.text());
    await started;
    // Drop all client sockets while the provider call remains in flight.
    companion.server.closeAllConnections();
    releaseModel();
    let completed;
    for (let i = 0; i < 150; i++) {
      completed = await (await get()).json();
      if (!completed.state.running) break;
      await new Promise(r => setTimeout(r, 30));
    }
    assert.equal(completed.state.running, false);
    assert.ok(completed.state.messages.some(m => m.text?.includes('Completed on the companion.')));
    const before = requests;
    assert.equal((await send(frame)).status, 200);
    await new Promise(r => setTimeout(r, 100)); assert.equal(requests, before);
    const journal = JSON.parse(await readFile(join(state, 'gator/conversations', id, 'requests.json'), 'utf8'));
    assert.equal(journal.requests.filter(r => r.client_id === 'stable-remote-request').length, 1);
    await writeFile(tokenFile, 'b'.repeat(64) + '\n');
    assert.equal((await get()).status, 401);
    assert.equal((await get('b'.repeat(64))).status, 200);
    assert.equal(requests, before);
    await companion.close();
    companion = createCompanion({ binary, workspace, env, tokenFile });
    const reopened = await companion.listen();
    assert.deepEqual(await (await create(`http://127.0.0.1:${reopened.port}/v1/chats`, 'b'.repeat(64))).json(), newChat, 'creation receipt must survive companion restart and token rotation');
    const retained = await fetch(`http://127.0.0.1:${reopened.port}/v1/chats/${id}`, { headers: { authorization: `Bearer ${'b'.repeat(64)}` } });
    const retainedState = await retained.json();
    assert.equal(retained.status, 200);
    assert.equal(retainedState.state.running, false);
    const retainedUpdate = retainedState.state.chat_updates.find(update => update.id === updateID);
    assert.equal(retainedUpdate.read, true);
    assert.equal(retainedUpdate.resolved, true);
    assert.equal(retainedUpdate.version, 3, 'follow-up decisions must survive companion restart');
    assert.ok(retainedState.state.messages.some(m => m.text?.includes('Completed on the companion.')));
    assert.equal(requests, before, 'restarting or reading must not replay work');
  } finally {
    releaseModel?.();
    if (companion) await companion.close();
    provider.closeAllConnections(); await new Promise(r => provider.close(r));
    await rm(root, { recursive: true, force: true });
  }
});

test('new chats reject unreviewed source or message fields and require stable identities', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-create-chat-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64), creations = [];
  let companion;
  try {
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    companion = createCompanion({ tokenFile, createChat: async requestID => { creations.push(requestID); return { request_id: requestID, conversation_id: 'work-new', source: '/host/workspace' }; }, spawnBackend() { throw new Error('Creating a chat must not open a worker.'); } });
    const address = await companion.listen(), url = `http://127.0.0.1:${address.port}/v1/chats`;
    const send = (body, authorization = `Bearer ${token}`) => fetch(url, { method: 'POST', headers: { authorization, 'content-type': 'application/json' }, body: JSON.stringify(body) });
    assert.equal((await send({ request_id: 'valid-creation-request' }, '')).status, 401);
    for (const body of [null, {}, [], { request_id: 'short' }, { request_id: '../unsafe-creation-request' }, { request_id: 'valid-creation-request', source: '/other' }, { request_id: 'valid-creation-request', text: 'Do work now' }]) assert.equal((await send(body)).status, 400);
    assert.equal((await send({ request_id: 'x'.repeat(5000) })).status, 413);
    assert.deepEqual(creations, []);
    const created = await send({ request_id: 'valid-creation-request' });
    assert.equal(created.status, 200);
    assert.equal((await created.json()).conversation_id, 'work-new');
    assert.deepEqual(creations, ['valid-creation-request']);
  } finally { await companion?.close(); await rm(root, { recursive: true, force: true }); }
});

test('catalog authorizes reads, bounds parameters and rechecks revocation after a queued read', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-catalog-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64);
  let companion, release, begin;
  const started = new Promise(resolve => begin = resolve), gate = new Promise(resolve => release = resolve), queries = [];
  try {
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    companion = createCompanion({ tokenFile, spawnBackend() { throw new Error('Catalog must not open a conversation.'); }, queryCatalog: async query => {
      queries.push(query);
      if (query.query === 'blocked') { begin(); await gate; }
      return { items: [{ id: 'work-private', title: 'Private title' }], next: 'cursor', incomplete: true };
    } });
    const address = await companion.listen(), url = `http://127.0.0.1:${address.port}/v1/chats`;
    const headers = { authorization: `Bearer ${token}` };
    assert.equal((await fetch(url)).status, 401);
    for (const query of ['?scope=invalid', '?q=a&q=b', '?unknown=true', '?q=' + 'x'.repeat(513), '?after=' + 'x'.repeat(1025)]) assert.equal((await fetch(url + query, { headers })).status, 400);
    assert.equal(queries.length, 0);
    const result = await fetch(url + '?q=Budget&scope=archived&after=cursor', { headers });
    assert.equal(result.status, 200);
    assert.equal((await result.json()).incomplete, true);
    assert.deepEqual(queries[0], { query: 'Budget', scope: 'archived', after: 'cursor' });
    const inflight = fetch(url + '?q=blocked', { headers });
    await started;
    await writeFile(tokenFile, 'b'.repeat(64) + '\n'); release();
    const revoked = await inflight;
    assert.equal(revoked.status, 401);
    assert.ok(!(await revoked.text()).includes('Private title'));
  } finally { release?.(); await companion?.close(); await rm(root, { recursive: true, force: true }); }
});

test('browsing more chats retires only idle workers and never pending work', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-catalog-workers-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64), workers = new Map();
  let companion;
  try {
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    companion = createCompanion({ tokenFile, spawnBackend: id => {
      const worker = { state: { conversation_id: id, running: id === 'work-0', queue: id === 'work-1' ? [{ text: 'Queued' }] : [], pending_approval: id === 'work-2' ? { id: 1 } : undefined, pending_connection: id === 'work-3' }, async ready() {}, async stop() { this.closed = true; } };
      workers.set(id, worker); return worker;
    } });
    const address = await companion.listen(), url = `http://127.0.0.1:${address.port}/v1/chats/`, headers = { authorization: `Bearer ${token}` };
    for (let i = 0; i < 4; i++) assert.equal((await fetch(url + `work-${i}`, { headers })).status, 200);
    assert.equal((await fetch(url + 'work-4', { headers })).status, 400);
    assert.ok([...workers.values()].every(worker => !worker.closed));
    workers.get('work-0').state.running = false;
    assert.equal((await fetch(url + 'work-4', { headers })).status, 200);
    assert.equal(workers.get('work-0').closed, true);
    for (const id of ['work-1', 'work-2', 'work-3']) assert.ok(!workers.get(id).closed);
    assert.equal((await fetch(url + 'work-0', { headers })).status, 200, 'retired idle chats can reopen');
    assert.equal(workers.get('work-4').closed, true);
  } finally { await companion?.close(); await rm(root, { recursive: true, force: true }); }
});

test('token files reject public permissions, symlinks and malformed values', { skip: process.platform === 'win32' }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-companion-token-'));
  const path = join(root, 'token');
  try {
    await writeFile(path, 'a'.repeat(64) + '\n', { mode: 0o600 });
    assert.equal(await readToken(path), 'a'.repeat(64));
    await chmod(path, 0o644); await assert.rejects(readToken(path));
    await chmod(path, 0o600);
    await symlink(path, join(root, 'link')); await assert.rejects(readToken(join(root, 'link')));
    await writeFile(path, 'not-a-token'); await assert.rejects(readToken(path));
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('public browser shell has no host data and rejects foreign hosts and origins', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-companion-shell-'));
  const tokenFile = join(root, 'token'); let companion;
  try {
    await writeFile(tokenFile, 'a'.repeat(64) + '\n', { mode: 0o600 });
    companion = createCompanion({ tokenFile, spawnBackend() { throw new Error('Shell must not start a runtime.'); } });
    const address = await companion.listen(), base = `http://127.0.0.1:${address.port}`;
    const page = await fetch(base);
    assert.equal(page.status, 200);
    assert.ok(page.headers.get('content-security-policy').includes("frame-ancestors 'none'"));
    assert.equal(page.headers.get('cache-control'), 'no-store');
    assert.match(await page.text(), /Gator companion/);
    assert.equal((await fetch(base + '/client.mjs')).status, 200);
    assert.equal((await fetch(base + '/v1/chats/work-private')).status, 401);
    const foreignHost = await new Promise((resolve, reject) => {
      httpGet(base, { headers: { Host: 'attacker.example' } }, res => { res.resume(); resolve(res.statusCode); }).on('error', reject);
    });
    assert.equal(foreignHost, 421);
    assert.equal((await fetch(base, { headers: { Origin: 'https://attacker.example' } })).status, 403);
    assert.equal((await fetch(base, { headers: { Origin: base } })).status, 200);
    assert.throws(() => createCompanion({ tokenFile, origin: 'https://example.test/path' }));
  } finally { await companion?.close(); await rm(root, { recursive: true, force: true }); }
});
