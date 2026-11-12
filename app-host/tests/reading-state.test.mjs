import { test } from "node:test";
import assert from "node:assert/strict";
import { lstat, mkdtemp, readFile, readdir, rm, stat, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { ReadingStore } from "../src/reading-state.mjs";

const anchor = { key: "a".repeat(64), index: 12, offset: 3, following: false, seen: "b".repeat(64), seen_version: "c".repeat(64) };
test("reading anchors reopen privately without storing conversation text and remain isolated by chat/profile", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-reading-"));
  try {
    const store = new ReadingStore(root);
    assert.equal(await store.read("conversation:one"), undefined);
    await Promise.all([store.save("conversation:one", anchor), store.save("conversation:two", { ...anchor, index: 25 })]);
    await store.flush();
    assert.deepEqual(await new ReadingStore(root).read("conversation:one"), anchor);
    assert.equal((await store.read("conversation:two")).index, 25);
    assert.equal(await new ReadingStore(join(root, "other-profile")).read("conversation:one"), undefined);
    const files = await readdir(root);
    assert.equal(files.length, 2);
    assert(files.every(file => /^[a-f0-9]{64}\.json$/.test(file)));
    if (process.platform !== "win32") assert.equal((await stat(join(root, files[0]))).mode & 0o777, 0o600);
    assert.equal((await readFile(join(root, files[0]), "utf8")).includes("conversation:"), false);
    for (const patch of [{ text: "private conversation" }, { index: -1 }, { offset: Infinity }, { key: "../path" }, { following: "yes" }]) await assert.rejects(store.save("conversation:one", { ...anchor, ...patch }));
    assert.throws(() => store.save("../other", anchor));
    assert.deepEqual(await store.read("conversation:one"), anchor);
    await store.save("conversation:one", { ...anchor, offset: -2 });
    assert.equal((await new ReadingStore(root).read("conversation:one")).offset, -2);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("invalid and linked anchors fail explicitly without modifying history or the retained file", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-reading-invalid-"));
  try {
    const store = new ReadingStore(root), file = join(root, createHash("sha256").update("conversation:one").digest("hex") + ".json");
    await writeFile(file, "{broken");
    await assert.rejects(store.read("conversation:one"), /Could not restore/);
    assert.equal(await readFile(file, "utf8"), "{broken");
    await writeFile(file, "x".repeat(4097));
    await assert.rejects(store.read("conversation:one"), /Could not restore/);
    if (process.platform !== "win32") {
      await rm(file); const target = join(root, "target");
      await writeFile(target, JSON.stringify({ version: 1, anchor })); await symlink(target, file);
      await assert.rejects(store.read("conversation:one"), /Could not restore/);
      await store.save("conversation:one", anchor);
      assert.deepEqual(await store.read("conversation:one"), anchor);
      assert.equal((await lstat(file)).isSymbolicLink(), false);
    }
  } finally { await rm(root, { recursive: true, force: true }); }
});
