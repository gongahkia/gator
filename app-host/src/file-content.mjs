import { extname } from "node:path";
import { inspectOffice, editOffice } from "./office.mjs";

export const fileLimit = 16 * 1024 * 1024;
const textExtensions = new Set("txt md markdown rst json jsonl yaml yml toml xml html htm svg log ini cfg conf js jsx ts tsx py go rs java c h cpp hpp css scss sh bash zsh sql rb swift kt vue svelte tex r gitignore".split(" "));
const decode = bytes => new TextDecoder("utf-8", { fatal: true }).decode(bytes);
export function parseTable(text, delimiter = ",") {
  const rows = []; let row = [], cell = "", quoted = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (ch === '"') { if (quoted && text[i + 1] === '"') { cell += '"'; i++; } else if (quoted || !cell) quoted = !quoted; else throw new Error("This table has an invalid quoted field."); }
    else if (!quoted && (ch === delimiter || ch === "\n" || ch === "\r")) {
      row.push(cell); cell = "";
      if (ch !== delimiter) { rows.push(row); row = []; if (ch === "\r" && text[i + 1] === "\n") i++; }
    } else cell += ch;
    if (cell.length > 256 * 1024 || row.length > 256 || rows.length > 10000) throw new Error("This table exceeds the inline editor's 10,000 row or 256 column limit.");
  }
  if (quoted) throw new Error("This table has an unterminated quoted field.");
  if (cell || row.length || !rows.length) { row.push(cell); rows.push(row); }
  if (rows.reduce((sum, row) => sum + row.length, 0) > 100000) throw new Error("This table exceeds the inline editor's 100,000 cell limit.");
  return rows;
}
export function encodeTable(rows, delimiter = ",", newline = "\n") {
  return rows.map(row => row.map(value => value.includes(delimiter) || /["\r\n]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value).join(delimiter)).join(newline) + newline;
}
export function inspectContent(name, bytes) {
  const extension = extname(name).slice(1).toLowerCase();
  if (["docx", "xlsx", "pptx"].includes(extension)) return inspectOffice(extension, bytes);
  if (["png", "jpg", "jpeg", "webp"].includes(extension)) {
    const mime = bytes.subarray(0, 8).equals(Buffer.from([137,80,78,71,13,10,26,10])) ? "image/png" : bytes[0] === 255 && bytes[1] === 216 ? "image/jpeg" : bytes.subarray(0,4).toString() === "RIFF" && bytes.subarray(8,12).toString() === "WEBP" ? "image/webp" : "";
    if (!mime) throw new Error("The image does not match a supported PNG, JPEG or WebP file.");
    return { kind: "image", editable: false, data: `data:${mime};base64,${bytes.toString("base64")}` };
  }
  if (extension === "pdf") {
    if (!bytes.subarray(0,1024).includes(Buffer.from("%PDF-"))) throw new Error("This file is not a valid PDF.");
    return { kind: "pdf", editable: false, data: bytes.toString("base64") };
  }
  if (!textExtensions.has(extension) && !["csv", "tsv"].includes(extension)) return { kind: "unsupported", editable: false, notice: "Preview is unavailable for this format. You can export the original file." };
  const text = decode(bytes);
  if (text.includes("\0")) throw new Error("Binary content cannot be opened in the text editor.");
  if (["csv", "tsv"].includes(extension)) return { kind: "table", editable: true, rows: parseTable(text, extension === "tsv" ? "\t" : ",") };
  return { kind: ["md", "markdown"].includes(extension) ? "markdown" : "text", editable: true, text, language: extension };
}
export function applyContent(name, bytes, edit) {
  if (!edit || typeof edit !== "object" || Array.isArray(edit)) throw new Error("Invalid file changes.");
  const extension = extname(name).slice(1).toLowerCase();
  if (["docx", "xlsx", "pptx"].includes(extension)) return editOffice(extension, bytes, edit);
  const original = inspectContent(name, bytes);
  if (!original.editable) throw new Error("This format is available for preview only.");
  let text;
  if (original.kind === "table") {
    if (!edit.cells || typeof edit.cells !== "object" || Array.isArray(edit.cells)) throw new Error("Invalid table changes.");
    const rows = original.rows.map(row => [...row]);
    for (const [key, value] of Object.entries(edit.cells)) {
      if (!/^\d{1,5}:\d{1,3}$/.test(key) || typeof value !== "string" || value.length > 256 * 1024) throw new Error("Invalid table cell.");
      const [row, col] = key.split(":").map(Number);
      if (row >= 10000 || col >= 256) throw new Error("This cell is outside the supported table area.");
      while (rows.length <= row) rows.push([]);
      while (rows[row].length <= col) rows[row].push(""); rows[row][col] = value;
    }
    text = encodeTable(rows, extension === "tsv" ? "\t" : ",", decode(bytes).includes("\r\n") ? "\r\n" : "\n");
  } else {
    if (typeof edit.text !== "string") throw new Error("Invalid text changes.");
    text = edit.text;
    if (extension === "json") { try { JSON.parse(text); } catch { throw new Error("Fix the JSON syntax before saving. Your edits are kept."); } }
  }
  if (text.includes("\0")) throw new Error("Text files cannot contain null bytes.");
  const bom = bytes[0] === 239 && bytes[1] === 187 && bytes[2] === 191;
  return Buffer.from((bom ? "\ufeff" : "") + text, "utf8");
}
