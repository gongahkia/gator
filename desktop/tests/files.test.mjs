import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { FileStore, fileReference, readBoundedFile, validateFileRequest } from "../electron/files.mjs";
import { selectedFileSource } from "../electron/file-sources.mjs";
import { applyContent, inspectContent, parseTable } from "../electron/file-content.mjs";

test("files retain full contents, recover drafts, reject stale saves and restore without touching the original", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-file-store-"));
  try {
    const original = Buffer.from("# Original\n"), ref = { kind: "upload", id: `${"a".repeat(32)}/notes.md` }, request = { context: "conversation:one", ref };
    const source = async () => ({ name: "notes.md", bytes: original }), store = new FileStore(root);
    const first = await store.request({ ...request, action: "open" }, source);
    await store.request({ ...request, action: "draft", expected: first.revision, edit: { text: "# Kept draft" } }, source);
    assert.equal((await new FileStore(root).request({ ...request, action: "open" }, source)).draft.text, "# Kept draft");
    const edited = await store.request({ ...request, action: "save", expected: first.revision, edit: { text: "# Edited\n" } }, source);
    assert.equal(edited.text, "# Edited\n"); assert.equal(edited.versions.length, 2); assert.equal(edited.draft, undefined);
    const attached = { kind: "upload", id: `${"b".repeat(32)}/notes.md` };
    await store.link(attached, ref, Buffer.from("# Edited\n"));
    const reopenedAlias = await new FileStore(root).request({ ...request, ref: attached, action: "open" }, async () => ({ name: "notes.md", bytes: Buffer.from("# Edited\n") }));
    assert.equal(reopenedAlias.current, edited.current); assert.equal(reopenedAlias.versions.length, 2);
    assert.equal(original.toString(), "# Original\n");
    await assert.rejects(store.request({ ...request, action: "save", expected: first.revision, edit: { text: "Stale overwrite" } }, source), /newer version/);
    const restored = await store.request({ ...request, action: "restore", expected: edited.revision, version: first.revision }, source);
    assert.equal(restored.text, "# Original\n"); assert.equal(restored.versions.length, 3);
    assert.equal((await store.request({ ...request, action: "export", version: edited.revision }, source)).bytes.toString(), "# Edited\n");
    await assert.rejects(store.request({ ...request, action: "open" }, async () => ({ name: "notes.md", bytes: Buffer.from("External change") })), /original file has changed/);
    await store.flush();
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("file sources require exact current conversation membership and reject paths, links and bad digests", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-file-authority-"));
  try {
    const ref = { kind: "upload", id: `${"a".repeat(32)}/notes.md` }, files = new FileStore(join(root, "files"));
    await files.retain(ref, "notes.md", Buffer.from("Retained upload"));
    const args = { state: { messages: [] }, context: "conversation:one", drafts: { drafts: [] }, imported: new Map(), uploads: { root: join(root, "missing") }, files };
    await assert.rejects(selectedFileSource(ref, args), /not selected/);
    const source = await selectedFileSource(ref, { ...args, state: { messages: [{ input_ids: [ref.id] }] } });
    assert.equal(source.bytes.toString(), "Retained upload");
    await assert.rejects(selectedFileSource({ kind: "output", bundle: "/other", path: "secret.txt" }, args), /not available/);
    for (const value of [{ kind: "upload", id: "../../secret" }, { kind: "output", bundle: root, path: "../secret" }, { kind: "output", bundle: root, path: "secret", extra: true }]) assert.throws(() => fileReference(value));
    assert.throws(() => validateFileRequest({ action: "exec", context: "one", ref }));
    assert.throws(() => validateFileRequest({ action: "save", context: "one", ref, expected: "bad", edit: { text: "x" } }));
    await writeFile(join(root, "source.txt"), "private");
    if (process.platform !== "win32") { await symlink(join(root, "source.txt"), join(root, "linked.txt")); await assert.rejects(readBoundedFile(root, "linked.txt"), /Linked files/); }
    await assert.rejects(files.request({ action: "open", context: "one", ref: { kind: "output", bundle: root, path: "source.txt" } }, async () => ({ name: "source.txt", root, path: "source.txt", sha256: "0".repeat(64) })), /no longer matches/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("CSV edits preserve quoting and CRLF, while invalid JSON cannot replace saved content", () => {
  const original = Buffer.from('name,notes\r\nAlice,"two, items"\r\nBob,"line\nwrap"\r\n');
  const result = applyContent("contacts.csv", original, { cells: { "1:1": 'A "quoted", value' } });
  assert.deepEqual(parseTable(result.toString()), [["name", "notes"], ["Alice", 'A "quoted", value'], ["Bob", "line\nwrap"]]);
  assert.ok(result.toString().includes("\r\n"));
  assert.throws(() => applyContent("data.json", Buffer.from('{"value":1}'), { text: "{" }), /JSON syntax/);
  assert.equal(inspectContent("view.html", Buffer.from("<script>danger()</script>")).kind, "text");
  assert.throws(() => inspectContent("bad.png", Buffer.from("not an image")), /does not match/);
});

test("a stale editor can discard its changes without deleting another editor's draft", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-file-conflict-"));
  try {
    const store = new FileStore(root), ref = { kind: "upload", id: `${"d".repeat(32)}/notes.md` }, source = async () => ({ name: "notes.md", bytes: Buffer.from("Original") });
    const request = { context: "one", ref }, first = await store.request({ ...request, action: "open" }, source);
    const saved = await store.request({ ...request, action: "save", expected: first.current, edit: { text: "A newer version" } }, source);
    await store.request({ ...request, action: "draft", expected: saved.current, edit: { text: "Another editor's new draft" } }, source);
    const latest = await store.request({ ...request, action: "discard", expected: first.current }, source);
    assert.equal(latest.current, saved.current); assert.equal(latest.draft.text, "Another editor's new draft");
    await store.request({ ...request, action: "discard", expected: saved.current }, source);
    assert.equal((await store.request({ ...request, action: "open" }, source)).draft, undefined);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("output working versions keep their identity after restoring a profile into another folder", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-file-restore-"));
  try {
    const { cp } = await import("node:fs/promises");
    const originalRoot = join(root, "old/gator"), newRoot = join(root, "restored/gator");
    const ref = { kind: "output", bundle: join(originalRoot, "workspaces/run-one"), path: "report.md" };
    const store = new FileStore(join(originalRoot, "files")), source = async () => ({ name: "report.md", bytes: Buffer.from("Original report") });
    const first = await store.request({ action: "open", context: "one", ref }, source);
    await store.request({ action: "save", context: "one", ref, expected: first.current, edit: { text: "My edited report" } }, source);
    await cp(originalRoot, newRoot, { recursive: true });
    const restored = await new FileStore(join(newRoot, "files")).request({ action: "open", context: "one", ref: { ...ref, bundle: join(newRoot, "workspaces/run-one") } }, source);
    assert.equal(restored.text, "My edited report"); assert.equal(restored.versions.length, 2);
  } finally { await rm(root, { recursive: true, force: true }); }
});
