import { lstat, readFile, readlink, realpath, rm } from "node:fs/promises";
import { join, resolve, dirname, basename, relative, isAbsolute, sep } from "node:path";
import { homedir } from "node:os";
import { acquireUpdateGuard } from "../../app-host/src/update-guard.mjs";
const prefix = process.argv[2] && resolve(process.argv[2]);
if (!prefix) throw new Error("Usage: node ui/scripts/uninstall.mjs PREFIX");
const root = join(prefix, "lib/gator-terminal");
if (!(await lstat(root)).isDirectory() || !(await lstat(join(root, "installation.json"))).isFile()) throw new Error("Installation ownership requires an ordinary directory and manifest.");
const manifest = JSON.parse(await readFile(join(root, "installation.json"), "utf8"));
const launcher = join(prefix, "bin", process.platform === "win32" ? "gator.cmd" : "gator");
if (manifest.product !== "gator-terminal" || manifest.root !== root || manifest.launcher !== launcher || manifest.prefix !== prefix || manifest.version !== 1) throw new Error("Installation ownership could not be verified.");
if (process.platform !== "win32") {
  if (!(await lstat(launcher)).isSymbolicLink() || await readlink(launcher) !== join("..", "lib/gator-terminal/bin/gator")) throw new Error("The launcher is no longer owned by this installation.");
} else if (await readFile(launcher, "utf8") !== '@echo off\r\nbun "%~dp0..\\lib\\gator-terminal\\bin\\gator" %*\r\n') throw new Error("The launcher changed after installation.");
// Preserve the same canonical cross-process/supervisor guard used by the app's
// updater. Windows cannot remove an executing runtime; release its idle lease
// after confirmation, immediately before deleting the owned installation.
const guard = await acquireUpdateGuard(join(root, "runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), process.cwd(), process.env);
// Resolve missing descendants through their nearest existing ancestor too:
// an external-looking path may enter the installation through a directory link.
async function canonical(path, depth = 0) {
  if (depth > 64) throw new Error("Profile location has too many unresolved path components.");
  try { return await realpath(path); }
  catch (error) {
    if (error.code !== "ENOENT" || dirname(path) === path) throw error;
    const info = await lstat(path).catch(error => { if (error.code === "ENOENT") return null; throw error; });
    if (info?.isSymbolicLink()) return canonical(resolve(dirname(path), await readlink(path)), depth + 1);
    return join(await canonical(dirname(path), depth + 1), basename(path));
  }
}
let released = false;
try {
  if (!Array.isArray(guard.profilePaths) || guard.profilePaths.length !== 2 || guard.profilePaths.some(path => typeof path !== "string" || !isAbsolute(path))) throw new Error("This runtime cannot verify profile locations for removal. Use a current terminal build before uninstalling.");
  // Match ui/src/index.tsx, and protect the retained desktop override even
  // when a separate terminal override currently takes precedence.
  const defaultUserData = process.platform === "darwin" ? join(homedir(), "Library/Application Support/Gator") : process.platform === "win32" ? join(process.env.APPDATA || homedir(), "Gator") : join(process.env.XDG_CONFIG_HOME || join(homedir(), ".config"), "Gator");
  const paths = [...guard.profilePaths, defaultUserData, process.env.GATOR_TERMINAL_USER_DATA, process.env.GATOR_DESKTOP_USER_DATA].filter(Boolean);
  const ownedRoot = await realpath(root);
  for (const path of paths) {
    const candidate = await canonical(resolve(path)), inside = relative(ownedRoot, candidate);
    if (!inside || !isAbsolute(inside) && inside !== ".." && !inside.startsWith(`..${sep}`)) throw new Error("A profile or draft storage location is inside this installation. Move that data outside the installation and select its new location before uninstalling; nothing was removed.");
  }
  if (process.platform === "win32") { await guard.release(); released = true; }
  await rm(launcher); await rm(root, { recursive: true });
} finally { if (!released) await guard.release(); }
console.log("Uninstalled Gator terminal. Checked profile and draft storage locations were retained. Background services are never removed implicitly; review them in Recovery before uninstalling.");
