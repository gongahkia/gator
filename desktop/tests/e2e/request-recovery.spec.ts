import { _electron as electron, expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdir, readFile, rm, utimes, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

test("admitted text and attachments survive session loss and resume only by explicit selection", async () => {
  const root = await temporaryDirectory(join(tmpdir(), "gator-request-recovery-"));
  const workspace = join(root, "workspace"), file = join(root, "brief.txt");
  await mkdir(workspace); await writeFile(file, "Retained reference: jade heron.");
  const calls: string[] = [];
  let release = () => {};
  const gate = new Promise<void>(resolve => { release = resolve; });
  const server = createServer((request, response) => {
    let body = ""; request.on("data", chunk => { body += chunk; });
    request.on("end", async () => {
      const index = calls.push(body);
      if (index === 1) await gate;
      if (response.destroyed) return;
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ id: `recovery-${index}`, choices: [{ message: { role: "assistant", content: "The retained brief is available." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    });
  });
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const cli = (args: string[]) => execFileSync(resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
  cli(["provider", "add", "recovery-fixture", "--base-url", `http://127.0.0.1:${(server.address() as { port: number }).port}/v1/chat/completions`, "--model", "fixture"]);
  cli(["config", "set", "default-provider", "recovery-fixture"]);
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const launch = () => electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  let app = await launch();
  try {
    let page = await app.firstWindow();
    let composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await composer.fill("Prepare the report");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect.poll(() => calls.length).toBe(1);
    await app.evaluate(({ dialog }, file) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [file] }); }, file);
    await page.getByRole("button", { name: "Attach files", exact: true }).click();
    await expect(page.getByRole("region", { name: "Attachments for your next message", exact: true })).toContainText("brief.txt");
    await composer.fill("Read this brief next");
    await page.getByRole("button", { name: "Queue message", exact: true }).click();
    await expect(composer).toHaveValue("");
    await composer.fill("Summarize afterwards");
    await page.getByRole("button", { name: "Queue message", exact: true }).click();
    await expect(page.getByRole("region", { name: "Queued messages", exact: true })).toContainText("2 queued");
    const id = await page.evaluate(async () => (await window.gator!.snapshot()).state!.conversation_id!);
    const journalPath = join(env.GATOR_STATE_DIR, "gator/conversations", id, "requests.json");
    const before = JSON.parse(await readFile(journalPath, "utf8"));
    expect(before.requests.map((request: { state: string }) => request.state)).toEqual(["running", "queued", "queued"]);
    const upload = before.requests[1].options.Uploads[0];
    await app.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1, checkboxChecked: false }); });
    await closeDesktop(app);
    // The selected source can disappear, and the staged file can be older than
    // the normal cleanup lifetime. Admission still owns the private copy.
    await rm(file); await utimes(dirname(join(env.GATOR_STATE_DIR, "gator/uploads", upload)), 0, 0);
    app = await launch(); page = await app.firstWindow();
    const queue = page.getByRole("region", { name: "Queued messages", exact: true });
    await expect(queue).toContainText("Saved messages restored");
    await expect(queue).toContainText("Some actions may already have completed");
    await expect(queue).toContainText("3 queued");
    await expect(page.getByRole("button", { name: "Stop current work", exact: true })).toHaveCount(0);
    expect(calls).toHaveLength(1);
    await page.screenshot({ path: "test-results/request-recovery-desktop.png" });
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    await expect(queue.getByRole("button", { name: "Send queued message 2 now", exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: "test-results/request-recovery-compact.png" });
    await queue.getByRole("button", { name: "Send queued message 2 now", exact: true }).click();
    await expect(page.locator(".assistant-message").getByText("The retained brief is available.", { exact: true })).toBeVisible();
    await expect(queue).toContainText("2 queued");
    expect(calls).toHaveLength(2); expect(calls[1]).toContain("jade heron");
    const after = JSON.parse(await readFile(journalPath, "utf8"));
    expect(after.requests.find((request: { text: string }) => request.text === "Read this brief next").state).toBe("completed");
    await queue.getByRole("button", { name: "Remove queued message 1", exact: true }).click();
    await expect(queue).toContainText("1 queued");
    await closeDesktop(app); app = await launch(); page = await app.firstWindow();
    await expect(page.getByRole("region", { name: "Queued messages", exact: true })).toContainText("Summarize afterwards");
    await expect(page.getByRole("button", { name: /^Retry interrupted/ })).toHaveCount(0);
    expect(calls).toHaveLength(2);
  } finally {
    release(); await closeDesktop(app); server.closeAllConnections();
    await new Promise<void>(resolve => server.close(() => resolve())); await rm(root, { recursive: true, force: true });
  }
});

test("a rejected admission keeps the composer draft and starts no request", async () => {
  const root = await temporaryDirectory(join(tmpdir(), "gator-admission-failure-"));
  const workspace = join(root, "workspace"); await mkdir(workspace);
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const cli = (args: string[]) => execFileSync(resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
  cli(["provider", "add", "admission-fixture", "--base-url", "http://127.0.0.1:1/v1/chat/completions", "--model", "fixture"]); cli(["config", "set", "default-provider", "admission-fixture"]);
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  try {
    const page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    const id = await page.evaluate(async () => (await window.gator!.snapshot()).state!.conversation_id!);
    await writeFile(join(env.GATOR_STATE_DIR, "gator/conversations", id, "requests.json"), "corrupted before admission");
    await composer.fill("Keep this message until it is accepted");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.locator(".composer-error")).toContainText("Message not sent");
    await expect(composer).toHaveValue("Keep this message until it is accepted");
    await expect(page.locator(".user-message")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Stop current work", exact: true })).toHaveCount(0);
  } finally { await closeDesktop(app); await rm(root, { recursive: true, force: true }); }
});
