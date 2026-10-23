import { _electron as electron, test, expect, type ElectronApplication, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

async function fixture(holdSend = false) {
  const root = await temporaryDirectory(join(tmpdir(), "gator-receipts-"));
  const workspace = join(root, "workspace"), configRoot = join(root, "config/gator");
  await mkdir(workspace); await mkdir(configRoot, { recursive: true });
  let releaseSend!: () => void;
  const sendGate = new Promise<void>(done => { releaseSend = done; });
  const requests: string[] = [];
  let messageID = "";
  const server = createServer(async (req, res) => {
    let body = ""; for await (const chunk of req) body += chunk;
    const url = new URL(req.url!, "http://localhost");
    res.setHeader("Content-Type", "application/json");
    if (url.pathname.startsWith("/gmail/")) {
      requests.push(`${req.method} ${url.pathname}`);
      if (req.method === "POST" && url.pathname.endsWith("/send")) {
        const mime = Buffer.from(JSON.parse(body).raw, "base64url").toString();
        messageID = mime.match(/Message-ID: (.+)\r\n/)![1];
        if (holdSend) await sendGate;
        if (!res.destroyed) res.end("{}"); // The provider did not return a receipt.
      } else if (url.pathname === "/gmail/v1/users/me/messages") {
        expect(url.searchParams.get("q")).toBe(`in:sent rfc822msgid:${messageID}`);
        res.end(JSON.stringify({ messages: [{ id: "sent-one" }] }));
      } else if (url.pathname === "/gmail/v1/users/me/messages/sent-one") {
        res.end(JSON.stringify({ id: "sent-one", labelIds: ["SENT"], payload: { headers: [{ name: "Message-ID", value: messageID }] } }));
      } else { res.statusCode = 404; res.end("{}"); }
      return;
    }
    const payload = JSON.parse(body);
    const send = payload.tools?.some((tool: { function: { name: string } }) => tool.function.name === "connector_google_gmail_send") && !payload.messages.some((message: { role: string }) => message.role === "tool");
    const message = send ? { role: "assistant", tool_calls: [{ id: "send-one", type: "function", function: { name: "connector_google_gmail_send", arguments: JSON.stringify({ payload: { from: "me@example.com", to: "venue@example.com", subject: "Venue update", body: "Please review the new date." } }) } }] } : { role: "assistant", content: "The send outcome is not confirmed. Verify Sent before trying again." };
    res.end(JSON.stringify({ choices: [{ message, finish_reason: send ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  const base = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
  await writeFile(join(configRoot, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base + "/chat", models: ["fixture"], default_model: "fixture" }], connectors: [{ version: 1, id: "google", name: "Local Google fixture", kind: "google_workspace", resource: base, authentication: "none" }] }), { mode: 0o600 });
  const env = Object.fromEntries(Object.entries({ ...process.env, GATOR_INTERNAL_COMMANDS: "", GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace }).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
  let app: ElectronApplication;
  return {
    root, requests, releaseSend,
    async launch() { const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE; app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env }); return app; },
    async close() { releaseSend(); if (app) await closeDesktop(app); server.closeAllConnections(); await new Promise<void>(done => server.close(() => done())); await rm(root, { recursive: true, force: true }); },
  };
}
const state = async (page: Page) => (await page.evaluate(() => window.gator!.snapshot())).state!;
async function prepareSend(page: Page) {
  const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
  await expect(composer).toBeVisible();
  for (const command of ["/mode act", "/connector add google"]) {
    await composer.fill(command); await composer.press("Enter");
    await expect(composer).toHaveValue("");
  }
  await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual(["google"]);
  await composer.fill("Send the reviewed venue update"); await composer.press("Enter");
  const approve = page.getByRole("button", { name: "Approve this action", exact: true });
  await expect(approve).toBeVisible();
  expect((await state(page)).pending_approval?.token).toBeTruthy();
  await approve.click();
}

test("uncertain sends have owned receipts and read-only Sent checks retained across restart", async () => {
  test.setTimeout(90000);
  const f = await fixture();
  try {
    let app = await f.launch(), page = await app.firstWindow();
    await prepareSend(page);
    await expect.poll(async () => (await state(page)).running).toBe(false);
    await expect(page.getByRole("heading", { name: "Request outcome unconfirmed", exact: true })).toBeVisible();
    const summary = page.locator(".connected-receipt > summary");
    await expect(summary).toHaveCount(1);
    await expect(summary).toContainText("Send not confirmed · venue@example.com");
    expect(f.requests).toEqual(["POST /gmail/v1/users/me/messages/send"]);
    const original = (await state(page)).messages.find(message => message.bundle?.Actions?.length)?.bundle!;
    const manifest = join(original.Path, "manifest.json"), before = await readFile(manifest, "utf8");
    await summary.click();
    await expect(page.getByRole("button", { name: "Check Sent", exact: true })).toBeEnabled();
    expect(f.requests).toHaveLength(1); // Opening reads only a saved local check.
    await page.getByRole("button", { name: "Check Sent", exact: true }).click();
    await expect(summary).toContainText("Found in Sent");
    await expect(page.locator(".connected-receipt").getByRole("status")).toContainText("not recipient delivery");
    expect(f.requests).toEqual(["POST /gmail/v1/users/me/messages/send", "GET /gmail/v1/users/me/messages", "GET /gmail/v1/users/me/messages/sent-one"]);
    expect(await readFile(manifest, "utf8")).toBe(before); // Sealed original outcome is immutable.
    await expect(page.locator(".request-recovery")).toHaveCount(0); // Bundle evidence is not repeated.
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    await page.getByRole("button", { name: "Check Sent", exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: "test-results/connected-receipt-compact.png", animations: "disabled" });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await expect(page.locator(".connected-receipt")).toHaveCount(0);
    await page.getByRole("button", { name: "Open conversation: Send the reviewed venue update", exact: true }).click();
    await closeDesktop(app); app = await f.launch(); page = await app.firstWindow();
    await page.locator(".connected-receipt > summary").click();
    await expect(page.locator(".connected-receipt > summary")).toContainText("Found in Sent");
    expect(f.requests).toHaveLength(3);
  } finally { await f.close(); }
});

test("a crashed dispatch reopens only in its own chat and never replays the request", async () => {
  test.skip(process.platform === "win32", "The process-tree kill probe uses Unix ps; Windows crash coverage needs its native harness.");
  test.setTimeout(90000);
  const f = await fixture(true);
  try {
    let app = await f.launch(), page = await app.firstWindow();
    await prepareSend(page);
    await expect.poll(() => f.requests.length).toBe(1);
    const owner = (await state(page)).conversation_id!;
    const effectsRoot = join(f.root, "state/gator/external-actions");
    const runs = await readdir(effectsRoot);
    const run = runs.find(name => !name.startsWith("."))!;
    const files = await readdir(join(effectsRoot, run));
    const receipt = JSON.parse(await readFile(join(effectsRoot, run, files.find(name => name.endsWith(".json"))!), "utf8"));
    expect(receipt.conversation_id).toBe(owner); expect(receipt.phase).toBe("executing"); expect(receipt.action.status).toBe("unknown");
    // Terminate only this isolated Electron main process's Go child, after the
    // fixture proves the approved POST is in flight and its receipt is on disk.
    const mainPID = app.process().pid!;
    const children = execFileSync("/bin/ps", ["-axo", "pid=,ppid=,command="], { encoding: "utf8" }).split("\n").map(line => line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/)).filter(match => match && Number(match[2]) === mainPID && match[3].includes("gator-runtime") && match[3].includes("ui-backend"));
    expect(children).toHaveLength(1);
    process.kill(Number(children[0]![1]), "SIGKILL");
    await expect(page.locator(".connection-error")).toBeVisible();
    await closeDesktop(app); f.releaseSend(); app = await f.launch(); page = await app.firstWindow();
    await expect.poll(async () => (await state(page)).conversation_id).toBe(owner);
    const recovery = page.locator(".request-recovery");
    await expect(recovery).toContainText("Saved request receipts · 1");
    await recovery.locator(":scope > summary").click();
    await recovery.locator(".connected-receipt > summary").click();
    await expect(recovery).toContainText("Send not confirmed · venue@example.com");
    await expect(recovery).toContainText("Reopening them does not retry an action");
    await expect(recovery.getByRole("button", { name: "Check Sent", exact: true })).toHaveCount(0);
    expect((await state(page)).action_recoveries?.[0].live).toBe(false);
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    await expect(page.getByRole("button", { name: "Send message", exact: true })).toBeInViewport();
    const bounds = await page.locator(".composer").boundingBox(); expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(560);
    await page.screenshot({ path: "test-results/request-receipt-recovery-compact.png", animations: "disabled" });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await expect(recovery).toHaveCount(0);
    await page.getByRole("button", { name: "Open conversation: Send the reviewed venue update", exact: true }).click();
    await expect(recovery).toContainText("Saved request receipts · 1");
    expect(f.requests).toHaveLength(1);
  } finally { await f.close(); }
});
