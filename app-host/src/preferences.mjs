import { constants } from "node:fs";
import { mkdir, mkdtemp, open, readFile, readdir, rename, rm, stat, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { randomUUID } from "node:crypto";

export const frontendDefaults = () => ({ sidebarCollapsed: false, groupChatsByFolder: false, workDetailsSections: { outputs: true, context: false, connections: false, workspace: false } });
const keys = { sidebarCollapsed: "gator:sidebar-collapsed", groupChatsByFolder: "gator:group-topics-by-folder", workDetailsSections: "gator:work-details-sections" };
const fingerprint = stat => [stat.dev, stat.ino, stat.size, stat.mtimeNs, stat.ctimeNs].join(":");
const valid = value => value && typeof value === "object" && !Array.isArray(value) && Object.keys(value).every(key => Object.hasOwn(keys, key)) && Object.entries(value).every(([key, item]) => key === "workDetailsSections" ? item && typeof item === "object" && !Array.isArray(item) && Object.keys(item).every(key => ["outputs", "context", "connections", "workspace"].includes(key)) && Object.values(item).every(value => typeof value === "boolean") : typeof item === "boolean");

// Copy just the LevelDB files, then open/recover the copy. Opening the original
// would lock it and can change its logs even when only performing get().
async function legacyPreferences(userData) {
  const source = join(userData, "Local Storage", "leveldb");
  let entries;
  try { entries = await readdir(source, { withFileTypes: true }); } catch (error) { if (error.code === "ENOENT") return; throw error; }
  const temporary = await mkdtemp(join(userData, ".preferences-import-"));
  let database;
  try {
    const manifest = entries.filter(entry => /^(CURRENT|MANIFEST-\d+|\d+\.(log|ldb|sst))$/.test(entry.name));
    if (!manifest.some(entry => entry.name === "CURRENT")) throw new Error("The existing preference database has no CURRENT manifest.");
    const before = new Map(); let total = 0;
    for (const entry of manifest) {
      if (!entry.isFile()) throw new Error("Linked preference database files cannot be imported.");
      const file = await open(join(source, entry.name), constants.O_RDONLY | (constants.O_NOFOLLOW || 0));
      try {
        const metadata = await file.stat({ bigint: true }); total += Number(metadata.size);
        if (!metadata.isFile() || total > 32 * 1024 * 1024) throw new Error("The existing preference database exceeds the 32 MiB import bound.");
        before.set(entry.name, fingerprint(metadata));
        const expected = Number(metadata.size), bytes = Buffer.alloc(expected + 1); let count = 0;
        while (count < bytes.length) { const result = await file.read(bytes, count, bytes.length - count, count); if (!result.bytesRead) break; count += result.bytesRead; }
        if (count !== expected) throw new Error("Preferences changed during import. Close the reference application and retry.");
        await writeFile(join(temporary, entry.name), bytes.subarray(0, count), { mode: 0o600, flag: "wx" });
      } finally { await file.close(); }
    }
    const after = (await readdir(source)).filter(name => /^(CURRENT|MANIFEST-\d+|\d+\.(log|ldb|sst))$/.test(name));
    if (after.length !== before.size) throw new Error("Preferences changed during import. Close the reference application and retry.");
    for (const name of after) if (fingerprint(await stat(join(source, name), { bigint: true })) !== before.get(name)) throw new Error("Preferences changed during import. Close the reference application and retry.");
    const { ClassicLevel } = await import("classic-level");
    database = new ClassicLevel(temporary, { keyEncoding: "buffer", valueEncoding: "buffer", createIfMissing: false });
    await database.open();
    const result = frontendDefaults();
    for (const [name, legacy] of Object.entries(keys)) {
      // Chromium stores an origin + NUL + string encoding marker + DOM key.
      for (const encoding of [1, 0]) {
        const key = Buffer.concat([Buffer.from("_gator://app\0"), Buffer.from([encoding]), Buffer.from(legacy, encoding ? "latin1" : "utf16le")]);
        const bytes = await database.get(key);
        if (!bytes) continue;
        if (bytes.length > 8192 || ![0, 1].includes(bytes[0])) throw new Error("Invalid retained frontend preference encoding.");
        const value = JSON.parse(bytes.subarray(1).toString(bytes[0] ? "latin1" : "utf16le"));
        if (!valid({ [name]: value })) throw new Error("Invalid retained frontend preference value.");
        if (name === "workDetailsSections") result[name] = { ...result[name], ...value }; else result[name] = value;
        break;
      }
    }
    return result;
  } finally { await database?.close(); await rm(temporary, { recursive: true, force: true }); }
}

export class FrontendPreferences {
  #pending = Promise.resolve();
  constructor(userData) { this.userData = userData; this.file = join(userData, "frontend-preferences.json"); }
  read() { return this.#queue(); }
  configure(patch) { if (!valid(patch)) return Promise.reject(new Error("Invalid frontend preferences.")); return this.#queue(patch); }
  flush() { return this.#pending; }
  #queue(patch) { const operation = this.#pending.then(() => this.#update(patch)); this.#pending = operation.catch(() => {}); return operation; }
  async #update(patch) {
    await mkdir(this.userData, { recursive: true, mode: 0o700 });
    const lock = this.file + ".lock"; let held;
    for (let attempt = 0; attempt < 2; attempt++) {
      try { held = await open(lock, "wx", 0o600); break; }
      catch (error) {
        if (error.code !== "EEXIST") throw error;
        const owner = Number(await readFile(lock, "utf8")); let live = true;
        if (Number.isSafeInteger(owner) && owner > 0) { try { process.kill(owner, 0); } catch (error) { if (error.code === "ESRCH") live = false; } }
        if (live || attempt) throw new Error("Another application is updating frontend preferences. Retry after it finishes.");
        await rm(lock);
      }
    }
    try {
      await held.writeFile(String(process.pid));
      let data;
      try {
        const raw = await readFile(this.file, "utf8");
        if (Buffer.byteLength(raw) > 16384) throw new Error("Oversized frontend preferences.");
        data = JSON.parse(raw);
        if (data.version !== 1 || !valid(data.preferences)) throw new Error("Invalid frontend preferences.");
      } catch (error) {
        if (error.code !== "ENOENT") throw error;
        const imported = await legacyPreferences(this.userData);
        data = { version: 1, preferences: imported ?? frontendDefaults(), legacyImport: imported ? "chromium-local-storage" : "absent" };
      }
      const preferences = { ...frontendDefaults(), ...data.preferences, ...patch, workDetailsSections: { ...frontendDefaults().workDetailsSections, ...data.preferences.workDetailsSections, ...patch?.workDetailsSections } };
      const next = JSON.stringify({ ...data, preferences });
      const existing = await readFile(this.file, "utf8").catch(error => { if (error.code === "ENOENT") return; throw error; });
      if (existing !== next) {
        const temporary = this.file + "." + randomUUID() + ".tmp";
        try { await writeFile(temporary, next, { mode: 0o600, flag: "wx" }); await rename(temporary, this.file); }
        finally { await rm(temporary, { force: true }); }
      }
      return preferences;
    } finally { await held.close(); await rm(lock, { force: true }); }
  }
}
