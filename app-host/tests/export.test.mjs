import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, writeFile, rm, symlink, link, mkdir, realpath } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ExportStore } from "../src/export.mjs";

test("export replaces only an explicitly reviewed unchanged destination and consumes its grant", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-export-"));
  try {
    const store = new ExportStore(), path = join(root, "copy.txt");
    assert.deepEqual(await store.review(path), { path: join(await realpath(root), "copy.txt"), exists: false });
    await store.write(path, Buffer.from("Existing long content"));
    await assert.rejects(store.write(path, Buffer.from("Unreviewed")), /EEXIST/);
    const review = await store.review(path);
    assert.equal(review.exists, true); assert.equal(review.bytes, 21);
    await store.write(path, Buffer.from("Exact"), review.token);
    assert.equal(await readFile(path, "utf8"), "Exact");
    await assert.rejects(store.write(path, Buffer.from("Replay"), review.token), /Review/);
    const stale = await store.review(path);
    await writeFile(path, "Concurrent edit");
    await assert.rejects(store.write(path, Buffer.from("Stale"), stale.token), /changed after review/);
    assert.equal(await readFile(path, "utf8"), "Concurrent edit");
    const other = join(root, "other.txt"); await writeFile(other, "Other");
    const wrong = await store.review(path);
    await assert.rejects(store.write(other, Buffer.from("Wrong"), wrong.token), /Review/);
    assert.equal(await readFile(other, "utf8"), "Other");
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("export preserves private storage and refuses linked or changed file identities", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-export-boundary-"));
  try {
    const privateRoot = join(root, "private"); await mkdir(privateRoot);
    const store = new ExportStore([privateRoot]), path = join(root, "copy.txt");
    await assert.rejects(store.write(join(privateRoot, "sealed.txt"), Buffer.from("x")), /private storage/);
    await writeFile(path, "Original");
    const reviewed = await store.review(path);
    await assert.rejects(store.write(path, Buffer.from("Wrong chat"), reviewed.token, () => { throw new Error("Chat changed"); }), /Chat changed/);
    assert.equal(await readFile(path, "utf8"), "Original");
    if (process.platform !== "win32") {
      const alias = join(root, "alias"); await symlink(privateRoot, alias);
      await assert.rejects(store.write(join(alias, "sealed.txt"), Buffer.from("x")), /private storage/);
      const destination = join(root, "linked.txt"); await symlink(path, destination);
      await assert.rejects(store.review(destination), /unlinked/);
      await rm(destination); await link(path, destination);
      await assert.rejects(store.review(destination), /unlinked/);
      assert.equal(await readFile(path, "utf8"), "Original");
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});
