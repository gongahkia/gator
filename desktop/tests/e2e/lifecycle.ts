import { _electron as electron, type ElectronApplication } from "@playwright/test";
import { mkdtemp, realpath } from "node:fs/promises";

// Match Go's canonical workspace paths, including Windows 8.3 aliases and
// macOS /var -> /private/var, before asserting a projected source identity.
export async function temporaryDirectory(prefix: string) {
  return realpath(await mkdtemp(prefix));
}

// Optional local regression mode: CDP still drives the real window, while the
// fixture cannot receive the operator's physical keystrokes. Paid live demos
// remain interactive/visible under their existing explicit opt-in command.
export async function launchDesktop(options: Parameters<typeof electron.launch>[0]) {
  const application = await electron.launch(options);
  applicationProcesses.set(application, application.process());
  if (process.env.GATOR_E2E_BACKGROUND === "1" && !process.env.GATOR_LIVE_UI_ENDPOINT && process.platform !== "linux") {
    await application.evaluate(({ app, BrowserWindow }) => {
      // launch() can return before the asynchronous backend is ready and its
      // first window exists. Cover future windows too, so an operator's real
      // keyboard input cannot enter a late-created fixture window.
      app.on("browser-window-created", (_event, window) => { window.setFocusable(false); window.blur(); });
      for (const window of BrowserWindow.getAllWindows()) { window.setFocusable(false); window.blur(); }
    });
  }
  return application;
}

// Let Playwright disconnect its inspector after requesting Gator's normal quit
// path; do not close its debugger connection from inside the application.
const applicationProcesses = new WeakMap<ElectronApplication, ReturnType<ElectronApplication["process"]>>();
export async function closeDesktop(application: ElectronApplication) {
  // Playwright may dispose its remote process object after close(). Retain the
  // real child handle for idempotent finally-cleanup and authoritative exit state.
  const child = applicationProcesses.get(application) ?? application.process();
  applicationProcesses.set(application, child);
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
