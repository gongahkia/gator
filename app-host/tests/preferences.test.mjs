import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, readdir, writeFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { ClassicLevel } from "classic-level";
import { FrontendPreferences } from "../src/preferences.mjs";

const dom = (value, encoding = 1) => Buffer.concat([Buffer.from([encoding]), Buffer.from(value, encoding ? "latin1" : "utf16le")]);
const key = (value, origin = "gator://app", encoding = 1) => Buffer.concat([Buffer.from("_" + origin + "\0"), dom(value, encoding)]);
const digest = async path => Object.fromEntries(await Promise.all((await readdir(path)).map(async name => [name, createHash("sha256").update(await readFile(join(path, name))).digest("hex")])));

test("frontend preferences import latest Chromium keys once without changing their database or Go theme", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-preferences-")), source = join(root, "Local Storage/leveldb");
  try {
    const database = new ClassicLevel(source, { keyEncoding: "buffer", valueEncoding: "buffer" });
    await database.put(key("gator:sidebar-collapsed"), dom("false"));
    await database.put(key("gator:sidebar-collapsed"), dom("true"));
    await database.put(key("gator:group-topics-by-folder", "gator://app", 0), dom("true", 0));
    await database.put(key("gator:work-details-sections"), dom(JSON.stringify({ context: true, outputs: false })));
    await database.put(key("gator:theme"), dom('"obsolete-local-theme"'));
    await database.put(key("credential"), dom('"DO NOT IMPORT"'));
    await database.put(key("gator:sidebar-collapsed", "https://other.example"), dom("false"));
    await database.close();
    const before = await digest(source), store = new FrontendPreferences(root), first = await store.read();
    assert.equal(first.sidebarCollapsed, true); assert.equal(first.groupChatsByFolder, true);
    assert.deepEqual(first.workDetailsSections, { outputs: false, context: true, connections: false, workspace: false });
    assert.deepEqual(await digest(source), before);
    const saved = await readFile(store.file, "utf8"); assert.ok(!saved.includes("DO NOT IMPORT") && !saved.includes("theme"));
    await new FrontendPreferences(root).read(); assert.equal(await readFile(store.file, "utf8"), saved);
    await store.configure({ sidebarCollapsed: false });
    assert.equal((await new FrontendPreferences(root).read()).sidebarCollapsed, false);
    assert.equal((await new FrontendPreferences(root).read()).groupChatsByFolder, true);
    assert.deepEqual(await digest(source), before);
    assert.ok(!(await readdir(root)).some(name => name.startsWith(".preferences-import-")));
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("preference patches serialize, reject extra authority and recover an abandoned writer lock", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-preference-lock-"));
  try {
    const store = new FrontendPreferences(root);
    await Promise.all([store.configure({ sidebarCollapsed: true }), store.configure({ groupChatsByFolder: true })]);
    assert.equal((await store.read()).sidebarCollapsed, true); assert.equal((await store.read()).groupChatsByFolder, true);
    await assert.rejects(store.configure({ theme: "override" }), /Invalid frontend/);
    await assert.rejects(store.configure(JSON.parse('{"__proto__":true}')), /Invalid frontend/);
    await writeFile(store.file + ".lock", String(process.pid));
    await assert.rejects(new FrontendPreferences(root).read(), /Another application/);
    const exited = execFileSync(process.execPath, ["-e", "process.stdout.write(String(process.pid))"], { encoding: "utf8" });
    await writeFile(store.file + ".lock", exited); assert.equal((await store.read()).sidebarCollapsed, true);
    await store.flush();
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("corrupt or interrupted legacy import retains source bytes and leaves no completed migration marker", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-preference-corrupt-")), source = join(root, "Local Storage/leveldb");
  try {
    await mkdir(source, { recursive: true }); await writeFile(join(source, "CURRENT"), "MANIFEST-999999\n");
    const before = await digest(source), store = new FrontendPreferences(root);
    await assert.rejects(store.read()); assert.deepEqual(await digest(source), before);
    assert.equal(await readFile(store.file).catch(error => error.code), "ENOENT");
    assert.ok(!(await readdir(root)).some(name => name.startsWith(".preferences-import-")));
    await rm(source, { recursive: true });
    const database = new ClassicLevel(source, { keyEncoding: "buffer", valueEncoding: "buffer" });
    await database.put(key("gator:sidebar-collapsed"), dom("true")); await database.del(key("gator:sidebar-collapsed")); await database.close();
    assert.equal((await store.read()).sidebarCollapsed, false);
  } finally { await rm(root, { recursive: true, force: true }); }
});
