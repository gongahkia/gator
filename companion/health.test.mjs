import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdtemp, mkdir, readFile, writeFile, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createCompanion } from './server.mjs';

test('real Go check health is conversation-scoped, reports offline/stale states and survives restart without execution', { timeout: 120000 }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-companion-health-'));
  const workspace = join(root, 'workspace'), stateDir = join(root, 'state'), configDir = join(root, 'config');
  const tokenFile = join(root, 'owner.token'), token = 'a'.repeat(64);
  const binary = process.env.GATOR_TEST_BINARY ? resolve(process.env.GATOR_TEST_BINARY) : join(root, process.platform === 'win32' ? 'gator.exe' : 'gator');
  const env = { ...process.env, GATOR_STATE_DIR: stateDir, GATOR_CONFIG_DIR: configDir, GATOR_INTERNAL_COMMANDS: '1' };
  let companion, base, requests = 0;
  const model = createServer((req, res) => { requests++; req.resume(); res.writeHead(500); res.end('This read-only fixture must not call a model.'); });
  const command = (...args) => execFileSync(binary, args, { env, cwd: workspace, encoding: 'utf8', timeout: 20000, stdio: 'pipe' });
  // Cold backend readiness allows 15s and catalog creation allows 20s. This
  // integration check tests health correctness, not a 3s startup latency SLO.
  const request = async (path, options = {}) => {
    try { return await fetch(base + path, { ...options, headers: { authorization: `Bearer ${token}`, ...options.headers }, signal: AbortSignal.timeout(25000) }); }
    catch (error) { throw new Error(`Health fixture request failed: ${options.method || "GET"} ${path}`, { cause: error }); }
  };
  async function state(id) { const response = await request(`/v1/chats/${id}`); assert.equal(response.status, 200); return (await response.json()).state; }
  async function until(read, label) {
    const deadline = Date.now() + 20000;
    while (Date.now() < deadline) { const value = await read(); if (value) return value; await new Promise(resolve => setTimeout(resolve, 100)); }
    throw new Error(label);
  }
  async function start() {
    companion = createCompanion({ binary, workspace, env, tokenFile });
    base = `http://127.0.0.1:${(await companion.listen()).port}`;
  }
  try {
    await mkdir(workspace); await mkdir(join(configDir, 'gator'), { recursive: true });
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
    await writeFile(join(configDir, 'gator/config.json'), JSON.stringify({ version: 4, defaults: { provider: 'fixture', model: 'fixture' }, custom_providers: [{ id: 'fixture', base_url: `http://127.0.0.1:${model.address().port}/chat`, models: ['fixture'], default_model: 'fixture' }] }));
    if (!process.env.GATOR_TEST_BINARY) execFileSync('go', ['build', '-o', binary, './cmd/gator'], { timeout: 90000 });
    await start();
    const ids = [];
    for (let index = 0; index < 2; index++) {
      const response = await request('/v1/chats', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ request_id: randomUUID() }) });
      assert.equal(response.status, 200); ids.push((await response.json()).conversation_id);
    }
    const added = command('job', 'add', 'Health fixture', '--schedule', '*/15 * * * *', '--timezone', 'UTC', '--conversation', ids[0], '--mode', 'inspect', '--', 'Check the fixture without sending anything.');
    const id = added.match(/Added job (\S+)/)?.[1]; assert.ok(id, added);
    const jobs = join(stateDir, 'gator/jobs'), definitionPath = join(jobs, 'definitions', id + '.json'), now = Date.now();
    const definition = JSON.parse(await readFile(definitionPath, 'utf8'));
    await writeFile(definitionPath, JSON.stringify({ ...definition, created_at: new Date(now - 7200000).toISOString() }));
    const availability = JSON.stringify({ LastSeen: new Date(now - 1200000).toISOString(), Gaps: [{ From: new Date(now - 3600000).toISOString(), To: new Date(now - 1200000).toISOString() }] });
    await writeFile(join(jobs, 'availability.json'), availability);
    const history = join(jobs, 'history', id);
    await mkdir(history, { recursive: true });
    await writeFile(join(history, 'attempt-limited.json'), JSON.stringify({ version: 2, id: 'attempt-limited', job_id: id, status: 'limited', summary: 'Daily model request limit reached', started_at: new Date(now - 1000).toISOString(), finished_at: new Date(now).toISOString() }));
    const owned = await until(async () => (await state(ids[0])).chat_checks?.find(check => check.id === id), 'Origin check health missing');
    assert.equal(owned.state, 'offline'); assert.equal(owned.issue, 'Daily model request limit reached');
    assert.equal(owned.unsuccessful, 1); assert.ok(owned.gap_from && owned.gap_to);
    assert.equal(owned.last_success, undefined); assert.equal(owned.next_due, undefined);
    assert.match(owned.next_action, /Start the local supervisor/);
    assert.deepEqual((await state(ids[1])).chat_checks || [], []);
    await writeFile(join(jobs, 'availability.json'), '{invalid');
    const stale = await until(async () => { const value = await state(ids[0]); return value.chat_checks_issue && value; }, 'Corrupt health storage was not disclosed');
    assert.match(stale.chat_checks_issue, /out of date/);
    assert.equal((await state(ids[1])).chat_checks_issue, undefined, 'Other chats must not inherit this health warning');
    await writeFile(join(jobs, 'availability.json'), availability);
    command('job', 'disable', id);
    await until(async () => { const value = await state(ids[0]); return !value.chat_checks_issue && value.chat_checks?.[0]?.state === 'paused'; }, 'Recovered health did not reflect paused check');
    await companion.close(); companion = null; await start();
    await until(async () => (await state(ids[0])).chat_checks?.[0]?.state === 'paused', 'Restart did not reload retained check health');
    assert.deepEqual((await state(ids[1])).chat_checks || [], []);
    assert.deepEqual(await readdir(history), ['attempt-limited.json']);
    assert.equal(requests, 0, 'Health reads must not execute checks or call the model');
    assert.equal((await fetch(base + `/v1/chats/${ids[0]}`)).status, 401);
  } finally {
    await companion?.close(); model.closeAllConnections(); await new Promise(resolve => model.close(resolve));
    await rm(root, { recursive: true, force: true });
  }
});
