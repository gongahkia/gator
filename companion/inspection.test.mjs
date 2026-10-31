import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { createCompanion } from './server.mjs';
import { addDevice, revokeDevice, inspectionDeviceActor } from './auth.mjs';
import { validateInspection } from './inspection.mjs';
import { Backend } from '../app-host/src/backend.mjs';

const execute = promisify(execFile);
const waitFor = async predicate => { for (let i = 0; i < 300; i++) { const value = await predicate(); if (value) return value; await new Promise(r => setTimeout(r, 50)); } throw new Error('Fixture did not reach expected state'); };
test('real Go owners behind Electron/OpenTUI/companion transports are inspected without replacement workers', { timeout: 240000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-inspection-real-'));
  const binary = join(root, process.platform === 'win32' ? 'runtime.exe' : 'runtime');
  try {
    await execute('go', ['build', '-o', binary, './cmd/gator'], { timeout: 90000 });
    for (const frontend of ['desktop', 'terminal', 'companion']) {
      const dir = join(root, frontend), workspace = join(dir, 'workspace'), config = join(dir, 'config'), state = join(dir, 'state'), tokenFile = join(dir, 'token');
      await mkdir(dir, { mode: 0o700 });
      await mkdir(workspace, { recursive: true }); await mkdir(join(config, 'gator'), { recursive: true });
      await writeFile(tokenFile, 'a'.repeat(64) + '\n', { mode: 0o600 });
      const exported = join(dir, 'device.token'), device = await addDevice(tokenFile, 'Fixture phone', exported), token = (await readFile(exported, 'utf8')).trim();
      const actor = await inspectionDeviceActor(tokenFile, device.id);
      let release, held = 0, calls = 0, host, otherHost, companion, spawns = 0;
      const gate = new Promise(r => release = r);
      const provider = createServer(async (req, res) => {
        let body = ''; for await (const chunk of req) body += chunk;
        const payload = JSON.parse(body), classify = payload.messages.some(m => m.role === 'system' && m.content?.includes("Classify the user's immediate request"));
        calls++;
        if (!classify && body.includes('T1-held')) { held++; await gate; }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: classify ? '{"mode":"inspect"}' : 'Fixture completed.' }, finish_reason: 'stop' }], usage: { prompt_tokens: 1, completion_tokens: 1 } }));
      });
      try {
        await new Promise(r => provider.listen(0, '127.0.0.1', r));
        await writeFile(join(config, 'gator/config.json'), JSON.stringify({ version: 4, defaults: { provider: 'fixture', model: 'fixture' }, custom_providers: [{ id: 'fixture', base_url: `http://127.0.0.1:${provider.address().port}/chat`, models: ['fixture'], default_model: 'fixture' }] }));
        const env = { ...process.env, GATOR_CONFIG_DIR: config, GATOR_STATE_DIR: state, GATOR_INTERNAL_COMMANDS: '1', GATOR_UI_SESSION_HOST: '1', GATOR_UI_FRONTEND: frontend };
        await execute(binary, ['work', '--source', workspace, '--mode', 'inspect', 'Seed inspection fixture'], { env, timeout: 30000 });
        const id = (await readdir(join(state, 'gator/conversations'))).find(n => n.startsWith('work-'));
        assert.ok(id);
        companion = createCompanion({ binary, workspace, env, tokenFile, spawnBackend: conversation => { spawns++; assert.equal(frontend, 'companion', 'inspection must never spawn a worker'); host = new Backend(binary, workspace, env, ['ui-backend', conversation, '']); return host; } });
        const address = await companion.listen(), base = `http://127.0.0.1:${address.port}`;
        const headers = { Authorization: `Bearer ${token}` };
        const get = async path => { const response = await fetch(base + path, { headers }); assert.equal(response.status, 200, await response.clone().text()); return response.json(); };
        if (frontend !== 'companion') { host = new Backend(binary, workspace, env, ['ui-backend', id, '']); await host.ready(); }
        else await get(`/v1/chats/${id}`);
        const frame = { type: 'upload_submit', upload: { request_id: `inspection-${frontend}`, context: `conversation:${id}`, text: 'T1-held: inspect this question', files: [] } };
        if (frontend === 'companion') {
          const response = await fetch(`${base}/v1/chats/${id}/actions`, { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json' }, body: JSON.stringify(frame) }); assert.equal(response.status, 200, await response.text());
        } else await host.submitUpload(frame);
        await waitFor(() => held);
        const controlDir = join(state, 'gator/work-control');
        const owner = await waitFor(async () => { for (const name of await readdir(controlDir)) { if (name.endsWith('.json')) { const value = JSON.parse(await readFile(join(controlDir, name), 'utf8')); if (value.inspection) return value; } } });
        assert.deepEqual((await get('/v1/work')).targets, [], 'conversation/device access is not an inspection grant');
        // Exercise the actual host-local enrollment-aware grant helper.
        await execute(process.execPath, [resolve('companion/inspection-access.mjs'), binary, tokenFile, owner.run_id, device.id, '60'], { env });
        if (frontend === 'desktop') {
          // A second real live conversation is not covered by the first grant.
          await execute(binary, ['work', '--source', workspace, '--mode', 'inspect', 'Another seed conversation'], { env, timeout: 30000 });
          const otherID = (await readdir(join(state, 'gator/conversations'))).find(n => n.startsWith('work-') && n !== id);
          assert.ok(otherID);
          otherHost = new Backend(binary, workspace, env, ['ui-backend', otherID, '']); await otherHost.ready();
          await otherHost.submitUpload({ type: 'upload_submit', upload: { request_id: 'other-inspection', context: `conversation:${otherID}`, text: 'T1-held second conversation', files: [] } });
          await waitFor(async () => (await readdir(controlDir)).filter(n => n.endsWith('.json')).length === 2);
          await waitFor(() => held === 2);
        }
        const before = calls, expectedSpawns = frontend === 'companion' ? 1 : 0;
        const page = await get('/v1/work'); assert.equal(page.targets.length, 1);
        const item = page.targets[0], binding = item.target.binding;
        assert.equal(item.conversation_id, id); assert.equal(item.run_id, owner.run_id); assert.equal(binding.owner, 'work-owner-' + owner.nonce);
        const path = `/v1/work/${binding.id}?generation=${binding.generation}`;
        const selected = await get(path); assert.equal(selected.targets[0].status, 'running');
        assert.deepEqual(selected.targets[0].target.capabilities, ['inspect']);
        assert.equal((await get(`/v1/work/${binding.id}/activity?generation=${binding.generation}`)).targets[0].run_id, owner.run_id);
        assert.equal((await get(`/v1/work/${binding.id}?generation=${'f'.repeat(32)}`)).unavailable, true);
        assert.equal((await get(`/v1/work/target-${'f'.repeat(32)}?generation=${binding.generation}`)).unavailable, true);
        assert.equal(spawns, expectedSpawns); assert.equal(calls, before, 'inspection invoked a model');
        const raw = JSON.stringify(selected); assert.ok(!raw.includes(owner.token)); assert.ok(!raw.includes(owner.url));
        assert.equal((await fetch(base + '/v1/work', { method: 'POST', headers })).status, 405);
        await execute(binary, ['inspection-access', owner.run_id, actor, '0'], { env });
        assert.equal((await get(path)).unavailable, true);
        await execute(binary, ['inspection-access', owner.run_id, actor, '60'], { env });
        await host.stop();
        assert.equal((await get(path)).unavailable, true, 'stopped owner must not be reconstructed from history');
        assert.equal(spawns, expectedSpawns); release();
      } finally {
        release(); await host?.stop(); await otherHost?.stop(); await companion?.close(); await new Promise(r => provider.close(r));
      }
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('inspection revocation while queued or reading suppresses private responses and never starts a worker', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-inspection-auth-')), tokenFile = join(root, 'owner.token'), exported = join(root, 'device.token');
  let companion, release, entered;
  const gate = new Promise(r => release = r), started = new Promise(r => entered = r);
  try {
    await writeFile(tokenFile, 'a'.repeat(64) + '\n', { mode: 0o600 });
    const device = await addDevice(tokenFile, 'Phone', exported), token = (await readFile(exported, 'utf8')).trim(), actor = await inspectionDeviceActor(tokenFile, device.id);
    companion = createCompanion({ binary: '', workspace: root, tokenFile, spawnBackend() { throw new Error('unexpected worker'); }, queryWork: async input => { assert.equal(input.actor, actor); entered(); await gate; return { targets: [] }; } });
    const address = await companion.listen(), url = `http://127.0.0.1:${address.port}/v1/work`;
    assert.equal((await fetch(url)).status, 401);
    const read = fetch(url, { headers: { Authorization: `Bearer ${token}` } }); await started;
    const queued = fetch(url, { headers: { Authorization: `Bearer ${token}` } });
    await revokeDevice(tokenFile, device.id); release();
    assert.equal((await read).status, 401); assert.equal((await queued).status, 401);
  } finally { release(); await companion?.close(); await rm(root, { recursive: true, force: true }); }
});

test('inspection remote contract rejects oversized and mismatched projections', () => {
  assert.throws(() => validateInspection({ targets: new Array(17).fill({}) }));
  assert.throws(() => validateInspection({ targets: [], private_payload: 'x'.repeat(129 * 1024) }));
  assert.deepEqual(validateInspection({ targets: [], token: 'never-forward' }).targets, []);
  assert.equal('token' in validateInspection({ targets: [], token: 'never-forward' }), false);
});
