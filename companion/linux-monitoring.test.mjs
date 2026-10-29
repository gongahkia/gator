import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdtemp, mkdir, cp, readFile, writeFile, readdir, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { prepare } from './linux-service.mjs';

const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(read, message, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const value = await read();
    if (value) return value;
    await pause(100);
  }
  throw new Error(message);
}

test('Linux supervisor finishes a scheduled check without the companion and exposes its retained update on reconnect', {
  skip: process.platform !== 'linux' || process.env.GATOR_COMPANION_SYSTEMD_TEST !== '1', timeout: 120000,
}, async () => {
  assert.ok(process.env.GATOR_TEST_BINARY, 'Set GATOR_TEST_BINARY to a Linux Go runtime.');
  const root = await mkdtemp(join(tmpdir(), 'gator-monitoring-')), suffix = randomUUID();
  const listenerName = `gator-monitor-listener-${suffix}.service`, supervisorName = `gator-monitor-supervisor-${suffix}.service`;
  const systemctl = (...args) => execFileSync('systemctl', ['--user', ...args], { encoding: 'utf8', timeout: 20000, stdio: 'pipe' });
  const workspace = join(root, 'workspace'), stateDir = join(root, 'state'), configDir = join(root, 'config');
  const runtime = join(root, 'gator-runtime'), tokenFile = join(root, 'private/access.token'), token = 'c'.repeat(64);
  // The terminal launcher sets this flag for its retained command families.
  const unitPath = join(root, listenerName), env = { ...process.env, GATOR_STATE_DIR: stateDir, GATOR_CONFIG_DIR: configDir, GATOR_INTERNAL_COMMANDS: '1' };
  const command = (...args) => execFileSync(runtime, args, { env, cwd: workspace, encoding: 'utf8', timeout: 20000, stdio: 'pipe' });
  let linked = false, supervisorStarted = false, requests = 0, providerError;
  let releaseModel;
  const gate = new Promise(resolve => releaseModel = resolve);
  const marker = 'Scheduled fixture completed while the companion was offline.';
  const provider = createServer(async (req, res) => {
    try {
      let text = ''; for await (const chunk of req) text += chunk;
      const payload = JSON.parse(text);
      assert.ok(payload.messages.some(message => typeof message.content === 'string' && message.content.includes('offline monitoring fixture')));
      requests++;
      await gate;
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: marker }, finish_reason: 'stop' }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
    } catch (error) { providerError = error; res.writeHead(500); res.end('Fixture failed.'); }
  });
  try {
    systemctl('show-environment');
    for (const directory of [workspace, stateDir, join(configDir, 'gator'), join(root, 'private')]) await mkdir(directory, { recursive: true, mode: 0o700 });
    await cp(resolve(process.env.GATOR_TEST_BINARY), runtime);
    for (const name of ['main.mjs', 'server.mjs', 'auth.mjs', 'public']) await cp(new URL(`./${name}`, import.meta.url), join(root, 'companion', name), { recursive: true });
    await cp(new URL('../app-host/src', import.meta.url), join(root, 'app-host/src'), { recursive: true });
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    await new Promise(resolve => provider.listen(0, '127.0.0.1', resolve));
    await writeFile(join(configDir, 'gator/config.json'), JSON.stringify({ version: 4, defaults: { provider: 'fixture', model: 'fixture' }, custom_providers: [{ id: 'fixture', base_url: `http://127.0.0.1:${provider.address().port}/chat`, models: ['fixture'], default_model: 'fixture' }] }), { mode: 0o600 });
    const probe = createServer(); await new Promise(resolve => probe.listen(0, '127.0.0.1', resolve));
    const port = probe.address().port; await new Promise(resolve => probe.close(resolve));
    await prepare({ node: process.execPath, entry: join(root, 'companion/main.mjs'), runtime, workspace, tokenFile, stateDir, configDir, port }, unitPath);
    systemctl('link', '--runtime', unitPath); linked = true;
    systemctl('daemon-reload'); systemctl('start', listenerName);
    const request = (path, options = {}) => fetch(`http://127.0.0.1:${port}${path}`, { ...options, headers: { authorization: `Bearer ${token}`, ...options.headers }, signal: AbortSignal.timeout(2000) });
    const ready = () => until(async () => { try { return (await request('/v1/unknown')).status === 404; } catch { return false; } }, 'Companion never became ready.');
    await ready();
    const initial = await request('/v1/chats'); assert.equal(initial.status, 200);
    assert.deepEqual((await initial.json()).items, []);
    const created = await request('/v1/chats', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ request_id: randomUUID() }) });
    assert.equal(created.status, 200);
    const origin = await created.json(); assert.ok(origin.conversation_id);
    const added = command('job', 'add', 'Offline monitoring fixture', '--schedule', '* * * * *', '--timezone', 'UTC', '--conversation', origin.conversation_id, '--mode', 'inspect', '--provider', 'fixture', '--model', 'fixture', '--max-steps', '1', '--max-model-requests', '4', '--timeout-seconds', '30', '--', 'Complete the offline monitoring fixture.');
    const jobID = added.match(/Added job (\S+)/)?.[1]; assert.ok(jobID, added);
    // A transient user service invokes the real supervisor command with the same
    // profile. It never installs or replaces the user's persistent supervisor.
    execFileSync('systemd-run', ['--user', `--unit=${supervisorName}`, '--service-type=exec', '--property=Restart=on-failure', '--property=RestartSec=5', '--property=UMask=0077', '--property=TimeoutStopSec=15', '--property=KillMode=mixed', `--setenv=GATOR_STATE_DIR=${stateDir}`, `--setenv=GATOR_CONFIG_DIR=${configDir}`, runtime, 'job', 'supervisor', '--notify=false'], { encoding: 'utf8', timeout: 20000, stdio: 'pipe' });
    supervisorStarted = true;
    await until(() => { if (providerError) throw providerError; return requests > 0; }, 'The supervisor did not dispatch its scheduled check.');
    assert.match(command('job', 'status'), /Job supervisor running/);
    // Pause future runs through the CLI; the admitted attempt remains live.
    command('job', 'disable', jobID);
    systemctl('stop', listenerName);
    await assert.rejects(request('/v1/chats'));
    assert.equal(systemctl('show', supervisorName, '--property=ActiveState', '--value').trim(), 'active');
    releaseModel();
    const history = join(stateDir, 'gator/jobs/history', jobID);
    const attempts = async () => {
      const names = await readdir(history).catch(error => { if (error.code === 'ENOENT') return []; throw error; });
      const results = [];
      for (const name of names) {
        try { results.push(JSON.parse(await readFile(join(history, name, 'result.json'), 'utf8'))); }
        catch (error) { if (error.code !== 'ENOENT') throw error; }
      }
      return results;
    };
    const completed = await until(async () => (await attempts())[0], 'The offline check did not retain its result.');
    assert.equal(completed.status, 'completed', JSON.stringify(completed));
    assert.ok(completed.conversation_id); assert.match(completed.summary, /Scheduled fixture completed/);
    assert.equal((await attempts()).length, 1);
    assert.equal(systemctl('show', listenerName, '--property=ActiveState', '--value').trim(), 'inactive');
    const beforeReconnect = requests;
    systemctl('start', listenerName); await ready();
    const catalog = await (await request('/v1/chats')).json();
    assert.ok(catalog.items.some(item => item.id === completed.conversation_id));
    const readUpdate = async () => {
      const response = await request(`/v1/chats/${origin.conversation_id}`);
      assert.equal(response.status, 200);
      const payload = await response.json();
      return payload.state.chat_updates?.find(update => update.job_id === jobID);
    };
    const update = await until(readUpdate, 'The completed check was not visible through the companion.');
    assert.equal(update.status, 'completed');
    assert.equal(update.summary, marker);
    assert.equal(update.read, false); assert.equal(update.resolved, false);
    const executionChat = await (await request(`/v1/chats/${completed.conversation_id}`)).json();
    assert.equal(executionChat.state.chat_updates?.some(item => item.id === update.id) || false, false, 'The execution chat must not inherit the originating chat’s update authority.');
    assert.equal(requests, beforeReconnect, 'Reconnecting must not execute the check again.');
    systemctl('restart', supervisorName);
    await until(() => { try { return /Job supervisor running/.test(command('job', 'status')); } catch { return false; } }, 'Supervisor did not restart.');
    assert.equal((await attempts()).length, 1);
    assert.equal(requests, beforeReconnect);
    assert.equal((await readUpdate()).id, update.id);
  } finally {
    releaseModel();
    try {
      if (supervisorStarted) systemctl('stop', supervisorName);
      if (linked) {
        systemctl('stop', listenerName); systemctl('disable', '--runtime', listenerName); systemctl('daemon-reload');
        assert.equal(systemctl('show', listenerName, '--property=LoadState', '--value').trim(), 'not-found');
      }
    } finally {
      provider.closeAllConnections(); await new Promise(resolve => provider.close(resolve));
      await rm(root, { recursive: true, force: true });
    }
  }
});
