import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, stat, utimes, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { UploadStore, validUploadID, uploadContext, uploadLimits } from "../src/uploads.mjs";
import { validateFrame } from "../src/policy.mjs";
import { DraftStore } from "../src/drafts.mjs";
import { retainedRequestUploads } from "../src/request-retention.mjs";

test("selected files become private independent copies and attachment-only drafts survive restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-uploads-"));
  try {
    const source = join(root, "plan.txt"), store = new UploadStore(join(root, "private"));
    await writeFile(source, "Selected original");
    const [first] = await store.import([source]);
    await writeFile(source, "Changed after selection");
    assert.equal(await readFile(join(store.root, first.id), "utf8"), "Selected original");
    assert.equal(first.name, "plan.txt");
    assert.notEqual((await store.import([source]))[0].id, first.id);
    if (process.platform !== "win32") assert.equal((await stat(join(store.root, first.id))).mode & 0o777, 0o600);
    const drafts = new DraftStore(join(root, "drafts.json"));
    await drafts.request({ action: "save", draft: { context: "conversation:one", text: "", title: "One", source: root, uploads: [first] } });
    assert.deepEqual((await new DraftStore(join(root, "drafts.json")).request({ action: "list" })).drafts[0].uploads, [first]);
    await store.check([first.id]);
    await utimes(join(store.root, first.id.split("/")[0]), 0, 0);
    await store.prune();
    await assert.rejects(store.check([first.id]));
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("imports reject unsupported, oversized and excessive inputs without leaving partial selections", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-upload-limits-"));
  try {
    const store = new UploadStore(join(root, "private")); await store.prune();
    const valid = join(root, "brief.md"), bad = join(root, "binary.exe"), big = join(root, "big.txt");
    await writeFile(valid, "Brief"); await writeFile(bad, "Binary"); await writeFile(big, Buffer.alloc(uploadLimits.file + 1));
    await assert.rejects(store.import([valid, bad]), /isn’t supported/);
    assert.deepEqual(await readdir(store.root), []);
    await assert.rejects(store.import([big]), /8 MiB/);
    await assert.rejects(store.import(Array(uploadLimits.count + 1).fill(valid)), /12 files/);
    await writeFile(big, Buffer.alloc(6 * 1024 * 1024));
    await assert.rejects(store.import([big, big, big]), /16 MiB/);
    assert.deepEqual(await readdir(store.root), []);
    for (const id of ["../secret.txt", "/secret.txt", `${"a".repeat(32)}/../secret`, `${"a".repeat(32)}/..`, `${"a".repeat(32)}/x\\y.txt`]) {
      assert.equal(validUploadID(id), false); await assert.rejects(store.check([id]));
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("upload submission carries bounded opaque identities and a conversation binding", () => {
  const upload = { request_id: "send-1", context: "conversation:one", text: "Read this", files: [`${"a".repeat(32)}/plan.txt`] };
  assert.equal(validateFrame({ type: "upload_submit", upload }).version, 1);
  assert.equal(validateFrame({ type: "upload_submit", upload: { ...upload, files: [] } }).version, 1);
  assert.throws(() => validateFrame({ type: "upload_submit", upload: { ...upload, files: [], text: " " } }));
  for (const change of [{ files: ["/private/secret.txt"] }, { files: Array(uploadLimits.count + 1).fill(upload.files[0]) }, { text: "x".repeat(131073) }, { request_id: "" }, { program: "sh" }]) assert.throws(() => validateFrame({ type: "upload_submit", upload: { ...upload, ...change } }));
  assert.equal(uploadContext({ source: "/project", primary: true }), "workspace:/project");
  assert.equal(uploadContext({ source: "/project", primary: false }), "workspace:topic:/project");
  assert.equal(uploadContext({ conversation_id: "one", source: "/project" }), "conversation:one");
});

test("cleanup keeps files belonging to admitted messages and defers unreadable journals", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-retained-uploads-"));
  try {
    const uploads = new UploadStore(join(root, "uploads")), chats = join(root, "conversations");
    const pending = await uploads.importContent("pending.txt", Buffer.from("Keep while queued"));
    const unused = await uploads.importContent("unused.txt", Buffer.from("Expired unselected file"));
    for (const file of [pending, unused]) await utimes(join(uploads.root, file.id.split("/")[0]), 0, 0);
    await mkdir(join(chats, "one"), { recursive: true });
    const path = join(chats, "one", "requests.json");
    await writeFile(path, JSON.stringify({ version: 1, requests: [{ state: "interrupted", options: { Uploads: [pending.id] } }] }));
    await uploads.prune(Date.now(), await retainedRequestUploads(chats));
    await uploads.check([pending.id]);
    await assert.rejects(uploads.check([unused.id]));
    await writeFile(path, "broken");
    await assert.rejects(retainedRequestUploads(chats));
    await uploads.check([pending.id]);
  } finally { await rm(root, { recursive: true, force: true }); }
});


test("larger document intake and all attachment guards share the Go runtime limits", async () => {
  const go = await readFile(new URL("../../internal/attachment/attachment.go", import.meta.url), "utf8");
  for (const [name, limit] of [["MaxInputs", uploadLimits.count], ["MaxImageBytes", uploadLimits.image], ["MaxDocumentBytes", uploadLimits.file], ["MaxTotalBytes", uploadLimits.total]]) {
    const expression = go.match(new RegExp(`${name}\\s*=\\s*(\\d+)( \\* 1024 \\* 1024)?`));
    assert.ok(expression); assert.equal(Number(expression[1]) * (expression[2] ? 1048576 : 1), limit);
  }
  const root = await mkdtemp(join(tmpdir(), "gator-larger-intake-"));
  try {
    const store = new UploadStore(join(root, "uploads")), large = join(root, "large.txt");
    await writeFile(large, Buffer.alloc(5 * 1024 * 1024, 65));
    const [file] = await store.import([large]); assert.equal(file.size, 5 * 1024 * 1024);
    const drafts = new DraftStore(join(root, "drafts.json"));
    await drafts.request({ action: "save", draft: { context: "conversation:large", title: "Large", source: root, text: "", uploads: [file] } });
    assert.deepEqual((await new DraftStore(join(root, "drafts.json")).request({ action: "list" })).drafts[0].uploads, [file]);
    const image = join(root, "too-large.png"); await writeFile(image, Buffer.alloc(uploadLimits.image + 1));
    await assert.rejects(store.import([image]), /4 MiB/);
    const paths = await Promise.all(Array.from({ length: uploadLimits.count }, async (_, i) => { const path = join(root, `part-${i}.txt`); await writeFile(path, `Part ${i}`); return path; }));
    const files = await store.import(paths); assert.equal(files.length, 12); await store.check(files.map(file => file.id));
    const submission = { type: "upload_submit", upload: { request_id: "many", context: "conversation:large", text: "Read these", files: files.map(file => file.id) } };
    assert.equal(validateFrame(submission).version, 1);
    assert.throws(() => validateFrame({ ...submission, upload: { ...submission.upload, files: [...submission.upload.files, file.id] } }), /Invalid/);
  } finally { await rm(root, { recursive: true, force: true }); }
});
