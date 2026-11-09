import { spawn } from "node:child_process";

// Private helper holds Go's nonblocking exclusive profile guard until the
// actual reference scan AND prune finish. A busy profile defers cleanup.
export async function withProfileMaintenance(binary, env, action) {
  const child = spawn(binary, ["ui-maintenance-lease"], { env: { ...env, GATOR_UI_BACKEND: "1" }, stdio: ["pipe", "pipe", "pipe"], windowsHide: true });
  let diagnostics = "", closed = false;
  const completion = new Promise(resolve => child.once("close", () => { closed = true; resolve(); }));
  child.stderr.on("data", value => { diagnostics = (diagnostics + value).slice(-2048); });
  child.stdin.on("error", () => {});
  try {
    const ready = await new Promise((resolve, reject) => {
      let data = "";
      const finish = (error, value) => { clearTimeout(timer); child.stdout.off("data", received); child.off("error", failed); child.off("close", failed); error ? reject(error) : resolve(value); };
      const failed = error => finish(error instanceof Error ? error : new Error(diagnostics || "The cleanup lease closed before confirming ownership."));
      const received = chunk => {
        data += chunk;
        if (data.length > 1024) return finish(new Error("Invalid cleanup lease response."));
        if (!data.includes("\n")) return;
        try { const value = JSON.parse(data); if (typeof value.ready !== "boolean") throw new Error(); finish(undefined, value.ready); }
        catch { finish(new Error("Invalid cleanup lease response.")); }
      };
      const timer = setTimeout(() => finish(new Error("Cleanup ownership was not confirmed; attachments were kept.")), 10000);
      child.stdout.on("data", received); child.once("error", failed); child.once("close", failed);
    });
    if (ready) { const assertOwned = () => { if (closed) throw new Error("Cleanup ownership was lost; attachments were kept."); }; assertOwned(); await action(assertOwned); }
    return ready;
  } finally { child.stdin.end(); if (!closed) { const timer = setTimeout(() => child.kill(), 1000); await completion; clearTimeout(timer); } }
}
