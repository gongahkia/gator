import { _electron as electron, test, expect } from "@playwright/test";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";

const launchDesktop = (env: Record<string, string | undefined>) => {
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const definedEnv = Object.fromEntries(Object.entries(env).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
  return electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env: definedEnv });
};

test("desktop uses real Go state, preserves drafts, edits native forms, and continues saved work", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-e2e-"));
  const workspace = join(directory, "notes");
  await mkdir(workspace);
  await writeFile(join(workspace, "project.md"), "A personal project with one open decision.\n");
  let requests = 0;
  const server = createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      requests++;
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ id: "local-desktop-test", choices: [{ message: { role: "assistant", content: "The desktop is connected to Gator’s Go runtime. Your project has one open decision." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    });
  });
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const address = server.address() as { port: number };
  const env = { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const binary = resolve("runtime/gator");
  const cli = (args: string[]) => execFileSync(binary, args, { env, encoding: "utf8" });
  cli(["provider", "add", "desktop-fixture", "--base-url", `http://127.0.0.1:${address.port}/v1/chat/completions`, "--model", "fixture-model"]);
  cli(["config", "set", "default-provider", "desktop-fixture"]);
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await expect(page.getByRole("heading", { name: "What’s on your mind?" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Everyday overview" })).toBeVisible();
    await expect(page.locator(".composer-model")).toContainText("fixture-model");
    await page.screenshot({ path: "test-results/desktop-home.png" });
    await page.getByRole("textbox", { name: "Message Gator" }).fill("keep my draft 🐊");
    await page.getByRole("button", { name: "Saved knowledge", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Saved knowledge", exact: true })).toBeVisible();
    await expect(page.getByText("Gator’s view could not load.")).toHaveCount(0);
    await expect(page.getByText("No saved knowledge yet.")).toBeVisible();
    await page.screenshot({ path: "test-results/desktop-knowledge-empty.png" });
    // The sources list is genuinely null in the Go wire format on this path.
    expect(await page.evaluate(async () => (await window.gator!.snapshot()).state?.panel?.blocks?.some((block) => block.lines === null))).toBe(true);
    const noteID = cli(["note", "add", "--title", "Desktop regression note", "--body", "Remember the project decision."]).split(/\s/)[0];
    await page.getByRole("button", { name: /^refresh/i }).click();
    await page.getByRole("button", { name: /Desktop regression note/ }).click();
    await expect(page.getByRole("heading", { name: /Desktop regression note/ })).toBeVisible();
    await expect(page.getByText("No saved knowledge yet.")).toHaveCount(0);
    await expect(page.getByText("Remember the project decision.", { exact: false }).first()).toBeVisible();
    await page.screenshot({ path: "test-results/desktop-knowledge.png" });
    // Leave no recall corpus for later inference turns: they must not depend
    // on a developer's local Ollama service or create embedding reservations.
    cli(["note", "remove", noteID, "--yes"]);
    await page.getByRole("button", { name: "Gator home" }).click();
    await page.getByRole("button", { name: "Overview", exact: true }).click();
    await expect(page.getByRole("heading", { name: /overview/i }).first()).toBeVisible();
    await page.getByRole("button", { name: "Back to conversation" }).click();
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toHaveValue("keep my draft 🐊");
    await page.getByRole("button", { name: "Show work details" }).click();
    await expect(page.getByRole("complementary", { name: "Work details" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Context", exact: true })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: "Deliverables" })).toHaveAttribute("aria-selected", "true");
    await page.getByRole("tab", { name: "Deliverables" }).click();
    await expect(page.getByText("Your work lands here")).toBeVisible();
    await page.getByRole("button", { name: "Hide work details" }).click();
    await page.getByRole("button", { name: "Follow-up checks", exact: true }).click();
    await page.getByRole("button", { name: /new check/i }).click();
    await expect(page.getByRole("textbox", { name: "Desired outcome", exact: true })).toBeVisible();
    await page.getByRole("textbox", { name: "Title", exact: true }).fill("Watch my project");
    await page.getByRole("textbox", { name: "Desired outcome", exact: true }).fill("Tell me when the open decision changes.");
    await page.getByRole("button", { name: /^review/i }).click();
    await expect(page.getByRole("heading", { name: "Review ongoing check" })).toBeVisible();
    await expect(page.getByText("Watch my project", { exact: false }).first()).toBeVisible();
    await page.screenshot({ path: "test-results/desktop-review.png" });
    // Back cancels the review; no background service or real check is started.
    await page.getByRole("button", { name: "Gator home" }).click();
    await page.getByRole("button", { name: "Leave view", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toHaveValue("keep my draft 🐊");
    await page.getByRole("textbox", { name: "Message Gator" }).fill("Summarize my project in one sentence.");
    await page.getByRole("button", { name: "Send message" }).click();
    await expect(page.getByText("The desktop is connected to Gator’s Go runtime.", { exact: false }).first()).toBeVisible();
    expect(requests).toBeGreaterThan(0);
    await page.getByRole("textbox", { name: "Message Gator" }).fill("What should I decide next?");
    await page.getByRole("button", { name: "Send message" }).click();
    await expect(page.locator(".message-byline", { hasText: "Gator" })).toHaveCount(2);
    const saved = JSON.parse(cli(["overview", "--json"]));
    expect(saved).toBeTruthy();
    await expect(page.getByRole("button", { name: "Usage & limits", exact: true })).toBeEnabled();
    await page.getByRole("button", { name: "Usage & limits", exact: true }).click();
    // Count every call seen by the local provider, including auxiliary work.
    await expect(page.getByRole("region", { name: "Provider requests", exact: true })).toContainText(`${requests} used`);
    await expect(page.getByRole("region", { name: "Tokens", exact: true })).toContainText(`${requests * 20} used`);
    await page.screenshot({ path: "test-results/desktop-usage-with-activity.png" });
    await page.getByRole("button", { name: "Back to conversation" }).click();
    for (const size of [{ width: 1440, height: 960 }, { width: 1024, height: 768 }, { width: 800, height: 600 }]) {
      await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].setSize(value.width, value.height), size);
      await expect(page.getByRole("textbox", { name: "Message Gator" })).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: `test-results/desktop-${size.width}.png` });
    }
    await page.getByRole("button", { name: "Show work details" }).click();
    await expect(page.getByRole("complementary", { name: "Work details" })).toBeVisible();
    await page.screenshot({ path: "test-results/desktop-details-800.png" });
    await page.getByRole("button", { name: "Hide work details" }).click();
    await page.getByRole("button", { name: "Search commands" }).click();
    await page.getByRole("textbox", { name: "Find a command" }).fill("help");
    await page.getByRole("textbox", { name: "Find a command" }).press("Enter");
    await expect(page.getByRole("dialog", { name: "Search commands" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Search commands" })).toBeFocused();
    // Exercise the titlebar-free grid used by fullscreen and Linux without
    // switching the developer's macOS Space during this test.
    await page.evaluate(() => { document.querySelector<HTMLElement>(".app-shell")!.dataset.fullscreen = "true"; });
    await expect(page.locator(".titlebar")).toBeHidden();
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toBeVisible();
    expect(await page.locator(".main-area").evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThan(300);
    expect(errors).toEqual([]);
    expect(await page.evaluate(() => typeof (window as unknown as { require?: unknown }).require)).toBe("undefined");
  } finally {
    await application.close();
    await new Promise<void>((done) => server.close(() => done()));
    await rm(directory, { recursive: true, force: true });
  }
});

test("dashboard scroll respects interaction and reduced motion; usage controls keep the Go review flow", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-dashboard-e2e-"));
  const env = { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const binary = resolve("runtime/gator");
  const cli = (args: string[]) => execFileSync(binary, args, { env, encoding: "utf8" });
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("heading", { name: "What’s on your mind?" })).toBeVisible();
    await page.emulateMedia({ reducedMotion: "no-preference" });
    for (const copy of ["Refreshes quietly every 15 seconds", "Your work stays connected. You choose what happens next.", "↵ send", "Same Gator profile as your terminal", "A little more continuity", "A good place to start", "Built around your ongoing work", "Personal workspace", "Local Gator profile"]) {
      await expect(page.getByText(copy, { exact: false })).toHaveCount(0);
    }
    const track = page.locator(".starter-track");
    await page.mouse.move(1, 1);
    // Inspect browser-managed state transitions with motion slowed to 10%.
    // Native scroll timing remains owned by Chromium.
    const session = await page.context().newCDPSession(page);
    await session.send("Animation.setPlaybackRate", { playbackRate: 0.1 });
    await expect.poll(() => track.evaluate((element) => element.scrollLeft), { timeout: 10_000 }).toBeGreaterThan(20);
    await session.send("Animation.setPlaybackRate", { playbackRate: 1 });
    await page.getByRole("button", { name: "Pause dashboard scrolling", exact: true }).click();
    await expect(page.getByRole("button", { name: "Resume dashboard scrolling", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Previous dashboard cards" }).click();
    await expect.poll(() => track.evaluate((element) => element.scrollLeft)).toBeLessThan(5);
    await page.clock.install();
    await page.getByRole("button", { name: "Resume dashboard scrolling", exact: true }).click();
    await page.mouse.move(1, 1);
    // Focus inside the carousel pauses it, even when the pointer leaves.
    await page.clock.runFor(6500);
    expect(await track.evaluate((element) => element.scrollLeft)).toBeLessThan(5);
    await page.getByRole("textbox", { name: "Message Gator" }).focus();
    await track.hover();
    await page.clock.runFor(6500);
    expect(await track.evaluate((element) => element.scrollLeft)).toBeLessThan(5);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await expect(page.getByRole("button", { name: "Resume dashboard scrolling", exact: true })).toBeDisabled();
    await page.mouse.move(1, 1);
    await page.clock.runFor(6500);
    expect(await track.evaluate((element) => element.scrollLeft)).toBeLessThan(5);
    await page.clock.resume();
    // Manual navigation remains available with reduced motion.
    await page.getByRole("button", { name: "Next dashboard cards" }).click();
    expect(await track.evaluate((element) => element.scrollLeft)).toBeGreaterThan(20);
    await page.getByRole("button", { name: /Find what you’ve saved/ }).click();
    await expect(page.getByRole("heading", { name: "Saved knowledge", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Usage & limits", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Usage & limits", exact: true })).toBeVisible();
    await expect(page.getByText("Uncapped", { exact: true })).toHaveCount(3);
    await expect(page.getByRole("meter")).toHaveCount(0);
    await page.screenshot({ path: "test-results/desktop-usage-uncapped.png" });
    await page.getByRole("button", { name: "Edit limits", exact: true }).click();
    await page.getByRole("textbox", { name: "Provider requests per day (0 uncapped)", exact: true }).fill("100");
    await page.getByRole("textbox", { name: "Tokens per day (0 uncapped)", exact: true }).fill("10000");
    await page.getByRole("textbox", { name: "Concurrent requests (0 uncapped)", exact: true }).fill("2");
    await page.getByRole("button", { name: /^review/i }).click();
    await expect(page.locator(".workflow")).toContainText("Provider requests per day (0 uncapped): 100");
    expect(cli(["usage", "status"])).toContain("uncapped requests/day");
    await page.getByRole("button", { name: /^confirm/i }).click();
    await expect(page.getByRole("meter", { name: "Provider requests remaining" })).toHaveAttribute("max", "100");
    await expect(page.getByRole("meter", { name: "Tokens remaining" })).toHaveAttribute("max", "10000");
    expect(cli(["usage", "status"])).toContain("100 requests/day");
    await page.getByRole("button", { name: "Pause requests", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Pause/resume provider requests", exact: true })).toBeVisible();
    expect(cli(["usage", "status"])).toContain("Requests allowed");
    await page.getByRole("button", { name: /^confirm/i }).click();
    await expect(page.getByRole("button", { name: "Resume requests", exact: true })).toBeVisible();
    expect(cli(["usage", "status"])).toContain("All new provider requests paused");
    await page.getByText("Usage details", { exact: true }).click();
    await expect(page.getByText("Failed attempts", { exact: true })).toBeVisible();
    await page.screenshot({ path: "test-results/desktop-usage-details.png" });
    await page.getByText("Usage details", { exact: true }).click();
    for (const size of [{ width: 1440, height: 960 }, { width: 1024, height: 768 }, { width: 800, height: 600 }]) {
      await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].setSize(value.width, value.height), size);
      await expect(page.getByRole("heading", { name: "Usage & limits", exact: true })).toBeVisible();
      await page.locator(".usage-page").evaluate((element) => element.scrollTo({ top: 0 }));
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: `test-results/desktop-usage-${size.width}.png` });
    }
  } finally { await application.close(); await rm(directory, { recursive: true, force: true }); }
});

test("first-run model setup uses native fields and keeps credentials out of display state", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-setup-e2e-"));
  const env = { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("heading", { name: "What’s on your mind?" })).toBeVisible();
    await page.getByRole("button", { name: "Model settings", exact: true }).click();
    await page.getByRole("button", { name: /^Cloud API key/ }).click();
    await expect(page.getByRole("heading", { name: "Cloud models", exact: true })).toBeVisible();
    await page.getByRole("button", { name: /^configure /i }).click();
    const key = page.locator('input[type="password"]');
    await expect(key).toHaveCount(1);
    await key.fill("isolated-test-value");
    await page.screenshot({ path: "test-results/desktop-model-setup.png" });
    const display = await page.evaluate(async () => JSON.stringify(await window.gator!.snapshot()));
    expect(display).not.toContain("isolated-test-value");
    await page.getByRole("button", { name: "Gator home" }).click();
    await expect(page.getByRole("alertdialog")).toBeVisible();
    await page.getByRole("button", { name: "Keep editing" }).press("Shift+Tab");
    await expect(page.getByRole("button", { name: "Leave view" })).toBeFocused();
    await page.getByRole("button", { name: "Keep editing" }).click();
    await expect(key).toHaveValue("isolated-test-value");
    await page.getByRole("button", { name: "Gator home" }).click();
    await page.getByRole("button", { name: "Leave view" }).click();
    await expect(page.getByRole("heading", { name: "What’s on your mind?" })).toBeVisible();
    expect(await page.evaluate(async () => JSON.stringify(await window.gator!.snapshot()))).not.toContain("isolated-test-value");
  } finally { await application.close(); await rm(directory, { recursive: true, force: true }); }
});
