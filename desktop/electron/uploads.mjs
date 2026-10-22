import { mkdir, open, readdir, rm, stat, writeFile } from "node:fs/promises";
import { basename, extname, join, isAbsolute } from "node:path";
import { constants } from "node:fs";
import { randomBytes } from "node:crypto";

export const uploadLimits = { count: 4, file: 4 * 1024 * 1024, total: 8 * 1024 * 1024 };
export const uploadExtensions = "png jpg jpeg webp pdf docx odt xlsx pptx txt md markdown rst csv tsv json jsonl yaml yml toml xml html htm log ini cfg conf js jsx ts tsx py go rs java c h cpp hpp css scss sh bash zsh sql rb swift kt vue svelte tex r".split(" ");
export const validUploadID = (id) => typeof id === "string" && /^[a-f0-9]{32}\/[^/\\\x00-\x1f]{1,200}$/.test(id) && ![".", ".."].includes(id.split("/")[1]);
export const validUpload = (file) => file && typeof file === "object" && !Array.isArray(file) && Object.keys(file).every((key) => ["id", "name", "size"].includes(key)) && validUploadID(file.id) && file.name === file.id.split("/")[1] && Number.isSafeInteger(file.size) && file.size > 0 && file.size <= uploadLimits.file;
export const uploadContext = (state) => state?.conversation_id ? `conversation:${state.conversation_id}` : state?.source ? `workspace:${state.primary === false ? "topic:" : ""}${state.source}` : "";

// The renderer receives opaque identities for private copies, never a general
// filesystem reader or writer. Originals and project folders stay untouched.
export class UploadStore {
  constructor(root) { this.root = root; }
  async importContent(name, bytes) {
    const id = `${randomBytes(16).toString("hex")}/${name}`;
    if (!validUploadID(id) || !uploadExtensions.includes(extname(name).slice(1).toLowerCase()) || !bytes.length || bytes.length > uploadLimits.file) throw new Error("The saved file can be exported, but chat attachments must be supported files up to 4 MiB.");
    await mkdir(join(this.root, id.split("/")[0]), { recursive: true, mode: 0o700 });
    await writeFile(join(this.root, id), bytes, { mode: 0o600, flag: "wx" });
    return { id, name, size: bytes.length };
  }
  async import(paths) {
    if (!Array.isArray(paths) || !paths.length || paths.length > uploadLimits.count) throw new Error("Choose up to four files per message.");
    const imported = [];
    try {
      let total = 0;
      for (const path of paths) {
        if (typeof path !== "string" || !isAbsolute(path)) throw new Error("Choose a file from your computer.");
        const name = basename(path);
        if (!validUploadID(`${"a".repeat(32)}/${name}`) || !uploadExtensions.includes(extname(name).slice(1).toLowerCase())) throw new Error(`${name}: this file type isn’t supported. Choose a document, text file, PNG, JPEG, or WebP.`);
        const file = await open(path, constants.O_RDONLY | (constants.O_NONBLOCK || 0));
        let bytes;
        try {
          const info = await file.stat();
          if (!info.isFile()) throw new Error(`${name}: choose a regular file.`);
          if (info.size <= 0 || info.size > uploadLimits.file) throw new Error(`${name}: files must be nonempty and no larger than 4 MiB.`);
          const buffer = Buffer.alloc(uploadLimits.file + 1);
          let length = 0;
          while (length < buffer.length) {
            const read = await file.read(buffer, length, buffer.length - length, null);
            if (!read.bytesRead) break;
            length += read.bytesRead;
          }
          if (!length || length > uploadLimits.file) throw new Error(`${name}: files must be nonempty and no larger than 4 MiB.`);
          bytes = buffer.subarray(0, length);
        } finally { await file.close(); }
        total += bytes.length;
        if (total > uploadLimits.total) throw new Error("Attachments must total no more than 8 MiB per message.");
        const directory = randomBytes(16).toString("hex");
        await mkdir(join(this.root, directory), { recursive: true, mode: 0o700 });
        const record = { id: `${directory}/${name}`, name, size: bytes.length };
        imported.push(record);
        await writeFile(join(this.root, record.id), bytes, { mode: 0o600, flag: "wx" });
      }
      return imported;
    } catch (error) {
      await Promise.all(imported.map((file) => rm(join(this.root, file.id.split("/")[0]), { recursive: true, force: true })));
      throw error;
    }
  }
  async check(ids) {
    if (!Array.isArray(ids) || !ids.length || ids.length > uploadLimits.count || new Set(ids).size !== ids.length || ids.some((id) => !validUploadID(id))) throw new Error("Invalid attachment selection.");
    let total = 0;
    for (const id of ids) {
      const info = await stat(join(this.root, id)).catch(() => { throw new Error(`${id.split("/")[1]} is no longer available. Remove it and attach it again.`); });
      if (!info.isFile() || info.size <= 0 || info.size > uploadLimits.file) throw new Error("An attachment is unavailable. Remove it and attach it again.");
      total += info.size;
    }
    if (total > uploadLimits.total) throw new Error("Attachments must total no more than 8 MiB per message.");
  }
  async prune(now = Date.now()) {
    await mkdir(this.root, { recursive: true, mode: 0o700 });
    for (const entry of await readdir(this.root, { withFileTypes: true })) {
      if (!entry.isDirectory() || !/^[a-f0-9]{32}$/.test(entry.name)) continue;
      const path = join(this.root, entry.name);
      if ((await stat(path)).mtimeMs < now - 7 * 86400000) await rm(path, { recursive: true, force: true });
    }
  }
}
