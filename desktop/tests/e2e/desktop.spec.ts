import { _electron as electron, test, expect, type Page } from "@playwright/test";
import { openSurface } from "./navigation";
import { closeDesktop, temporaryDirectory as mkdtemp } from "./lifecycle";
import { mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";

const launchDesktop = (env: Record<string, string | undefined>) => {
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const definedEnv = Object.fromEntries(Object.entries(env).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
  return electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env: { ...definedEnv, GATOR_INTERNAL_COMMANDS: "" } });
};

const openSettingsCategory = async (page: Page, name: string) => {
  const navigation = page.getByRole("navigation", { name: "Settings categories" });
  if (!await navigation.isVisible()) await page.getByRole("button", { name: "Settings", exact: true }).click();
  const category = navigation.getByRole("button", { name, exact: true });
  if (await category.getAttribute("aria-current") !== "page") await category.click();
};

const settlePresentation = async (page: Page) => {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished));
  });
};

test("relative profile overrides keep the existing platform interpretation", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-relative-profile-e2e-"));
  const workspace = join(directory, "workspace");
  await mkdir(workspace);
  const root = process.platform === "win32" ? directory : workspace;
  const binary = resolve(`runtime/gator-runtime${process.platform === "win32" ? ".exe" : ""}`);
  const env = { ...process.env, GATOR_STATE_DIR: "profile-state", GATOR_CONFIG_DIR: "profile-config", GATOR_DESKTOP_WORKSPACE: workspace, GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  execFileSync(binary, ["note", "add", "--title", "Kept relative profile", "--body", "Existing private data remains in the same profile."], { cwd: directory, env: { ...env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(root, "profile-state"), GATOR_CONFIG_DIR: join(root, "profile-config") } });
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const application = await electron.launch({ executablePath, args: executablePath ? [] : [resolve(".")], cwd: directory, env: { ...Object.fromEntries(Object.entries(env).filter((entry): entry is [string, string] => typeof entry[1] === "string")), GATOR_INTERNAL_COMMANDS: "" } });
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toBeVisible();
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/note list" }));
    await expect(page.locator(".main-area")).toContainText("Kept relative profile");
    await closeDesktop(application);
    // The update guard must resolve the very same profile from the same cwd.
    execFileSync(binary, ["desktop-update-guard"], { cwd: workspace, input: "", env: { ...env, GATOR_DESKTOP_UPDATE_GUARD: "1", ...(process.platform === "win32" ? { GATOR_STATE_DIR: join(root, "profile-state") } : {}) } });
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("About & updates explains development builds and preserves an unsent message", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-updates-e2e-"));
  const application = await launchDesktop({ ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") });
  try {
    const page = await application.firstWindow();
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep this message through update review");
    await page.getByRole("button", { name: "Settings", exact: true }).click();
    await page.getByRole("button", { name: "About & updates", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "About & updates", exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Manual updates", { exact: true })).toBeVisible();
    await expect(dialog.getByText(/This development build uses manual updates/)).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Check for updates", exact: true })).toHaveCount(0);
    const rejection = await page.evaluate(async () => {
      try { await window.gator!.updates({ action: "install" }); return "accepted"; }
      catch (error) { return String(error); }
    });
    expect(rejection).toContain("manual updates");
    expect((await page.evaluate(() => window.gator!.snapshot())).updates?.phase).toBe("manual");
    const bundledUpdater = await application.evaluate(async ({ app }) => {
      const path = process.getBuiltinModule("path");
      const require = process.getBuiltinModule("module").createRequire(path.join(app.getAppPath(), "package.json"));
      const library = require("electron-updater");
      const controller = require("./electron/updates.mjs");
      const updates = new controller.Updates({ updater: library.autoUpdater, version: app.getVersion(), supported: true, blocked: () => "", confirm: async () => false, prepare: async () => async () => {} });
      return { phase: updates.snapshot().phase, autoDownload: library.autoUpdater.autoDownload, autoInstall: library.autoUpdater.autoInstallOnAppQuit };
    });
    expect(bundledUpdater).toEqual({ phase: "idle", autoDownload: false, autoInstall: false });
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-about-updates.png" });
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(page.getByRole("button", { name: "About & updates", exact: true })).toBeFocused();
    await page.getByRole("button", { name: "Back to app", exact: true }).click();
    await expect(composer).toHaveValue("Keep this message through update review");
    expect((await page.evaluate(() => window.gator!.snapshot())).state!.messages).toEqual([]);
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("unsent drafts survive reload and reconnect without submission, and retention can be cleared", async () => {
  test.setTimeout(90_000); // Reconnect plus two isolated app launches and native teardown.
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-drafts-e2e-"));
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  let application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep my unsent work 🐊\nNothing should run automatically.");
    await expect.poll(() => page.evaluate(async () => (await window.gator!.drafts({ action: "list" })).drafts[0]?.text)).toContain("Keep my unsent work");
    await page.reload();
    await expect(composer).toHaveValue("Keep my unsent work 🐊\nNothing should run automatically.");
    await expect(page.getByText("Draft recovered. Review it before sending.")).toBeVisible();
    const snapshot = await page.evaluate(() => window.gator!.snapshot());
    await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", { ...value, connection: "disconnected", error: "Simulated transport loss" }), snapshot);
    await expect(page.getByText("Connection lost", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Send message", exact: true })).toBeDisabled();
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-disconnected.png" });
    await composer.fill("Still editable while disconnected");
    await page.getByRole("button", { name: "Reconnect", exact: true }).click();
    await expect(page.getByText(/A new local session is ready/)).toBeVisible();
    await expect(composer).toHaveValue("Still editable while disconnected");
    expect((await page.evaluate(() => window.gator!.snapshot())).state!.messages).toEqual([]);
    await page.getByRole("button", { name: "Saved drafts", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Saved drafts", exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Still editable while disconnected", { exact: true })).toBeVisible();
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-saved-drafts.png" });
    await dialog.getByRole("checkbox", { name: "Keep unsent messages on this device" }).uncheck();
    await expect(dialog.getByText("No saved drafts.", { exact: true })).toBeVisible();
    await dialog.getByRole("button", { name: "Close saved drafts" }).click();
    await expect(page.getByRole("button", { name: "Saved drafts", exact: true })).toBeFocused();
    await page.reload();
    await expect(composer).toHaveValue("");
    expect((await page.evaluate(() => window.gator!.drafts({ action: "list" }))).enabled).toBe(false);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-draft-recovery.png" });
    await closeDesktop(application);
    application = await launchDesktop({ ...env, GATOR_STATE_DIR: join(directory, "other-state"), GATOR_CONFIG_DIR: join(directory, "other-config") });
    const otherPage = await application.firstWindow();
    await expect(otherPage.getByRole("textbox", { name: "Message Gator", exact: true })).toHaveValue("");
    expect(await otherPage.evaluate(() => window.gator!.drafts({ action: "list" }))).toEqual({ enabled: true, drafts: [] });
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("long conversations retain context while searching and signal activity without moving the reader", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-transcript-e2e-"));
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toBeVisible();
    const snapshot = await page.evaluate(() => window.gator!.snapshot());
    const messages = Array.from({ length: 300 }, (_, index) => ({ role: index % 2 ? "Gator" : "You", text: `Message ${index}: surrounding context stays readable.\n\n${index === 101 || index === 201 ? "Find this needle **in** context. Literal [.*] stays literal." : "A useful project detail with enough text to wrap naturally."}` }));
    const state = { ...snapshot.state!, home: false, conversation_id: "work-long-transcript", title: "Long transcript", messages };
    await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", value), { ...snapshot, state });
    await expect(page.locator(".message")).toHaveCount(300);
    const scroll = page.locator(".conversation-scroll");
    await scroll.evaluate((element) => { element.scrollTop = 0; });
    await expect(page.getByRole("button", { name: "Jump to latest", exact: true })).toBeVisible();
    const updated = { ...state, messages: [...messages, { role: "Gator", text: "A new update while you were reading." }] };
    await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", value), { ...snapshot, state: updated });
    await expect(page.getByRole("button", { name: "New activity. Jump to latest", exact: true })).toBeVisible();
    expect(await scroll.evaluate((element) => element.scrollTop)).toBe(0);
    await page.keyboard.press(`${process.platform === "darwin" ? "Meta" : "Control"}+f`);
    const search = page.getByRole("textbox", { name: "Search conversation" });
    await search.fill("needle in context");
    await expect(page.locator(".search-count")).toHaveText("1 of 2");
    await expect(page.locator(".message")).toHaveCount(301);
    expect(await page.evaluate(() => [...CSS.highlights.get("gator-search-active")!].map((range) => range.toString()))).toEqual(["needle in context"]);
    await search.press("Enter");
    await expect(page.locator(".search-count")).toHaveText("2 of 2");
    await search.press("Shift+Enter");
    await expect(page.locator(".search-count")).toHaveText("1 of 2");
    await search.fill("[.*]");
    await expect(page.locator(".search-count")).toHaveText("1 of 2");
    await search.fill("not present anywhere");
    await expect(page.locator(".search-count")).toHaveText("No matches");
    await expect(page.locator(".message")).toHaveCount(301);
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 600));
    await search.fill("needle in context");
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-transcript-search-800.png" });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.keyboard.press("Escape");
    await expect(search).toHaveCount(0);
    expect(await page.evaluate(() => CSS.highlights.size)).toBe(0);
    const readingPosition = await scroll.evaluate((element) => element.scrollTop);
    await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", value), { ...snapshot, state: { ...updated, mode: "panel", panel: { kind: "report", title: "Reading details", blocks: [{ title: "Details", lines: ["A temporary view."] }] } } });
    await expect(page.getByRole("heading", { name: "Reading details", exact: true })).toBeVisible();
    await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", value), { ...snapshot, state: updated });
    await expect.poll(() => scroll.evaluate((element) => element.scrollTop)).toBeCloseTo(readingPosition, 0);
    await page.getByRole("button", { name: /Jump to latest/ }).click();
    await expect(page.getByText("A new update while you were reading.", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: /Jump to latest/ })).toHaveCount(0);
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toBeFocused();
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("recent topics default to a flat list, offer folder grouping, and isolate drafts", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-recents-e2e-"));
  const first = join(directory, "area-a/shared"), second = join(directory, "area-b/shared");
  await mkdir(first, { recursive: true }); await mkdir(second, { recursive: true });
  for (let index = 0; index < 6; index++) {
    const id = `work-test-${index}`, root = join(directory, "state/gator/conversations", id);
    await mkdir(root, { recursive: true });
    await writeFile(join(root, "conversation.json"), JSON.stringify({ version: 2, id, title: `Plan ${index}`, source_path: index < 5 ? first : second, snapshot_id: "snap-test", created_at: new Date(0).toISOString(), updated_at: new Date(1000000 - index).toISOString() }), { mode: 0o600 });
  }
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: first };
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    const recent = page.getByRole("region", { name: "Recent conversations", exact: true });
    await expect(recent).toBeVisible();
    await expect(recent.locator(".recent-workspace")).toHaveCount(0);
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 4", exact: true })).toBeVisible();
    await recent.getByRole("button", { name: "Group topics by folder", exact: true }).click();
    await expect(recent.getByRole("button", { name: "area-a/shared", exact: true })).toBeVisible();
    await expect(recent.getByRole("button", { name: "area-b/shared", exact: true })).toBeVisible();
    await recent.getByRole("button", { name: "Show 1 more", exact: true }).click();
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 4", exact: true })).toBeVisible();
    await recent.getByRole("button", { name: "Show fewer", exact: true }).click();
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 4", exact: true })).toHaveCount(0);
    await recent.getByRole("button", { name: "area-a/shared", exact: true }).press("Enter");
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 0", exact: true })).toBeHidden();
    const filter = recent.getByRole("searchbox", { name: "Filter recent conversations" });
    await filter.fill("Plan 4");
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 4", exact: true })).toBeVisible();
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 0", exact: true })).toHaveCount(0);
    await filter.fill("no matching title");
    await expect(recent.getByText("No matching recent conversations", { exact: true })).toBeVisible();
    await filter.press("Escape");
    await expect(filter).toHaveValue("");
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 0", exact: true })).toBeHidden();
    await recent.getByRole("button", { name: "area-a/shared", exact: true }).press("Enter");
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 0", exact: true })).toBeVisible();
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 4", exact: true })).toHaveCount(0);
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    const open = async (index: number) => { await recent.getByRole("button", { name: `Open conversation: Plan ${index}`, exact: true }).click(); await expect.poll(() => page.evaluate(async () => (await window.gator!.snapshot()).state?.conversation_id)).toBe(`work-test-${index}`); };
    await composer.fill("An unsent message for Gator");
    await page.getByRole("button", { name: "New topic", exact: true }).click();
    await expect(composer).toHaveValue("");
    await composer.fill("A separate topic in the same folder");
    await page.getByRole("button", { name: "Talk with Gator", exact: true }).click();
    await expect(composer).toHaveValue("An unsent message for Gator");
    expect((await page.evaluate(() => window.gator!.snapshot())).state!.conversation_id).toBeUndefined();
    await open(0); await composer.fill("Draft for the first project");
    await open(5); await expect(composer).toHaveValue(""); await composer.fill("Draft for the second project");
    await open(0); await expect(composer).toHaveValue("Draft for the first project");
    await page.reload(); await expect(composer).toHaveValue("Draft for the first project");
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 0", exact: true })).toHaveAttribute("aria-current", "page");
    const current = await page.evaluate(() => window.gator!.snapshot());
    await application.evaluate(({ BrowserWindow }, snapshot) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", { ...snapshot, state: { ...snapshot.state!, running: true, pending_approval: { id: 99 } } }), current);
    await expect(recent.getByText("Needs review", { exact: true })).toBeVisible();
    await expect(recent.getByRole("button", { name: "Open conversation: Plan 0", exact: true })).toBeDisabled();
    await application.evaluate(({ BrowserWindow }, snapshot) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", snapshot), current);
    await openSurface(page, "Follow-up checks");
    await page.getByRole("button", { name: /new check/i }).click();
    await page.getByRole("textbox", { name: "Title", exact: true }).fill("Keep my form edit");
    await recent.getByRole("button", { name: "Open conversation: Plan 5", exact: true }).click();
    await expect(page.getByRole("alertdialog")).toBeVisible();
    await page.getByRole("button", { name: "Keep editing", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Keep my form edit");
    await recent.getByRole("button", { name: "Open conversation: Plan 5", exact: true }).click();
    await page.getByRole("button", { name: "Leave view", exact: true }).click();
    await expect(composer).toHaveValue("Draft for the second project");
    await page.getByRole("button", { name: "Saved drafts", exact: true }).click();
    const manager = page.getByRole("dialog", { name: "Saved drafts", exact: true });
    await manager.locator("article").filter({ hasText: "Draft for the first project" }).getByRole("button", { name: "Add to current message", exact: true }).click();
    await expect(composer).toHaveValue("Draft for the second project\n\nDraft for the first project");
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(1024, 768));
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-recent-work.png" });
    await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
    await expect(recent).toBeHidden();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("motion is interruptible, releases dismissed controls, and honors reduced motion", async () => {
  test.setTimeout(120_000); // Includes inspection at 10% playback and native startup.
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-motion-e2e-"));
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.emulateMedia({ reducedMotion: "no-preference" });
    await expect(page.getByRole("heading", { name: "Hi. I’m Gator." })).toBeVisible();
    const shell = page.locator(".app-shell");
    const sidebar = page.getByRole("complementary", { name: "Main navigation" });
    const width = () => sidebar.evaluate((element) => element.getBoundingClientRect().width);
    const toggle = page.locator(".collapse-button");
    expect(await shell.evaluate((element) => element.getAnimations().length)).toBe(0);
    const session = await page.context().newCDPSession(page);
    await session.send("Animation.setPlaybackRate", { playbackRate: 0.1 });
    await toggle.click();
    await expect.poll(width).toBeLessThan(239);
    expect(await width()).toBeGreaterThan(68);
    await page.evaluate(() => {
      for (const animation of document.getAnimations()) {
        if (animation instanceof CSSTransition) { animation.pause(); animation.currentTime = 100; }
      }
    });
    await page.screenshot({ path: "test-results/desktop-motion-sidebar-midpoint.png" });
    // Reverse before completion. The focused toggle and its DOM node persist.
    await session.send("Animation.setPlaybackRate", { playbackRate: 1 });
    await page.evaluate(() => document.getAnimations().forEach((animation) => animation.play()));
    await toggle.press("Enter");
    await expect.poll(width).toBeCloseTo(240, 0);
    await expect(toggle).toBeFocused();
    await toggle.press("Enter");
    await toggle.press("Enter");
    await expect.poll(width).toBeCloseTo(240, 0);

    await session.send("Animation.setPlaybackRate", { playbackRate: 0.1 });
    await page.getByRole("button", { name: "Conversation information" }).click();
    await expect(page.getByRole("complementary", { name: "Conversation information" })).toBeVisible();
    await page.getByRole("button", { name: "Hide conversation information" }).click();
    await expect(page.locator('.workspace-panel[data-open="false"]')).toHaveAttribute("inert", "");
    await expect(page.getByRole("complementary", { name: "Conversation information" })).toHaveCount(0);
    await session.send("Animation.setPlaybackRate", { playbackRate: 1 });
    await expect(page.locator(".workspace-panel")).toHaveCount(0);

    await session.send("Animation.setPlaybackRate", { playbackRate: 0.1 });
    await page.getByRole("button", { name: "Search commands", exact: true }).press("Enter");
    await expect(page.getByRole("textbox", { name: "Find a command" })).toBeFocused();
    await page.screenshot({ path: "test-results/desktop-motion-command-menu.png" });
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Search commands" })).toHaveCount(0);
    await expect(page.locator('.modal-backdrop[data-open="false"]')).toHaveAttribute("inert", "");
    await expect(page.getByRole("button", { name: "Search commands", exact: true })).toBeFocused();
    // Reopen while its exit is still running; no stale completion may remove it.
    await page.keyboard.press(`${process.platform === "darwin" ? "Meta" : "Control"}+k`);
    await expect(page.getByRole("textbox", { name: "Find a command" })).toBeFocused();
    await session.send("Animation.setPlaybackRate", { playbackRate: 1 });
    await page.keyboard.press("Escape");
    await expect(page.locator(".modal-backdrop")).toHaveCount(0);

    await page.getByRole("button", { name: "Search commands", exact: true }).click();
    await page.getByRole("textbox", { name: "Find a command" }).fill("Following");
    await page.getByRole("textbox", { name: "Find a command" }).press("Enter");
    await expect(page.getByRole("complementary", { name: "Following", exact: true })).toBeVisible();
    await expect(page.getByRole("dialog", { name: "Search commands", exact: true })).toHaveCount(0);
    await settlePresentation(page);
    await session.send("Animation.setPlaybackRate", { playbackRate: 0.1 });
    await page.getByRole("button", { name: "Conversation information", exact: true }).click();
    await expect(page.locator('.library-drawer[data-open="false"]')).toContainText("Your ongoing goals and configured checks.");
    await expect(page.locator('.library-drawer[data-open="false"]')).toHaveAttribute("inert", "");
    await session.send("Animation.setPlaybackRate", { playbackRate: 1 });
    await expect(page.locator(".library-drawer")).toHaveCount(0);
    await page.getByRole("button", { name: "Hide conversation information", exact: true }).click();
    await expect(page.locator(".workspace-panel")).toHaveCount(0);

    await page.evaluate(() => {
      const state = window as unknown as { viewAnimations: number };
      state.viewAnimations = 0;
      const animate = Element.prototype.animate;
      Element.prototype.animate = function (keyframes, options) {
        if (this.classList.contains("view-content")) state.viewAnimations++;
        return animate.call(this, keyframes, options);
      };
    });
    const entrances = () => page.evaluate(() => (window as unknown as { viewAnimations: number }).viewAnimations);
    await openSurface(page, "Saved knowledge");
    await expect(page.getByRole("heading", { name: "Saved knowledge", exact: true })).toBeVisible();
    expect(await entrances()).toBe(1);
    await page.getByRole("button", { name: /^refresh$/i }).click();
    expect(await entrances()).toBe(1);
    await openSurface(page, "Follow-up checks");
    await page.getByRole("button", { name: /new check/i }).click();
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toBeVisible();
    const beforeTyping = await entrances();
    await title.fill("Keep my form during motion");
    expect(await entrances()).toBe(beforeTyping);

    await page.emulateMedia({ reducedMotion: "reduce" });
    await toggle.click();
    await expect.poll(width).toBeCloseTo(68, 0);
    expect(await shell.evaluate((element) => element.getAnimations().length)).toBe(0);
    await page.getByRole("button", { name: "Gator home" }).click();
    await expect(page.getByRole("alertdialog")).toBeVisible();
    await page.getByRole("button", { name: "Keep editing" }).press("Shift+Tab");
    await expect(page.getByRole("button", { name: "Leave view" })).toBeFocused();
    await page.getByRole("button", { name: "Keep editing" }).click();
    await expect(title).toHaveValue("Keep my form during motion");
    await expect(page.locator(".modal-backdrop")).toHaveCount(0);
    await page.getByRole("button", { name: "Gator home" }).click();
    await page.getByRole("button", { name: "Leave view" }).click();
    await openSurface(page, "Saved knowledge");
    await expect(page.getByRole("heading", { name: "Saved knowledge", exact: true })).toBeVisible();
    expect(await entrances()).toBe(beforeTyping);
    expect(await page.locator(".view-content").evaluate((element) => element.getAnimations().length)).toBe(0);
    for (const size of [{ width: 1024, panelWidth: 275 }, { width: 800, panelWidth: 300 }]) {
      await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].setSize(value, 768), size.width);
      await page.getByRole("button", { name: "Conversation information" }).click();
      await expect.poll(width).toBeCloseTo(68, 0);
      await expect.poll(() => page.locator(".workspace-panel").evaluate((element) => element.getBoundingClientRect().width)).toBeCloseTo(size.panelWidth, 0);
      await page.screenshot({ path: `test-results/desktop-motion-details-${size.width}.png` });
      if (size.width === 800) {
        await page.evaluate(() => { document.querySelector<HTMLElement>(".app-shell")!.dataset.fullscreen = "true"; });
        expect(await page.locator(".workspace-panel").evaluate((element) => element.getBoundingClientRect().top)).toBe(0);
        await page.evaluate(() => { document.querySelector<HTMLElement>(".app-shell")!.dataset.fullscreen = "false"; });
      }
      await page.getByRole("button", { name: "Close conversation information" }).click();
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    expect(errors).toEqual([]);
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("queued messages can be edited, reordered, removed, and resumed without losing drafts", async () => {
  test.setTimeout(90_000);
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-queue-e2e-"));
  const workspace = join(directory, "workspace");
  await mkdir(workspace);
  await writeFile(join(workspace, "project.md"), "A local queue test.\n");
  let hold = true;
  const waiting: (() => void)[] = [];
  const requests: string[] = [];
  const release = () => { hold = false; waiting.splice(0).forEach((reply) => reply()); };
  const server = createServer((req, res) => {
    let body = "";
    req.on("data", (chunk) => { body += chunk; });
    req.on("end", () => {
      requests.push(body);
      const routing = body.includes("Classify the user's immediate request");
      const reply = () => {
        if (res.destroyed) return;
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ id: "local-queue-test", choices: [{ message: { role: "assistant", content: routing ? '{"mode":"inspect"}' : "Finished the local task." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
      };
      if (hold && !routing) waiting.push(reply); else reply();
    });
  });
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const address = server.address() as { port: number };
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const cli = (args: string[]) => execFileSync(resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), args, { env, encoding: "utf8" });
  cli(["provider", "add", "queue-fixture", "--base-url", `http://127.0.0.1:${address.port}/v1/chat/completions`, "--model", "fixture-model"]);
  cli(["config", "set", "default-provider", "queue-fixture"]);
  const application = await launchDesktop(env);
  const page = await application.firstWindow();
  const state = () => page.evaluate(async () => (await window.gator!.snapshot()).state!);
  try {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    const composer = page.getByRole("textbox", { name: "Message Gator" });
    const queue = page.getByRole("region", { name: "Queued messages" });
    await composer.fill("Start the first task.");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect.poll(() => requests.length).toBeGreaterThan(0);
    await expect.poll(async () => (await state()).guidance_id).toBeTruthy();
    for (const text of ["First follow-up", "Second follow-up", "Remove this request"]) {
      await composer.fill(text);
      await composer.press("Enter");
      await expect(composer).toHaveValue("");
    }
    await expect(queue.locator(".queue-preview")).toHaveText(["First follow-up", "Second follow-up", "Remove this request"]);
    await expect(page.getByRole("combobox", { name: "When to use this message" })).toHaveCount(0);
    await composer.fill("Preserve my separate draft");
    await queue.getByRole("button", { name: "Edit queued message 1", exact: true }).click();
    const editor = queue.getByRole("textbox", { name: "Edit queued message", exact: true });
    await expect(editor).toBeFocused();
    await editor.fill("Discard this edit");
    await editor.press("Escape");
    expect((await state()).running).toBe(true);
    await expect(composer).toHaveValue("Preserve my separate draft");
    await queue.getByRole("button", { name: "Edit queued message 1", exact: true }).click();
    await editor.fill("Edited first follow-up 🐊\nKeep the useful detail.");
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 600));
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-message-queue-edit.png" });
    expect(await composer.evaluate((element) => element.getBoundingClientRect().bottom <= innerHeight)).toBe(true);
    expect(await queue.getByRole("button", { name: "Save changes", exact: true }).evaluate((element) => element.getBoundingClientRect().bottom <= element.closest(".message-queue")!.getBoundingClientRect().bottom)).toBe(true);
    await editor.press("Control+Enter");
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(1440, 960));
    await expect(editor).toHaveCount(0);
    await expect(queue.locator(".queue-preview").first()).toContainText("Edited first follow-up 🐊");
    await expect(composer).toHaveValue("Preserve my separate draft");
    await queue.getByRole("button", { name: "Remove queued message 3", exact: true }).click();
    await expect(queue.locator(".queued-message")).toHaveCount(2);
    await expect(queue.getByRole("button", { name: /^Move queued message .* (up|down)$/ })).toHaveCount(0);
    await queue.getByRole("button", { name: "Reorder queued message 2", exact: true }).dragTo(queue.locator(".queued-message").first(), { targetPosition: { x: 80, y: 3 } });
    await expect(queue.locator(".queue-preview").first()).toHaveText("Second follow-up");
    const handle = queue.getByRole("button", { name: "Reorder queued message 1", exact: true });
    await handle.focus(); await handle.press("Space"); await handle.press("ArrowDown");
    await expect(queue.locator('[data-drop-edge="after"]')).toHaveCount(1);
    await handle.press("Escape");
    expect((await state()).running).toBe(true);
    await expect(queue.locator(".queue-preview").first()).toHaveText("Second follow-up");
    await handle.press("Space"); await handle.press("ArrowDown"); await handle.press("Enter");
    await expect(queue.locator(".queue-preview").last()).toHaveText("Second follow-up");
    await queue.getByRole("button", { name: "Reorder queued message 2", exact: true }).press("Space");
    await page.getByRole("button", { name: "Reorder queued message 2", exact: true }).press("ArrowUp");
    await page.getByRole("button", { name: "Reorder queued message 2", exact: true }).press("Enter");
    await expect(queue.locator(".queue-preview").first()).toHaveText("Second follow-up");
    await composer.fill("Focus on the current task.");
    await composer.press("Control+Enter");
    await expect(composer).toHaveValue("");
    await expect(page.locator(".run-progress")).toContainText("Steering submitted to current task");
    await expect(queue.locator(".queued-message")).toHaveCount(2);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-message-queue.png" });
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 768));
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-message-queue-narrow.png" });
    expect(await queue.locator(".icon-button").evaluateAll((buttons) => buttons.every((button) => button.getBoundingClientRect().width >= 40 && button.getBoundingClientRect().height >= 40))).toBe(true);
    release();
    await expect.poll(async () => (await state()).running).toBe(false);
    await expect(queue).toHaveCount(0);
    expect((await state()).messages.filter((message) => message.role === "You").map((message) => message.text)).toEqual(["Start the first task.", "Second follow-up", "Edited first follow-up 🐊\nKeep the useful detail."]);
    expect(requests.some((body) => body.includes("Remove this request"))).toBe(false);

    hold = true;
    await composer.fill("Run to stop");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect.poll(() => waiting.length).toBeGreaterThan(0);
    await composer.fill("Run after stop");
    await composer.press("Enter");
    await expect(queue.locator(".queued-message")).toHaveCount(1);
    await page.getByRole("button", { name: "Stop current work", exact: true }).click();
    await expect.poll(async () => (await state()).running).toBe(false);
    await expect(queue.getByText("Paused", { exact: true })).toBeVisible();
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-message-queue-paused.png" });
    await page.getByRole("button", { name: "New topic", exact: true }).click();
    await expect(page.getByRole("alertdialog", { name: "Leave queued messages?" })).toBeVisible();
    await page.getByRole("button", { name: "Keep working", exact: true }).click();
    await expect(queue.locator(".queued-message")).toHaveCount(1);
    release();
    hold = true;
    await queue.getByRole("button", { name: "Send queued message 1 now", exact: true }).click();
    await expect.poll(() => waiting.length).toBeGreaterThan(0);
    await composer.fill("Original waiting message");
    await composer.press("Enter");
    await queue.getByRole("button", { name: "Edit queued message 1", exact: true }).click();
    await editor.fill("Revised after the message started");
    await composer.fill("Keep this other draft");
    release();
    await expect.poll(async () => (await state()).running).toBe(false);
    await expect(queue.getByText("This message has left the queue. Your edits are kept here.", { exact: true })).toBeVisible();
    await expect(editor).toHaveValue("Revised after the message started");
    await queue.getByRole("button", { name: "Keep as new draft", exact: true }).click();
    await expect(composer).toHaveValue("Keep this other draft\n\nRevised after the message started");
    await expect(queue).toHaveCount(0);
    expect((await state()).messages.filter((message) => message.role === "You").map((message) => message.text).slice(-2)).toEqual(["Run after stop", "Original waiting message"]);
    expect(errors).toEqual([]);
  } finally {
    release();
    await page.evaluate(async () => { if ((await window.gator?.snapshot())?.state?.running) await window.gator?.send({ type: "key", key: "esc" }); }).catch(() => {});
    await expect.poll(async () => (await state()).running).toBe(false);
    await closeDesktop(application);
    server.closeAllConnections();
    await new Promise<void>((done) => server.close(() => done()));
    await rm(directory, { recursive: true, force: true });
  }
});

test("desktop uses real Go state, preserves drafts, edits native forms, and continues saved work", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-e2e-"));
  const workspace = join(directory, "notes");
  await mkdir(workspace);
  await writeFile(join(workspace, "project.md"), "A personal project with one open decision.\n");
  let requests = 0;
  const answer = "The desktop is connected to Gator’s Go runtime. Your project has one open decision.\n\n```js\nconsole.log('gator');\n```";
  const server = createServer((req, res) => {
    req.resume();
    req.on("end", () => {
      requests++;
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ id: "local-desktop-test", choices: [{ message: { role: "assistant", content: answer }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    });
  });
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const address = server.address() as { port: number };
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const binary = resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime");
  const cli = (args: string[]) => execFileSync(binary, args, { env, encoding: "utf8" });
  cli(["provider", "add", "desktop-fixture", "--base-url", `http://127.0.0.1:${address.port}/v1/chat/completions`, "--model", "fixture-model"]);
  cli(["config", "set", "default-provider", "desktop-fixture"]);
  let application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await expect(page.getByRole("heading", { name: "Hi. I’m Gator." })).toBeVisible();
    await expect(page.getByRole("region", { name: "Everyday overview" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Talk with Gator", exact: true })).toHaveAttribute("aria-current", "page");
    await expect(page.getByRole("button", { name: "Conversation options", exact: true })).toBeVisible();
    const homeLayout = await page.evaluate(() => ({
      heading: document.querySelector(".home-content h1")!.getBoundingClientRect().bottom,
      composerTop: document.querySelector(".composer")!.getBoundingClientRect().top,
      composerBottom: document.querySelector(".composer")!.getBoundingClientRect().bottom,
    }));
    expect(homeLayout.heading).toBeLessThan(homeLayout.composerTop);
    expect(homeLayout.composerBottom).toBeLessThanOrEqual(await page.evaluate(() => innerHeight));
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-home.png" });
    await page.getByRole("textbox", { name: "Message Gator" }).fill("keep my draft 🐊");
    await openSurface(page, "Saved knowledge");
    await expect(page.getByRole("heading", { name: "Saved knowledge", exact: true })).toBeVisible();
    await expect(page.getByText("Gator’s view could not load.")).toHaveCount(0);
    await expect(page.getByText("No saved knowledge yet.")).toBeVisible();
    const contextID = await page.evaluate(async () => (await window.gator!.snapshot()).state?.panel?.id);
    const explanation = page.locator(".panel-disclosure").filter({ has: page.locator("summary", { hasText: "How to read this" }) });
    await expect(explanation).not.toHaveAttribute("open", "");
    await explanation.locator("summary").click();
    await expect(explanation).toHaveAttribute("open", "");
    expect(await page.evaluate(async () => (await window.gator!.snapshot()).state?.panel?.id)).toBe(contextID);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-knowledge-empty.png" });
    // The sources list is genuinely null in the Go wire format on this path.
    expect(await page.evaluate(async () => (await window.gator!.snapshot()).state?.panel?.blocks?.some((block) => block.lines === null))).toBe(true);
    const noteID = cli(["note", "add", "--title", "Desktop regression note", "--body", "Remember the project decision."]).split(/\s/)[0];
    await page.getByRole("button", { name: /^refresh/i }).click();
    await page.getByRole("button", { name: /Desktop regression note/ }).click();
    await expect(page.getByRole("heading", { name: /Desktop regression note/ })).toBeVisible();
    await expect(page.getByText("No saved knowledge yet.")).toHaveCount(0);
    await expect(page.getByText("Remember the project decision.", { exact: false }).first()).toBeVisible();
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-knowledge.png" });
    // Leave no recall corpus for later inference turns: they must not depend
    // on a developer's local Ollama service or create embedding reservations.
    cli(["note", "remove", noteID, "--yes"]);
    await page.getByRole("button", { name: "Gator home" }).click();
    await openSurface(page, "Overview");
    await expect(page.getByRole("heading", { name: /overview/i }).first()).toBeVisible();
    await page.getByRole("button", { name: "Back to conversation" }).click();
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toHaveValue("keep my draft 🐊");
    await page.getByRole("button", { name: "Conversation information" }).click();
    await expect(page.getByRole("complementary", { name: "Conversation information" })).toBeVisible();
    const workDetails = page.getByRole("complementary", { name: "Conversation information" });
    await expect(workDetails.getByRole("button", { name: /^Outputs/ })).toHaveAttribute("aria-expanded", "true");
    await expect(workDetails.getByText("Files and proposed changes appear here when a task produces them.")).toBeVisible();
    await workDetails.getByRole("button", { name: /^Sources & memory/ }).press("Enter");
    await expect(workDetails.getByText("Context capacity isn’t measured yet. Daily token usage is tracked separately.")).toHaveCount(0);
    await expect(workDetails.getByRole("button", { name: /^Inspect saved context/ })).toBeVisible();
    await expect(workDetails.getByRole("progressbar")).toHaveCount(0);
    await page.getByRole("button", { name: "Hide conversation information" }).click();
    await openSurface(page, "Follow-up checks");
    await page.getByRole("button", { name: /new check/i }).click();
    await expect(page.getByRole("textbox", { name: "Desired outcome", exact: true })).toBeVisible();
    await page.getByRole("textbox", { name: "Title", exact: true }).fill("Watch my project");
    await page.getByRole("textbox", { name: "Desired outcome", exact: true }).fill("Tell me when the open decision changes.");
    await page.getByRole("button", { name: /^review/i }).click();
    await expect(page.getByRole("heading", { name: "Review ongoing check" })).toBeVisible();
    await expect(page.getByText("Watch my project", { exact: false }).first()).toBeVisible();
    await settlePresentation(page);
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
    await expect(page.locator(".message-avatar")).toHaveCount(0);
    await expect(page.getByRole("article", { name: "Your message", exact: true })).toHaveCount(2);
    const conversationLayout = await page.evaluate(() => ({
      user: document.querySelector(".user-message .message-content")!.getBoundingClientRect().left,
      assistant: document.querySelector(".assistant-message .message-content")!.getBoundingClientRect().left,
    }));
    expect(conversationLayout.user).toBeGreaterThan(conversationLayout.assistant);
    // Keep the system clipboard untouched while exercising exact output and
    // permission-denied feedback through the real rendered response controls.
    await page.evaluate(() => {
      const capture = { value: "" };
      Object.defineProperty(window, "__copyCapture", { value: capture, configurable: true });
      Object.defineProperty(navigator.clipboard, "writeText", { configurable: true, value: async (text: string) => { capture.value = text; } });
    });
    await page.getByRole("button", { name: "Copy code", exact: true }).first().click();
    await expect(page.locator(".code-block-heading").first()).toContainText("Copied");
    expect(await page.evaluate(() => (window as unknown as { __copyCapture: { value: string } }).__copyCapture.value)).toBe("console.log('gator');\n");
    await page.getByRole("button", { name: "Copy response", exact: true }).first().click();
    expect(await page.evaluate(() => (window as unknown as { __copyCapture: { value: string } }).__copyCapture.value)).toBe(answer);
    await page.evaluate(() => {
      Object.defineProperty(navigator.clipboard, "writeText", { configurable: true, value: async () => { throw new Error("Clipboard denied by test"); } });
      document.execCommand = () => false;
    });
    await page.getByRole("button", { name: "Copy response", exact: true }).last().click();
    await expect(page.getByRole("alert").filter({ hasText: "Couldn’t copy. Select the text to copy it." })).toBeVisible();
    const saved = JSON.parse(cli(["overview", "--json"]));
    expect(saved).toBeTruthy();
    await expect(page.getByRole("button", { name: "Settings", exact: true })).toBeEnabled();
    await openSettingsCategory(page, "Usage & limits");
    // Count every call seen by the local provider, including auxiliary work.
    await expect(page.getByRole("region", { name: "Provider requests", exact: true })).toContainText(`${requests} used`);
    await expect(page.getByRole("region", { name: "Tokens", exact: true })).toContainText(`${requests * 20} used`);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-usage-with-activity.png" });
    await page.getByRole("button", { name: "Back to conversation" }).click();
    await page.getByRole("textbox", { name: "Message Gator" }).fill("keep my continuation draft");
    await openSurface(page, "Conversations");
    await expect(page.getByRole("button", { name: "Work history", exact: true })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Conversations", exact: true })).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Conversation views" })).toHaveCount(0);
    // Two turns belong to a single conversation, with two retained task runs.
    await expect(page.locator(".panel-row")).toHaveCount(1);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-conversations.png" });
    await openSurface(page, "Task runs");
    await expect(page.locator(".panel-row")).toHaveCount(2);
    await page.locator(".panel-row").first().focus();
    await page.locator(".panel-row").first().press("ArrowDown");
    await expect(page.locator(".panel-row").last()).toBeFocused();
    await expect.poll(async () => (await page.evaluate(async () => (await window.gator!.snapshot()).state?.panel?.selected))).toBe(1);
    await expect(page.locator(".workflow")).toHaveAttribute("aria-busy", "false");
    await page.locator(".panel-row").last().press("Home");
    await expect(page.locator(".panel-row").first()).toBeFocused();
    await expect(page.locator(".workflow")).toHaveAttribute("aria-busy", "false");
    await page.locator(".panel-row").first().press("f");
    await expect(page.locator(".workflow-heading p")).toContainText("running");
    await expect(page.locator(".panel-row")).toHaveCount(0);
    await expect(page.getByRole("combobox", { name: "Filter task runs" })).toHaveValue("running");
    await page.getByRole("combobox", { name: "Filter task runs" }).selectOption("completed");
    await expect(page.locator(".workflow-heading p")).toContainText("completed");
    await expect(page.locator(".panel-row")).toHaveCount(2);
    await expect(page.locator(".main-header")).toContainText("Task runs");
    await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
    await expect(page.locator(".main-header")).toContainText("Task runs");
    await application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 600));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-task-runs-800.png" });
    await page.locator(".panel-row").first().click();
    await expect(page.getByRole("heading", { name: "Artifacts", exact: true })).toBeVisible();
    await expect(page.locator(".main-header")).toContainText("Task runs");
    await page.getByRole("button", { name: /^conversation C$/i }).click();
    await expect(page.locator(".message-byline", { hasText: "Gator" })).toHaveCount(2);
    await expect(page.getByRole("textbox", { name: "Message Gator" })).toHaveValue("keep my continuation draft");
    await page.getByRole("button", { name: "Expand sidebar", exact: true }).click();
    for (const size of [{ width: 1440, height: 960 }, { width: 1024, height: 768 }, { width: 800, height: 600 }]) {
      await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].setSize(value.width, value.height), size);
      await expect(page.getByRole("textbox", { name: "Message Gator" })).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await settlePresentation(page);
    await page.screenshot({ path: `test-results/desktop-${size.width}.png` });
    }
    await page.getByRole("button", { name: "Conversation information" }).click();
    await expect(page.getByRole("dialog", { name: "Conversation information" })).toBeVisible();
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-details-800.png" });
    await page.getByRole("button", { name: "Close conversation information" }).click();
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
    const mainID = (await page.evaluate(() => window.gator!.snapshot())).state!.conversation_id;
    expect(mainID).toBeTruthy();
    await page.getByRole("button", { name: "New topic", exact: true }).click();
    await expect(page.locator(".assistant-message")).toHaveCount(0);
    await page.getByRole("textbox", { name: "Message Gator", exact: true }).fill("Separate topic draft");
    await page.getByRole("button", { name: "Talk with Gator", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toHaveValue("keep my continuation draft");
    const beforeRestart = requests;
    await closeDesktop(application);
    application = await launchDesktop(env);
    const reopened = await application.firstWindow();
    await expect.poll(() => reopened.evaluate(async () => (await window.gator!.snapshot()).state?.conversation_id)).toBe(mainID);
    await expect(reopened.getByRole("button", { name: "Talk with Gator", exact: true })).toHaveAttribute("aria-current", "page");
    await expect(reopened.locator(".assistant-message")).toHaveCount(2);
    await expect(reopened.getByRole("textbox", { name: "Message Gator", exact: true })).toHaveValue("keep my continuation draft");
    expect(requests).toBe(beforeRestart);
    await reopened.getByRole("button", { name: "New topic", exact: true }).click();
    await expect(reopened.getByRole("textbox", { name: "Message Gator", exact: true })).toHaveValue("Separate topic draft");

  } finally {
    await closeDesktop(application);
    await new Promise<void>((done) => server.close(() => done()));
    await rm(directory, { recursive: true, force: true });
  }
});

test("the conversational home has no dashboard and usage controls keep the Go review flow", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-dashboard-e2e-"));
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const binary = resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime");
  const cli = (args: string[]) => execFileSync(binary, args, { env, encoding: "utf8" });
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("heading", { name: "Hi. I’m Gator." })).toBeVisible();
    await page.emulateMedia({ reducedMotion: "no-preference" });
    for (const copy of ["Refreshes quietly every 15 seconds", "Your work stays connected. You choose what happens next.", "↵ send", "Same Gator profile as your terminal", "A little more continuity", "A good place to start", "Built around your ongoing work", "Personal workspace", "Local Gator profile"]) {
      await expect(page.getByText(copy, { exact: false })).toHaveCount(0);
    }
    await expect(page.locator(".starter-track, .overview-grid")).toHaveCount(0);
    await page.getByRole("textbox", { name: "Message Gator", exact: true }).fill("Keep my question while I set up");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Models", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Back to app", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toHaveValue("Keep my question while I set up");
    expect((await page.evaluate(() => window.gator!.snapshot())).state!.messages).toEqual([]);
    await openSettingsCategory(page, "Usage & limits");
    await expect(page.getByRole("heading", { name: "Usage & limits", exact: true })).toBeVisible();
    await expect(page.getByText("Uncapped", { exact: true })).toHaveCount(3);
    await expect(page.getByRole("meter")).toHaveCount(0);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-usage-uncapped.png" });
    await page.getByRole("button", { name: "Edit limits", exact: true }).click();
    await page.getByRole("spinbutton", { name: "Provider requests per day (0 uncapped)", exact: true }).fill("100");
    await page.getByRole("spinbutton", { name: "Tokens per day (0 uncapped)", exact: true }).fill("10000");
    await page.getByRole("spinbutton", { name: "Concurrent requests (0 uncapped)", exact: true }).fill("2");
    await page.getByRole("button", { name: /^review/i }).click();
    await expect(page.locator(".panel-facts")).toContainText("Provider requests per day (0 uncapped)");
    await expect(page.locator(".panel-facts dd").first()).toHaveText("100");
    expect(cli(["usage", "status"])).toContain("uncapped requests/day");
    await page.getByRole("button", { name: /^Save limits/i }).click();
    await expect(page.getByRole("meter", { name: "Provider requests remaining" })).toHaveAttribute("max", "100");
    await expect(page.getByRole("meter", { name: "Tokens remaining" })).toHaveAttribute("max", "10000");
    expect(cli(["usage", "status"])).toContain("100 requests/day");
    await page.getByRole("button", { name: "Pause requests", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Pause provider requests", exact: true })).toBeVisible();
    expect(cli(["usage", "status"])).toContain("Requests allowed");
    await page.getByRole("button", { name: /^Pause provider requests/i }).click();
    await expect(page.getByRole("button", { name: "Resume requests", exact: true })).toBeVisible();
    expect(cli(["usage", "status"])).toContain("All new provider requests paused");
    await page.getByText("Usage details", { exact: true }).click();
    await expect(page.getByText("Failed attempts", { exact: true })).toBeVisible();
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-usage-details.png" });
    await page.getByText("Usage details", { exact: true }).click();
    for (const size of [{ width: 1440, height: 960 }, { width: 1024, height: 768 }, { width: 800, height: 600 }]) {
      await application.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].setSize(value.width, value.height), size);
      await expect(page.getByRole("heading", { name: "Usage & limits", exact: true })).toBeVisible();
      await page.locator(".usage-page").evaluate((element) => element.scrollTo({ top: 0 }));
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await settlePresentation(page);
    await page.screenshot({ path: `test-results/desktop-usage-${size.width}.png` });
    }
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});

test("review presentation preserves exact previews and only the live approval action", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-review-ui-"));
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("heading", { name: "Hi. I’m Gator." })).toBeVisible();
    const base = await page.evaluate(async () => (await window.gator!.snapshot()).state!);
    const preview = { action: "write_file", path: "plan.txt", content: "A change you must review first." };
    await application.evaluate(({ BrowserWindow, ipcMain }, state) => {
      const frames: unknown[] = [];
      Object.defineProperty(globalThis, "__reviewFrames", { value: frames, configurable: true });
      // Observe presentation actions without writing to the real Go runtime.
      ipcMain.removeHandler("gator:send");
      ipcMain.handle("gator:send", (_event, frame) => { frames.push(frame); });
      BrowserWindow.getAllWindows()[0].webContents.send("gator:update", { state });
    }, { ...base, first_run: false, home: false, running: true, pending_approval: { id: 17 }, messages: [
      { role: "Gator", text: "", result: { kind: "approval", title: "Previous file write", tone: "success", fields: [{ label: "Request", value: "16" }], footer: "Approved" } },
      { role: "Gator", text: "", result: { kind: "approval", title: "Review proposed file write", tone: "warning", summary: "Write the proposed plan.", fields: [{ label: "Request", value: "17" }], preview } },
    ] } as typeof base);
    const current = page.getByRole("region", { name: "Review proposed file write", exact: true });
    const previous = page.getByRole("region", { name: "Previous file write", exact: true });
    await expect(current.getByText("Needs review", { exact: true })).toBeVisible();
    await expect(current.locator(".result-action-details")).toHaveAttribute("open", "");
    await expect(current.locator(".preview")).toBeVisible();
    expect(JSON.parse((await current.locator(".preview").textContent())!)).toEqual(preview);
    await expect(previous.getByRole("button", { name: "Approve this action" })).toHaveCount(0);
    await settlePresentation(page);
    await page.screenshot({ path: "test-results/desktop-approval-review.png" });
    await current.getByRole("button", { name: "Approve this action", exact: true }).click();
    expect(await application.evaluate(() => (globalThis as unknown as { __reviewFrames: unknown[] }).__reviewFrames)).toEqual([{ type: "approval", interaction_id: 17, approved: true }]);
  } finally {
    await closeDesktop(application);
    await rm(directory, { recursive: true, force: true });
  }
});

test("first-run model setup uses native fields and keeps credentials out of display state", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-setup-e2e-"));
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "1", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop") };
  const application = await launchDesktop(env);
  try {
    const page = await application.firstWindow();
    await expect(page.getByRole("heading", { name: "Hi. I’m Gator." })).toBeVisible();
    await openSettingsCategory(page, "Model settings");
    await page.getByRole("button", { name: /^Cloud API key/ }).click();
    await expect(page.getByRole("heading", { name: "Cloud models", exact: true })).toBeVisible();
    await page.getByRole("button", { name: /^configure /i }).click();
    const key = page.locator('input[type="password"]');
    await expect(key).toHaveCount(1);
    await key.fill("isolated-test-value");
    await settlePresentation(page);
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
    await expect(page.getByRole("heading", { name: "Hi. I’m Gator." })).toBeVisible();
    expect(await page.evaluate(async () => JSON.stringify(await window.gator!.snapshot()))).not.toContain("isolated-test-value");
  } finally { await closeDesktop(application); await rm(directory, { recursive: true, force: true }); }
});
