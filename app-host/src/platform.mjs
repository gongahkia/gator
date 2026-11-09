import { join, resolve } from "node:path";
import { homedir } from "node:os";

export function runtimeName(platform = process.platform) {
  return platform === "win32" || platform === "windows" ? "gator-runtime.exe" : "gator-runtime";
}

export function stateDirectory(env = process.env, platform = process.platform) {
  const override = env.GATOR_STATE_DIR?.trim() || env.XDG_STATE_HOME?.trim();
  if (override) return resolve(override);
  if (platform === "win32") {
    if (!env.LOCALAPPDATA?.trim()) throw new Error("Windows Local AppData is unavailable.");
    return join(env.LOCALAPPDATA, "Gator", "state");
  }
  return join(homedir(), ".local", "state");
}

// Unix Go owners retain workspace-relative overrides. The host's existing
// upload/draft namespace remains launch-relative for compatibility, so use this
// location specifically for authoritative native journals and profile guards.
export function backendStateDirectory(env, workspace, platform = process.platform) {
  const override = env.GATOR_STATE_DIR?.trim() || env.XDG_STATE_HOME?.trim();
  return platform !== "win32" && override ? resolve(workspace, override) : stateDirectory(env, platform);
}

// Windows native programs use the CommandLineToArgvW/CRT quoting rules. This
// string is passed straight to ProcessStartInfo, never to cmd.exe or a shell.
export function windowsArguments(args) {
  return args.map((value) => `"${String(value).replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/g, "$1$1")}"`).join(" ");
}

export function powershellPath(env = process.env) {
  if (!env.SystemRoot && !env.SYSTEMROOT) throw new Error("Windows SystemRoot is unavailable.");
  return join(env.SystemRoot || env.SYSTEMROOT, "System32", "WindowsPowerShell", "v1.0", "powershell.exe");
}
