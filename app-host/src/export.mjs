import { constants } from "node:fs";
import { lstat, open, realpath } from "node:fs/promises";
import { basename, dirname, isAbsolute, relative, resolve, join } from "node:path";
import { randomUUID } from "node:crypto";

const fingerprint = stat => [stat.dev, stat.ino, stat.size, stat.mtimeNs, stat.ctimeNs].join(":");
const inside = (root, path) => { const part = relative(root, path); return !part || (!isAbsolute(part) && part !== ".." && !part.startsWith("../") && !part.startsWith("..\\")); };

// Replacement grants are short-lived, single-use reviews of a particular file.
// Open the reviewed inode without truncating; check it before writing any bytes.
export class ExportStore {
  reviews = new Map();
  constructor(protectedRoots = []) { this.protectedRoots = protectedRoots; }
  async path(destination) {
    if (typeof destination !== "string" || !destination.trim() || destination.includes("\0")) throw new Error("Choose an export file path.");
    const path = resolve(destination), parent = await realpath(dirname(path));
    const canonical = join(parent, basename(path));
    for (const root of this.protectedRoots) {
      const protectedPath = await realpath(root).catch(() => resolve(root));
      if (inside(protectedPath, canonical)) throw new Error("Export outside Gator's private storage to preserve retained files and sealed outputs.");
    }
    return canonical;
  }
  async review(destination) {
    const path = await this.path(destination), stat = await lstat(path, { bigint: true }).catch(error => { if (error.code === "ENOENT") return; throw error; });
    if (!stat) return { path, exists: false };
    if (!stat.isFile() || stat.nlink !== 1n) throw new Error("Choose a regular, unlinked export destination.");
    for (const [token, value] of this.reviews) if (value.expires < Date.now()) this.reviews.delete(token);
    if (this.reviews.size >= 16) this.reviews.delete(this.reviews.keys().next().value);
    const token = randomUUID();
    this.reviews.set(token, { path, fingerprint: fingerprint(stat), expires: Date.now() + 600000 });
    return { path, exists: true, bytes: Number(stat.size), modified: new Date(Number(stat.mtimeMs)).toISOString(), token };
  }
  async write(destination, bytes, token, current = () => {}) {
    const path = await this.path(destination);
    let file;
    try {
      if (token === undefined) {
        current();
        file = await open(path, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, 0o600);
      } else {
        const review = this.reviews.get(token); this.reviews.delete(token);
        if (!review || review.path !== path || review.expires < Date.now()) throw new Error("Review this export destination again before replacing it.");
        file = await open(path, constants.O_RDWR | (constants.O_NOFOLLOW || 0));
        const stat = await file.stat({ bigint: true }), named = await lstat(path, { bigint: true });
        if (!stat.isFile() || stat.nlink !== 1n || fingerprint(stat) !== review.fingerprint || fingerprint(named) !== review.fingerprint) throw new Error("The destination changed after review. Review it again; no bytes were replaced.");
      }
      current();
      await file.writeFile(bytes); await file.truncate(bytes.length); await file.sync();
    } finally { await file?.close(); }
  }
}
