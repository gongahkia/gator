import { createHash } from "node:crypto";
import { join } from "node:path";
import { readBoundedFile } from "@gator/application-host/files";

const hash = bytes => createHash("sha256").update(bytes).digest("hex");
const identifier = value => typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(value);
const sourcePath = value => typeof value === "string" && value.startsWith("source/") && value.length <= 1024 && !/[\\\x00-\x1f]/.test(value) && !value.split("/").some(part => !part || part === "." || part === "..");

// Read-only projection of already selected, sealed evidence. Never open the
// live workspace, follow a model-supplied absolute path, or expose private replay.
export async function sourcePreview(request, { state, stateRoot, current = () => true }) {
  if (!request || Object.keys(request).some(key => !["context", "run", "path", "start", "end"].includes(key)) || !identifier(request.run) || !sourcePath(request.path)) throw new Error("Choose a retained source reference in this conversation.");
  if (!state?.conversation_id || request.context !== `conversation:${state.conversation_id}` || !current()) throw new Error("Return to the original conversation before opening its source.");
  for (const key of ["start", "end"]) if (request[key] !== undefined && (!Number.isSafeInteger(request[key]) || request[key] < 1 || request[key] > 1000000)) throw new Error("Invalid cited line range.");
  if (request.end !== undefined && request.end < (request.start ?? 1)) throw new Error("Invalid cited line range.");
  const bundle = state.messages?.find(message => message.bundle?.RunID === request.run)?.bundle;
  if (!bundle || !/^[a-f0-9]{64}$/.test(bundle.ManifestSHA256 || "")) throw new Error("The retained evidence for this answer is unavailable.");
  const raw = await readBoundedFile(bundle.Path, "manifest.json", 2 * 1024 * 1024);
  if (hash(raw) !== bundle.ManifestSHA256) throw new Error("The retained source manifest has changed.");
  const manifest = JSON.parse(raw);
  if (manifest.run_id !== request.run) throw new Error("The source belongs to another answer.");
  const evidence = manifest.evidence?.find(entry => entry.locator === request.path);
  if (!evidence || !/^[a-f0-9]{64}$/.test(evidence.sha256 || "") || evidence.snapshot_path !== `evidence/${evidence.sha256}.txt`) throw new Error("This file was not retained as read evidence for this answer.");
  const bytes = await readBoundedFile(bundle.Path, evidence.snapshot_path, 512 * 1024);
  if (hash(bytes) !== evidence.sha256) throw new Error("The retained source bytes have changed.");
  const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  if (text.includes("\0")) throw new Error("A text preview is unavailable for this source.");
  const lines = text.split("\n"), reads = [];
  try {
    const history = JSON.parse(await readBoundedFile(join(stateRoot, "gator/work-history"), `${request.run}.json`, 128 * 1024));
    if (history.id !== request.run || history.conversation_id !== state.conversation_id || !identifier(history.revision_id)) throw new Error();
    const revision = JSON.parse(await readBoundedFile(join(stateRoot, "gator/conversations", state.conversation_id, "revisions"), `${history.revision_id}.json`, 8 * 1024 * 1024));
    if (revision.conversation_id !== state.conversation_id || revision.bundle_path !== bundle.Path) throw new Error();
    const messages = revision.replay?.messages ?? [], calls = new Map();
    const turn = messages.slice(messages.findLastIndex(message => message.role === "user") + 1);
    for (const message of turn) {
      for (const call of message.tool_calls ?? []) calls.set(call.id, call.name);
      if (message.role !== "tool" || (message.tool_name || calls.get(message.tool_call_id)) !== "read_file") continue;
      let result; try { const value = JSON.parse(message.content); if (value.ok) result = value.result; } catch { continue; }
      if (!result || result.path !== request.path || !Number.isSafeInteger(result.start_line) || !Number.isSafeInteger(result.end_line) || result.start_line < 1 || result.end_line < result.start_line || result.end_line > lines.length || typeof result.content !== "string") continue;
      const exact = lines.slice(result.start_line - 1, result.end_line).join("\n");
      if (result.truncated ? !exact.startsWith(result.content) : exact !== result.content) continue;
      const end = Math.min(result.end_line, result.start_line + result.content.split("\n").length - 1);
      if (reads.length < 64) reads.push({ start: result.start_line, end, truncated: !!result.truncated });
    }
  } catch { /* Compacted/old replay may lack ranges; never invent a read claim. */ }
  const cited = request.start !== undefined ? { start: request.start, end: request.end ?? request.start } : undefined;
  if (cited && cited.end > lines.length) throw new Error("The cited lines are outside the retained file.");
  const anchor = cited?.start ?? reads[0]?.start ?? 1;
  const firstLine = Math.max(1, anchor - 15), selected = lines.slice(firstLine - 1, firstLine - 1 + 400);
  let preview = selected.join("\n"), truncated = firstLine > 1 || firstLine - 1 + selected.length < lines.length;
  if (Buffer.byteLength(preview) > 64 * 1024) { preview = new TextDecoder().decode(Buffer.from(preview).subarray(0, 64 * 1024), { stream: true }); truncated = true; }
  if (!current()) throw new Error("The conversation changed before this source could be shown.");
  return { run: request.run, path: request.path, sha256: evidence.sha256, text: preview, firstLine, totalLines: lines.length, truncated, cited, reads, notice: reads.length ? undefined : "The original source snapshot is retained, but an exact read range is unavailable in this older or compacted record. Cited lines are the answer’s reference, not proof of a tool read." };
}
