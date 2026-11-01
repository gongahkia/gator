import { createHash } from "node:crypto";
import { join } from "node:path";
import { readBoundedFile } from "@gator/application-host/files";
const hash = bytes => createHash("sha256").update(bytes).digest("hex");
const identifier = value => typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(value);
// Exact, sealed candidate evidence only. No Git subprocess, live source read,
// editable file version, or renderer-provided filesystem path is involved.
export async function codePreview(request, { state, current = () => true }) {
  if (!request || Object.keys(request).some(key => !["context", "run", "candidate"].includes(key)) || !identifier(request.run) || !identifier(request.candidate)) throw new Error("Choose a retained code candidate.");
  if (!state?.conversation_id || request.context !== `conversation:${state.conversation_id}` || !current()) throw new Error("Return to the original conversation to inspect this change.");
  const bundles = [...(state.messages ?? []).flatMap(message => message.bundle ? [message.bundle] : []), state.work_details?.latest_bundle].filter(Boolean);
  const bundle = bundles.find(value => value.RunID === request.run && value.Candidates?.some(candidate => candidate.ID === request.candidate));
  if (!bundle || !/^[a-f0-9]{64}$/.test(bundle.ManifestSHA256 || "")) throw new Error("The retained code evidence is unavailable.");
  const raw = await readBoundedFile(bundle.Path, "manifest.json", 2 * 1024 * 1024);
  if (hash(raw) !== bundle.ManifestSHA256) throw new Error("The retained manifest has changed.");
  const manifest = JSON.parse(raw), candidate = manifest.code_candidates?.find(value => value.id === request.candidate);
  if (manifest.run_id !== request.run || !candidate || !/^[a-f0-9]{64}$/.test(candidate.sha256 || "") || typeof candidate.patch_path !== "string" || !/^code\/[a-zA-Z0-9_-]+\.patch$/.test(candidate.patch_path)) throw new Error("This candidate has no available retained patch.");
  const bytes = await readBoundedFile(join(bundle.Path, "output"), candidate.patch_path, 16 * 1024 * 1024);
  if (hash(bytes) !== candidate.sha256) throw new Error("The retained patch has changed.");
  const full = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  if (full.includes("\0")) throw new Error("This patch cannot be displayed as text.");
  let patch = new TextDecoder().decode(bytes.subarray(0, 128 * 1024), { stream: true });
  const lines = patch.split("\n"); if (lines.length > 2000) patch = lines.slice(0, 2000).join("\n");
  if (!Array.isArray(candidate.changed_paths) || candidate.changed_paths.length > 4096 || candidate.changed_paths.some(path => typeof path !== "string" || path.length > 4096)) throw new Error("Invalid changed-file evidence.");
  if (!Array.isArray(candidate.verification) || candidate.verification.length > 64) throw new Error("Invalid verification evidence.");
  const verification = candidate.verification.map(check => { if (!Array.isArray(check.argv) || check.argv.length > 128 || check.argv.some(part => typeof part !== "string" || part.length > 4096) || typeof check.passed !== "boolean") throw new Error("Invalid verification evidence."); return { argv: check.argv, passed: check.passed }; });
  if (!current()) throw new Error("The conversation changed before this patch could be shown.");
  const result = { run: request.run, candidate: request.candidate, sha256: candidate.sha256, status: candidate.status, changedPaths: candidate.changed_paths, patch, truncated: patch !== full, verification };
  if (Buffer.byteLength(JSON.stringify(result)) > 256 * 1024) throw new Error("This candidate's metadata exceeds the bounded preview limit.");
  return result;
}
