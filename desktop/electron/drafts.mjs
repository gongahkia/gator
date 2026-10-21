import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import { randomUUID } from "node:crypto";

const lifetime = 7 * 24 * 60 * 60 * 1000;
const limit = 32;
const textLimit = 128 * 1024;
const validContext = (value) => typeof value === "string" && /^(conversation|workspace):.+$/.test(value) && value.length <= 8192;

// Unsent text is private local application data, just like saved conversations.
// The renderer never chooses a filename or writes credentials/form fields here.
export class DraftStore {
  #file;
  #pending = Promise.resolve();
  #now;
  constructor(file, now = Date.now) { this.#file = file; this.#now = now; }
  flush() { return this.#pending; }

  request(request) {
    const operation = this.#pending.then(() => this.#request(request));
    this.#pending = operation.catch(() => {});
    return operation;
  }

  async #request(request) {
    if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("Invalid draft request");
    const allowed = { list: ["action"], save: ["action", "draft"], remove: ["action", "context"], clear: ["action"], configure: ["action", "enabled"] }[request.action];
    if (!allowed || Object.keys(request).some((key) => !allowed.includes(key))) throw new Error("Invalid draft action");
    if (request.action === "save") {
      const draft = request.draft;
      if (!draft || typeof draft !== "object" || Array.isArray(draft) || Object.keys(draft).some((key) => !["context", "text", "title", "source"].includes(key)) || !validContext(draft.context) || typeof draft.text !== "string" || Buffer.byteLength(draft.text) > textLimit || typeof draft.title !== "string" || draft.title.length > 512 || typeof draft.source !== "string" || draft.source.length > 8192) throw new Error("Invalid draft");
    }
    if (request.action === "remove" && !validContext(request.context)) throw new Error("Invalid draft identity");
    if (request.action === "configure" && typeof request.enabled !== "boolean") throw new Error("Invalid draft preference");

    let data = { version: 1, enabled: true, drafts: [] };
    try {
      const raw = await readFile(this.#file, "utf8");
      if (Buffer.byteLength(raw) > 6 * 1024 * 1024) throw new Error("Oversized draft file");
      data = JSON.parse(raw);
      if (data.version !== 1 || typeof data.enabled !== "boolean" || !Array.isArray(data.drafts)) throw new Error("Invalid draft file");
    } catch (error) {
      if (error.code !== "ENOENT" && request.action !== "clear" && !(request.action === "configure" && !request.enabled)) throw new Error("Saved drafts could not be read. Clear saved drafts to reset them.");
      if (request.action === "clear" || request.action === "configure") data = { version: 1, enabled: request.action === "clear", drafts: [] };
    }
    const previous = JSON.stringify(data);
    const now = this.#now();
    data.drafts = data.enabled ? data.drafts.filter((draft) => validContext(draft.context) && typeof draft.text === "string" && Buffer.byteLength(draft.text) <= textLimit && typeof draft.title === "string" && typeof draft.source === "string" && Number.isFinite(draft.updated) && draft.updated > now - lifetime && draft.updated <= now).slice(0, limit) : [];
    if (request.action === "configure") { data.enabled = request.enabled; if (!data.enabled) data.drafts = []; }
    if (request.action === "clear") data.drafts = [];
    if (["save", "remove"].includes(request.action)) {
      const context = request.draft?.context ?? request.context;
      data.drafts = data.drafts.filter((draft) => draft.context !== context);
      if (request.action === "save" && data.enabled && request.draft.text.trim()) data.drafts.unshift({ ...request.draft, updated: now });
      data.drafts = data.drafts.slice(0, limit);
    }
    if (previous !== JSON.stringify(data) || ["clear", "configure"].includes(request.action)) {
      await mkdir(dirname(this.#file), { recursive: true, mode: 0o700 });
      const temporary = `${this.#file}.${randomUUID()}.tmp`;
      try { await writeFile(temporary, JSON.stringify(data), { mode: 0o600, flag: "wx" }); await rename(temporary, this.#file); }
      finally { await rm(temporary, { force: true }); }
    }
    return { enabled: data.enabled, drafts: data.drafts };
  }
}
