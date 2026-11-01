import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { sourcePreview } from "../electron/source-preview.mjs";
const hash = text => createHash("sha256").update(text).digest("hex");
async function fixture(run) {
  const root = await mkdtemp(join(tmpdir(), "gator-source-preview-")), path = join(root, "bundle"), text = "Title\nOwner: Mina\nDeadline: Friday\nBudget: unapproved\n";
  const digest = hash(text), file = `evidence/${digest}.txt`, manifest = JSON.stringify({ run_id: "run-one", evidence: [{ locator: "source/project.md", sha256: digest, snapshot_path: file }] });
  const revision = { conversation_id: "chat-one", bundle_path: path, replay: { messages: [{ role: "user", content: "Read the project" }, { role: "tool", tool_name: "read_file", content: JSON.stringify({ ok: true, result: { path: "source/project.md", start_line: 2, end_line: 3, content: "Owner: Mina\nDeadline: Friday", truncated: false } }) }] } };
  const revisions = join(root, "gator/conversations/chat-one/revisions"), history = join(root, "gator/work-history");
  await mkdir(join(path, "evidence"), { recursive: true }); await mkdir(revisions, { recursive: true }); await mkdir(history, { recursive: true });
  await writeFile(join(path, file), text); await writeFile(join(path, "manifest.json"), manifest);
  await writeFile(join(history, "run-one.json"), JSON.stringify({ id: "run-one", conversation_id: "chat-one", revision_id: "rev-one" }));
  await writeFile(join(revisions, "rev-one.json"), JSON.stringify(revision));
  const state = { conversation_id: "chat-one", messages: [{ bundle: { Path: path, RunID: "run-one", ManifestSHA256: hash(manifest) } }] };
  const request = { context: "conversation:chat-one", run: "run-one", path: "source/project.md", start: 2, end: 3 };
  try { await run({ root, path, file, text, revision, revisions, state, request, read: value => sourcePreview(value ?? request, { stateRoot: root, state }) }); } finally { await rm(root, { recursive: true, force: true }); }
}
test("source preview shows verified retained bytes, cited lines and independently recorded read ranges", () => fixture(async f => {
  const result = await f.read(); assert.equal(result.text, f.text); assert.equal(result.sha256, hash(f.text)); assert.deepEqual(result.cited, { start: 2, end: 3 }); assert.deepEqual(result.reads, [{ start: 2, end: 3, truncated: false }]); assert.equal(result.notice, undefined);
}));
test("source references cannot open another run, conversation, unselected source or arbitrary path", () => fixture(async f => {
  for (const change of [{ context: "conversation:other" }, { run: "run-other" }, { path: "source/secret.md" }, { path: "source/../secret" }, { path: "/etc/passwd" }, { path: "source/\\secret" }, { start: 0 }, { end: 999 }, { executable: "/bin/sh" }]) await assert.rejects(f.read({ ...f.request, ...change }));
  await assert.rejects(sourcePreview(f.request, { stateRoot: f.root, state: f.state, current: () => false }));
}));
test("source tampering fails closed; no fallback to the current workspace", () => fixture(async f => {
  await writeFile(join(f.path, f.file), "Changed private data"); await assert.rejects(f.read(), /changed/);
}));
test("linked source bytes cannot bypass the retained-evidence boundary", { skip: process.platform === "win32" }, () => fixture(async f => {
  // Windows file symlink creation requires additional OS privileges; do not
  // require elevation or weaken the ordinary cross-platform tamper check.
  await rm(join(f.path, f.file)); await writeFile(join(f.root, "elsewhere.txt"), f.text); await symlink(join(f.root, "elsewhere.txt"), join(f.path, f.file)); await assert.rejects(f.read(), /Linked/);
}));
test("unverified or missing replay is explicit and never invented from a model's cited range", () => fixture(async f => {
  f.revision.replay.messages[1].content = JSON.stringify({ ok: true, result: { path: "source/project.md", start_line: 1, end_line: 3, content: "Not the selected file" } });
  await writeFile(join(f.revisions, "rev-one.json"), JSON.stringify(f.revision));
  const result = await f.read(); assert.deepEqual(result.reads, []); assert.match(result.notice, /exact read range is unavailable/);
}));
test("source authority is checked again after reading", () => fixture(async f => {
  let checks = 0; await assert.rejects(sourcePreview(f.request, { stateRoot: f.root, state: f.state, current: () => ++checks === 1 }), /conversation changed/); assert.equal(checks, 2);
}));
test("bounded tool reads retain the exact partial final line instead of claiming it was fully read", () => fixture(async f => {
  f.revision.replay.messages[1].content = JSON.stringify({ ok: true, result: { path: "source/project.md", start_line: 2, end_line: 3, content: "Owner: Mina\nDead", truncated: true } });
  await writeFile(join(f.revisions, "rev-one.json"), JSON.stringify(f.revision));
  assert.deepEqual((await f.read()).reads, [{ start: 2, end: 3, truncated: true, endColumn: 4 }]);
}));
test("large retained text is bounded without splitting UTF-8", () => fixture(async f => {
  const text = "λ🦦".repeat(12000), digest = hash(text), file = `evidence/${digest}.txt`;
  const manifest = JSON.stringify({ run_id: "run-one", evidence: [{ locator: "source/project.md", sha256: digest, snapshot_path: file }] });
  await writeFile(join(f.path, file), text); await writeFile(join(f.path, "manifest.json"), manifest); f.state.messages[0].bundle.ManifestSHA256 = hash(manifest);
  const result = await f.read({ ...f.request, start: 1, end: 1 }); assert.ok(Buffer.byteLength(result.text) <= 65536); assert.ok(!result.text.includes("�")); assert.equal(result.truncated, true);
}));
