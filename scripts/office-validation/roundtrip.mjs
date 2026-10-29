import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { inspectOffice, editOffice } from "../../app-host/src/office.mjs";

const directory = process.argv[2];
if (!directory) throw new Error("Usage: node roundtrip.mjs fixture-directory");
const original = await readFile(join(directory, "rich-template.pptx"));
const before = inspectOffice("pptx", original);
assert.equal(before.slides[0].shapes[1].paragraphs.length, 3);
const value = "Edited through the shared file editor.\nSecond line\tSummary 🐊";
const updated = editOffice("pptx", original, { paragraphs: { "s:0:p:1": value } });
const after = inspectOffice("pptx", updated);
assert.equal(after.slides[0].shapes[1].paragraphs[0].text, value);
assert.equal(after.slides[0].shapes[1].paragraphs[0].runs.map(run => run.text).join(""), value);
assert.equal(after.slides[0].shapes[1].paragraphs[1].text, before.slides[0].shapes[1].paragraphs[1].text);
await writeFile(join(directory, "host-roundtrip.pptx"), updated);
console.log("Shared Office editor roundtrip passed; validate the saved package with Open XML SDK.");
