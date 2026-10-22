import { mkdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { runtimeName } from "../electron/platform.mjs";

const root = fileURLToPath(new URL("../../", import.meta.url));
const runtime = fileURLToPath(new URL("../runtime/", import.meta.url));
mkdirSync(runtime, { recursive: true });
const metadata = [process.env.VERSION, process.env.COMMIT, process.env.BUILD_DATE];
const target = process.env.GOOS || process.platform;
const linker = target === "windows" || target === "win32" ? ["-H=windowsgui"] : [];
if (metadata.some(Boolean)) linker.push(...["version", "commit", "date"].map((name, index) => `-X main.${name}=${metadata[index] || (index === 0 ? "dev" : "unknown")}`));
const flags = linker.length ? ["-ldflags", linker.join(" ")] : [];
const result = spawnSync("go", ["build", "-trimpath", ...flags, "-o", join(runtime, runtimeName(target)), "./cmd/gator"], { cwd: root, stdio: "inherit", windowsHide: true });
if (result.error) console.error(result.error.message);
if (result.status === 0) {
  for (const stale of ["gator", "gator.exe", runtimeName(target) === "gator-runtime.exe" ? "gator-runtime" : "gator-runtime.exe"]) rmSync(join(runtime, stale), { force: true });
}
process.exit(result.status ?? 1);
