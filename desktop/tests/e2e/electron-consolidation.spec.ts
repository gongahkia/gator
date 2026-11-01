import { _electron as electron, expect, test } from "@playwright/test";
import { mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join, resolve, delimiter } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

async function fixture() {
  const root = await temporaryDirectory(join(tmpdir(), "gator-electron-only-")), workspace = join(root, "workspace"), config = join(root, "config/gator");
  await mkdir(workspace); await mkdir(config, { recursive: true });
  let requests = 0;
  const server = createServer(async (request, response) => {
    let body = ""; for await (const chunk of request) body += chunk;
    const payload = JSON.parse(body); requests++;
    const classifier = payload.messages.some((message: { role: string; content: string }) => message.role === "system" && message.content.includes("Classify the user's immediate request"));
    response.writeHead(200, { "Content-Type": "application/json" }); response.end(JSON.stringify({ choices: [{ message: { role: "assistant", content: classifier ? '{"mode":"inspect"}' : "Fixture request complete." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${(server.address() as { port: number }).port}/chat`, models: ["fixture"], default_model: "fixture" }] }));
  const bin = join(root, "bin"), editor = join(root, "editor.mjs"), control = join(root, "editor.json"); await mkdir(bin);
  if (process.platform !== "win32") {
    // Owned fake launcher executes only this test's Go-prepared handoff. No
    // actual Terminal window or developer session is opened.
    await writeFile(join(bin, process.platform === "darwin" ? "open" : "x-terminal-emulator"), `#!/bin/sh\n/bin/sh "$${process.platform === "darwin" ? "3" : "2"}" >/dev/null 2>&1 &\nexit 0\n`, { mode: 0o700 });
    await writeFile(editor, `import {readFile,writeFile} from 'node:fs/promises';const c=JSON.parse(await readFile(process.env.GATOR_EDITOR_FIXTURE,'utf8'));await writeFile(process.argv[2],c.text);process.exitCode=c.fail?3:0;`);
  }
  const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'";
  const env = { ...process.env, PATH: `${bin}${delimiter}${process.env.PATH}`, VISUAL: "", EDITOR: `${quote(process.execPath)} ${quote(editor)}`, GATOR_EDITOR_FIXTURE: control, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace, GATOR_INTERNAL_COMMANDS: "" };
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  const page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
  await expect(composer).toBeVisible();
  return { root, app, page, composer, control, requests: () => requests, close: async () => { try { await app.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1, checkboxChecked: false }); }); await closeDesktop(app); } finally { server.closeAllConnections(); await new Promise<void>(done => server.close(() => done())); await rm(root, { recursive: true, force: true }); } } };
}

test("message reuse and command arguments keep the independent draft and attachments without sending", async () => {
  const f = await fixture();
  try {
    const text = 'Retained <img src="https://untrusted.invalid/image"> message';
    await f.composer.fill(text); await f.composer.press("Enter");
    await expect.poll(async () => (await f.page.evaluate(() => window.gator!.snapshot())).state?.running).toBe(false);
    await expect(f.page.getByText("Fixture request complete.", { exact: true })).toBeVisible();
    const count = f.requests(), path = join(f.root, "notes.txt"); await writeFile(path, "Kept attachment");
    await f.app.evaluate(({ dialog }, path) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [path] }); }, path);
    await f.page.getByRole("button", { name: "Attach files", exact: true }).click();
    await f.composer.fill("Independent draft");
    await f.page.getByRole("button", { name: "Message tools", exact: true }).click();
    const tools = f.page.getByRole("dialog", { name: "Message tools", exact: true });
    await tools.getByRole("button", { name: /Reuse a message/ }).click();
    await tools.getByRole("button", { name: "Append to draft", exact: true }).click();
    await expect(f.composer).toHaveValue(`Independent draft\n\n${text}`);
    await expect(f.page.locator('img[src="https://untrusted.invalid/image"]')).toHaveCount(0);
    await f.page.getByRole("button", { name: "Message tools", exact: true }).click(); await tools.getByRole("button", { name: /Reuse a message/ }).click();
    await tools.getByRole("button", { name: "Review replacement", exact: true }).click();
    await tools.getByRole("textbox", { name: "Reviewed message text", exact: true }).fill("Reviewed request");
    await tools.getByRole("button", { name: "Replace draft text", exact: true }).click(); await expect(f.composer).toHaveValue("Reviewed request");
    await f.page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
    await f.page.getByRole("textbox", { name: "Find a command", exact: true }).fill("/mode");
    await f.page.getByRole("dialog", { name: "Search commands", exact: true }).getByText("/mode", { exact: true }).click();
    const args = f.page.getByRole("dialog", { name: "Arguments for /mode", exact: true });
    await args.getByRole("textbox", { name: "Arguments for /mode", exact: true }).fill("inspect"); await args.getByRole("button", { name: "Continue", exact: true }).click();
    await expect(args).toHaveCount(0); await expect(f.composer).toHaveValue("Reviewed request");
    await expect(f.page.getByRole("region", { name: "Attachments for your next message" })).toContainText("notes.txt");
    expect(f.requests()).toBe(count);
    await f.app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 650));
    await f.page.getByRole("button", { name: "Message tools", exact: true }).click();
    expect(await tools.evaluate(element => { const box = element.getBoundingClientRect(); return box.x >= 0 && box.right <= innerWidth && box.y >= 0 && box.bottom <= innerHeight; })).toBe(true);
    await f.page.screenshot({ path: "test-results/electron-message-tools.png" });
  } finally { await f.close(); }
});

test("external editor returns text through the real Go owner without sending and retains failed or oversized edits", async () => {
  test.skip(process.platform === "win32", "Existing editor requires POSIX sh; no native Windows editor acceptance claimed");
  const f = await fixture();
  try {
    await f.composer.fill("Original draft");
    const open = async (text: string, fail = false) => {
      await writeFile(f.control, JSON.stringify({ text, fail }));
      await f.page.getByRole("button", { name: "Message tools", exact: true }).click();
      await f.page.getByRole("button", { name: /Edit draft in external editor/ }).click();
    };
    await open("Edited λ\nSecond line"); await expect(f.composer).toHaveValue("Edited λ\nSecond line", { timeout: 20000 });
    await open("Saved despite editor exit", true); await expect(f.composer).toHaveValue("Saved despite editor exit");
    await expect(f.page.getByRole("dialog", { name: "Message tools", exact: true })).toContainText("status 3");
    await f.page.getByRole("button", { name: "Close message tools", exact: true }).click();
    await open("x".repeat(128 * 1024 + 1));
    const tools = f.page.getByRole("dialog", { name: "Message tools", exact: true });
    await expect(tools).toContainText("Recover editor text"); await expect(f.composer).toHaveValue("Saved despite editor exit");
    await tools.getByRole("textbox", { name: "Reviewed message text", exact: true }).fill("Shortened editor result");
    await tools.getByRole("button", { name: "Add to kept draft", exact: true }).click();
    await expect(f.composer).toHaveValue("Saved despite editor exit\n\nShortened editor result");
    expect(f.requests()).toBe(0);
  } finally { await f.close(); }
});

test("Electron exports use reviewed replacements, reject changed destinations and protect private storage", async () => {
  const f = await fixture();
  try {
    const input = join(f.root, "input.txt"), destination = join(f.root, "export.txt"); await writeFile(input, "Exact selected bytes"); await writeFile(destination, "Kept existing file");
    await f.app.evaluate(({ dialog }, input) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [input] }); }, input);
    const context = await f.page.evaluate(async () => `conversation:${(await window.gator!.snapshot()).state!.conversation_id}`);
    const selected = await f.page.evaluate(async context => (await window.gator!.chooseFiles(context))[0], context);
    const request = { action: "export" as const, context, ref: { kind: "upload" as const, id: selected.id } };
    const exportFile = () => f.page.evaluate(request => window.gator!.files<{ exported: boolean }>(request), request);
    await f.app.evaluate(({ dialog }, filePath) => { dialog.showSaveDialog = async () => ({ canceled: false, filePath }); dialog.showMessageBox = async () => ({ response: 0, checkboxChecked: false }); }, destination);
    expect((await exportFile()).exported).toBe(false); expect(await readFile(destination, "utf8")).toBe("Kept existing file");
    await f.app.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1, checkboxChecked: false }); });
    expect((await exportFile()).exported).toBe(true); expect(await readFile(destination, "utf8")).toBe("Exact selected bytes");
    await f.app.evaluate(async ({ dialog }, path) => { const fs = process.getBuiltinModule("fs"); dialog.showMessageBox = async () => { fs.writeFileSync(path, "Concurrent destination edit"); return { response: 1, checkboxChecked: false }; }; }, destination);
    await expect(exportFile()).rejects.toThrow(/changed after review/); expect(await readFile(destination, "utf8")).toBe("Concurrent destination edit");
    await f.app.evaluate(({ dialog }, filePath) => { dialog.showSaveDialog = async () => ({ canceled: false, filePath }); }, join(f.root, "state/gator/forbidden.txt"));
    await expect(exportFile()).rejects.toThrow(/private storage/); expect(f.requests()).toBe(0);
  } finally { await f.close(); }
});
