import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createServer } from 'node:net';
import { mkdtemp, mkdir, writeFile, readFile, rm, cp, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { launchAgent, prepare } from './macos-service.mjs';

const example = { node: '/usr/local/bin/node', entry: '/private/companion/main.mjs', runtime: '/private/runtime', workspace: '/private/workspace', tokenFile: '/private/access.token', configDir: '/private/config', stateDir: '/private/state', logFile: '/private/log/host.log', port: 7341 };
test('service plan binds exact argv/profile without embedding credentials', () => {
  const xml = launchAgent({ ...example, workspace: '/private/work & review <one>', origin: 'https://host.example' });
  assert.ok(xml.includes('<string>/private/work &amp; review &lt;one&gt;</string>'));
  assert.ok(xml.includes('<key>GATOR_CONFIG_DIR</key><string>/private/config</string>'));
  assert.ok(xml.includes('<key>Umask</key><integer>63</integer>'));
  assert.equal(xml.includes('<key>GATOR_GOOGLE_DESKTOP_CLIENT_SECRET</key>'), false);
  for (const change of [{ port: 0 }, { port: 65536 }, { runtime: 'relative' }, { tokenFile: '/path\0bad' }, { label: 'com.apple.other' }, { origin: 'http://remote.example' }, { origin: 'https://host.example/path' }, { secret: 'not-supported' }]) assert.throws(() => launchAgent({ ...example, ...change }));
});

test('native launchd starts, restarts and removes an isolated companion listener', {
  skip: process.platform !== 'darwin' || process.env.GATOR_COMPANION_LAUNCHD_TEST !== '1', timeout: 60000,
}, async () => {
  const root = await realpath(await mkdtemp(join(tmpdir(), 'gator-launchd-')));
  const label = `com.gator.companion.test-${randomUUID()}`, domain = `gui/${process.getuid()}`, target = `${domain}/${label}`;
  const plist = join(root, 'service.plist'), tokenFile = join(root, 'access.token'), token = 'a'.repeat(64);
  let loaded = false;
  try {
    for (const name of ['workspace', 'config', 'state', 'logs', 'companion', 'app-host']) await mkdir(join(root, name), { mode: 0o700 });
    // Launchd should not need Desktop access or the development checkout.
    for (const name of ['main.mjs', 'server.mjs', 'public']) await cp(new URL(`./${name}`, import.meta.url), join(root, 'companion', name), { recursive: true });
    await cp(new URL('../app-host/src', import.meta.url), join(root, 'app-host/src'), { recursive: true });
    await writeFile(tokenFile, token + '\n', { mode: 0o600 });
    const portProbe = createServer();
    await new Promise(resolve => portProbe.listen(0, '127.0.0.1', resolve));
    const port = portProbe.address().port;
    await new Promise(resolve => portProbe.close(resolve));
    // This lifecycle probe makes no conversation requests and never invokes the
    // runtime argument. Real-Go conversation durability has a separate test.
    const config = { node: process.execPath, entry: join(root, 'companion/main.mjs'), runtime: process.execPath,
      workspace: join(root, 'workspace'), tokenFile, configDir: join(root, 'config'), stateDir: join(root, 'state'),
      logFile: join(root, 'logs/service.log'), port, label };
    await prepare(config, plist);
    await assert.rejects(prepare(config, plist), { code: 'EEXIST' });
    execFileSync('/usr/bin/plutil', ['-lint', plist]);
    execFileSync('/bin/launchctl', ['bootstrap', domain, plist]); loaded = true;
    const status = () => execFileSync('/bin/launchctl', ['print', target], { encoding: 'utf8' });
    const ready = async oldPID => {
      const deadline = Date.now() + 25000;
      while (Date.now() < deadline) {
        const pid = Number(status().match(/\bpid = (\d+)/)?.[1]);
        if (pid && pid !== oldPID) {
          try {
            const response = await fetch(`http://127.0.0.1:${port}/v1/unknown`, { headers: { authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(1000) });
            if (response.status === 404) return pid;
          } catch {}
        }
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      throw new Error(`Companion did not become ready: ${await readFile(config.logFile, 'utf8').catch(() => 'no service log')}`);
    };
    const firstPID = await ready();
    process.kill(firstPID, 'SIGKILL');
    assert.notEqual(await ready(firstPID), firstPID);
    const unauthenticated = await fetch(`http://127.0.0.1:${port}/v1/unknown`);
    assert.equal(unauthenticated.status, 401);
    execFileSync('/bin/launchctl', ['bootout', target]);
    // bootout acknowledges removal before the process has necessarily drained.
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
      try { execFileSync('/bin/launchctl', ['print', target], { stdio: 'pipe' }); }
      catch { loaded = false; break; }
      await new Promise(resolve => setTimeout(resolve, 50));
    }
    assert.equal(loaded, false, 'launchd did not finish removing the service');
  } finally {
    if (loaded) execFileSync('/bin/launchctl', ['bootout', target], { stdio: 'pipe' });
    await rm(root, { recursive: true, force: true });
  }
});
