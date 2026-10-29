// Installed-prefix continuity and rollback, using only bundled services/runtime.
// Run with Bun: verify-upgrade.mjs OLD_ARTIFACT NEW_ARTIFACT [NEW_REPORT_JSON].
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, rm, realpath, symlink, copyFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

assert.ok(process.versions.bun, 'Run with Bun.');
assert.ok(process.argv[2] && process.argv[3], 'Supply old and new production artifacts.');
const artifacts = await Promise.all(process.argv.slice(2, 4).map(path => realpath(resolve(path))));
const root = await mkdtemp(join(tmpdir(), 'gator-upgrade-'));
const prefixes = [join(root, 'old install'), join(root, 'new install')];
const installs = prefixes.map(prefix => join(prefix, 'lib/gator-terminal'));
const workspace = join(root, 'original workspace'), otherWorkspace = join(root, 'different launch directory'), userData = join(root, 'app');
const env = { ...process.env, PATH: join(root, 'path'), GATOR_STATE_DIR: join(root, 'state'), GATOR_CONFIG_DIR: join(root, 'config'), GATOR_TERMINAL_USER_DATA: userData };
const runtimeName = process.platform === 'win32' ? 'gator-runtime.exe' : 'gator-runtime';
let requests = 0, primaryFailure;
const model = createServer((req, res) => { requests++; req.resume(); res.writeHead(500); res.end('No model request is allowed in this continuity fixture.'); });
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const run = args => {
  const result = spawnSync(process.execPath, args, { env, cwd: workspace, encoding: 'utf8', timeout: 60000 });
  assert.equal(result.status, 0, result.stderr || result.error?.message); return result.stdout;
};
const launch = (index, ...args) => run([join(installs[index], 'bin/gator'), ...args]);
async function runPhase(index, phase, retained = {}, firstText) {
  const fixture = { root, workspace, otherWorkspace, userData, runtimeName, install: installs[index], ...retained, phase, firstText };
  // Closing the Go worker alone does not unload JavaScript modules. Exit the
  // entire application process before removing its installation on Windows.
  const result = await new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [fileURLToPath(new URL('./verify-upgrade-phase.mjs', import.meta.url))], { env, cwd: workspace, windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
    let stdout = '', stderr = '', timedOut = false;
    const timer = setTimeout(() => { timedOut = true; child.kill('SIGKILL'); }, 60000);
    child.stdout.on('data', chunk => { stdout += chunk; });
    child.stderr.on('data', chunk => { stderr += chunk; });
    child.stdin.on('error', () => {});
    child.on('error', error => { clearTimeout(timer); reject(error); });
    child.on('close', code => {
      clearTimeout(timer);
      if (code !== 0 || timedOut) { reject(new Error(`Installed phase ${phase} failed (${timedOut ? 'timeout' : code}): ${stderr}`)); return; }
      try { resolve(JSON.parse(stdout)); } catch (error) { reject(error); }
    });
    child.stdin.end(JSON.stringify(fixture));
  });
  assert.equal(requests, 0, 'No model requests are allowed in an installed phase.');
  return result;
}
try {
  for (const directory of [workspace, otherWorkspace, env.PATH, join(env.GATOR_CONFIG_DIR, 'gator')]) await mkdir(directory, { recursive: true });
  if (process.platform === 'win32') await copyFile(process.execPath, join(env.PATH, 'bun.exe'));
  else await symlink(process.execPath, join(env.PATH, 'bun'));
  await new Promise(resolve => model.listen(0, '127.0.0.1', resolve));
  const configPath = join(env.GATOR_CONFIG_DIR, 'gator/config.json');
  await writeFile(configPath, JSON.stringify({ version: 4, defaults: { provider: 'fixture', model: 'fixture' }, custom_providers: [{ id: 'fixture', base_url: `http://127.0.0.1:${model.address().port}/chat`, models: ['fixture'], default_model: 'fixture' }] }), { mode: 0o600 });
  const configBefore = await readFile(configPath);
  for (let index = 0; index < 2; index++) run([resolve('ui/scripts/install.mjs'), artifacts[index], prefixes[index]]);
  const versions = [launch(0, '--version').trim(), launch(1, '--version').trim()];
  launch(0, 'note', 'add', '--title', 'Upgrade retained note', '--body', 'A synthetic note survives switching installations.');
  const retained = await runPhase(0, 'create');
  async function verify(index, firstText, update = false) {
    assert.equal((await runPhase(index, update ? 'verify-update' : 'verify', retained, firstText)).verified, true);
    assert.match(launch(index, 'note', 'list'), /Upgrade retained note/);
  }
  await verify(1, retained.draft.text, true);
  await verify(0, 'Draft updated in the new installation.');
  // The old application process has exited, including its JavaScript loader.
  // Now remove its installed package and verify the independent profile again.
  run([join(installs[0], 'ui/scripts/uninstall.mjs'), prefixes[0]]);
  await verify(1, 'Draft updated in the new installation.');
  assert.deepEqual(await readFile(configPath), configBefore);
  const runtimes = await Promise.all(artifacts.map(async artifact => hash(await readFile(join(artifact, 'runtime', runtimeName)))));
  const report = { platform: process.platform, arch: process.arch, versions, runtime_sha256: runtimes, same_runtime: runtimes[0] === runtimes[1], phases: ['new installation', 'rollback to old', 'new after old uninstall'], retained_chat_drafts: 2, retained_file_versions: 2, retained_edit_draft: true, unchanged_config: true, model_requests: requests, scope: 'Installed bundled-host continuity for these artifacts. No schema-changing release, service relocation, OS installer or signed update is implied.' };
  if (process.argv[4]) await writeFile(resolve(process.argv[4]), JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
  console.log('PASS: installation switch, rollback and old-package removal retain selected chat/source, two drafts, attachment, file versions/edit draft, preferences, notes and configuration; no model requests.');
  console.log(JSON.stringify(report));
} catch (error) {
  primaryFailure = error;
  console.error('Installation continuity failed before cleanup:', error);
  throw error;
} finally {
  try {
    model.closeAllConnections(); await new Promise(resolve => model.close(resolve));
    await rm(root, { recursive: true, force: true, maxRetries: 8, retryDelay: 100 });
  } catch (error) {
    // Windows can temporarily retain executable handles. Cleanup must not hide
    // the original assertion or installation failure needed to diagnose it.
    if (primaryFailure) console.error('Fixture cleanup also failed:', error);
    else throw error;
  }
}
