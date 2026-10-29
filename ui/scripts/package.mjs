import { cp, mkdir, readFile, writeFile, readdir, chmod, access, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { resolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const destination = resolve(process.argv[2] || join(root, "ui/release/gator-terminal"));
// Never replace a tree silently. Packaging into a fresh directory is reproducible
// and cannot delete an installed product or a user profile.
if (existsSync(destination)) throw new Error("Package destination already exists. Choose a fresh directory.");
const runtime = process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime";
await access(join(root, "ui/runtime", runtime)); await access(join(root, "ui/dist/index.js"));
await mkdir(destination, { recursive: true, mode: 0o700 });
for (const directory of ["bin", "runtime", "ui/dist", "ui/scripts", "app-host/src", "app-host/contracts", "licenses"]) await mkdir(join(destination, directory), { recursive: true });
await cp(join(root, "ui/launcher/gator.mjs"), join(destination, "bin/gator")); await chmod(join(destination, "bin/gator"), 0o755);
await writeFile(join(destination, "bin/gator.cmd"), '@echo off\r\nbun "%~dp0gator" %*\r\n');
await cp(join(root, "ui/runtime", runtime), join(destination, "runtime", runtime));
await cp(join(root, "ui/dist"), join(destination, "ui/dist"), { recursive: true });
await cp(join(root, "app-host/src"), join(destination, "app-host/src"), { recursive: true });
await cp(join(root, "app-host/contracts"), join(destination, "app-host/contracts"), { recursive: true });
for (const [source, target] of [["ui/package.json", "ui/package.json"], ["ui/bun.lock", "ui/bun.lock"], ["app-host/package.json", "app-host/package.json"], ["ui/UPSTREAM.md", "licenses/OPENCODE-UPSTREAM.md"], ["ui/upstream/LICENSE", "licenses/OPENCODE-LICENSE"], ["docs/TERMINAL.md", "TERMINAL.md"]]) await cp(join(root, source), join(destination, target));
await cp(join(root, "ui/patches"), join(destination, "ui/patches"), { recursive: true });
for (const name of ["install.mjs", "uninstall.mjs"]) await cp(join(root, "ui/scripts", name), join(destination, "ui/scripts", name));
const result = spawnSync("bun", ["install", "--cwd", join(destination, "ui"), "--production", "--frozen-lockfile"], { stdio: "inherit", windowsHide: true });
if (result.error || result.status !== 0) throw result.error || new Error("Production dependency installation failed.");
// Bun can install file: dependencies as absolute per-file symlinks on macOS.
// Those resolve from app-host/ (outside ui/node_modules), losing its transitive
// dependencies, and break when the artifact moves. Materialize our own package
// beside its dependencies before verification/distribution. This destination is
// a newly created artifact, never an existing installation.
const installedHost = join(destination, "ui/node_modules/@gator/application-host");
await rm(installedHost, { recursive: true, force: true });
await cp(join(destination, "app-host"), installedHost, { recursive: true, dereference: true });
const metadata = { product: "gator-terminal", version: 1, platform: process.platform, arch: process.arch, upstream: "f669c46c9604ea3d70b8a7ed1f0c10f3a5ac1f61", runtime: "Bun >=1.4.2", electron: false };
await writeFile(join(destination, "gator-distribution.json"), JSON.stringify(metadata, null, 2) + "\n");
const notices = ["Gator terminal production JavaScript dependency notices.\nOpenCode's source license is retained separately.\n"];
async function scan(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.name.startsWith(".")) continue;
    const path = join(directory, entry.name);
    if (entry.name.startsWith("@") && entry.isDirectory()) { await scan(path); continue; }
    if (!entry.isDirectory() && !entry.isSymbolicLink()) continue;
    let metadata; try { metadata = JSON.parse(await readFile(join(path, "package.json"), "utf8")); } catch { continue; }
    notices.push(`\n${"=".repeat(72)}\n${metadata.name}@${metadata.version}\nLicense: ${metadata.license || "See package metadata"}\n`);
    for (const name of await readdir(path)) if (/^licen[sc]e(?:[.-]|$)|^copying(?:[.-]|$)/i.test(name)) {
      try { notices.push(`\n${name}\n${await readFile(join(path, name), "utf8")}\n`); } catch (error) { if (error.code !== "EISDIR") throw error; }
    }
  }
}
await scan(join(destination, "ui/node_modules"));
await writeFile(join(destination, "licenses/DEPENDENCIES.txt"), notices.join(""));
const verification = spawnSync("bun", [join(root, "ui/scripts/verify-package-services.mjs"), destination], { stdio: "inherit", windowsHide: true });
if (verification.error || verification.status !== 0) throw verification.error || new Error("Production service verification failed. Refresh the local host dependency lock with bun update @gator/application-host from ui/ and rebuild.");
console.log(destination);
