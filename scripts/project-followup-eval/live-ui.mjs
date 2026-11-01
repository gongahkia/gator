// Explicit live command only. Never source .env as shell code or pass its key
// into Electron/browser fixtures. The Go harness owns all real API admission.
import { readFile, mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";

const root = fileURLToPath(new URL("../../", import.meta.url));
const env = { ...process.env };
const coding = process.argv.slice(2).includes("--coding");
if (process.argv.slice(2).some(value => value !== "--coding")) throw new Error("Use only --coding to select the coding journeys.");
if (coding) env.GATOR_LIVE_DESKTOP_SUITE = "coding";
if (!env.OPENAI_API_KEY) {
  const text = await readFile(resolve(root, ".env"), "utf8");
  const line = text.split(/\r?\n/).find(line => /^\s*(?:export\s+)?OPENAI_API_KEY\s*=/.test(line));
  let value = line?.replace(/^\s*(?:export\s+)?OPENAI_API_KEY\s*=\s*/, "").trim();
  if (value?.startsWith('"') && value.endsWith('"') || value?.startsWith("'") && value.endsWith("'")) value = value.slice(1, -1);
  if (!value) throw new Error("Configure OPENAI_API_KEY in the local environment or Git-ignored .env; do not paste it into a test.");
  env.OPENAI_API_KEY = value;
}
const results = resolve(root, "scripts/project-followup-eval/results");
await mkdir(results, { recursive: true, mode: 0o700 });
env.GATOR_LIVE_DESKTOP_REPORT = resolve(results, `${coding ? "coding-ui" : "ui"}-${new Date().toISOString().replaceAll(/[:.]/g, "-")}.json`);
const child = spawn("go", ["test", "./cmd/gator", "-run", "^TestLiveDesktopUI$", "-count=1", "-v", "-timeout", coding ? "12m" : "8m"], { cwd: root, env, stdio: "inherit" });
child.once("error", () => { console.error("Could not start the existing Go toolchain."); process.exitCode = 1; });
child.once("exit", code => { process.exitCode = code ?? 1; });
