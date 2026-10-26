import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { compareFileContent } from "../src/file-comparison.mjs";
import { FileStore, validateFileRequest } from "../src/files.mjs";
import { editOffice } from "../src/office.mjs";
import { wordFixture, workbookFixture, slidesFixture } from "./office-fixtures.mjs";
const compare = (a, b) => compareFileContent("notes.md", Buffer.from(a), Buffer.from(b));

test("line comparison separates inserts, removals, unchanged context and blank lines", () => {
  const result = compare("Title\nKeep\nOld\nStay\nRemove\nEnd", "Title\nAdded\nKeep\nNew\nStay\nEnd");
  assert.equal(result.identical, false); assert.equal(result.truncated, false);
  assert.deepEqual(result.changes.map(({ before, after }) => [before, after]), [["", "Added"], ["Old", "New"], ["Remove", ""]]);
  const blank = compare("", "\n"); assert.equal(blank.changes.length, 1); assert.equal(blank.changes[0].beforeLines, 0); assert.equal(blank.changes[0].afterLines, 1);
  const endings = compare("one\r\ntwo", "one\ntwo"); assert.equal(endings.changes.length, 0); assert.match(endings.notices[0], /line endings/);
  assert.equal(compare("same", "same").identical, true);
  const sparse = compare("Same\n".repeat(10000) + "old", "Same\n".repeat(10000) + "new"); assert.equal(sparse.changes.length, 1); assert.equal(sparse.truncated, false);
});

test("large or numerous changes are explicitly partial and bounded", () => {
  const massive = compare(Array.from({ length: 2000 }, (_, i) => `old${i}`).join("\n"), Array.from({ length: 2000 }, (_, i) => `new${i}`).join("\n"));
  assert.equal(massive.truncated, true); assert.equal(massive.changes.length, 0);
  const many = compareFileContent("table.csv", Buffer.from("A,B\n".repeat(501)), Buffer.from("C,D\n".repeat(501)));
  assert.equal(many.changes.length, 200); assert.equal(many.truncated, true);
  const long = compare("a".repeat(30000), "b".repeat(30000)); assert.equal(long.truncated, true); assert.ok(JSON.stringify(long).length < 16000);
});

test("Office comparisons show entered content without claiming layout or formula recalculation", () => {
  for (const [extension, original, edit, expected] of [
    ["docx", wordFixture(), { paragraphs: { "p:0": "New Word paragraph" } }, "New Word paragraph"],
    ["xlsx", workbookFixture(), { cells: { "0!B1": "125", "0!C1": "=B1*3" } }, "=B1*3"],
    ["pptx", slidesFixture(), { paragraphs: { "s:0:p:0": "New slide title" } }, "New slide title"],
  ]) {
    const edited = editOffice(extension, original, edit), result = compareFileContent(`file.${extension}`, original, edited);
    assert.equal(result.identical, false); assert.ok(result.changes.some(change => change.after === expected)); assert.ok(result.notices.some(notice => notice.includes("not compared")));
    assert.ok(!result.changes.some(change => change.location.includes("undefined")));
  }
  const binary = compareFileContent("unknown.bin", Buffer.from("old"), Buffer.from("new")); assert.equal(binary.changes.length, 0); assert.match(binary.notices[0], /native app/);
});

test("comparison is read-only, scoped to exact file versions and verifies retained bytes", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-file-compare-"));
  try {
    const store = new FileStore(root), ref = { kind: "upload", id: `${"a".repeat(32)}/notes.md` }, request = { context: "conversation:one", ref };
    const original = Buffer.from("Original"), source = async () => ({ name: "notes.md", bytes: original });
    const first = await store.request({ ...request, action: "open" }, source);
    const second = await store.request({ ...request, action: "save", expected: first.revision, edit: { text: "Edited" } }, source);
    const compare = { ...request, action: "compare", base: first.revision, version: second.revision };
    const result = await store.request(compare, source);
    assert.equal(result.from.number, 1); assert.equal(result.to.number, 2); assert.equal(result.changes[0].before, "Original"); assert.equal(result.changes[0].after, "Edited");
    const latest = await store.request({ ...request, action: "open" }, source); assert.equal(latest.current, second.current); assert.equal(latest.versions.length, 2); assert.equal(original.toString(), "Original");
    await assert.rejects(store.request({ ...compare, base: "b".repeat(8) + "-" + first.revision.slice(9) }, source), /unavailable/);
    assert.throws(() => validateFileRequest({ ...compare, base: "../../other" }), /version/);
    assert.throws(() => validateFileRequest({ ...compare, edit: { text: "overwrite" } }), /Choose/);
    await writeFile(join(store.directory(ref), `${first.revision}.bin`), "Tampered");
    await assert.rejects(store.request(compare, source), /changed on disk/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("line changes reconstruct each later version without losing repeated or blank lines", () => {
  let seed = 19;
  const next = () => (seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0);
  for (let trial = 0; trial < 300; trial++) {
    const lines = () => Array.from({ length: 1 + next() % 20 }, () => ["", "alpha", "beta", "🐊"][next() % 4]);
    const a = lines(), b = lines(), result = compare(a.join("\n"), b.join("\n")), restored = [...a];
    assert.equal(result.truncated, false);
    for (const change of [...result.changes].reverse()) {
      const start = Number(change.location.match(/^Lines (\d+)/)[1]) - 1;
      restored.splice(start, change.beforeLines, ...(change.afterLines ? change.after.split("\n") : []));
    }
    assert.deepEqual(restored, b);
  }
});
