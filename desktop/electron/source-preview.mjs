import { createHash } from "node:crypto";
import { join } from "node:path";
import { readBoundedFile } from "@gator/application-host/files";

const hash = bytes => createHash("sha256").update(bytes).digest("hex");
const identifier = value => typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(value);
const sourcePath = value => typeof value === "string" && value.startsWith("source/") && value.length <= 1024 && !/[\\\x00-\x1f]/.test(value) && !value.split("/").some(part => !part || part === "." || part === "..");

const evidenceID = value => typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(value);
function webURL(value) {
  if (typeof value !== "string" || value.length > 8192 || /[\x00-\x20]/.test(value)) return;
  try { const url = new URL(value); if (["http:", "https:"].includes(url.protocol) && !url.username && !url.password) return url; } catch {}
}
function describe(entry) {
  const locator = entry.locator;
  const url = webURL(locator);
  const kind = sourcePath(locator) ? "file" : url ? "web" : /^connector:[a-zA-Z0-9_-]+\/[a-zA-Z0-9_-]+$/.test(locator) ? "connected" : locator?.startsWith("attachment/") ? "attachment" : "unsupported";
  const label = kind === "file" || kind === "attachment" ? locator.split("/").at(-1) : url ? url.hostname : kind === "connected" ? locator.slice(10).replace("/", " · ") : "Retained source";
  return { id: entry.id, locator, kind, label: String(label).slice(0, 160), captured_at: Number.isFinite(Date.parse(entry.retrieved_at)) ? entry.retrieved_at : undefined, retained: entry.snapshot_path === `evidence/${entry.sha256}.txt`, ...(url ? { original_url: url.href } : {}) };
}
async function retainedManifest(request, {state,current}) {
  if (!state?.conversation_id || request.context !== `conversation:${state.conversation_id}` || !current()) throw new Error("Return to the original conversation before opening its source.");
  if (!identifier(request.run)) throw new Error("Invalid source run.");
  const bundle = state.messages?.find(message => message.bundle?.RunID === request.run)?.bundle;
  if (!bundle || !/^[a-f0-9]{64}$/.test(bundle.ManifestSHA256 || "")) throw new Error("The retained evidence for this answer is unavailable.");
  const raw = await readBoundedFile(bundle.Path, "manifest.json", 2 * 1024 * 1024);
  if (hash(raw) !== bundle.ManifestSHA256) throw new Error("The retained source manifest has changed.");
  const manifest = JSON.parse(raw);
  if (manifest.run_id !== request.run || !Array.isArray(manifest.evidence)) throw new Error("The source belongs to another answer or its evidence is unavailable.");
  return {bundle,entries:manifest.evidence};
}
// Metadata comes only from the sealed manifest. No source/network/model fetch.
export async function sourceCatalog(request, {state,current=()=>true}) {
  if (!request || Object.keys(request).some(key=>!["context","run"].includes(key))) throw new Error("Invalid source catalogue request.");
  const {entries}=await retainedManifest(request,{state,current});
  const references=[]; let bytes=0, truncated=false;
  for (const entry of entries) {
    if (!evidenceID(entry.id) || typeof entry.locator!=="string" || entry.locator.length>8192 || /[\x00-\x1f]/.test(entry.locator) || !/^[a-f0-9]{64}$/.test(entry.sha256||"")) continue;
    const reference=describe(entry), size=Buffer.byteLength(JSON.stringify(reference));
    if (references.length>=256 || bytes+size>60*1024) {truncated=true;break;}
    references.push(reference);bytes+=size;
  }
  if (!current()) throw new Error("The conversation changed before its sources could be shown.");
  return {run:request.run,references,truncated};
}
// Read-only projection of already selected, sealed evidence. Never open the
// live workspace, follow a model-supplied absolute path, or expose private replay.
export async function sourcePreview(request, { state, stateRoot, current = () => true }) {
  if (!request || Object.keys(request).some(key => !["context", "run", "path", "start", "end"].includes(key)) || typeof request.path!=="string" || !(sourcePath(request.path) || request.path.startsWith("evidence:") && evidenceID(request.path.slice(9)) || webURL(request.path) || /^connector:[a-zA-Z0-9_-]+\/[a-zA-Z0-9_-]+$/.test(request.path))) throw new Error("Choose a retained source reference in this conversation.");
  for (const key of ["start", "end"]) if (request[key] !== undefined && (!Number.isSafeInteger(request[key]) || request[key] < 1 || request[key] > 1000000)) throw new Error("Invalid cited line range.");
  if (request.end !== undefined && request.end < (request.start ?? 1)) throw new Error("Invalid cited line range.");
  const {bundle,entries}=await retainedManifest(request,{state,current});
  const matches=entries.filter(entry=>request.path.startsWith("evidence:") ? entry.id===request.path.slice(9) : entry.locator===request.path);
  if (matches.length!==1) throw new Error(matches.length ? "This reference matches multiple captures. Choose its exact evidence ID." : "This source was not retained as read evidence for this answer.");
  const evidence=matches[0];
  if(typeof evidence.locator!=="string" || evidence.locator.length>8192 || /[\x00-\x1f]/.test(evidence.locator)) throw new Error("Invalid retained source identity.");
  const metadata=describe(evidence);
  if(metadata.kind==="unsupported") throw new Error("This retained source format is unsupported.");
  if (!/^[a-f0-9]{64}$/.test(evidence.sha256 || "") || evidence.snapshot_path !== `evidence/${evidence.sha256}.txt`) throw new Error("A retained text excerpt is unavailable for this source; binary/PDF page capture may be unsupported.");
  const bytes = await readBoundedFile(bundle.Path, evidence.snapshot_path, 512 * 1024);
  if (hash(bytes) !== evidence.sha256) throw new Error("The retained source bytes have changed.");
  const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  if (text.includes("\0")) throw new Error("A text preview is unavailable for this source.");
  const lines = text.split("\n"), reads = [];
  let observedPreview;
  try {
    const history = JSON.parse(await readBoundedFile(join(stateRoot, "gator/work-history"), `${request.run}.json`, 128 * 1024));
    if (history.id !== request.run || history.conversation_id !== state.conversation_id || !identifier(history.revision_id)) throw new Error();
    const revision = JSON.parse(await readBoundedFile(join(stateRoot, "gator/conversations", state.conversation_id, "revisions"), `${history.revision_id}.json`, 8 * 1024 * 1024));
    if (revision.conversation_id !== state.conversation_id || revision.bundle_path !== bundle.Path) throw new Error();
    const messages = revision.replay?.messages ?? [], calls = new Map();
    const turn = messages.slice(messages.findLastIndex(message => message.role === "user") + 1);
    for (const message of turn) {
      for (const call of message.tool_calls ?? []) calls.set(call.id, call.name);
      if (message.role !== "tool") continue;
      const tool = message.tool_name || calls.get(message.tool_call_id);
      let result; try { const value = JSON.parse(message.content); result = value.ok ? value.result : value; } catch { continue; }
      if (["read_evidence", "web_fetch"].includes(tool) && result?.evidence?.id === evidence.id && result.evidence.sha256 === evidence.sha256 && typeof result.text === "string") {
        if(tool === "web_fetch" && result.preview_format === "readable_text" && Buffer.byteLength(result.text,"utf8")<=16384 && !result.text.includes("\0")) observedPreview=result.text;
        const offset = tool === "read_evidence" ? result.offset_bytes : 0;
        const fragment = Buffer.from(result.text, "utf8");
        if (Number.isSafeInteger(offset) && offset >= 0 && fragment.length && offset + fragment.length <= bytes.length && bytes.subarray(offset, offset + fragment.length).equals(fragment)) {
          const prefix = new TextDecoder("utf-8", { fatal: true }).decode(bytes.subarray(0, offset));
          const start = prefix.split("\n").length, startColumn = prefix.split("\n").at(-1).length;
          let end = start + result.text.split("\n").length - 1;
          if (result.text.endsWith("\n")) end--;
          const last = result.text.endsWith("\n") ? result.text.split("\n").at(-2) : result.text.split("\n").at(-1);
          const endColumn = end === start ? startColumn + last.length : last.length;
          if (reads.length < 64) reads.push({ start, end, truncated: !!result.truncated, ...(startColumn ? { startColumn } : {}), ...(endColumn < lines[end-1].length ? {endColumn} : {}) });
        }
        continue;
      }
      if (tool !== "read_file") continue;
      if (!result || result.path !== evidence.locator || !Number.isSafeInteger(result.start_line) || !Number.isSafeInteger(result.end_line) || result.start_line < 1 || result.end_line < result.start_line || result.end_line > lines.length || typeof result.content !== "string") continue;
      const exact = lines.slice(result.start_line - 1, result.end_line).join("\n");
      if (result.truncated ? !exact.startsWith(result.content) : exact !== result.content) continue;
      const end = Math.min(result.end_line, result.start_line + result.content.split("\n").length - 1);
      const last = result.content.split("\n").at(-1), endColumn = last.length < lines[end - 1].length ? last.length : undefined;
      if (reads.length < 64) reads.push({ start: result.start_line, end, truncated: !!result.truncated, ...(endColumn !== undefined ? { endColumn } : {}) });
    }
  } catch { /* Compacted/old replay may lack ranges; never invent a read claim. */ }
  const cited = request.start !== undefined ? { start: request.start, end: request.end ?? request.start } : undefined;
  if (cited && cited.end > lines.length) throw new Error("The cited lines are outside the retained file.");
  const anchor = cited?.start ?? reads[0]?.start ?? 1;
  const firstLine = Math.max(1, anchor - 15), selected = lines.slice(firstLine - 1, firstLine - 1 + 400);
  let preview = selected.join("\n"), truncated = firstLine > 1 || firstLine - 1 + selected.length < lines.length;
  if (Buffer.byteLength(preview) > 64 * 1024) { preview = new TextDecoder().decode(Buffer.from(preview).subarray(0, 64 * 1024), { stream: true }); truncated = true; }
  if (!current()) throw new Error("The conversation changed before this source could be shown.");
  return { ...metadata, run: request.run, path: evidence.locator, sha256: evidence.sha256, text: preview, firstLine, totalLines: lines.length, truncated, cited, reads, observed_preview: observedPreview, notice: reads.length ? undefined : metadata.kind === "connected" ? "The connected response is retained. An exact read range is unavailable; this is not proof every field was inspected." : metadata.kind === "web" ? "Fetched source retained. An exact read range is unavailable for the normalized preview or this older record." : "The original source snapshot is retained, but an exact read range is unavailable in this older or compacted record. Cited lines are the answer’s reference, not proof of a tool read." };
}
