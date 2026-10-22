import { spawn } from "node:child_process";

// This role only acquires a private-data lease and checks supervisor ownership.
// It does not cancel work or run profile maintenance. There are no shell args.
export function acquireUpdateGuard(binary, cwd, env) {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ["desktop-update-guard"], { cwd, env: { ...env, GATOR_DESKTOP_UPDATE_GUARD: "1" }, windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
    let output = "", diagnostics = "", settled = false;
    const timer = setTimeout(() => fail(new Error("Update preparation timed out. Your saved work is unchanged.")), 8000);
    const fail = (error) => { if (!settled) { settled = true; clearTimeout(timer); child.kill(); reject(error); } };
    child.stdin.on("error", () => {});
    child.on("error", fail);
    child.stderr.on("data", (chunk) => { diagnostics = (diagnostics + chunk.toString()).slice(-3000); });
    child.on("close", () => { if (!settled) fail(new Error(diagnostics.trim().replace(/^Gator:\s*/, "") || "Other work is using this profile. Finish it before updating.")); });
    child.stdout.on("data", (chunk) => {
      output += chunk.toString();
      if (output.length > 256) { fail(new Error("Invalid update preparation response.")); return; }
      if (!output.includes("\n")) return;
      try { if (JSON.parse(output).ready !== true) throw new Error(); }
      catch { fail(new Error("Invalid update preparation response.")); return; }
      if (settled) return;
      settled = true; clearTimeout(timer);
      resolve({ release: () => new Promise((done, failed) => {
        if (child.exitCode !== null || child.signalCode !== null) { failed(new Error("Update preparation ended unexpectedly.")); return; }
        const deadline = setTimeout(() => { child.kill(); failed(new Error("Update preparation could not finish.")); }, 4000);
        child.once("close", (code) => { clearTimeout(deadline); code === 0 ? done() : failed(new Error("Update preparation could not finish.")); });
        child.stdin.end();
      }) });
    });
  });
}
