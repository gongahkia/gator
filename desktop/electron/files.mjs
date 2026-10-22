import { mkdir, open, lstat, readFile, rename, rm } from "node:fs/promises";
import { constants } from "node:fs";
import { basename, dirname, join, resolve, relative, sep, isAbsolute } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { applyContent, inspectContent, fileLimit } from "./file-content.mjs";
import { validUploadID } from "./uploads.mjs";

const digest = bytes => createHash("sha256").update(bytes).digest("hex");
const revisionID = value => typeof value === "string" && /^[a-f0-9-]{36}$/.test(value);
export function fileReference(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("Invalid file selection.");
  if (value.kind === "upload" && Object.keys(value).length === 2 && validUploadID(value.id)) return { kind: "upload", id: value.id };
  if (value.kind === "output" && Object.keys(value).length === 3 && typeof value.bundle === "string" && value.bundle.length <= 8192 && typeof value.path === "string" && value.path.length <= 1024 && value.path && !value.path.includes("\\") && !value.path.startsWith("/") && !value.path.split("/").some(part => ["", ".", ".."].includes(part)) && !/[\x00-\x1f]/.test(value.path)) return { kind: "output", bundle: value.bundle, path: value.path };
  throw new Error("Invalid file selection.");
}
export function validateFileRequest(request) {
  if (!request || typeof request !== "object" || Array.isArray(request) || !["open", "save", "draft", "discard", "restore", "attach", "export"].includes(request.action) || typeof request.context !== "string" || !request.context || request.context.length > 8192 || Object.keys(request).some(key => !["action", "context", "ref", "expected", "version", "edit"].includes(key))) throw new Error("Invalid file request.");
  fileReference(request.ref);
  if (["save", "draft", "discard", "restore"].includes(request.action) && !revisionID(request.expected)) throw new Error("Reopen this file before changing it.");
  if (request.version !== undefined && !revisionID(request.version)) throw new Error("Invalid file version.");
  if (["save", "draft"].includes(request.action) && (!request.edit || typeof request.edit !== "object" || Array.isArray(request.edit) || Buffer.byteLength(JSON.stringify(request.edit)) > fileLimit)) throw new Error("These changes exceed the file editor's limit.");
  return request;
}
// Check every path component before opening. No renderer-supplied filesystem
// path reaches this helper without a matching trusted source descriptor.
export async function readBoundedFile(root, path, limit = fileLimit) {
  root = resolve(root); path = resolve(root, path);
  const suffix = relative(root, path);
  if (!suffix || suffix === ".." || suffix.startsWith(`..${sep}`)) throw new Error("Invalid file location.");
  let current = root;
  for (const part of ["", ...suffix.split(sep)]) { current = part ? join(current, part) : current; const info = await lstat(current); if (info.isSymbolicLink()) throw new Error("Linked files cannot be opened by the inline editor."); }
  const file = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0) | (constants.O_NONBLOCK || 0));
  try {
    const info = await file.stat(); if (!info.isFile() || info.size > limit) throw new Error(`This file exceeds the ${Math.ceil(limit / 1048576)} MiB editor limit.`);
    const bytes = Buffer.alloc(Math.min(info.size + 1, limit + 1)); let size = 0;
    while (size < bytes.length) { const result = await file.read(bytes, size, bytes.length - size, null); if (!result.bytesRead) break; size += result.bytesRead; }
    if (size > limit || size > info.size) throw new Error("The file changed while it was being read. Open it again.");
    return bytes.subarray(0, size);
  } finally { await file.close(); }
}
async function atomic(path, bytes) {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  const temporary = `${path}.${randomUUID()}.tmp`;
  try { const file = await open(temporary, "wx", 0o600); try { await file.writeFile(bytes); await file.sync(); } finally { await file.close(); } await rename(temporary, path); }
  finally { await rm(temporary, { force: true }); }
}

async function acquireSaveLock(directory) {
  const path = join(directory, "saving.lock");
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const lock = await open(path, "wx", 0o600);
      try { await lock.writeFile(JSON.stringify({ pid: process.pid })); }
      catch (error) { await lock.close(); await rm(path, { force: true }); throw error; }
      return lock;
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
      let abandoned = false;
      try {
        const bytes = await readBoundedFile(directory, "saving.lock", 1024);
        if (!bytes.length) abandoned = (await lstat(path)).mtimeMs < Date.now() - 30000;
        else {
          const owner = JSON.parse(bytes.toString());
          if (Number.isInteger(owner.pid) && owner.pid > 0) {
            try { process.kill(owner.pid, 0); }
            catch (error) { abandoned = error.code === "ESRCH"; }
          }
        }
      } catch (error) { if (error.code === "ENOENT") continue; throw error; }
      if (!abandoned) break;
      await rm(path, { force: true });
    }
  }
  throw new Error("Another editor is saving this file. Try again shortly; your edits are kept.");
}

export class FileStore {
  constructor(root) { this.root = resolve(root); this.pending = new Map(); this.aliases = new Map(); }
  sourceIdentity(value) {
    const ref = fileReference(value);
    if (ref.kind === "output") {
      // Bundle paths change when a private profile is restored elsewhere. Bind
      // versions to the bundle's stable location inside that profile instead.
      const portable = relative(dirname(this.root), ref.bundle);
      if (portable && portable !== ".." && !portable.startsWith(`..${sep}`) && !isAbsolute(portable)) {
        ref.bundle = portable.split(sep).join("/");
      }
    }
    return digest(JSON.stringify(ref));
  }
  identity(ref) { const source = this.sourceIdentity(ref); return this.aliases.get(source)?.target ?? source; }
  directory(ref) { return join(this.root, this.identity(ref)); }
  async lookup(ref) {
    const source = this.sourceIdentity(ref);
    if (!this.aliases.has(source)) try {
      const link = JSON.parse((await readBoundedFile(join(this.root, source), "link.json", 2048)).toString());
      if (!/^[a-f0-9]{64}$/.test(link.target) || !/^[a-f0-9]{64}$/.test(link.hash)) throw new Error("Invalid retained file link.");
      this.aliases.set(source, link);
    } catch (error) { if (error.code !== "ENOENT") throw error; }
  }
  async link(ref, original, bytes) {
    await this.lookup(original);
    const link = { target: this.identity(original), hash: digest(bytes) };
    await atomic(join(this.root, this.sourceIdentity(ref), "link.json"), JSON.stringify(link));
    this.aliases.set(this.sourceIdentity(ref), link);
  }
  async original(ref) {
    await this.lookup(ref);
    const metadata = JSON.parse((await readBoundedFile(this.directory(ref), "file.json", 1024 * 1024)).toString());
    const hash = this.aliases.get(this.sourceIdentity(ref))?.hash;
    const version = hash ? metadata.versions.find(version => version.hash === hash) : metadata.versions[0];
    if (!version) throw new Error("The attached version is unavailable. Reattach the file to continue.");
    return this.contents(ref, metadata, version.id);
  }
  async flush() { await Promise.all([...this.pending.values()]); }
  async retain(ref, name, bytes) {
    await this.lookup(ref);
    const directory = this.directory(ref); await mkdir(directory, { recursive: true, mode: 0o700 });
    const path = join(directory, "file.json");
    try { const metadata = JSON.parse((await readBoundedFile(directory, "file.json", 1024 * 1024)).toString()); if ((this.aliases.get(this.sourceIdentity(ref))?.hash ?? metadata.original) !== digest(bytes)) throw new Error("The original file has changed. Your existing edits are retained; reattach the changed file to work on it separately."); return metadata; }
    catch (error) { if (error.code !== "ENOENT") throw error; }
    const version = { id: randomUUID(), hash: digest(bytes), bytes: bytes.length, at: new Date().toISOString(), author: "Original" };
    const metadata = { schema: 1, name: basename(name), ref: fileReference(ref), original: version.hash, current: version.id, versions: [version] };
    await atomic(join(directory, `${version.id}.bin`), bytes); await atomic(path, JSON.stringify(metadata)); return metadata;
  }
  async contents(ref, metadata, version = metadata.current) {
    const entry = metadata.versions.find(item => item.id === version); if (!entry || !revisionID(entry.id)) throw new Error("This file version is unavailable.");
    const bytes = await readBoundedFile(this.directory(ref), `${entry.id}.bin`);
    if (digest(bytes) !== entry.hash) throw new Error("The retained file has changed on disk. Your original is kept.");
    return bytes;
  }
  async view(ref, metadata, version) {
    const selected = version || metadata.current, bytes = await this.contents(ref, metadata, selected);
    let draft; try { draft = JSON.parse((await readBoundedFile(this.directory(ref), "draft.json")).toString()); } catch (error) { if (error.code !== "ENOENT") throw error; }
    return { id: this.identity(ref), name: metadata.name, current: metadata.current, revision: selected, versions: metadata.versions, ...inspectContent(metadata.name, bytes), draft: draft?.revision === metadata.current && selected === metadata.current ? draft.edit : undefined, staleDraft: draft && draft.revision !== metadata.current ? draft.revision : undefined };
  }
  request(request, source) {
    validateFileRequest(request); const id = this.identity(request.ref);
    const operation = (this.pending.get(id) ?? Promise.resolve()).catch(() => {}).then(() => this.perform(request, source));
    this.pending.set(id, operation);
    void operation.finally(() => { if (this.pending.get(id) === operation) this.pending.delete(id); }).catch(() => {});
    return operation;
  }
  async perform(request, source) {
    validateFileRequest(request); const ref = fileReference(request.ref); await this.lookup(ref); const directory = this.directory(ref);
    const descriptor = await source(ref);
    const original = descriptor.bytes ?? await readBoundedFile(descriptor.root, descriptor.path);
    if (descriptor.sha256 && digest(original) !== descriptor.sha256) throw new Error("This output no longer matches its retained version. Your original is kept.");
    let metadata = await this.retain(ref, descriptor.name, original);
    if (request.action === "open") return this.view(ref, metadata, request.version);
    if (["export", "attach"].includes(request.action)) return { name: metadata.name, path: join(directory, `${request.version || metadata.current}.bin`), bytes: await this.contents(ref, metadata, request.version) };
    const lock = await acquireSaveLock(directory);
    try {
      metadata = JSON.parse((await readBoundedFile(directory, "file.json", 1024 * 1024)).toString());
      if (request.action === "discard") {
        // A stale editor may discard its own draft and reload the latest file,
        // but must never remove another editor's newer draft.
        let draft;
        try { draft = JSON.parse((await readBoundedFile(directory, "draft.json")).toString()); } catch (error) { if (error.code !== "ENOENT") throw error; }
        if (draft?.revision === request.expected) await rm(join(directory, "draft.json"), { force: true });
        return this.view(ref, metadata);
      }
      if (metadata.current !== request.expected) throw new Error("A newer version was saved. Copy the edits you want to keep, then Discard to reload the latest version. Your edits remain here until you decide.");
      if (request.action === "draft") { await atomic(join(directory, "draft.json"), JSON.stringify({ revision: request.expected, edit: request.edit })); return { retained: true }; }
      const previous = await this.contents(ref, metadata);
      const bytes = request.action === "restore" ? await this.contents(ref, metadata, request.version) : applyContent(metadata.name, previous, request.edit);
      if (bytes.length > fileLimit) throw new Error("This file exceeds the 16 MiB editor limit. Your edits are kept.");
      const version = { id: randomUUID(), hash: digest(bytes), bytes: bytes.length, at: new Date().toISOString(), author: request.action === "restore" ? "Restored by you" : "Edited by you" };
      await atomic(join(directory, `${version.id}.bin`), bytes);
      metadata.versions.push(version); metadata.current = version.id;
      await atomic(join(directory, "file.json"), JSON.stringify(metadata)); await rm(join(directory, "draft.json"), { force: true });
      return this.view(ref, metadata);
    } finally { await lock.close(); await rm(join(directory, "saving.lock"), { force: true }); }
  }
}
