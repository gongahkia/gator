import { lstat, readFile, readlink, rm } from "node:fs/promises";
import { join, resolve } from "node:path";
import { acquireUpdateGuard } from "../../app-host/src/update-guard.mjs";
const prefix = process.argv[2] && resolve(process.argv[2]);
if (!prefix) throw new Error("Usage: node ui/scripts/uninstall.mjs PREFIX");
const root = join(prefix, "lib/gator-terminal"), manifest = JSON.parse(await readFile(join(root, "installation.json"), "utf8"));
const launcher = join(prefix, "bin", process.platform === "win32" ? "gator.cmd" : "gator");
if (manifest.product !== "gator-terminal" || manifest.root !== root || manifest.launcher !== launcher || manifest.prefix !== prefix || manifest.version !== 1) throw new Error("Installation ownership could not be verified.");
if (process.platform !== "win32") {
  if (!(await lstat(launcher)).isSymbolicLink() || await readlink(launcher) !== join("..", "lib/gator-terminal/bin/gator")) throw new Error("The launcher is no longer owned by this installation.");
} else if (await readFile(launcher, "utf8") !== '@echo off\r\nbun "%~dp0..\\lib\\gator-terminal\\bin\\gator" %*\r\n') throw new Error("The launcher changed after installation.");
// Preserve the same canonical cross-process/supervisor guard used by the app's
// updater. Windows cannot remove an executing runtime; release its idle lease
// after confirmation, immediately before deleting the owned installation.
const guard = await acquireUpdateGuard(join(root, "runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), process.cwd(), process.env);
if (process.platform === "win32") await guard.release();
try { await rm(launcher); await rm(root, { recursive: true }); }
finally { if (process.platform !== "win32") await guard.release(); }
console.log("Uninstalled Gator terminal. User profiles, credentials, conversations, files and drafts are retained. Background services are never removed implicitly; review them in Recovery before uninstalling.");
