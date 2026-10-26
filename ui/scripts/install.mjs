import { cp, mkdir, readFile, writeFile, access, symlink, lstat } from "node:fs/promises";
import { join, resolve, dirname } from "node:path";
import { spawnSync } from "node:child_process";

const [artifact, prefix] = process.argv.slice(2).map(path => resolve(path));
if (!artifact || !prefix) throw new Error("Usage: node ui/scripts/install.mjs ARTIFACT PREFIX");
const metadata = JSON.parse(await readFile(join(artifact, "gator-distribution.json"), "utf8"));
if (metadata.product !== "gator-terminal" || metadata.platform !== process.platform || metadata.arch !== process.arch) throw new Error("This artifact is not a Gator terminal build for this platform/architecture.");
const root = join(prefix, "lib/gator-terminal"), launcher = join(prefix, "bin", process.platform === "win32" ? "gator.cmd" : "gator");
for (const path of [root, launcher]) { try { await lstat(path); throw new Error(`Installation path already exists: ${path}. Use a new prefix; existing products are not replaced.`); } catch (error) { if (error.code !== "ENOENT") throw error; } }
await mkdir(dirname(root), { recursive: true }); await mkdir(dirname(launcher), { recursive: true });
await cp(artifact, root, { recursive: true, dereference: true });
if (process.platform === "win32") await writeFile(launcher, `@echo off\r\nbun "%~dp0..\\lib\\gator-terminal\\bin\\gator" %*\r\n`);
else await symlink(join("..", "lib/gator-terminal/bin/gator"), launcher);
await writeFile(join(root, "installation.json"), JSON.stringify({ product: "gator-terminal", root, launcher, prefix, version: 1 }) + "\n");
console.log(`Installed ${launcher}. Add ${dirname(launcher)} to PATH. Profiles and background checks were not changed.`);
