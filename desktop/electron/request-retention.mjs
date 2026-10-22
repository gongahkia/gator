import { readdir } from "node:fs/promises";
import { readBoundedFile } from "./files.mjs";
import { validUploadID } from "./uploads.mjs";

// Cleanup reads references only; Go remains the owner of admission and task
// state. An unreadable journal defers pruning instead of deleting uncertain data.
export async function retainedRequestUploads(conversations) {
  const retained = new Set();
  let entries;
  try { entries = await readdir(conversations, { withFileTypes: true }); }
  catch (error) { if (error.code === "ENOENT") return retained; throw error; }
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    let journal;
    try { journal = JSON.parse((await readBoundedFile(conversations, `${entry.name}/requests.json`, 34 * 1024 * 1024)).toString()); }
    catch (error) { if (error.code === "ENOENT") continue; throw error; }
    if (journal.version !== 1 || !Array.isArray(journal.requests) || journal.requests.length > 128) throw new Error("Unknown retained message journal");
    for (const request of journal.requests) {
      if (!["queued", "running", "interrupted"].includes(request.state)) continue;
      const ids = request.options?.Uploads ?? [];
      if (!Array.isArray(ids) || ids.some(id => !validUploadID(id))) throw new Error("Invalid retained attachment references");
      ids.forEach(id => retained.add(id));
    }
  }
  return retained;
}
