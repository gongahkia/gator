import { spawn } from "node:child_process";
import { mkdir, readFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const directory = dirname(fileURLToPath(import.meta.url));
const repository = resolve(directory, "../..");
try { process.loadEnvFile(join(repository, ".env")); }
catch (error) { if (error.code !== "ENOENT") throw new Error("Cannot read local evaluation environment"); }
if (!process.env.OPENAI_API_KEY) throw new Error("Configure OPENAI_API_KEY locally before a live evaluation");
const authorization = JSON.parse(await readFile(join(directory, "authorization.json"), "utf8"));
if (authorization.total_usd !== 10 || authorization.ledger !== "results/budget.json") throw new Error("Evaluation authorization changed; review the cumulative allowance");
const results = join(directory, "results");
await mkdir(results, { recursive: true, mode: 0o700 });
const report = join(results, `agents-${new Date().toISOString().replaceAll(":", "-")}.json`);
const child = spawn("go", ["test", "./cmd/gator", "-run", "^TestLiveDynamicAgents$", "-count=1", "-v", "-timeout", "5m"], {
  cwd: repository, env: { ...process.env, GATOR_LIVE_DYNAMIC_AGENTS_REPORT: report }, stdio: "inherit",
});
child.on("error", () => { console.error("Could not start live specialist evaluation"); process.exitCode = 1; });
child.on("exit", (code) => { console.log(`Retained synthetic report: ${report}`); process.exitCode = code ?? 1; });
