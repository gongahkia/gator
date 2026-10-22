import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, readdir, rm, stat, utimes, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { UploadStore, validUploadID, uploadContext } from "../electron/uploads.mjs";
import { validateFrame } from "../electron/policy.mjs";
import { DraftStore } from "../electron/drafts.mjs";

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
    await writeFile(valid, "Brief"); await writeFile(bad, "Binary"); await writeFile(big, Buffer.alloc(4 * 1024 * 1024 + 1));
    await assert.rejects(store.import([valid, bad]), /isn’t supported/);
    assert.deepEqual(await readdir(store.root), []);
    await assert.rejects(store.import([big]), /4 MiB/);
    await assert.rejects(store.import(Array(5).fill(valid)), /four files/);
    await writeFile(big, Buffer.alloc(3 * 1024 * 1024));
    await assert.rejects(store.import([big, big, big]), /8 MiB/);
    assert.deepEqual(await readdir(store.root), []);
    for (const id of ["../secret.txt", "/secret.txt", `${"a".repeat(32)}/../secret`, `${"a".repeat(32)}/..`, `${"a".repeat(32)}/x\\y.txt`]) {
      assert.equal(validUploadID(id), false); await assert.rejects(store.check([id]));
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("upload submission carries bounded opaque identities and a conversation binding", () => {
  const upload = { request_id: "send-1", context: "conversation:one", text: "Read this", files: [`${"a".repeat(32)}/plan.txt`] };
  assert.equal(validateFrame({ type: "upload_submit", upload }).version, 1);
  for (const change of [{ files: ["/private/secret.txt"] }, { files: Array(5).fill(upload.files[0]) }, { text: "x".repeat(131073) }, { request_id: "" }, { program: "sh" }]) assert.throws(() => validateFrame({ type: "upload_submit", upload: { ...upload, ...change } }));
  assert.equal(uploadContext({ source: "/project", primary: true }), "workspace:/project");
  assert.equal(uploadContext({ source: "/project", primary: false }), "workspace:topic:/project");
  assert.equal(uploadContext({ conversation_id: "one", source: "/project" }), "conversation:one");
});
