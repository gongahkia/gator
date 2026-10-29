// Installed-prefix continuity and rollback, using only bundled services/runtime.
// Run with Bun: verify-upgrade.mjs OLD_ARTIFACT NEW_ARTIFACT [NEW_REPORT_JSON].
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile, rm, realpath, symlink, copyFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

assert.ok(process.versions.bun, 'Run with Bun.');
assert.ok(process.argv[2] && process.argv[3], 'Supply old and new production artifacts.');
const artifacts = await Promise.all(process.argv.slice(2, 4).map(path => realpath(resolve(path))));
const root = await mkdtemp(join(tmpdir(), 'gator-upgrade-'));
const prefixes = [join(root, 'old install'), join(root, 'new install')];
const installs = prefixes.map(prefix => join(prefix, 'lib/gator-terminal'));
const workspace = join(root, 'original workspace'), otherWorkspace = join(root, 'different launch directory'), userData = join(root, 'app');
const env = { ...process.env, PATH: join(root, 'path'), GATOR_STATE_DIR: join(root, 'state'), GATOR_CONFIG_DIR: join(root, 'config'), GATOR_TERMINAL_USER_DATA: userData };
const runtimeName = process.platform === 'win32' ? 'gator-runtime.exe' : 'gator-runtime';
let host, requests = 0;
const model = createServer((req, res) => { requests++; req.resume(); res.writeHead(500); res.end('No model request is allowed in this continuity fixture.'); });
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const run = args => {
  const result = spawnSync(process.execPath, args, { env, cwd: workspace, encoding: 'utf8', timeout: 60000 });
  assert.equal(result.status, 0, result.stderr || result.error?.message); return result.stdout;
};
const launch = (index, ...args) => run([join(installs[index], 'bin/gator'), ...args]);
async function open(index, source = otherWorkspace) {
  const { Application } = await import(pathToFileURL(join(installs[index], 'ui/node_modules/@gator/application-host/src/application.mjs')).href);
  host = new Application({ binary: join(installs[index], 'runtime', runtimeName), workspace: source, userData, env });
  await host.start();
}
async function close() { await host?.close(); host = null; }
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
  await open(0, workspace);
  const first = host.context;
  const original = join(root, 'original.txt'); await writeFile(original, 'Original file bytes.');
  const [upload] = await host.importFiles([original], first);
  const draft = { context: first, title: 'Original chat', source: workspace, text: 'First unsent draft 🐊', uploads: [upload] };
  await host.drafts.request({ action: 'save', draft });
  const request = { context: first, ref: { kind: 'upload', id: upload.id } };
  const initial = await host.file({ ...request, action: 'open' });
  const saved = await host.file({ ...request, action: 'save', expected: initial.current, edit: { text: 'Saved working version.' } });
  await host.file({ ...request, action: 'draft', expected: saved.current, edit: { text: 'Unfinished file edit.' } });
  await host.preferences.configure({ sidebarCollapsed: true });
  await host.sessions.open({ new: true });
  const second = host.context; assert.notEqual(second, first);
  await host.drafts.request({ action: 'save', draft: { context: second, title: 'Second chat', source: workspace, text: 'Second independent draft', uploads: [] } });
  await close();
  async function verify(index, firstText) {
    await open(index);
    assert.equal(host.context, second, 'The last selected chat must survive the installation change.');
    assert.equal(await realpath(host.snapshot().state.source), await realpath(workspace), 'Launching elsewhere must not widen the restored chat source.');
    assert.equal((await host.preferences.read()).sidebarCollapsed, true);
    const drafts = (await host.drafts.request({ action: 'list' })).drafts;
    assert.equal(drafts.find(item => item.context === first)?.text, firstText);
    assert.equal(drafts.find(item => item.context === second)?.text, 'Second independent draft');
    assert.equal(drafts.find(item => item.context === first)?.uploads[0].id, upload.id);
    assert.equal(host.snapshot().state.messages.length, 0);
    await host.sessions.open({ conversation_id: first.slice('conversation:'.length) });
    const view = await host.file({ ...request, action: 'open' });
    assert.equal(view.current, saved.current); assert.equal(view.versions.length, 2);
    assert.equal(view.draft.text, 'Unfinished file edit.');
    const destination = join(root, `export-${index}-${Date.now()}.txt`);
    await host.exportFile({ ...request, action: 'export', version: saved.revision }, destination);
    assert.equal(await readFile(destination, 'utf8'), 'Saved working version.');
    assert.equal(await readFile(original, 'utf8'), 'Original file bytes.');
    assert.match(launch(index, 'note', 'list'), /Upgrade retained note/);
    assert.equal(host.snapshot().state.messages.length, 0);
    assert.equal(requests, 0);
    await host.sessions.open({ conversation_id: second.slice('conversation:'.length) });
  }
  await verify(1, draft.text);
  await host.drafts.request({ action: 'save', draft: { ...draft, text: 'Draft updated in the new installation.' } });
  await close();
  await verify(0, 'Draft updated in the new installation.');
  await close();
  // After confirming rollback, remove only the old owned installation and
  // verify the new one can still read the external profile and every draft.
  run([join(installs[0], 'ui/scripts/uninstall.mjs'), prefixes[0]]);
  await verify(1, 'Draft updated in the new installation.');
  await close();
  assert.deepEqual(await readFile(configPath), configBefore);
  const runtimes = await Promise.all(artifacts.map(async artifact => hash(await readFile(join(artifact, 'runtime', runtimeName)))));
  const report = { platform: process.platform, arch: process.arch, versions, runtime_sha256: runtimes, same_runtime: runtimes[0] === runtimes[1], phases: ['new installation', 'rollback to old', 'new after old uninstall'], retained_chat_drafts: 2, retained_file_versions: 2, retained_edit_draft: true, unchanged_config: true, model_requests: requests, scope: 'Installed bundled-host continuity for these artifacts. No schema-changing release, service relocation, OS installer or signed update is implied.' };
  if (process.argv[4]) await writeFile(resolve(process.argv[4]), JSON.stringify(report, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
  console.log('PASS: installation switch, rollback and old-package removal retain selected chat/source, two drafts, attachment, file versions/edit draft, preferences, notes and configuration; no model requests.');
  console.log(JSON.stringify(report));
} finally {
  await close(); model.closeAllConnections(); await new Promise(resolve => model.close(resolve));
  await rm(root, { recursive: true, force: true });
}
