import { _electron as electron, expect, test, type ElectronApplication } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

const pick = (app: ElectronApplication, paths: string[]) => app.evaluate(({ dialog }, paths) => {
  dialog.showOpenDialog = async () => ({ canceled: !paths.length, filePaths: paths });
}, paths);
const settle = () => document.fonts.ready.then(() => Promise.allSettled(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished)));

test("attachments stay with drafts and queued messages and reach the real model only on send", async () => {
  const root = await temporaryDirectory(join(tmpdir(), "gator-attachment-e2e-"));
  const workspace = join(root, "workspace"), brief = join(root, "brief.txt"), followup = join(root, "follow-up.csv");
  await mkdir(workspace); await writeFile(brief, "Selected brief: violet kiwi plan."); await writeFile(followup, "name,choice\nfollowup,cobalt tide\n");
  const saved = join(root, "state/gator/conversations/work-attachment-side");
  await mkdir(saved, { recursive: true });
  const at = new Date().toISOString();
  await writeFile(join(saved, "conversation.json"), JSON.stringify({ version: 2, id: "work-attachment-side", title: "Another saved conversation", source_path: workspace, snapshot_id: "snap-side", created_at: at, updated_at: at }));
  const requests: { messages: { role: string; content: unknown }[] }[] = [];
  let release = () => {};
  const gate = new Promise<void>((resolve) => { release = resolve; });
  const server = createServer((request, response) => {
    let body = ""; request.on("data", (chunk) => { body += chunk; });
    request.on("end", async () => {
      const index = requests.push(JSON.parse(body));
      if (index === 1) await gate;
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ id: `file-${index}`, choices: [{ message: { role: "assistant", content: index === 1 ? "I read your brief." : index === 2 ? "I read your follow-up." : "I read the file-only message." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const port = (server.address() as { port: number }).port;
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const binary = resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime");
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
  cli(["provider", "add", "file-fixture", "--base-url", `http://127.0.0.1:${port}/v1/chat/completions`, "--model", "fixture-model"]); cli(["config", "set", "default-provider", "file-fixture"]);
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const launch = () => electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  let app = await launch();
  try {
    let page = await app.firstWindow();
    let composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await pick(app, [brief]); await page.getByRole("button", { name: "Attach files", exact: true }).click();
    let pending = page.getByRole("region", { name: "Attachments for your next message", exact: true });
    await expect(pending.getByText("brief.txt", { exact: true })).toBeVisible();
    await expect(composer).toBeFocused();
    await page.getByRole("button", { name: "Conversation options", exact: true }).click();
    const info = page.getByRole("complementary", { name: "Conversation information", exact: true });
    await info.getByRole("button", { name: /^Sources & memory/ }).click();
    await expect(info.getByText("brief.txt", { exact: true })).toBeVisible();
    await expect(info.getByText("No selected files or retained source references in this conversation yet.", { exact: true })).toHaveCount(0);
    await info.getByRole("button", { name: "Close conversation information", exact: true }).click();
    await composer.fill("Read my chosen brief.");
    const side = page.getByRole("button", { name: "Open conversation: Another saved conversation", exact: true });
    await side.click(); await expect(side).toHaveAttribute("aria-current", "page");
    await expect(composer).toHaveValue(""); await expect(pending).toHaveCount(0);
    await pick(app, [followup]); await page.getByRole("button", { name: "Attach files", exact: true }).click();
    await expect(pending.getByText("follow-up.csv", { exact: true })).toBeVisible();
    await composer.fill("A separate unsent message");
    const main = page.getByRole("button", { name: "Open conversation: New chat", exact: true });
    await main.click(); await expect(main).toHaveAttribute("aria-current", "page");
    await expect(composer).toHaveValue("Read my chosen brief.");
    await expect(pending.getByText("brief.txt", { exact: true })).toBeVisible();
    await expect(pending.getByText("follow-up.csv", { exact: true })).toHaveCount(0);
    await page.evaluate(async () => { await window.gator!.drafts({ action: "list" }); });
    await page.evaluate(settle); await page.screenshot({ path: "test-results/desktop-attachments-pending.png" });
    expect(requests).toHaveLength(0);
    await closeDesktop(app); app = await launch(); page = await app.firstWindow();
    composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    pending = page.getByRole("region", { name: "Attachments for your next message", exact: true });
    await expect(composer).toHaveValue("Read my chosen brief."); await expect(pending.getByText("brief.txt", { exact: true })).toBeVisible();
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    // The staged copy is independent of later edits to the original.
    await writeFile(brief, "Original changed after selection.");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect.poll(() => requests.length).toBe(1);
    expect(JSON.stringify(requests[0])).toContain("violet kiwi plan"); expect(JSON.stringify(requests[0])).not.toContain("Original changed after selection");
    expect(JSON.stringify(requests[0])).not.toContain("cobalt tide");
    await expect(pending).toHaveCount(0);
    await pick(app, [followup]); await page.getByRole("button", { name: "Attach files", exact: true }).click();
    await expect(pending.getByText("follow-up.csv", { exact: true })).toBeVisible();
    await composer.fill("Read the follow-up after this.");
    await expect(page.getByRole("button", { name: "Guide now", exact: true })).toBeDisabled();
    await page.getByRole("button", { name: "Queue message", exact: true }).click();
    await expect(page.locator(".message-queue").getByText("follow-up.csv", { exact: true })).toBeVisible();
    await expect(pending).toHaveCount(0); expect(requests).toHaveLength(1);
    await page.evaluate(settle); await page.screenshot({ path: "test-results/desktop-attachments-queued.png" });
    release();
    await expect(page.locator(".assistant-message").getByText("I read your follow-up.", { exact: true })).toBeVisible();
    expect(requests).toHaveLength(2);
    expect(JSON.stringify(requests[1].messages.filter((message) => message.role === "user").at(-1))).toContain("cobalt tide");
    await expect(page.locator(".user-message .message-inputs")).toHaveCount(2);
    expect(await readdir(workspace)).toEqual([]); expect(await readFile(brief, "utf8")).toBe("Original changed after selection.");
    // Files alone are a valid message, without requiring filler text.
    await pick(app, [brief]); await page.getByRole("button", { name: "Attach files", exact: true }).click();
    await expect(pending.getByText("brief.txt", { exact: true })).toBeVisible(); await expect(composer).toHaveValue("");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.locator(".assistant-message").getByText("I read the file-only message.", { exact: true })).toBeVisible();
    await expect(page.getByRole("region", { name: "Gator activity", exact: true })).toHaveCount(0);
    await closeDesktop(app); app = await launch(); page = await app.firstWindow();
    await expect(page.getByText("I read your follow-up.", { exact: true })).toBeVisible();
    await expect(page.locator(".user-message .message-inputs")).toHaveCount(3);
    expect(requests).toHaveLength(3);
  } finally { release(); await closeDesktop(app); server.closeAllConnections(); await new Promise<void>((resolve) => server.close(() => resolve())); await rm(root, { recursive: true, force: true }); }
});

test("attachment cancellation, removal and validation failures preserve the unsent message", async () => {
  const root = await temporaryDirectory(join(tmpdir(), "gator-attachment-errors-e2e-"));
  const workspace = join(root, "workspace"), valid = join(root, "plan.md"), unsupported = join(root, "program.exe"), fake = join(root, "fake.png");
  await mkdir(workspace); await writeFile(valid, "# A useful plan"); await writeFile(unsupported, "Executable"); await writeFile(fake, "This is not a PNG.");
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const binary = resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime");
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
  cli(["provider", "add", "unused-fixture", "--base-url", "http://127.0.0.1:1/v1/chat/completions", "--model", "fixture-model"]); cli(["config", "set", "default-provider", "unused-fixture"]);
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  try {
    const page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true }), button = page.getByRole("button", { name: "Attach files", exact: true });
    await expect(composer).toBeVisible(); await composer.fill("Keep this request");
    const pending = page.getByRole("region", { name: "Attachments for your next message", exact: true });
    await pick(app, []); await button.click(); await expect(pending).toHaveCount(0); await expect(composer).toHaveValue("Keep this request");
    await pick(app, [unsupported]); await button.click(); await expect(page.locator(".composer-error")).toContainText("isn’t supported"); await expect(pending).toHaveCount(0);
    await pick(app, [valid]); await button.click(); await expect(pending.getByText("plan.md", { exact: true })).toBeVisible();
    await pending.getByRole("button", { name: "Remove attachment plan.md", exact: true }).click(); await expect(pending).toHaveCount(0); await expect(composer).toHaveValue("Keep this request");
    // A disk-backed File crosses the isolated preload bridge for drag/drop.
    await page.evaluate(() => { const input = document.createElement("input"); input.type = "file"; input.id = "drop-fixture"; input.hidden = true; document.body.append(input); });
    await page.locator("#drop-fixture").setInputFiles(valid);
    await page.evaluate(() => {
      const transfer = new DataTransfer(); transfer.items.add((document.querySelector("#drop-fixture") as HTMLInputElement).files![0]);
      document.querySelector(".composer")!.dispatchEvent(new DragEvent("dragover", { bubbles: true, cancelable: true, dataTransfer: transfer }));
    });
    await expect(page.getByText("Drop files to attach to this message", { exact: true })).toBeVisible();
    await page.evaluate(() => {
      const transfer = new DataTransfer(); transfer.items.add((document.querySelector("#drop-fixture") as HTMLInputElement).files![0]);
      document.querySelector(".composer")!.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer: transfer }));
    });
    await expect(pending.getByText("plan.md", { exact: true })).toBeVisible();
    await pending.getByRole("button", { name: "Remove attachment plan.md", exact: true }).click();
    // Invalid content is rejected by the real Go loader before inference.
    await pick(app, [fake]); await button.click(); await expect(pending.getByText("fake.png", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.locator(".composer-error")).toContainText("must be PNG, JPEG, or WebP");
    await expect(page.locator(".composer-error")).not.toContainText("invoking remote method");
    await expect(page.locator(".composer-error")).not.toContainText("gator:send");
    await expect(page.locator(".composer-error")).not.toHaveText(/@[a-f0-9]{32}/);
    await expect(composer).toHaveValue("Keep this request"); await expect(pending.getByText("fake.png", { exact: true })).toBeVisible();
    await expect(page.locator(".connection-error")).toHaveCount(0);
    await expect(page.locator(".user-message")).toHaveCount(0);
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 700));
    await page.emulateMedia({ reducedMotion: "reduce" }); await page.evaluate(settle); await page.screenshot({ path: "test-results/desktop-attachments-error-narrow.png" });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  } finally { await closeDesktop(app); await rm(root, { recursive: true, force: true }); }
});
