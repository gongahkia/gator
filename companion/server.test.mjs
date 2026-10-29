import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer, get as httpGet } from 'node:http';
import { execFileSync, execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, mkdir, writeFile, rm, readdir, readFile, chmod, symlink } from 'node:fs/promises';
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
    assert.equal((await get('bad')).status, 401);
    assert.equal((await get(token, { origin: 'https://attacker.example' })).status, 403);
    const first = await (await get()).json(); assert.equal(first.state.conversation_id, id);
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
    const retained = await fetch(`http://127.0.0.1:${reopened.port}/v1/chats/${id}`, { headers: { authorization: `Bearer ${'b'.repeat(64)}` } });
    const retainedState = await retained.json();
    assert.equal(retained.status, 200);
    assert.equal(retainedState.state.running, false);
    assert.ok(retainedState.state.messages.some(m => m.text?.includes('Completed on the companion.')));
    assert.equal(requests, before, 'restarting or reading must not replay work');
  } finally {
    releaseModel?.();
    if (companion) await companion.close();
    provider.closeAllConnections(); await new Promise(r => provider.close(r));
    await rm(root, { recursive: true, force: true });
  }
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
