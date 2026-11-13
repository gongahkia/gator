import { lstat, readdir } from "node:fs/promises";
import { join } from "node:path";
import { readBoundedFile } from "./files.mjs";
import { validUploadID } from "./uploads.mjs";

function retainInputs(retained, options) {
  const ids = options?.Uploads ?? [];
  if (!Array.isArray(ids) || ids.some(id => !validUploadID(id))) throw new Error("Invalid retained attachment references");
  ids.forEach(id => retained.add(id));
}

async function retainDialogueUploads(conversations, conversation, retained) {
  const directory = join(conversations, conversation, "dialogue");
  let info;
  try { info = await lstat(directory); }
  catch (error) { if (error.code === "ENOENT") return; throw error; }
  if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Retained conversation messages are not an ordinary directory");
  const states = new Set(["received", "routing", "ready", "dispatching", "clarification", "answered", "guided", "delegated", "stopped", "failed", "interrupted"]);
  for (const entry of await readdir(directory, {withFileTypes: true})) {
    if (entry.name.startsWith(".")) continue;
    const match = /^([0-9]{20})-([a-f0-9]{64})\.json$/.exec(entry.name);
    if (!match || !entry.isFile()) throw new Error("Unknown retained conversation message");
    const turn = JSON.parse((await readBoundedFile(conversations, `${conversation}/dialogue/${entry.name}`, 512 * 1024)).toString());
    if (turn.version !== 1 || turn.conversation_id !== conversation || turn.id !== match[2] || !Number.isSafeInteger(turn.sequence) || turn.sequence < 1 || String(turn.sequence).padStart(20, "0") !== match[1] || !states.has(turn.state) || !turn.options || typeof turn.options !== "object" || Array.isArray(turn.options)) throw new Error("Invalid retained conversation message");
    // A retained conversation turn owns its displayed input references even
    // after routing finishes. This is retention, never file-access authority.
    retainInputs(retained, turn.options);
    if (Object.hasOwn(turn.options, "run_options")) {
      const options = turn.options.run_options;
      if (!options || typeof options !== "object" || Array.isArray(options)) throw new Error("Invalid captured conversation inputs");
      retainInputs(retained, options);
    }
  }
}

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
    catch (error) { if (error.code !== "ENOENT") throw error; }
    if (journal !== undefined && (!journal || typeof journal !== "object" || Array.isArray(journal) || journal.version !== 1 || !Array.isArray(journal.requests) || journal.requests.length > 128)) throw new Error("Unknown retained message journal");
    for (const request of journal?.requests ?? []) {
      if (!["queued", "running", "interrupted"].includes(request.state) && !(request.state === "failed" && request.recovery !== undefined)) continue;
      if (request.recovery !== undefined && (!request.recovery || typeof request.recovery !== "object" || Array.isArray(request.recovery) || typeof request.recovery.retry_safe !== "boolean")) throw new Error("Invalid retained request recovery");
      retainInputs(retained, request.options);
			if (request.plan?.input_changes !== undefined) {
				const changes = request.plan.input_changes;
				if (!Array.isArray(changes) || changes.length > 32) throw new Error("Invalid retained input scope history");
				for (const change of changes) {
					if (!change || typeof change !== "object" || !change.before_options || typeof change.before_options !== "object" || Array.isArray(change.before_options)) throw new Error("Invalid retained original inputs");
					retainInputs(retained, change.before_options);
				}
			}
    }
    await retainDialogueUploads(conversations, entry.name, retained);
  }
  return retained;
}
