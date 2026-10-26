import { mkdirSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("../../", import.meta.url));
const output = fileURLToPath(new URL("../runtime/", import.meta.url));
mkdirSync(output, { recursive: true });
const values = [process.env.VERSION || "dev", process.env.COMMIT || "unknown", process.env.BUILD_DATE || "unknown"];
const flags = ["version", "commit", "date"].map((name, index) => `-X main.${name}=${values[index]}`);
const result = spawnSync("go", ["build", "-trimpath", "-ldflags", flags.join(" "), "-o", join(output, process.env.GOOS === "windows" || !process.env.GOOS && process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), "./cmd/gator"], { cwd: root, stdio: "inherit", windowsHide: true });
if (result.error) console.error(result.error.message);
process.exit(result.status ?? 1);
