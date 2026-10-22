import { _electron as electron, test, expect } from "@playwright/test";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { mkdir, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { execFileSync } from "node:child_process";
import { closeDesktop, temporaryDirectory } from "./lifecycle";
import type { BackendActivity, BackendState } from "../../src/protocol";

test("public activity interleaves, folds above the answer, and preserves the reader", async () => {
  const directory = await temporaryDirectory(join(tmpdir(), "gator-activity-e2e-"));
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env: { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") } });
  try {
    const page = await app.firstWindow();
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    const snapshot = await page.evaluate(() => window.gator!.snapshot());
    const started = "2026-10-22T01:00:00Z", finished = "2026-10-22T01:00:12Z";
    const activity: BackendActivity = { started_at: started, state: "running", entries: [
      { id: "text-1", kind: "text", text: "I’ll check your notes first.", state: "completed", started_at: started, finished_at: started },
      { id: "read-1", kind: "tool", tool: "read_file", state: "running", started_at: started },
      { id: "read-2", kind: "tool", tool: "search_files", state: "completed", started_at: started, finished_at: started },
      { id: "text-2", kind: "text", text: "The decision is recorded in the project notes.", state: "running", started_at: started },
    ] };
    const state: BackendState = { ...snapshot.state!, home: false, running: true, elapsed: "12s", status: "Reviewing selected sources", conversation_id: "activity-display", messages: [{ role: "You", text: "What did we decide?" }], activity };
    const publish = async (value: BackendState) => app.evaluate(({ BrowserWindow }, snapshot) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", snapshot), { ...snapshot, state: value });
    await publish(state);
    const live = page.getByRole("region", { name: "Gator activity", exact: true });
    await expect(live).toContainText("I’ll check your notes first.");
    await composer.fill("Keep my next question");
    await expect(live.locator(".activity-entries > *")).toHaveCount(3);
    await live.getByRole("button", { name: /Using 2 tools/ }).click();
    await expect(live.getByText("Running", { exact: true })).toBeVisible();
    await expect(live.locator(".activity-tool-list li")).toHaveCount(2);
    // Completion updates the same tool row without moving it past narration.
    activity.entries[1] = { ...activity.entries[1], state: "failed", finished_at: finished };
    await publish({ ...state, activity: structuredClone(activity) });
    await expect(live.locator(".activity-tool-list li")).toHaveCount(2);
    await expect(live.getByText("Failed", { exact: true })).toBeVisible();
    await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished)); });
    await page.screenshot({ path: "test-results/desktop-activity-live.png" });
    const completed = { ...activity, state: "completed" as const, finished_at: finished, entries: activity.entries.map((entry) => entry.state === "running" ? { ...entry, state: "completed" as const, finished_at: finished } : entry) };
    const done: BackendState = { ...state, running: false, activity: undefined, messages: [...state.messages, { role: "Gator", text: "You chose the compact direction.", activity: completed }] };
    await publish(done);
    const history = page.getByRole("button", { name: /Worked for 12s/ });
    await expect(history).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByText("You chose the compact direction.", { exact: true })).toBeVisible();
    await history.click();
    await expect(history).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByText("I’ll check your notes first.", { exact: true })).toBeVisible();
    // Inspect the opening at 10% speed, then interrupt it with a close.
    const slowed = await page.evaluate(() => {
      const animations = document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity);
      animations.forEach((animation) => { animation.playbackRate = .1; });
      return animations.length;
    });
    expect(slowed).toBeGreaterThan(0);
    await page.screenshot({ path: "test-results/desktop-activity-motion.png" });
    await history.click();
    await expect(history).toHaveAttribute("aria-expanded", "false");
    await expect(page.locator(".activity-history .activity-collapse").first()).toHaveAttribute("inert", "");
    await history.click();
    await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => { animation.playbackRate = 1; return animation.finished; })); });
    await page.screenshot({ path: "test-results/desktop-activity-completed.png" });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await history.click();
    await expect(history).toHaveAttribute("aria-expanded", "false");
    expect(await page.evaluate(() => document.getAnimations().filter((animation) => animation.playState === "running").length)).toBe(0);
    await expect(composer).toHaveValue("Keep my next question");
    // New live content must not pull someone away from an earlier message.
    const long = Array.from({ length: 50 }, (_, i) => ({ role: i % 2 ? "Gator" : "You", text: `Earlier message ${i}.\n\nKeep this paragraph in view while Gator works.` }));
    await publish({ ...state, messages: long });
    const scroll = page.locator(".conversation-scroll");
    await scroll.evaluate((element) => { element.scrollTop = 0; });
    await expect(page.getByRole("button", { name: "Jump to latest", exact: true })).toBeVisible();
    await publish({ ...state, messages: long, activity: { ...activity, entries: [...activity.entries, { id: "text-3", kind: "text", text: "New public update", state: "running", started_at: started }] } });
    await expect(page.getByRole("button", { name: "New activity. Jump to latest", exact: true })).toBeVisible();
    expect(await scroll.evaluate((element) => element.scrollTop)).toBe(0);
    expect(errors).toEqual([]);
  } finally { await closeDesktop(app); await rm(directory, { recursive: true, force: true }); }
});

test("real runner narration and tools survive a desktop restart without replaying work", async () => {
  const directory = await temporaryDirectory(join(tmpdir(), "gator-activity-runtime-e2e-"));
  const workspace = join(directory, "workspace");
  await mkdir(workspace);
  await writeFile(join(workspace, "note.md"), "Decision: use sage Tide. Internal fixture body must stay out of the activity projection.");
  let release = () => {};
  const gate = new Promise<void>((resolve) => { release = resolve; });
  let requests = 0;
  const server = createServer((req, res) => {
    let body = "";
    req.on("data", (data) => { body += data; });
    req.on("end", async () => {
      requests++;
      const request = JSON.parse(body);
      const hasResult = request.messages?.some((message: { role: string }) => message.role === "tool");
      if (hasResult) await gate;
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ id: "activity-fixture", choices: [{ message: hasResult ? { role: "assistant", content: "Sage Tide is selected." } : { role: "assistant", content: "I’ll read the note.", tool_calls: [{ id: "read-note", type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/note.md" }) } }] }, finish_reason: hasResult ? "stop" : "tool_calls" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  const env = { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const binary = resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime");
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
  cli(["provider", "add", "activity-fixture", "--base-url", `http://127.0.0.1:${port}/v1/chat/completions`, "--model", "fixture-model"]);
  cli(["config", "set", "default-provider", "activity-fixture"]);
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const launch = () => electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  let app = await launch();
  try {
    let page = await app.firstWindow();
    await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toBeVisible();
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await expect.poll(() => page.evaluate(async () => (await window.gator!.snapshot()).state?.work_details?.mode)).toBe("inspect");
    await page.getByRole("textbox", { name: "Message Gator", exact: true }).fill("Read the note and tell me the selected background.");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    const live = page.getByRole("region", { name: "Gator activity", exact: true });
    await expect(live.getByText("I’ll read the note.", { exact: true })).toBeVisible();
    await expect(live.getByRole("button", { name: /Read a file/ })).toBeVisible();
    release();
    await expect(page.getByText("Sage Tide is selected.", { exact: true })).toBeVisible();
    await expect(live).toHaveCount(0);
    const snapshot = await page.evaluate(() => window.gator!.snapshot());
    const trace = snapshot.state?.messages.find((message) => message.activity)?.activity;
    expect(trace?.state).toBe("completed");
    expect(trace?.entries.map((entry) => entry.kind)).toEqual(["text", "tool"]);
    expect(JSON.stringify(trace)).not.toContain("Internal fixture body");
    expect(JSON.stringify(trace)).not.toContain("source/note.md");
    await closeDesktop(app);
    app = await launch();
    page = await app.firstWindow();
    const history = page.getByRole("button", { name: /Worked for/ });
    await expect(history).toHaveAttribute("aria-expanded", "false");
    await history.click();
    await expect(page.getByText("I’ll read the note.", { exact: true })).toBeVisible();
    await expect(page.getByText("Sage Tide is selected.", { exact: true })).toBeVisible();
    expect(requests).toBe(2);
  } finally {
    release(); await closeDesktop(app); server.closeAllConnections();
    await new Promise<void>((resolve) => server.close(() => resolve()));
    await rm(directory, { recursive: true, force: true });
  }
});
