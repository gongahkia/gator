import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { shellQuote } from "./policy.mjs";

export function terminalScript(job, result) {
  const environment = (job.env ?? []).filter((value) => /^(GATOR_STATE_DIR|GATOR_CONFIG_DIR)=/.test(value));
  // App-owned handoffs can use private runtime handlers; this never starts a Gator terminal UI.
  const command = ["env", "GATOR_INTERNAL_COMMANDS=1", ...environment.map(shellQuote), shellQuote(job.program), ...(job.args ?? []).map(shellQuote)].join(" ");
  const onExit = `gator_exit=$?; printf '%s' "$gator_exit" > ${shellQuote(result)}`;
  return `#!/bin/sh\numask 077\ntrap ${shellQuote(onExit)} EXIT\ntrap 'exit 130' INT TERM HUP\ncd ${shellQuote(job.dir || process.cwd())} || exit 1\n${command}\ngator_exit=$?\nprintf '\\nGator action finished. You can close this window.\\n'\nexit "$gator_exit"\n`;
}

// Only a Go-produced handoff can reach this function. The renderer cannot
// supply a program, argument, directory, environment, or shell fragment.
export async function terminalHandoff(job) {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-terminal-"));
  const script = join(directory, "Gator.command");
  const result = join(directory, "result");
  await writeFile(script, terminalScript(job, result), { mode: 0o700 });
  try {
    const launch = process.platform === "darwin"
      ? spawn("open", ["-a", "Terminal", script], { stdio: "ignore" })
      : spawn("x-terminal-emulator", ["-e", script], { stdio: "ignore" });
    await new Promise((resolve, reject) => {
      launch.once("error", reject);
      launch.once("exit", (code) => code === 0 ? resolve() : reject(new Error("Could not open the system terminal.")));
    });
    const deadline = Date.now() + 30 * 60 * 1000;
    while (Date.now() < deadline) {
      try {
        const code = (await readFile(result, "utf8")).trim();
        return code === "0" ? "" : `The terminal action ended with status ${code}.`;
      } catch (error) { if (error.code !== "ENOENT") throw error; }
      await new Promise((resolve) => setTimeout(resolve, 350));
    }
    return "The terminal action has not returned. Check its terminal window before trying again.";
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}
