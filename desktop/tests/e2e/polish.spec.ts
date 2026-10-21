import { _electron as electron, expect, test, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { access, mkdtemp, mkdir, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop } from "./lifecycle";

async function profile() {
  const directory = await realpath(await mkdtemp(join(tmpdir(), "gator-polish-e2e-")));
  const workspace = join(directory, "workspace");
  await mkdir(workspace, { recursive: true });
  await writeFile(join(workspace, "project.md"), "# Offsite\nReview the venue shortlist and budget.\n");
  const env = Object.fromEntries(Object.entries({ ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace }).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
  const cli = (...args: string[]) => execFileSync(resolve("runtime/gator"), args, { env, encoding: "utf8" }).trim();
  return { directory, workspace, env, cli };
}
async function launch(env: Record<string, string>) {
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  const page = await app.firstWindow();
  await expect(page.getByRole("textbox", { name: "Message Gator", exact: true })).toBeVisible();
  await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(1440, 960));
  return { app, page };
}
const snapshot = (page: Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);
async function navigate(page: Page, route: string) {
  await page.evaluate(async (text) => window.gator!.send({ type: "navigate", text }), route);
  await expect.poll(async () => { const state = await snapshot(page); return state && !state.panel?.title?.startsWith("Loading ") && !state.panel?.notice?.startsWith("Working"); }).toBeTruthy();
}
async function key(page: Page, key: string) {
  const state = await snapshot(page);
  await page.evaluate(async ({ id, key }) => window.gator!.send({ type: "panel", key, panel: { id, action: "key" } }), { id: state.panel!.id!, key });
}
async function capture(page: Page, name: string) {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished));
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: `playwright-report/polish-round/${name}.png`, animations: "disabled" });
}

test("Inbox opens without acknowledging results, and explicit read and rating actions persist", async () => {
  const p = await profile();
  const inbox = join(p.directory, "state/gator/inbox/inbox-polish.json");
  await mkdir(join(p.directory, "state/gator/inbox"), { recursive: true });
  await writeFile(inbox, JSON.stringify({ version: 1, id: "inbox-polish", kind: "job", title: "Venue shortlist changed", summary: "Two new options are available. Review the shortlist before choosing a venue.", status: "completed", urgent: true, created_at: new Date().toISOString() }), { mode: 0o600 });
  const { app, page } = await launch(p.env);
  try {
    await navigate(page, "/inbox");
    await capture(page, "inbox-list");
    await page.getByRole("button", { name: /Venue shortlist changed/ }).click();
    await expect(page.getByRole("heading", { name: "Venue shortlist changed", exact: true })).toBeVisible();
    await expect(page.getByText("Two new options are available. Review the shortlist before choosing a venue.", { exact: true })).toBeVisible();
    expect(JSON.parse(await readFile(inbox, "utf8")).read_at).toBeFalsy();
    expect(JSON.parse(await readFile(inbox, "utf8")).rating).toBeFalsy();
    await capture(page, "inbox-detail-unread");
    await page.getByRole("button", { name: /^Mark as read/ }).click();
    await expect.poll(async () => JSON.parse(await readFile(inbox, "utf8")).read_at).toBeTruthy();
    await page.getByRole("button", { name: /^Useful/ }).click();
    await expect.poll(async () => JSON.parse(await readFile(inbox, "utf8")).rating).toBe("useful");
    await capture(page, "inbox-detail-rated");
    await page.getByRole("button", { name: "Back to Inbox", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Inbox", exact: true })).toBeVisible();
    await page.getByRole("button", { name: /Venue shortlist changed/ }).click();
    await page.getByRole("button", { name: /^Clear rating/ }).click();
    await expect.poll(async () => JSON.parse(await readFile(inbox, "utf8")).rating || "").toBe("");
  } finally { await closeDesktop(app); await rm(p.directory, { recursive: true, force: true }); }
});

test("knowledge and model filters preserve the selected record and original model identity", async () => {
  const p = await profile();
  p.cli("note", "add", "--title", "Offsite planning notes", "--body", "Venue shortlist: Sentosa or East Coast. Budget decision due Friday.");
  p.cli("note", "add", "--title", "Communication preferences", "--body", "Prefer concise summaries with explicit next steps.");
  const { app, page } = await launch(p.env);
  try {
    await navigate(page, "/knowledge");
    const filter = page.getByRole("searchbox", { name: "Filter loaded knowledge" });
    await filter.fill("Sentosa");
    await expect(page.locator(".panel-row")).toHaveCount(1);
    await expect(page.getByText(/loaded items · up to 100/)).toBeVisible();
    await capture(page, "knowledge-filtered");
    await filter.fill("not in these records");
    await expect(page.getByRole("heading", { name: "No matching loaded items" })).toBeVisible();
    await filter.fill("Sentosa");
    await page.getByRole("button", { name: /Offsite planning notes/ }).click();
    await capture(page, "knowledge-detail");
    await key(page, "e");
    await capture(page, "knowledge-correction");
    await key(page, "esc");
    await key(page, "f");
    await expect(page.locator(".workflow[data-kind='confirmation']")).toBeVisible();
    await capture(page, "knowledge-forget-review");
    await key(page, "esc");
    await navigate(page, "/model");
    await page.getByRole("button", { name: /^Cloud API key/ }).click();
    const models = page.getByRole("searchbox", { name: "Filter models" });
    await models.fill("anthropic");
    await expect(page.locator(".panel-row").first()).toContainText("anthropic");
    await capture(page, "models-filtered");
    await page.getByRole("button", { name: /^Configure/ }).click();
    await expect(page.getByRole("heading").filter({ hasText: /anthropic/i })).toBeVisible();
    await capture(page, "model-credential-form");
    await key(page, "esc");
    await models.fill("no matching model");
    await expect(page.getByRole("heading", { name: "No matching models" })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Configure/ })).toBeDisabled();
    await capture(page, "models-empty-filter");
  } finally { await closeDesktop(app); await rm(p.directory, { recursive: true, force: true }); }
});

test("native follow-up fields review exact values and path selection never switches workspace", async () => {
  const p = await profile();
  const picked = join(p.directory, "replacement workspace");
  await mkdir(picked);
  const { app, page } = await launch(p.env);
  try {
    await navigate(page, "/followup"); await key(page, "n");
    await page.getByRole("textbox", { name: "Title", exact: true }).fill("Watch the offsite plan");
    await page.getByRole("textbox", { name: "Desired outcome", exact: true }).fill("Tell me when the venue or budget changes.");
    await app.evaluate(({ dialog }, path) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [path] }); }, picked);
    await page.getByRole("button", { name: "Browse local workspace", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Local workspace", exact: true })).toHaveValue(picked);
    expect((await snapshot(page)).source).toBe(p.workspace);
    expect(await page.evaluate(() => window.gator!.choosePath({ panel_id: "stale-panel", kind: "directory" }))).toBeNull();
    await page.getByRole("combobox", { name: "Check frequency" }).selectOption("30");
    await page.getByRole("combobox", { name: "What Gator may do" }).selectOption("inspect");
    await page.getByRole("spinbutton", { name: "Provider requests", exact: true }).fill("16");
    await page.getByRole("spinbutton", { name: "Tokens", exact: true }).fill("50000");
    await page.getByRole("spinbutton", { name: "Time (seconds)", exact: true }).fill("120");
    await page.getByRole("spinbutton", { name: "Checks per UTC day", exact: true }).fill("6");
    await capture(page, "followup-native-form");
    await page.getByRole("textbox", { name: "Title", exact: true }).focus();
    await page.keyboard.press("Meta+Enter");
    await expect(page.locator(".workflow[data-kind='confirmation']")).toBeVisible();
    const review = (await snapshot(page)).panel!;
    const facts = JSON.stringify(review);
    for (const exact of ["Watch the offsite plan", picked, "every 30 minutes", "6 Work checks/day", "16 requests", "50000 tokens", "120s", "Read and recommend"]) expect(facts).toContain(exact);
    await capture(page, "followup-complete-review");
    await key(page, "esc");
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Watch the offsite plan");
    await navigate(page, "/data"); await key(page, "i");
    await page.getByRole("button", { name: "Add folder mapping", exact: true }).click();
    await page.getByRole("textbox", { name: "Original folder 1", exact: true }).fill("/original project");
    await page.getByRole("textbox", { name: "Replacement folder 1", exact: true }).fill(picked);
    await page.getByRole("button", { name: "Add folder mapping", exact: true }).click();
    await page.getByRole("button", { name: "Remove folder mapping 2", exact: true }).click();
    const archive = join(p.directory, "example.gator"), destination = join(p.directory, "restored profile");
    await app.evaluate(({ dialog }, paths) => {
      dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [paths.archive] });
      dialog.showSaveDialog = async () => ({ canceled: false, filePath: paths.destination });
    }, { archive, destination });
    await page.getByRole("button", { name: "Browse existing backup archive", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "Existing backup archive", exact: true })).toHaveValue(archive);
    await page.getByRole("button", { name: "Browse new profile directory", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "New profile directory", exact: true })).toHaveValue(destination);
    expect(await access(archive).then(() => true, () => false)).toBe(false);
    expect(await access(destination).then(() => true, () => false)).toBe(false);
    await page.getByRole("button", { name: /^Review/ }).click();
    await expect(page.locator(".workflow[data-kind='confirmation']")).toBeVisible();
    expect(JSON.stringify((await snapshot(page)).panel)).toContain(`/original project=${picked}`);
    await capture(page, "restore-mapping-review");
    await expect(page.getByRole("button", { name: /^Preview restore/ })).toBeVisible();
    await navigate(page, "/recovery"); await key(page, "a");
    await expect(page.locator(".workflow[data-kind='confirmation']")).toBeVisible();
    await expect(page.getByText("Nothing here yet.", { exact: true })).toHaveCount(0);
    await capture(page, "recovery-login-review");
  } finally { await closeDesktop(app); await rm(p.directory, { recursive: true, force: true }); }
});

test("saved guidance and terminal status line controls persist explicit native choices", async () => {
  const p = await profile();
  const { app, page } = await launch(p.env);
  try {
    await navigate(page, "/learnings"); await key(page, "n");
    await page.getByRole("combobox", { name: "Type", exact: true }).selectOption("procedure");
    await page.getByRole("combobox", { name: "Scope", exact: true }).selectOption("project");
    await page.getByRole("textbox", { name: "Key", exact: true }).fill("offsite-review");
    await page.getByRole("textbox", { name: "Text", exact: true }).fill("Review venue capacity before proposing a shortlist.");
    await capture(page, "guidance-native-form");
    await page.locator(".workflow-footer").getByRole("button", { name: /^Save/ }).click();
    await expect(page.getByRole("heading", { name: "Guidance details", exact: true })).toBeVisible();
    await expect(page.getByText("Review venue capacity before proposing a shortlist.", { exact: true })).toBeVisible();
    await capture(page, "guidance-detail");
    await key(page, "e");
    await expect(page.getByRole("combobox", { name: "Type", exact: true })).toBeDisabled();
    await expect(page.getByRole("combobox", { name: "Scope", exact: true })).toBeDisabled();
    await key(page, "esc");
    await navigate(page, "/learnings");
    await page.getByRole("combobox", { name: "Filter saved guidance", exact: true }).selectOption("disabled");
    await expect(page.locator(".panel-row")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "No guidance with this status", exact: true })).toBeVisible();
    await capture(page, "guidance-filter-empty");
    await page.getByRole("combobox", { name: "Filter saved guidance", exact: true }).selectOption("active");
    await expect(page.locator(".panel-row")).toHaveCount(1);
    await navigate(page, "/statusline");
    // These controls wait for the authoritative Go projection after IPC.
    await page.getByRole("switch", { name: "Show in Terminal", exact: true }).click();
    await expect(page.getByRole("switch", { name: "Show in Terminal", exact: true })).toBeChecked();
    await page.getByRole("checkbox", { name: "Effort", exact: true }).click();
    await expect(page.getByRole("checkbox", { name: "Effort", exact: true })).toBeChecked();
    const position = (await snapshot(page)).panel!.rows!.find((row) => row.label === "Effort")!.position!;
    await page.getByRole("button", { name: "Move Effort earlier", exact: true }).click();
    await expect.poll(async () => (await snapshot(page)).panel!.rows!.find((row) => row.label === "Effort")!.position).toBe(position - 1);
    await capture(page, "statusline-native-options");
    const selected = (await snapshot(page)).panel!.rows!.filter((row) => row.checked).sort((a, b) => a.position! - b.position!).map((row) => row.label);
    await page.getByRole("button", { name: "Save changes", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Terminal status line", exact: true })).toHaveCount(0);
    await navigate(page, "/statusline");
    await expect(page.getByRole("switch", { name: "Show in Terminal", exact: true })).toBeChecked();
    await expect(page.getByRole("checkbox", { name: "Effort", exact: true })).toBeChecked();
    expect((await snapshot(page)).panel!.rows!.filter((row) => row.checked).sort((a, b) => a.position! - b.position!).map((row) => row.label)).toEqual(selected);
  } finally { await closeDesktop(app); await rm(p.directory, { recursive: true, force: true }); }
});
