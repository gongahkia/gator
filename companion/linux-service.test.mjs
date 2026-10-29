import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createServer } from 'node:net';
import { mkdtemp, mkdir, writeFile, readFile, rm, cp, lstat, chmod, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { userService, prepare } from './linux-service.mjs';

const example = { node: '/usr/bin/node', entry: '/opt/gator/companion/main.mjs', runtime: '/opt/gator/runtime', workspace: '/home/gator/workspace', tokenFile: '/home/gator/private/access.token', configDir: '/home/gator/config', stateDir: '/home/gator/state', port: 7341 };
test('Linux service preserves literal arguments, explicit profile and origin without token contents', () => {
  const unit = userService({ ...example, workspace: '/work %n ${HOME} "quoted" \\ ü', origin: 'https://host.example' });
  assert.ok(unit.includes('ExecStart=:"/usr/bin/node"'));
  assert.ok(unit.includes('"/work %%n ${HOME} \\"quoted\\" \\\\ ü"'));
  assert.ok(unit.includes('Environment="GATOR_STATE_DIR=/home/gator/state"'));
  assert.ok(unit.includes('Environment="GATOR_COMPANION_ORIGIN=https://host.example"'));
  assert.ok(userService(example).includes('Environment="GATOR_COMPANION_ORIGIN="'));
  assert.match(unit, /UMask=0077/); assert.match(unit, /KillMode=mixed/);
  for (const change of [{ port: 0 }, { port: '7341' }, { port: 65536 }, { node: '/bad:path/node' }, { runtime: 'relative' }, { workspace: '/work/../elsewhere' }, { tokenFile: '/path\0bad' }, { stateDir: '/path\n[Service]' }, { origin: '' }, { origin: 'http://remote.example' }, { origin: 'https://host.example/path' }, { secret: 'not-supported' }]) assert.throws(() => userService({ ...example, ...change }));
});

test('Linux service preparation checks files and token privacy before exclusive publication', { skip: process.platform === 'win32' }, async () => {
  const root = await mkdtemp(join(tmpdir(), 'gator-systemd-plan-'));
  try {
    const tokenFile = join(root, 'private/access.token'), output = join(root, 'gator-companion.service');
    for (const name of ['private', 'workspace', 'config', 'state']) await mkdir(join(root, name), { mode: 0o700 });
    await writeFile(tokenFile, 'a'.repeat(64) + '\n', { mode: 0o600 });
    const config = { node: process.execPath, entry: resolve('companion/main.mjs'), runtime: process.execPath, workspace: join(root, 'workspace'), tokenFile, configDir: join(root, 'config'), stateDir: join(root, 'state'), port: 7341 };
    await chmod(tokenFile, 0o644);
    await assert.rejects(prepare(config, output), /private token/);
    await assert.rejects(lstat(output), { code: 'ENOENT' });
    await chmod(tokenFile, 0o600); await chmod(join(root, 'private'), 0o755);
    await assert.rejects(prepare(config, output), /private token directory/);
    await chmod(join(root, 'private'), 0o700);
    await assert.rejects(prepare({ ...config, runtime: root }, output), /runtime is not a file/);
    await assert.rejects(prepare({ ...config, stateDir: tokenFile }, output), /stateDir is not a directory/);
    const alias = join(root, 'private/alias'); await symlink(tokenFile, alias);
    await assert.rejects(prepare({ ...config, tokenFile: alias }, output));
    await prepare(config, output);
    const content = await readFile(output, 'utf8');
    assert.equal(content.includes('a'.repeat(64)), false);
    assert.equal((await lstat(output)).mode & 0o777, 0o600);
    await assert.rejects(prepare(config, output), { code: 'EEXIST' });
    assert.equal(await readFile(output, 'utf8'), content);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('native Linux user manager starts, restarts and removes an isolated companion with retained Go chats', {
  skip: process.platform !== 'linux' || process.env.GATOR_COMPANION_SYSTEMD_TEST !== '1', timeout: 90000,
}, async () => {
  assert.ok(process.env.GATOR_TEST_BINARY, 'Set GATOR_TEST_BINARY to a Linux Go runtime.');
  const root = await mkdtemp(join(tmpdir(), 'gator-systemd-'));
  const name = `gator-companion-test-${randomUUID()}.service`, unitPath = join(root, name);
  const systemctl = (...args) => execFileSync('systemctl', ['--user', ...args], { encoding: 'utf8', timeout: 10000, stdio: 'pipe' });
  let linked = false;
  try {
    systemctl('show-environment'); // An explicitly requested native test must not silently skip a missing manager.
    const workspace = join(root, 'work %n ${HOME} "quoted" \\ ü'), stateDir = join(root, 'state %n ${HOME}'), configDir = join(root, 'config %n ${HOME}');
    for (const directory of [workspace, stateDir, configDir, join(root, 'private')]) await mkdir(directory, { mode: 0o700 });
    for (const name of ['main.mjs', 'server.mjs', 'public']) await cp(new URL(`./${name}`, import.meta.url), join(root, 'companion', name), { recursive: true });
    await cp(new URL('../app-host/src', import.meta.url), join(root, 'app-host/src'), { recursive: true });
    const runtime = join(root, 'gator-runtime'); await cp(resolve(process.env.GATOR_TEST_BINARY), runtime);
    const tokenFile = join(root, 'private/access.token'), token = 'b'.repeat(64);
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    const probe = createServer(); await new Promise(resolve => probe.listen(0, '127.0.0.1', resolve));
    const port = probe.address().port; await new Promise(resolve => probe.close(resolve));
    const config = { node: process.execPath, entry: join(root, 'companion/main.mjs'), runtime, workspace, tokenFile, stateDir, configDir, port };
    await prepare(config, unitPath);
    execFileSync('systemd-analyze', ['--user', 'verify', unitPath], { timeout: 10000, stdio: 'pipe' });
    systemctl('link', '--runtime', unitPath); linked = true;
    systemctl('daemon-reload'); systemctl('start', name);
    const request = (path, options = {}) => fetch(`http://127.0.0.1:${port}${path}`, { ...options, headers: { authorization: `Bearer ${token}`, ...options.headers }, signal: AbortSignal.timeout(2000) });
    const ready = async previous => {
      const deadline = Date.now() + 25000;
      while (Date.now() < deadline) {
        const pid = Number(systemctl('show', name, '--property=MainPID', '--value').trim());
        if (pid && pid !== previous) {
          try { if ((await request('/v1/unknown')).status === 404) return pid; } catch {}
        }
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      throw new Error(`Companion startup timed out: ${systemctl('show', name, '--property=Result', '--property=ExecMainStatus')}`);
    };
    const firstPID = await ready();
    assert.equal((await fetch(`http://127.0.0.1:${port}/v1/chats`)).status, 401);
    const createdResponse = await request('/v1/chats', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ request_id: randomUUID() }) });
    assert.equal(createdResponse.status, 200);
    const created = await createdResponse.json(); assert.equal(created.source, workspace);
    process.kill(firstPID, 'SIGKILL'); assert.notEqual(await ready(firstPID), firstPID);
    const catalogResponse = await request('/v1/chats'); assert.equal(catalogResponse.status, 200);
    const catalog = await catalogResponse.json();
    assert.ok(catalog.items.some(item => item.id === created.conversation_id));
    systemctl('stop', name);
    assert.equal(systemctl('show', name, '--property=ActiveState', '--value').trim(), 'inactive');
    await assert.rejects(request('/v1/unknown'));
    const env = { ...process.env, GATOR_CONFIG_DIR: configDir, GATOR_STATE_DIR: stateDir, GATOR_COMPANION_CATALOG: '1' };
    const retained = JSON.parse(execFileSync(runtime, ['companion-catalog', '', 'active', ''], { cwd: workspace, env, encoding: 'utf8', timeout: 20000 }));
    assert.ok(retained.items.some(item => item.id === created.conversation_id));
  } finally {
    if (linked) {
      systemctl('stop', name);
      systemctl('disable', '--runtime', name);
      systemctl('daemon-reload');
      assert.equal(systemctl('show', name, '--property=LoadState', '--value').trim(), 'not-found');
    }
    await rm(root, { recursive: true, force: true });
  }
});
