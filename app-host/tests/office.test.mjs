import test from "node:test"; import assert from "node:assert/strict"; import { unzipSync, strFromU8 } from "fflate";
import { inspectOffice, editOffice } from "../src/office.mjs";
import { wordFixture, workbookFixture, slidesFixture, packageOffice } from "./office-fixtures.mjs";
import { DOMParser } from "@xmldom/xmldom";

test("Word text and table edits round-trip without changing styles or embedded graphics", () => {
  const original = wordFixture(), before = unzipSync(original), model = inspectOffice("docx", original);
  assert.equal(model.blocks[0].text, "Hello world."); assert.equal(model.blocks[1].rows[0][0][0].text, "A table cell");
  const result = editOffice("docx", original, { paragraphs: { "p:0": "Hello there world.", "p:1": "Updated table cell" } }), after = unzipSync(result);
  assert.equal(inspectOffice("docx", result).blocks[0].text, "Hello there world.");
  assert.deepEqual(after["word/styles.xml"], before["word/styles.xml"]); assert.deepEqual(after["word/media/image.png"], before["word/media/image.png"]);
  assert.match(strFromU8(after["word/document.xml"]), /<w:i\/>/); assert.match(strFromU8(after["word/document.xml"]), /<w:t>world\.<\/w:t>/);
  assert.equal(inspectOffice("docx", editOffice("docx", result, { paragraphs: { "p:0": "A line\nAnother line\tEnd 🐊" } })).blocks[0].text, "A line\nAnother line\tEnd 🐊");
  assert.throws(() => editOffice("docx", original, { paragraphs: { "p:99": "Unknown" } }), /no longer exists/);
});
test("Excel preserves native cells and charts, edits formulas and clears stale results", () => {
  const original = workbookFixture(), model = inspectOffice("xlsx", original); assert.equal(model.sheets[0].name, "Budget");
  const result = editOffice("xlsx", original, { cells: { "0!B1": "6", "0!D2": "=SUM(B1:C1)", "0!A2": "00123" } }), after = unzipSync(result), values = inspectOffice("xlsx", result).sheets[0].cells;
  assert.equal(values.find(cell => cell.address === "B1").value, "6"); assert.equal(values.find(cell => cell.address === "C1").value, ""); assert.equal(values.find(cell => cell.address === "D2").formula, "SUM(B1:C1)"); assert.equal(values.find(cell => cell.address === "A2").value, "00123");
  assert.deepEqual(after["xl/charts/chart1.xml"], unzipSync(original)["xl/charts/chart1.xml"]); assert.deepEqual(after["xl/styles.xml"], unzipSync(original)["xl/styles.xml"]); assert.match(strFromU8(after["xl/workbook.xml"]), /fullCalcOnLoad="1"/);
  assert.throws(() => editOffice("xlsx", original, { cells: { "0!ZZZ1": "x" } }), /Invalid spreadsheet address/);
});
test("PowerPoint respects slide relationship IDs and retains masters and media through text edits", () => {
  const original = slidesFixture(), model = inspectOffice("pptx", original); assert.equal(model.slides.length, 1); assert.equal(model.slides[0].shapes[0].paragraphs[0].text, "Quarterly update");
  const result = editOffice("pptx", original, { paragraphs: { "s:0:p:0": "Updated quarterly review" } }), after = unzipSync(result), before = unzipSync(original);
  assert.equal(inspectOffice("pptx", result).slides[0].shapes[0].paragraphs[0].text, "Updated quarterly review");
  for (const path of ["ppt/presentation.xml", "ppt/slideMasters/slideMaster1.xml", "ppt/media/logo.png"]) assert.deepEqual(after[path], before[path]);
});
test("PowerPoint line breaks and tabs export valid DrawingML runs and preserve their formatting", () => {
  const original = slidesFixture(), value = "Quarterly\n\nResults\tSummary 🐊\n";
  const updated = editOffice("pptx", original, { paragraphs: { "s:0:p:0": value } });
  assert.equal(inspectOffice("pptx", updated).slides[0].shapes[0].paragraphs[0].text, value);
  assert.equal(inspectOffice("pptx", updated).slides[0].shapes[0].paragraphs[0].runs.map(run => run.text).join(""), value);
  const doc = new DOMParser().parseFromString(strFromU8(unzipSync(updated)["ppt/slides/slide1.xml"]), "application/xml");
  const drawing = "http://schemas.openxmlformats.org/drawingml/2006/main";
  const runs = Array.from(doc.getElementsByTagNameNS(drawing, "r"));
  assert.equal(runs.length, 2);
  for (const run of runs) {
    assert.deepEqual(Array.from(run.childNodes).filter(node => node.nodeType === 1).map(node => node.localName), ["rPr", "t"]);
    assert.equal(run.getElementsByTagNameNS(drawing, "rPr")[0].getAttribute("sz"), "2800");
  }
  for (const br of Array.from(doc.getElementsByTagNameNS(drawing, "br"))) assert.equal(br.parentNode.localName, "p");
  assert.equal(doc.getElementsByTagNameNS(drawing, "tab").length, 0);
  const shortened = editOffice("pptx", updated, { paragraphs: { "s:0:p:0": "Results" } });
  assert.equal(inspectOffice("pptx", shortened).slides[0].shapes[0].paragraphs[0].text, "Results");
});
test("PowerPoint automatic fields are retained rather than silently converted to manual text", () => {
  const files = unzipSync(slidesFixture());
  files["ppt/slides/slide1.xml"] = new TextEncoder().encode(strFromU8(files["ppt/slides/slide1.xml"]).replace("<a:r>", '<a:fld id="{942A23BE-4663-4A60-B96F-33FBD7926F00}" type="slidenum">').replace("</a:r>", "</a:fld>"));
  const original = packageOffice(files), before = Buffer.from(original);
  assert.equal(inspectOffice("pptx", original).slides[0].shapes[0].paragraphs[0].runs.map(run => run.text).join(""), "Quarterly update");
  assert.throws(() => editOffice("pptx", original, { paragraphs: { "s:0:p:0": "New label" } }), /Automatic slide fields/);
  assert.deepEqual(original, before);
});
test("Office packages reject custom entities, signed packages and invalid shapes before edits", () => {
  assert.throws(() => inspectOffice("docx", packageOffice({ "word/document.xml": '<!DOCTYPE x [<!ENTITY x "secret">]><document>&x;</document>' })), /entities/);
  assert.throws(() => inspectOffice("docx", packageOffice({ "word/document.xml": "<document/>", "_xmlsignatures/sig1.xml": "signed" })), /Signed/);
  assert.throws(() => inspectOffice("xlsx", Buffer.from("Not a ZIP")));
});
