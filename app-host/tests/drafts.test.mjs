import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { DraftStore } from "../src/drafts.mjs";

test("update reply drafts retain their referent independently of text across restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-reply-drafts-"));
  try {
    const file = join(root, "drafts.json"), store = new DraftStore(file);
    const draft = { context: "conversation:one", text: "", title: "One", source: "/project", reply_to_update_id: "inbox-one" };
    await store.request({ action: "save", draft });
    const retained = await new DraftStore(file).request({ action: "list" });
    assert.equal(retained.drafts[0].reply_to_update_id, "inbox-one");
    await assert.rejects(store.request({ action: "save", draft: { ...draft, reply_to_update_id: "../other" } }));
    await store.request({ action: "configure", enabled: false });
    assert.equal((await new DraftStore(file).request({ action: "list" })).drafts.length, 0);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("pending draft failures remain visible to update and shutdown preparation", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-draft-flush-"));
  try {
    const file = join(root, "drafts.json");
    await writeFile(file, "{broken");
    const store = new DraftStore(file);
    await assert.rejects(store.request({ action: "list" }));
    await assert.rejects(store.flush());
    await store.request({ action: "clear" });
    await store.flush();
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("drafts survive restart, stay scoped, and save serially with private permissions", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-drafts-"));
  try {
    const file = join(root, "profile-a.json");
    const store = new DraftStore(file);
    await Promise.all(["one", "two"].map((id) => store.request({ action: "save", draft: { context: `conversation:${id}`, text: `Unsent ${id} 🐊`, title: id, source: "/project" } })));
    assert.equal((await new DraftStore(file).request({ action: "list" })).drafts.length, 2);
    assert.equal((await new DraftStore(join(root, "profile-b.json")).request({ action: "list" })).drafts.length, 0);
    // Windows privacy is enforced by the native DACL bootstrap and tested in Go.
    if (process.platform !== "win32") assert.equal((await stat(file)).mode & 0o777, 0o600);
    await store.request({ action: "remove", context: "conversation:one" });
    assert.deepEqual((await store.request({ action: "list" })).drafts.map((item) => item.context), ["conversation:two"]);
    // An unsent main message and a new topic in the same folder are separate.
    for (const context of ["workspace:/project", "workspace:topic:/project"]) await store.request({ action: "save", draft: { context, text: context, title: "Unsent", source: "/project" } });
    assert.deepEqual((await new DraftStore(file).request({ action: "list" })).drafts.map((item) => item.context), ["workspace:topic:/project", "workspace:/project", "conversation:two"]);
    const interim = JSON.parse(await readFile(file, "utf8"));
    interim.drafts[0].context = "topic:/project";
    await writeFile(file, JSON.stringify(interim));
    const migrated = await new DraftStore(file).request({ action: "list" });
    assert.equal(migrated.drafts[0].context, "workspace:topic:/project");
    assert.equal(migrated.drafts[0].text, "workspace:topic:/project");
    // This is the pre-conversational store's accepted identity contract.
    assert(migrated.drafts.every((draft) => /^(conversation|workspace):.+$/.test(draft.context)));
    assert.equal(JSON.parse(await readFile(file, "utf8")).drafts[0].context, "workspace:topic:/project");
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("expired drafts are removed and disabling clears retained text", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-drafts-"));
  try {
    let now = 10;
    const file = join(root, "drafts.json");
    const store = new DraftStore(file, () => now);
    const save = () => store.request({ action: "save", draft: { context: "workspace:/project", text: "Private draft", title: "New message", source: "/project" } });
    await save(); now += 8 * 86400000;
    assert.equal((await store.request({ action: "list" })).drafts.length, 0);
    assert.equal((await readFile(file, "utf8")).includes("Private draft"), false);
    await save(); await store.request({ action: "configure", enabled: false }); await save();
    assert.deepEqual(await new DraftStore(file).request({ action: "list" }), { enabled: false, drafts: [] });
    await store.request({ action: "configure", enabled: true }); await save();
    assert.equal((await store.request({ action: "clear" })).drafts.length, 0);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("draft requests reject arbitrary fields, large text, and malformed stores recover explicitly", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-drafts-"));
  try {
    const file = join(root, "drafts.json"); const store = new DraftStore(file);
    for (const request of [{ action: "list", file: "/tmp/other" }, { action: "configure", enabled: "yes" }, { action: "remove", context: "" }, { action: "save", draft: { context: "conversation:one", title: "One", source: "/project", text: "🐊".repeat(32769) } }]) await assert.rejects(store.request(request));
    await writeFile(file, "{broken");
    await assert.rejects(store.request({ action: "list" }), /Clear saved drafts/);
    assert.deepEqual((await store.request({ action: "clear" })).drafts, []);
  } finally { await rm(root, { recursive: true, force: true }); }
});
