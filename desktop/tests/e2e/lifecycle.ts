import type { ElectronApplication } from "@playwright/test";
import { mkdtemp, realpath } from "node:fs/promises";

// Match Go's canonical workspace paths, including Windows 8.3 aliases and
// macOS /var -> /private/var, before asserting a projected source identity.
export async function temporaryDirectory(prefix: string) {
  return realpath(await mkdtemp(prefix));
}

// Let Playwright disconnect its inspector after requesting Gator's normal quit
// path; do not close its debugger connection from inside the application.
export async function closeDesktop(application: ElectronApplication) {
  const child = application.process();
  if (child.exitCode !== null || child.signalCode !== null) return;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_, reject) => {
    timeout = setTimeout(() => {
      const status = child.exitCode === null ? "main process is still running" : `main process exited with ${child.exitCode}`;
      // Playwright launches Electron in its own process group on Unix. Include
      // its Chromium helpers when a failed test has to be stopped.
      try {
        if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL");
        else if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL");
      } catch { /* The isolated process may already have exited. */ }
      reject(new Error(`The isolated Gator test application did not close (${status}).`));
    }, 10_000);
  });
  try {
    await Promise.race([application.close(), deadline]);
    if (child.exitCode !== 0) throw new Error("The isolated Gator test application exited unsuccessfully.");
  } finally { clearTimeout(timeout); }
}
