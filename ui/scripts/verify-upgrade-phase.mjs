// Each installed application gets a disposable process, just like a real app
// restart. The parent must not retain imports/handles into an uninstalled tree.
import assert from 'node:assert/strict';
import { readFile, writeFile, realpath } from 'node:fs/promises';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const input = [];
for await (const chunk of process.stdin) input.push(chunk);
const fixture = JSON.parse(Buffer.concat(input).toString());
const { phase, install, runtimeName, workspace, otherWorkspace, userData, root } = fixture;
const { Application } = await import(pathToFileURL(join(install, 'ui/node_modules/@gator/application-host/src/application.mjs')).href);
const host = new Application({ binary: join(install, 'runtime', runtimeName), workspace: phase === 'create' ? workspace : otherWorkspace, userData, env: process.env });
let result;
try {
  await host.start();
  if (phase === 'create') {
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
    result = { first, second, original, upload, draft, request, saved };
  } else {
    assert.ok(phase === 'verify' || phase === 'verify-update');
    const { first, second, original, upload, draft, request, saved, firstText } = fixture;
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
    const destination = join(root, `export-${process.pid}.txt`);
    await host.exportFile({ ...request, action: 'export', version: saved.revision }, destination);
    assert.equal(await readFile(destination, 'utf8'), 'Saved working version.');
    assert.equal(await readFile(original, 'utf8'), 'Original file bytes.');
    assert.equal(host.snapshot().state.messages.length, 0);
    await host.sessions.open({ conversation_id: second.slice('conversation:'.length) });
    if (phase === 'verify-update') await host.drafts.request({ action: 'save', draft: { ...draft, text: 'Draft updated in the new installation.' } });
    result = { verified: true };
  }
} finally {
  await host.close();
}
console.log(JSON.stringify(result));
