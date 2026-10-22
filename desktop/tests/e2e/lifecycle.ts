import type { ElectronApplication } from "@playwright/test";
import { mkdtemp, realpath } from "node:fs/promises";

// Match Go's canonical workspace paths, including Windows 8.3 aliases and
// macOS /var -> /private/var, before asserting a projected source identity.
export async function temporaryDirectory(prefix: string) {
  return realpath(await mkdtemp(prefix));
}

// The Node debugger can keep Electron alive after normal app shutdown, and a
// second context.close() can wait on that inspector. Detach only this test's
// inspector after the evaluation returns, then use Gator's normal quit path.
export async function closeDesktop(application: ElectronApplication) {
  const child = application.process();
  if (child.exitCode !== null || child.signalCode !== null) return;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
  const deadline = new Promise<never>((_, reject) => {
    timeout = setTimeout(() => {
      child.kill("SIGKILL");
      reject(new Error("The isolated Gator test application did not close."));
    }, 10_000);
  });
  try {
    const closing = application.evaluate(({ app }) => {
      setImmediate(() => { process.getBuiltinModule("inspector").close(); app.quit(); });
    });
    await Promise.race([Promise.all([closing, exited]), deadline]);
  } finally { clearTimeout(timeout); }
}
