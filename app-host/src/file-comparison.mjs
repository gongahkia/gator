import { inspectContent } from "./file-content.mjs";

const maxChanges = 200, maxField = 6000, maxOutput = 256 * 1024;
const supportedText = new Set(["text", "markdown"]);
const cellName = (col) => { let name = ""; do { name = String.fromCharCode(65 + col % 26) + name; col = Math.floor(col / 26) - 1; } while (col >= 0); return name; };

// Compare only content we can interpret. Binary equality is a separate fact:
// unchanged text must never be reported as unchanged native layout or bytes.
export function compareFileContent(name, beforeBytes, afterBytes) {
  const result = { identical: beforeBytes.equals(afterBytes), changes: [], truncated: false, notices: [] };
  if (result.identical) return result;
  if (/\.(?:pdf|png|jpe?g|webp)$/i.test(name)) {
    result.notices.push("These file bytes differ. Export both versions to compare their visual content in a native app.");
    return result;
  }
  if (Math.max(beforeBytes.length, afterBytes.length) > 4 * 1024 * 1024) {
    result.truncated = true;
    result.notices.push("These versions exceed the 4 MiB inline comparison limit. The files remain available to preview or export for a full comparison.");
    return result;
  }
  let output = 0;
  const add = (location, before, after, lines) => {
    if (before === after && (!lines || lines.beforeLines === lines.afterLines)) return;
    if (result.changes.length >= maxChanges || output >= maxOutput) { result.truncated = true; return; }
    if (before.length > maxField || after.length > maxField) result.truncated = true;
    const change = { location: location.slice(0, 300), before: before.slice(0, maxField), after: after.slice(0, maxField), ...lines };
    const size = Buffer.byteLength(JSON.stringify(change));
    if (output + size > maxOutput) { result.truncated = true; return; }
    output += size;
    result.changes.push(change);
  };
  let before, after;
  try { before = inspectContent(name, beforeBytes); after = inspectContent(name, afterBytes); }
  catch { result.notices.push("A content comparison is unavailable for this file. Export the selected versions to compare them in their native app."); return result; }
  if (before.kind !== after.kind) { result.notices.push("The file formats differ. Export both versions to inspect the change."); return result; }
  if (supportedText.has(before.kind)) {
    const a = before.text.split(/\r\n|\n|\r/), b = after.text.split(/\r\n|\n|\r/);
    let first = 0, lastA = a.length, lastB = b.length;
    while (first < lastA && first < lastB && a[first] === b[first]) first++;
    while (lastA > first && lastB > first && a[lastA - 1] === b[lastB - 1]) { lastA--; lastB--; }
    const n = lastA - first, m = lastB - first;
    if (n * m > 1_000_000 || n + m > 20_000) {
      result.truncated = true;
      result.notices.push("The changed section is too large for an inline line comparison. Export both versions for a full comparison.");
    } else {
      const width = m + 1, lcs = new Uint16Array((n + 1) * width);
      for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) lcs[i * width + j] = a[first + i] === b[first + j] ? 1 + lcs[(i + 1) * width + j + 1] : Math.max(lcs[(i + 1) * width + j], lcs[i * width + j + 1]);
      let i = 0, j = 0;
      while (i < n || j < m) {
        if (i < n && j < m && a[first + i] === b[first + j]) { i++; j++; continue; }
        const startA = i, startB = j, removed = [], added = [];
        while (i < n || j < m) {
          if (i < n && j < m && a[first + i] === b[first + j]) break;
          if (j < m && (i === n || lcs[i * width + j + 1] > lcs[(i + 1) * width + j])) added.push(b[first + j++]);
          else removed.push(a[first + i++]);
        }
        const range = (start, count) => count > 1 ? `${first + start + 1}–${first + start + count}` : `${first + start + 1}`;
        add(`Lines ${range(startA, i - startA)} → ${range(startB, j - startB)}`, removed.join("\n"), added.join("\n"), { beforeLines: removed.length, afterLines: added.length });
      }
    }
    if (!result.changes.length && !result.truncated) result.notices.push("Text matches; encoding or line endings differ. The file bytes are different.");
  } else if (["table", "word", "workbook", "slides"].includes(before.kind)) {
    const units = value => {
      const entries = [];
      if (value.kind === "table") value.rows.forEach((row, r) => row.forEach((text, c) => { if (text) entries.push([`${r}:${c}`, `${cellName(c)}${r + 1}`, text]); }));
      if (value.kind === "word") value.blocks.forEach(block => {
        const paragraphs = block.type === "paragraph" ? [block] : block.rows.flat(2);
        paragraphs.forEach(p => entries.push([p.id, `Paragraph ${Number(p.id.split(":").at(-1)) + 1}`, p.text]));
      });
      if (value.kind === "workbook") value.sheets.forEach((sheet, i) => {
        entries.push([`sheet:${i}`, `Sheet ${i + 1} name`, sheet.name]);
        sheet.cells.forEach(cell => { const text = cell.formula ? `=${cell.formula}` : cell.value; if (text) entries.push([`${i}:${cell.address}`, `${sheet.name}!${cell.address}`, text]); });
      });
      if (value.kind === "slides") value.slides.forEach((slide, i) => slide.shapes.forEach(shape => shape.paragraphs.forEach(p => entries.push([p.id, `Slide ${i + 1} · Text ${Number(p.id.split(":").at(-1)) + 1}`, p.text]))));
      return new Map(entries.map(([id, location, text]) => [id, { location, text }]));
    };
    const a = units(before), b = units(after);
    for (const id of new Set([...a.keys(), ...b.keys()])) add((b.get(id) ?? a.get(id)).location, a.get(id)?.text ?? "", b.get(id)?.text ?? "");
    result.notices.push(before.kind === "workbook" ? "Compares entered cell values and formula expressions. Cached results, formatting, charts and layout are not compared." : before.kind === "table" ? "Compares cell values. Quoting and line endings are not compared." : "Compares paragraph text at its recorded position. Formatting, images and page or slide layout are not compared; inserted paragraphs can shift later positions.");
    if (!result.changes.length) result.notices.push("No supported content changes were found. The file bytes still differ.");
  } else {
    result.notices.push("These file bytes differ. Export both versions to compare their visual content in a native app.");
  }
  if (result.truncated) result.notices.push("This is a partial comparison. Some changes or long values are omitted; export both versions for the full files.");
  return result;
}
