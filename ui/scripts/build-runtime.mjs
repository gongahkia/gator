import { googleOAuthFlags } from "../../scripts/google-oauth-build.mjs";
import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("../../", import.meta.url));
const output = fileURLToPath(new URL("../runtime/", import.meta.url));
mkdirSync(output, { recursive: true });
const values = [process.env.VERSION || "dev", process.env.COMMIT || "unknown", process.env.BUILD_DATE || "unknown"];
const flags = ["version", "commit", "date"].map((name, index) => `-X main.${name}=${values[index]}`);
flags.push(...googleOAuthFlags());
// /tmp may be a small RAM filesystem even when the checkout has ample space.
// Keep Go's linker scratch beside its output, honoring an explicit GOTMPDIR.
const scratch = process.env.GOTMPDIR ? undefined : mkdtempSync(join(output, ".build-"));
let result;
try {
  result = spawnSync("go", ["build", "-trimpath", "-ldflags", flags.join(" "), "-o", join(output, process.env.GOOS === "windows" || !process.env.GOOS && process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), "./cmd/gator"], { cwd: root, stdio: "inherit", windowsHide: true, env: { ...process.env, GOTMPDIR: process.env.GOTMPDIR || scratch } });
} finally {
  if (scratch) rmSync(scratch, { recursive: true, force: true });
}
if (result.error) console.error(result.error.message);
process.exit(result.status ?? 1);
