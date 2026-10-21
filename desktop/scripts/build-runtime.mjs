import { mkdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const root = fileURLToPath(new URL("../../", import.meta.url));
const runtime = fileURLToPath(new URL("../runtime/", import.meta.url));
mkdirSync(runtime, { recursive: true });
const metadata = [process.env.VERSION, process.env.COMMIT, process.env.BUILD_DATE];
const flags = metadata.some(Boolean) ? ["-ldflags", ["version", "commit", "date"].map((name, index) => `-X main.${name}=${metadata[index] || (index === 0 ? "dev" : "unknown")}`).join(" ")] : [];
const result = spawnSync("go", ["build", "-trimpath", ...flags, "-o", `${runtime}gator-runtime`, "./cmd/gator"], { cwd: root, stdio: "inherit" });
if (result.error) console.error(result.error.message);
if (result.status === 0) rmSync(`${runtime}gator`, { force: true });
process.exit(result.status ?? 1);
