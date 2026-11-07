// Migration reference probe, excluded from production. Uses an isolated real
// Electron profile, then tests its Chromium data through the independent Bun UI.
import { createRequire } from "node:module";
import { mkdtemp, mkdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";
const root = await mkdtemp(join(tmpdir(), "gator-reference-preferences-"));
const repository = resolve(import.meta.dirname, "../..");
const { _electron } = createRequire(join(repository, "desktop/package.json"))("@playwright/test");
let app;
let child;
async function close() {
  if (!app || child.exitCode !== null) return;
  let timer;
  try {
    await Promise.race([app.close(), new Promise((_, reject) => { timer = setTimeout(() => {
      try { if (process.platform !== "win32") process.kill(-child.pid, "SIGKILL"); else child.kill("SIGKILL"); } catch {}
      reject(new Error("The isolated preference reference did not close."));
    }, 10000); })]);
    assert.equal(child.exitCode, 0);
  } finally { clearTimeout(timer); }
}
try {
  const userData = join(root, "reference");
  await mkdir(join(root, "workspace"));
  app = await _electron.launch({ args: ["."], cwd: join(repository, "desktop"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: userData, GATOR_DESKTOP_WORKSPACE: join(root, "workspace") } });
  child = app.process();
  const page = await app.firstWindow({ timeout: 15000 });
  await page.waitForFunction(async () => !!(await window.gator?.snapshot())?.state?.conversation_id, undefined, { timeout: 15000 });
  await page.evaluate(() => {
    localStorage.setItem("gator:sidebar-collapsed", "true");
    localStorage.setItem("gator:group-topics-by-folder", "true");
    localStorage.setItem("gator:work-details-sections", JSON.stringify({ outputs: false, context: true, connections: false, workspace: false }));
  });
  await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].webContents.session.flushStorageData());
  await close();
  const result = spawnSync("bun", ["test", "--cwd", join(repository, "ui"), "--preload", "./scripts/preload.ts", "tests/preferences.test.tsx"], { env: { ...process.env, GATOR_MIGRATION_PREFERENCES_FIXTURE: userData }, stdio: "inherit" });
  if (result.error || result.status !== 0) throw result.error || new Error("The actual Chromium preference migration probe failed.");
  console.log("PASS: actual Electron localStorage imports through the Bun terminal; source database bytes remain unchanged.");
} finally { await close(); await rm(root, { recursive: true, force: true }); }
