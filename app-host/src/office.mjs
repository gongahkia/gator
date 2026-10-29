import { unzipSync, zipSync } from "fflate";
import { DOMParser, XMLSerializer } from "@xmldom/xmldom";
import { posix } from "node:path";

const ns = { w: "http://schemas.openxmlformats.org/wordprocessingml/2006/main", s: "http://schemas.openxmlformats.org/spreadsheetml/2006/main", a: "http://schemas.openxmlformats.org/drawingml/2006/main", r: "http://schemas.openxmlformats.org/officeDocument/2006/relationships" };
const elements = (node, name) => Array.from(node.getElementsByTagNameNS("*", name));
const children = (node, name) => Array.from(node.childNodes).filter(child => child.nodeType === 1 && (!name || child.localName === name));
const first = (node, name) => node && elements(node, name)[0];
const attr = (node, name) => node ? Array.from(node.attributes ?? []).find(attribute => attribute.localName === name)?.value ?? "" : "";
const text = node => node?.textContent ?? "";
const boolean = node => !!node && !["0", "false", "off"].includes(attr(node, "val"));
const relationshipID = node => node.getAttributeNS(ns.r, "id") || Array.from(node.attributes).find(attribute => attribute.prefix === "r" && attribute.localName === "id")?.value;
const colour = value => /^[a-f0-9]{6}$/i.test(value) ? `#${value}` : undefined;
function xml(bytes) {
  const source = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  if (/<!DOCTYPE|<!ENTITY/i.test(source)) throw new Error("Documents with custom XML entities cannot be edited.");
  return new DOMParser({ onError: level => { if (level !== "warning") throw new Error("This Office file contains invalid XML."); } }).parseFromString(source, "application/xml");
}
function archive(bytes, extension) {
  let total = 0, count = 0;
  const files = unzipSync(bytes, { filter: entry => {
    if (++count > 4096 || entry.originalSize > 16 * 1024 * 1024 || (total += entry.originalSize) > 48 * 1024 * 1024 || entry.name.startsWith("/") || entry.name.split("/").includes("..") || entry.name.includes("\\")) throw new Error("This Office package exceeds the inline editor's limits.");
    return true;
  } });
  if (!files["[Content_Types].xml"] || !files[{ docx: "word/document.xml", xlsx: "xl/workbook.xml", pptx: "ppt/presentation.xml" }[extension]]) throw new Error("This is not a supported Office Open XML file.");
  if (Object.keys(files).some(name => name.startsWith("_xmlsignatures/") || /vbaProject\.bin$/i.test(name))) throw new Error("Signed and macro-enabled Office files cannot be changed in the inline editor.");
  return files;
}
const load = (files, path) => { if (!files[path]) throw new Error(`A required Office part is missing: ${path}`); return xml(files[path]); };
const write = (files, path, document) => { files[path] = new TextEncoder().encode(new XMLSerializer().serializeToString(document)); };
function relationships(files, part) {
  const path = posix.join(posix.dirname(part), "_rels", `${posix.basename(part)}.rels`), result = new Map();
  if (!files[path]) return result;
  for (const rel of elements(load(files, path), "Relationship")) {
    if (attr(rel, "TargetMode") === "External") continue;
    const target = attr(rel, "Target"), resolved = target.startsWith("/") ? target.slice(1) : posix.normalize(posix.join(posix.dirname(part), target));
    if (!resolved.startsWith("../") && !resolved.includes("\\")) result.set(attr(rel, "Id"), resolved);
  }
  return result;
}
function image(files, path) {
  const bytes = files[path]; if (!bytes || bytes.length > 2 * 1024 * 1024) return undefined;
  const mime = /\.png$/i.test(path) ? "image/png" : /\.jpe?g$/i.test(path) ? "image/jpeg" : undefined;
  return mime ? `data:${mime};base64,${Buffer.from(bytes).toString("base64")}` : undefined;
}
function leaves(paragraph) {
  const found = [];
  const walk = node => {
    for (const child of children(node)) {
      if (child.localName === "p") continue;
      if (["t", "tab", "br", "cr"].includes(child.localName)) found.push(child);
      else walk(child);
    }
  }; walk(paragraph); return found;
}
const leafText = node => node.localName === "t" ? text(node) : node.localName === "tab" ? "\t" : "\n";
const paragraphText = paragraph => leaves(paragraph).map(leafText).join("");
function runStyle(run) {
  const props = children(run, "rPr")[0];
  return { bold: boolean(first(props, "b")) || attr(props, "b") === "1", italic: boolean(first(props, "i")) || attr(props, "i") === "1", underline: boolean(first(props, "u")), color: colour(attr(first(props, "color"), "val") || attr(first(props, "srgbClr"), "val")), size: Number(attr(first(props, "sz"), "val") || attr(props, "sz")) / (props?.namespaceURI?.includes("wordprocessing") ? 2 : 100) || undefined };
}
function paragraphModel(node, id) {
  const runs = node.namespaceURI === ns.a ? children(node).filter(child => ["r", "br", "fld"].includes(child.localName)) : elements(node, "r");
  return { id, text: paragraphText(node), style: attr(first(node, "pStyle"), "val"), align: attr(first(node, "jc"), "val"), runs: runs.map(run => ({ text: run.localName === "br" ? "\n" : leaves(run).map(leafText).join(""), ...runStyle(run) })) };
}
function validText(value) {
  if (typeof value !== "string" || value.length > 1024 * 1024 || !value.isWellFormed() || /[\x00-\x08\x0b\x0c\x0e-\x1f]/.test(value)) throw new Error("An edited text block contains invalid or excessive text.");
}
// Keep untouched run properties and the common prefix/suffix of a changed run.
// Inserted text inherits the run at the edit; untouched package parts stay byte-identical.
function replaceParagraph(paragraph, value) {
  validText(value); const nodes = leaves(paragraph), before = nodes.map(leafText).join("");
  if (before === value) return;
  if (paragraph.namespaceURI === ns.a && nodes.some(node => node.parentNode?.localName === "fld")) throw new Error("Automatic slide fields cannot be replaced as plain text.");
  if (paragraph.namespaceURI === ns.a && nodes.some(node => !(node.localName === "br" && node.parentNode === paragraph) && !(node.localName === "t" && node.parentNode.localName === "r" && children(node.parentNode).filter(child => child.localName !== "rPr").length === 1))) throw new Error("This slide contains an unsupported text-run structure.");
  let start = 0, end = before.length, tail = value.length;
  while (start < end && start < tail && before[start] === value[start]) start++;
  if (start && /[\ud800-\udbff]/.test(before[start - 1])) start--;
  while (end > start && tail > start && before[end - 1] === value[tail - 1]) { end--; tail--; }
  if (end < before.length && /[\udc00-\udfff]/.test(before[end])) { end++; tail++; }
  if (!nodes.length) {
    const prefix = paragraph.prefix || (paragraph.namespaceURI?.includes("wordprocessing") ? "w" : "a");
    const run = paragraph.ownerDocument.createElementNS(paragraph.namespaceURI, `${prefix}:r`), leaf = paragraph.ownerDocument.createElementNS(paragraph.namespaceURI, `${prefix}:t`);
    run.appendChild(leaf); paragraph.appendChild(run); nodes.push(leaf);
  }
  let offset = 0, inserted = false;
  for (const node of nodes) {
    const old = leafText(node), next = offset + old.length;
    let result = old.slice(0, Math.max(0, Math.min(old.length, start - offset)));
    if (!inserted && start <= next) { result += value.slice(start, tail); inserted = true; }
    result += old.slice(Math.max(0, Math.min(old.length, end - offset)));
    offset = next; if (result === old) continue;
    if (paragraph.namespaceURI === ns.a) { replaceDrawingLeaf(node, result); continue; }
    const parent = node.parentNode, document = node.ownerDocument, prefix = node.prefix || parent.prefix || "w", namespace = node.namespaceURI;
    for (const segment of result.split(/([\n\t])/)) {
      if (!segment) continue;
      const replacement = document.createElementNS(namespace, `${prefix}:${segment === "\n" ? "br" : segment === "\t" ? "tab" : "t"}`);
      if (!["\n", "\t"].includes(segment)) { replacement.setAttributeNS("http://www.w3.org/XML/1998/namespace", "xml:space", "preserve"); replacement.textContent = segment; }
      parent.insertBefore(replacement, node);
    }
    parent.removeChild(node);
  }
}

// DrawingML breaks are paragraph children, unlike Word breaks inside a run.
// Each regular DrawingML run contains one text element; tabs are text characters.
function replaceDrawingLeaf(node, value) {
  const document = node.ownerDocument, prefix = node.prefix || "a";
  const reference = node.parentNode.localName === "r" ? node.parentNode : node;
  const paragraph = reference.parentNode;
  let style = children(reference, "rPr")[0];
  if (!style && reference.localName === "br") {
    for (let prior = reference.previousSibling; prior; prior = prior.previousSibling) {
      if (prior.nodeType === 1 && prior.localName === "r") { style = children(prior, "rPr")[0]; break; }
    }
  }
  const make = name => document.createElementNS(ns.a, `${prefix}:${name}`);
  for (const [index, line] of value.split("\n").entries()) {
    if (index) {
      const lineBreak = make("br"); if (style) lineBreak.appendChild(style.cloneNode(true));
      paragraph.insertBefore(lineBreak, reference);
    }
    if (!line) continue;
    const run = make("r"), text = make("t");
    if (style) run.appendChild(style.cloneNode(true));
    text.setAttributeNS("http://www.w3.org/XML/1998/namespace", "xml:space", "preserve"); text.textContent = line;
    run.appendChild(text); paragraph.insertBefore(run, reference);
  }
  paragraph.removeChild(reference);
}
function word(files) {
  const path = "word/document.xml", document = load(files, path), paragraphs = elements(document, "p"), nodeIDs = new Map(paragraphs.map((node, index) => [node, `p:${index}`]));
  const links = relationships(files, path);
  const paragraph = node => ({ type: "paragraph", ...paragraphModel(node, nodeIDs.get(node)), images: elements(node, "blip").map(blip => image(files, links.get(attr(blip, "embed")) || "")).filter(Boolean) });
  const blocks = children(first(document, "body")).flatMap(node => node.localName === "p" ? [paragraph(node)] : node.localName === "tbl" ? [{ type: "table", rows: children(node, "tr").map(row => children(row, "tc").map(cell => children(cell, "p").map(paragraph))) }] : []);
  if (paragraphs.length > 5000) throw new Error("This document exceeds the inline editor's 5,000 paragraph limit.");
  const protection = files["word/settings.xml"] && first(load(files, "word/settings.xml"), "documentProtection");
  return { model: { kind: "word", editable: !(protection && ["1", "true"].includes(attr(protection, "enforcement"))), blocks, notice: "Text editing preserves document parts and existing run styles. Page layout, tracked changes and embedded objects are not editable here." }, document, path, nodes: new Map(paragraphs.map(node => [nodeIDs.get(node), node])) };
}
const colNumber = letters => [...letters].reduce((n, ch) => n * 26 + ch.charCodeAt(0) - 64, 0);
const address = value => /^([A-Z]{1,3})([1-9]\d{0,5})$/.exec(value);
function workbook(files) {
  const path = "xl/workbook.xml", document = load(files, path), links = relationships(files, path);
  const shared = files["xl/sharedStrings.xml"] ? elements(load(files, "xl/sharedStrings.xml"), "si").map(node => elements(node, "t").map(text).join("")) : [];
  const sheets = [], docs = [];
  for (const [index, sheet] of elements(document, "sheet").entries()) {
    const part = links.get(relationshipID(sheet)); if (!part || !/^xl\/worksheets\//.test(part)) continue;
    const doc = load(files, part), cells = []; let maxRow = 1, maxCol = 1;
    for (const node of elements(doc, "c")) {
      const ref = attr(node, "r"), match = address(ref); if (!match) continue;
      const row = Number(match[2]), col = colNumber(match[1]); maxRow = Math.max(maxRow, row); maxCol = Math.max(maxCol, col);
      const raw = text(children(node, "v")[0]), type = attr(node, "t"), formula = text(children(node, "f")[0]);
      const value = type === "s" ? shared[Number(raw)] ?? "" : type === "inlineStr" ? elements(node, "t").map(text).join("") : type === "b" ? raw === "1" ? "TRUE" : "FALSE" : raw;
      cells.push({ address: ref, row, col, value, formula });
    }
    if (cells.length > 100000 || sheets.length >= 128) throw new Error("This workbook exceeds the inline editor's 100,000 cell or 128 sheet limit.");
    sheets.push({ id: index, name: attr(sheet, "name"), cells, maxRow, maxCol, protected: !!first(doc, "sheetProtection"), merges: elements(doc, "mergeCell").map(node => attr(node, "ref")) });
    docs.push({ index, part, document: doc });
  }
  return { model: { kind: "workbook", editable: sheets.some(sheet => !sheet.protected), sheets, notice: "Edit cell values and formulas. Excel recalculates formulas when the exported workbook opens; charts, layout and formatting stay in the file." }, document, path, docs };
}
function slides(files) {
  const path = "ppt/presentation.xml", document = load(files, path), links = relationships(files, path), size = first(document, "sldSz");
  const width = Number(attr(size, "cx")) || 12192000, height = Number(attr(size, "cy")) || 6858000, result = [], nodes = new Map(), docs = [];
  for (const [index, slide] of elements(document, "sldId").entries()) {
    if (index >= 512) throw new Error("This presentation exceeds the inline editor's 512 slide limit.");
    const part = links.get(relationshipID(slide)); if (!part || !/^ppt\/slides\//.test(part)) continue;
    const doc = load(files, part), relations = relationships(files, part), paras = elements(doc, "p"), shapes = [];
    paras.forEach((node, i) => nodes.set(`s:${index}:p:${i}`, node));
    for (const shape of children(first(doc, "spTree"))) {
      if (!["sp", "pic"].includes(shape.localName)) continue;
      const transform = first(shape, "xfrm"), off = first(transform, "off"), ext = first(transform, "ext"), blip = first(shape, "blip");
      shapes.push({ id: attr(first(shape, "cNvPr"), "id"), x: Number(attr(off, "x")) / width * 100 || 0, y: Number(attr(off, "y")) / height * 100 || 0, width: Number(attr(ext, "cx")) / width * 100 || 90, height: Number(attr(ext, "cy")) / height * 100 || 20, fill: colour(attr(first(children(shape, "spPr")[0], "srgbClr"), "val")), image: blip ? image(files, relations.get(attr(blip, "embed")) || "") : undefined, paragraphs: elements(shape, "p").map(node => paragraphModel(node, `s:${index}:p:${paras.indexOf(node)}`)) });
    }
    result.push({ id: index, shapes }); docs.push({ part, document: doc });
  }
  return { model: { kind: "slides", editable: true, width, height, slides: result, notice: "Edit text in existing slide shapes. Original masters, graphics and layouts are preserved; this simplified preview does not reproduce every visual effect." }, nodes, docs };
}
export function inspectOffice(extension, bytes) {
  const files = archive(bytes, extension);
  return (extension === "docx" ? word(files) : extension === "xlsx" ? workbook(files) : slides(files)).model;
}
export function editOffice(extension, bytes, edit) {
  const files = archive(bytes, extension), parsed = extension === "docx" ? word(files) : extension === "xlsx" ? workbook(files) : slides(files);
  if (!parsed.model.editable) throw new Error("This document is protected and available for preview only.");
  if (extension !== "xlsx") {
    if (!edit.paragraphs || typeof edit.paragraphs !== "object" || Array.isArray(edit.paragraphs) || Object.keys(edit.paragraphs).length > 5000) throw new Error("Invalid document changes.");
    for (const [id, value] of Object.entries(edit.paragraphs)) { const node = parsed.nodes.get(id); if (!node) throw new Error("This text block no longer exists. Reopen the file."); replaceParagraph(node, value); }
    if (extension === "docx") write(files, parsed.path, parsed.document);
    else for (const doc of parsed.docs) if ([...Object.keys(edit.paragraphs)].some(id => parsed.nodes.get(id)?.ownerDocument === doc.document)) write(files, doc.part, doc.document);
  } else {
    if (!edit.cells || typeof edit.cells !== "object" || Array.isArray(edit.cells) || Object.keys(edit.cells).length > 100000) throw new Error("Invalid spreadsheet changes.");
    for (const [key, value] of Object.entries(edit.cells)) {
      validText(value); if (value.length > 32767) throw new Error("Excel cells support at most 32,767 characters.");
      const match = /^(\d{1,3})!([A-Z]{1,3}[1-9]\d{0,5})$/.exec(key), doc = match && parsed.docs.find(item => item.index === Number(match[1]));
      if (!doc || parsed.model.sheets.find(sheet => sheet.id === doc.index)?.protected) throw new Error("This cell is unavailable or protected.");
      const ref = match[2], coordinates = address(ref); if (colNumber(coordinates[1]) > 16384 || Number(coordinates[2]) > 1048576) throw new Error("Invalid spreadsheet address.");
      const document = doc.document, namespace = document.documentElement.namespaceURI || ns.s;
      const make = tag => document.createElementNS(namespace, tag);
      let data = first(document, "sheetData"); if (!data) { data = make("sheetData"); document.documentElement.appendChild(data); }
      let row = children(data, "row").find(node => attr(node, "r") === coordinates[2]);
      if (!row) { row = make("row"); row.setAttribute("r", coordinates[2]); data.insertBefore(row, children(data, "row").find(node => Number(attr(node, "r")) > Number(coordinates[2])) || null); }
      let cell = children(row, "c").find(node => attr(node, "r") === ref);
      if (!cell) { cell = make("c"); cell.setAttribute("r", ref); row.insertBefore(cell, children(row, "c").find(node => colNumber(address(attr(node, "r"))?.[1] || "") > colNumber(coordinates[1])) || null); }
      for (const node of children(cell)) if (["v", "f", "is"].includes(node.localName)) cell.removeChild(node);
      cell.removeAttribute("t");
      if (value.startsWith("=") && value.length > 1) { const formula = make("f"); formula.textContent = value.slice(1); cell.appendChild(formula); }
      else if (/^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(value) && Number.isFinite(Number(value))) { const number = make("v"); number.textContent = value; cell.appendChild(number); }
      else { cell.setAttribute("t", "inlineStr"); const inline = make("is"), content = make("t"); content.setAttributeNS("http://www.w3.org/XML/1998/namespace", "xml:space", "preserve"); content.textContent = value; inline.appendChild(content); cell.appendChild(inline); }
    }
    for (const doc of parsed.docs) {
      for (const cell of elements(doc.document, "c")) if (children(cell, "f").length) for (const cached of children(cell, "v")) cell.removeChild(cached);
      write(files, doc.part, doc.document);
    }
    let calc = first(parsed.document, "calcPr"); if (!calc) { calc = parsed.document.createElementNS(parsed.document.documentElement.namespaceURI || ns.s, "calcPr"); parsed.document.documentElement.appendChild(calc); }
    calc.setAttribute("fullCalcOnLoad", "1"); calc.setAttribute("forceFullCalc", "1"); calc.setAttribute("calcMode", "auto"); write(files, parsed.path, parsed.document);
  }
  return Buffer.from(zipSync(files, { level: 6 }));
}
