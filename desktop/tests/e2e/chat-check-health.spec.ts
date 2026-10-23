import { _electron as electron, expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

test("chat health reports offline checks and observed gaps only in their owning chat", async () => {
  const root = await temporaryDirectory(join(tmpdir(), "gator-chat-health-"));
  const workspace = join(root, "workspace"); await mkdir(workspace);
  const config = join(root, "config", "gator"); await mkdir(config, { recursive: true });
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "health-fixture", model: "fixture" }, custom_providers: [{ id: "health-fixture", base_url: "http://127.0.0.1:9/v1/chat/completions", models: ["fixture"], default_model: "fixture" }] }));
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  try {
    const page = await app.firstWindow();
    await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toBeVisible();
    const origin = await page.evaluate(async () => (await window.gator!.snapshot()).state!.conversation_id!);
    const output = execFileSync(resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), ["job", "add", "Project deadline", "--schedule", "*/15 * * * *", "--source", workspace, "--mode", "inspect", "--", "Check project deadline"], { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
    const id = output.match(/Added job (job-[a-zA-Z0-9-]+)/)![1];
    const jobs = join(env.GATOR_STATE_DIR, "gator", "jobs"), path = join(jobs, "definitions", `${id}.json`);
    const definition = JSON.parse(await readFile(path, "utf8"));
    const now = Date.now(); definition.origin_conversation_id = origin; definition.created_at = new Date(now - 7_200_000).toISOString();
    await writeFile(path, JSON.stringify(definition));
    await writeFile(join(jobs, "availability.json"), JSON.stringify({ LastSeen: new Date(now - 1_200_000).toISOString(), Gaps: [{ From: new Date(now - 3_600_000).toISOString(), To: new Date(now - 1_200_000).toISOString() }] }));
    await mkdir(join(jobs, "history", id), { recursive: true });
    await writeFile(join(jobs, "history", id, "attempt-limited.json"), JSON.stringify({ version: 2, id: "attempt-limited", job_id: id, status: "limited", summary: "Daily model request limit reached", started_at: new Date(now - 1_000).toISOString(), finished_at: new Date(now).toISOString() }));
    const health = page.locator(".chat-check-health");
    await expect(health.getByText("1 check needs attention", { exact: true })).toBeVisible({ timeout: 25_000 });
    await expect(health).not.toHaveAttribute("open");
    await health.locator("summary").click();
    await expect(health.getByText("offline", { exact: true })).toBeVisible();
    await expect(health.getByText(/No successful check recorded/)).toBeVisible();
    await expect(health.getByText(/1 consecutive unsuccessful check/)).toContainText("Daily model request limit reached");
    await expect(health.getByText(/Observed supervisor gap/)).toContainText("does not establish what changed");
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toBeVisible();
    await page.screenshot({ path: "test-results/chat-check-health.png" });
    await app.evaluate(({BrowserWindow})=>{const win=BrowserWindow.getAllWindows()[0]; win.show();win.focus();});
    await page.bringToFront();
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await expect.poll(() => page.evaluate(async (original) => { const snapshot = await window.gator!.snapshot(); return !!snapshot.state?.conversation_id && snapshot.state.conversation_id !== original && !snapshot.sessionSwitching; }, origin)).toBe(true);
    await expect(health).toHaveCount(0);
  } finally {
    try { await closeDesktop(app); } finally { await rm(root, { recursive: true, force: true }); }
  }
});
