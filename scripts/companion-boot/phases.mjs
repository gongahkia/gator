// Invoked only inside the disposable image by run.py. Never run on a real host.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomBytes, randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';
import { prepare } from '../../companion/linux-service.mjs';
import { addDevice } from '../../companion/auth.mjs';

assert.equal(process.platform, 'linux');
assert.equal(process.getuid(), 1000);
assert.equal(await readFile('/gator-boot-fixture', 'utf8'), 'gator disposable boot fixture\n');
const root = '/home/node/fixture', runtime = '/validation/gator-runtime';
const stateDir = join(root, 'state'), configDir = join(root, 'config'), workspace = join(root, 'workspace');
const tokenFile = join(root, 'private/owner.token'), deviceFile = join(root, 'private/device.token');
const units = '/home/node/.config/systemd/user', listener = 'gator-companion.service', supervisor = 'gator-work-supervisor.service';
const env = { ...process.env, GATOR_STATE_DIR: stateDir, GATOR_CONFIG_DIR: configDir, GATOR_INTERNAL_COMMANDS: '1', XDG_CONFIG_HOME: '/home/node/.config' };
const command = (...args) => execFileSync(runtime, args, { env, cwd: workspace, encoding: 'utf8', stdio: 'pipe', timeout: 20000 });
const systemctl = (...args) => execFileSync('systemctl', ['--user', ...args], { encoding: 'utf8', stdio: 'pipe', timeout: 20000 });
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(read, message) {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    const result = await read(); if (result) return result;
    await pause(100);
  }
  throw new Error(message);
}
async function request(path, options = {}) {
  const token = (await readFile(deviceFile, 'utf8')).trim();
  return fetch('http://127.0.0.1:7341' + path, { ...options, headers: { authorization: `Bearer ${token}`, ...options.headers }, signal: AbortSignal.timeout(3000) });
}
async function ready() {
  await until(async () => {
    try { return (await request('/v1/chats')).status === 200; }
    catch { return false; }
  }, 'Authenticated companion did not start automatically.');
}
async function attempts(id) {
  const directory = join(stateDir, 'gator/jobs/history', id);
  const names = await readdir(directory).catch(error => { if (error.code === 'ENOENT') return []; throw error; });
  const results = [];
  for (const name of names) {
    try { results.push(JSON.parse(await readFile(join(directory, name, 'result.json'), 'utf8'))); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  return results;
}
const invocation = name => systemctl('show', name, '--property=InvocationID', '--value').trim();
const requests = async () => (await readFile(join(root, 'requests.jsonl'), 'utf8')).trim().split('\n').filter(Boolean).length;
const save = async (name, value) => writeFile(join(root, name), JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });

switch (process.argv[2]) {
  case 'setup': {
    for (const path of [root, workspace, stateDir, join(configDir, 'gator'), join(root, 'private'), units]) await mkdir(path, { recursive: true, mode: 0o700 });
    await writeFile(tokenFile, randomBytes(32).toString('hex') + '\n', { flag: 'wx', mode: 0o600 });
    await addDevice(tokenFile, 'Boot fixture device', deviceFile);
    await writeFile(join(root, 'requests.jsonl'), '', { flag: 'wx', mode: 0o600 });
    await writeFile(join(configDir, 'gator/config.json'), JSON.stringify({ version: 4, defaults: { provider: 'fixture', model: 'fixture' }, custom_providers: [{ id: 'fixture', base_url: 'http://127.0.0.1:7342/chat', models: ['fixture'], default_model: 'fixture' }] }), { mode: 0o600 });
    await prepare({ node: process.execPath, entry: '/validation/companion/main.mjs', runtime, workspace, tokenFile, stateDir, configDir, port: 7341 }, join(units, listener));
    // The local provider is ready before the supervisor, avoiding a fixture-only
    // startup race. It has no external network or paid credentials.
    await writeFile(join(units, 'gator-fixture-provider.service'), `[Unit]\nBefore=${supervisor}\n[Service]\nType=notify\nNotifyAccess=all\nExecStart=${process.execPath} /validation/scripts/companion-boot/provider.mjs\nUMask=0077\n[Install]\nWantedBy=default.target\n`, { mode: 0o600, flag: 'wx' });
    systemctl('daemon-reload');
    systemctl('enable', '--now', listener, 'gator-fixture-provider.service');
    await ready();
    // Exercise the actual supported installer, then stop before adding work.
    // Both enabled units must start themselves on the next container boot.
    assert.match(command('job', 'systemd', 'install'), /Installed local user service/);
    systemctl('stop', supervisor);
    const response = await request('/v1/chats', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ request_id: randomUUID() }) });
    assert.equal(response.status, 200);
    const origin = await response.json();
    const added = command('job', 'add', 'Unattended boot fixture', '--schedule', '* * * * *', '--timezone', 'UTC', '--conversation', origin.conversation_id, '--mode', 'inspect', '--provider', 'fixture', '--model', 'fixture', '--max-steps', '1', '--max-model-requests', '4', '--timeout-seconds', '30', '--', 'Complete the unattended boot fixture.');
    const jobID = added.match(/Added job (\S+)/)?.[1]; assert.ok(jobID, added);
    assert.equal((await attempts(jobID)).length, 0);
    assert.equal(await requests(), 0);
    for (const unit of [listener, supervisor]) assert.equal(systemctl('is-enabled', unit).trim(), 'enabled');
    await save('setup.json', { jobID, conversationID: origin.conversation_id, listenerInvocation: invocation(listener) });
    console.log('Prepared enabled listener/supervisor, enrolled device and unexecuted scheduled check.');
    break;
  }
  case 'first':
  case 'second': {
    const setup = JSON.parse(await readFile(join(root, 'setup.json'), 'utf8'));
    await ready();
    await until(() => { try { return /Job supervisor running/.test(command('job', 'status')); } catch { return false; } }, 'Supervisor did not start automatically.');
    for (const unit of [listener, supervisor]) assert.equal(systemctl('is-active', unit).trim(), 'active');
    assert.notEqual(invocation(listener), setup.listenerInvocation);
    const completed = await until(async () => (await attempts(setup.jobID))[0], 'Scheduled check did not complete after boot.');
    assert.equal(completed.status, 'completed', JSON.stringify(completed));
    // Future scheduled slots are unrelated work; disable before the second boot.
    if (process.argv[2] === 'first') command('job', 'disable', setup.jobID);
    const update = await until(async () => {
      const response = await request(`/v1/chats/${setup.conversationID}`);
      assert.equal(response.status, 200);
      return (await response.json()).state.chat_updates?.find(item => item.job_id === setup.jobID);
    }, 'Scheduled result did not appear in the originating chat.');
    assert.equal(update.status, 'completed');
    assert.equal(update.summary, 'Scheduled fixture completed after unattended boot.');
    assert.equal(update.read, false); assert.equal(update.resolved, false);
    assert.equal((await attempts(setup.jobID)).length, 1);
    assert.equal(await requests(), 1);
    assert.equal((await fetch('http://127.0.0.1:7341/v1/chats')).status, 401);
    if (process.argv[2] === 'second') {
      const first = JSON.parse(await readFile(join(root, 'first.json'), 'utf8'));
      assert.equal(update.id, first.updateID);
      assert.notEqual(invocation(listener), first.listenerInvocation);
      assert.notEqual(invocation(supervisor), first.supervisorInvocation);
      // Readiness precedes the supervisor's initial due-work scan. Observe past
      // its 15-second polling interval rather than immediately claiming no replay.
      const deadline = Date.now() + 17000;
      while (Date.now() < deadline) {
        assert.equal(await requests(), 1);
        assert.equal((await attempts(setup.jobID)).length, 1);
        await pause(200);
      }
      assert.match(command('job', 'status'), /Job supervisor running/);
      assert.equal((await request(`/v1/chats/${setup.conversationID}`)).status, 200);
    }
    await save(process.argv[2] + '.json', { phase: process.argv[2], updateID: update.id, attempts: 1, modelRequests: 1, listenerInvocation: invocation(listener), supervisorInvocation: invocation(supervisor), authenticatedDevice: true, unauthenticatedStatus: 401, nonReplayObservationMs: process.argv[2] === 'second' ? 17000 : 0 });
    console.log(`${process.argv[2]} boot passed: both services automatic, device authenticated, one retained attempt/update/model request.`);
    break;
  }
  default: throw new Error('Use setup, first or second.');
}
