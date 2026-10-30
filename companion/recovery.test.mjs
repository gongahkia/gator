import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { createCompanion } from './server.mjs';
import { checkListener } from './doctor.mjs';

test('idle crash recovery is read-only, rate-limited, and health never starts a worker', async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-recovery-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64);
  let companion, latest, starts = 0, actions = 0, now = 1000000;
  try {
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    companion = createCompanion({ tokenFile, now: () => now, spawnBackend: id => {
      starts++;
      latest = { state: { conversation_id: id, running: false, queue: [] }, async ready() {}, async stop() { this.closed = true; }, async apply() { actions++; } };
      return latest;
    } });
    const { port } = await companion.listen(), origin = `http://127.0.0.1:${port}`, url = origin + '/v1/chats/work-idle', headers = { authorization: `Bearer ${token}` };
    assert.equal((await fetch(origin + '/v1/health')).status, 401);
    assert.equal((await checkListener(origin, tokenFile)).sessions, 0);
    assert.equal(starts, 0);
    assert.equal((await fetch(url, { headers })).status, 200);
    latest.closed = true;
    assert.equal((await checkListener(origin, tokenFile)).stopped_sessions, 1);
    const action = { type: 'queue', queue: { action: 'run' } };
    assert.equal((await fetch(url + '/actions', { method: 'POST', headers: { ...headers, 'content-type': 'application/json' }, body: JSON.stringify(action) })).status, 400);
    assert.equal(starts, 1, 'POST cannot restart a dead session');
    assert.equal((await fetch(url, { headers })).status, 200);
    latest.closed = true;
    assert.equal((await fetch(url, { headers })).status, 400, 'immediate restart throttled');
    for (let index = 0; index < 2; index++) {
      now += 10000;
      assert.equal((await fetch(url, { headers })).status, 200);
      latest.closed = true;
    }
    now += 10000;
    assert.equal((await fetch(url, { headers })).status, 400, 'three restart attempts per five minutes');
    assert.equal(starts, 4);
    now += 300000;
    assert.equal((await fetch(url, { headers })).status, 200);
    assert.equal(actions, 0, 'recovery never replays an action');
    await assert.rejects(checkListener('http://remote.example', tokenFile));
  } finally { await companion?.close(); await rm(root, { recursive: true, force: true }); }
});

test('active work, approvals, and unconfirmed admission cannot restart automatically', async () => {
  for (const state of [{ running: true }, { pending_approval: { id: 1 } }, { queue: [{ text: 'pending' }] }, {}]) {
    const root = await mkdtemp(join(tmpdir(), 'gator-recovery-')), tokenFile = join(root, 'token'), token = 'a'.repeat(64);
    let companion, latest, starts = 0;
    try {
      await writeFile(tokenFile, token + '\n', { mode: 0o600 });
      companion = createCompanion({ tokenFile, spawnBackend: id => {
        starts++;
        latest = { state: { conversation_id: id, ...state }, async ready() {}, async stop() { this.closed = true; }, async submitUpload() { this.closed = true; throw new Error('unconfirmed'); } };
        return latest;
      } });
      const { port } = await companion.listen(), url = `http://127.0.0.1:${port}/v1/chats/work-pending`, headers = { authorization: `Bearer ${token}` };
      assert.equal((await fetch(url, { headers })).status, 200);
      if (!Object.keys(state).length) {
        const upload = { type: 'upload_submit', upload: { context: 'conversation:work-pending', request_id: 'stable-uncertain-request', text: 'test', files: [] } };
        assert.equal((await fetch(url + '/actions', { method: 'POST', headers: { ...headers, 'content-type': 'application/json' }, body: JSON.stringify(upload) })).status, 400);
      } else latest.closed = true;
      assert.equal((await fetch(url, { headers })).status, 400);
      assert.equal(starts, 1);
    } finally { await companion?.close(); await rm(root, { recursive: true, force: true }); }
  }
});
