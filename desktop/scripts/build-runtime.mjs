import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const root = fileURLToPath(new URL("../../", import.meta.url));
const runtime = fileURLToPath(new URL("../runtime/", import.meta.url));
mkdirSync(runtime, { recursive: true });
const result = spawnSync("go", ["build", "-o", `${runtime}gator`, "./cmd/gator"], { cwd: root, stdio: "inherit" });
if (result.error) console.error(result.error.message);
process.exit(result.status ?? 1);
