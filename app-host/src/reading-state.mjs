import { constants } from "node:fs";
import { mkdir, open, rename, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { createHash, randomUUID } from "node:crypto";

const contextOK = value => typeof value === "string" && /^(conversation|workspace):.+$/.test(value) && value.length <= 8192;
const digestOK = value => typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
const numberOK = value => Number.isSafeInteger(value) && value >= 0 && value <= 1e9;
export function validReadingAnchor(value) {
  return !!value && typeof value === "object" && !Array.isArray(value)
    && Object.keys(value).every(key => ["key", "index", "offset", "following", "seen", "seen_version"].includes(key))
    && digestOK(value.key) && numberOK(value.index) && Number.isSafeInteger(value.offset) && value.offset >= -1024 && value.offset <= 1e9 && typeof value.following === "boolean"
    && digestOK(value.seen) && digestOK(value.seen_version);
}

// Presentation state only: no conversation text, draft, model setting, action
// token, permission or execution state. Each profile/chat has its own file;
// simultaneous terminals cannot overwrite another chat's reading position.
export class ReadingStore {
  #pending = Promise.resolve();
  constructor(root) { this.root = root; }
  #file(context) {
    if (!contextOK(context)) throw new Error("Invalid reading context.");
    return join(this.root, createHash("sha256").update(context).digest("hex") + ".json");
  }
  async read(context) {
    await this.#pending;
    const path = this.#file(context);
    let file;
    try {
      file = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0));
      const meta = await file.stat();
      if (!meta.isFile() || meta.size > 4096) throw new Error("Invalid saved reading position.");
      const bytes = Buffer.alloc(4097), count = (await file.read(bytes, 0, bytes.length, 0)).bytesRead;
      if (count > 4096) throw new Error("Invalid saved reading position.");
      const record = JSON.parse(bytes.subarray(0, count).toString("utf8"));
      if (record.version !== 1 || !validReadingAnchor(record.anchor)) throw new Error("Invalid saved reading position.");
      return record.anchor;
    } catch (error) {
      if (error.code === "ENOENT") return undefined;
      throw new Error("Could not restore this chat’s reading position. History and drafts are kept; /latest resumes at the newest content.");
    } finally { await file?.close(); }
  }
  save(context, anchor) {
    const path = this.#file(context);
    if (!validReadingAnchor(anchor)) return Promise.reject(new Error("Invalid reading position."));
    // Capture the reviewed presentation data before the asynchronous write.
    const bytes = JSON.stringify({ version: 1, anchor });
    const operation = this.#pending.then(async () => {
      await mkdir(this.root, { recursive: true, mode: 0o700 });
      const temporary = path + "." + randomUUID() + ".tmp";
      try { await writeFile(temporary, bytes, { mode: 0o600, flag: "wx" }); await rename(temporary, path); }
      finally { await rm(temporary, { force: true }); }
    });
    this.#pending = operation.catch(() => {});
    return operation;
  }
  flush() { return this.#pending; }
}
