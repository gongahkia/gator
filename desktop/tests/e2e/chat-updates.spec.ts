import { _electron as electron, expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

test("durable follow-up cards return to their chat and retain explicit reply ownership", async () => {
  test.setTimeout(100_000);
  const root = await temporaryDirectory(join(tmpdir(), "gator-chat-updates-"));
  const workspace = join(root, "workspace"); await mkdir(workspace);
  const calls: string[] = [];
  const server = createServer((request, response) => {
    let body = ""; request.on("data", chunk => { body += chunk; });
    request.on("end", () => {
      const index = calls.push(body);
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ id: `update-${index}`, choices: [{ message: { role: "assistant", content: `Retained reply ${index}.` }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }));
    });
  });
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const cli = (args: string[]) => execFileSync(resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" });
  cli(["provider", "add", "updates-fixture", "--base-url", `http://127.0.0.1:${(server.address() as { port: number }).port}/v1/chat/completions`, "--model", "fixture"]);
  cli(["config", "set", "default-provider", "updates-fixture"]);
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const launch = () => electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  let app = await launch();
  try {
    let page = await app.firstWindow();
    const composer = () => page.getByRole("textbox", { name: "Message Gator", exact: true });
    const snapshot = () => page.evaluate(() => window.gator!.snapshot());
    await expect(composer()).toBeVisible();
    const origin = (await snapshot()).state!.conversation_id!;
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await composer().fill("Plan the offsite"); await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.getByText("Retained reply 1.", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await expect.poll(async () => (await snapshot()).state!.conversation_id).not.toBe(origin);
    const inboxPath = join(env.GATOR_STATE_DIR, "gator/inbox"); await mkdir(inboxPath, { recursive: true });
    const entry = { version: 1, id: "inbox-offsite", kind: "job", title: "Offsite venue", summary: "The venue changed rooms. Option one is the garden; option two is the library.", status: "completed", origin_conversation_id: origin, conversation_id: "work-background", job_id: "job-offsite", attempt_id: "attempt-one", created_at: new Date().toISOString(), next_step: "Choose a room before Friday.", evidence: ["source/offsite.md:12"] };
    const file = join(inboxPath, `${entry.id}.json`);
    await writeFile(file, JSON.stringify(entry));
    await writeFile(join(inboxPath, "inbox-private.json"), JSON.stringify({ ...entry, id: "inbox-private", origin_conversation_id: "work-private-other", summary: "PRIVATE OTHER CHAT" }));
    // Polling discovers the originating chat even when that worker is not visible.
    const activity = page.getByRole("navigation", { name: "Work in other chats" });
    await expect(activity).toContainText("New update", { timeout: 25_000 });
    await expect(page.getByRole("article", { name: "Follow-up: Offsite venue" })).toHaveCount(0);
    await activity.getByRole("button").filter({ hasText: "New update" }).click();
    await expect.poll(async () => (await snapshot()).state!.conversation_id).toBe(origin);
    const card = () => page.getByRole("article", { name: "Follow-up: Offsite venue" });
    await expect(card()).toBeVisible();
    await expect.poll(async () => JSON.parse(await readFile(file, "utf8")).delivered_at || "0001-01-01T00:00:00Z").not.toBe("0001-01-01T00:00:00Z");
    let stored = JSON.parse(await readFile(file, "utf8"));
    expect(stored.read_at).toBe("0001-01-01T00:00:00Z"); expect(stored.resolved_at).toBe("0001-01-01T00:00:00Z");
    await card().getByText("Details", { exact: true }).click();
    await expect(card()).toContainText("source/offsite.md:12");
    await card().getByRole("button", { name: "Done", exact: true }).click();
    await expect(card().getByRole("button", { name: "Reopen" })).toBeVisible();
    stored = JSON.parse(await readFile(file, "utf8")); expect(stored.read_at).toBe("0001-01-01T00:00:00Z"); expect(stored.resolution_version).toBe(1); expect(calls).toHaveLength(1);
    await page.evaluate(({ origin, id }) => window.gator!.send({ type: "update", update: { conversation_id: origin, id, action: "reopen", version: 0 } }), { origin, id: entry.id });
    await expect(page.getByRole("alert").filter({ hasText: "this update changed" })).toBeVisible();
    await card().getByRole("button", { name: "Reopen" }).click();
    await card().getByRole("button", { name: "Mark read" }).click();
    await expect(card().getByRole("button", { name: "Mark read" })).toHaveCount(0);
    await card().getByRole("button", { name: "Reply", exact: true }).click();
    await composer().fill("Use the second option");
    await expect(page.locator(".update-reply-chip")).toContainText("Offsite venue");
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await expect(page.locator(".update-reply-chip")).toHaveCount(0);
    await page.getByRole("button", { name: "Open conversation: Plan the offsite", exact: true }).click();
    await expect(composer()).toHaveValue("Use the second option");
    await expect(page.locator(".update-reply-chip")).toContainText("Offsite venue");
    await expect.poll(async () => !!(await snapshot()).allChatsBusy).toBe(false);
    await closeDesktop(app); app = await launch(); page = await app.firstWindow();
    await expect(composer()).toHaveValue("Use the second option");
    await expect(page.locator(".update-reply-chip")).toContainText("Offsite venue");
    await expect(card()).toHaveCount(1);
    // The app restores automatic routing; keep this interaction fixture on a
    // fixed route so provider-call counts measure execution and deduplication.
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.getByText("Retained reply 2.", { exact: true })).toBeVisible();
    await expect.poll(async () => (await snapshot()).state!.running).toBe(false);
    expect(calls[1]).toContain("selected_update_id"); expect(calls[1]).toContain("inbox-offsite"); expect(calls[1]).toContain("job-offsite"); expect(calls[1]).not.toContain("PRIVATE OTHER CHAT");
    const journal = JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/conversations", origin, "requests.json"), "utf8"));
    const reply = journal.requests.find((item: { text: string }) => item.text === "Use the second option");
    expect(reply.options.ReplyToUpdateID).toBe(entry.id);
    const revision = JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/conversations", origin, "revisions", `${reply.revision_id}.json`), "utf8"));
    expect(revision.reply_to_update_id).toBe(entry.id);
    // A lost transport acknowledgement may resend the same admission, not execute twice.
    await page.evaluate(({ origin, id, client }) => window.gator!.send({ type: "upload_submit", upload: { request_id: client, context: `conversation:${origin}`, text: "Use the second option", files: [], reply_to_update_id: id } }), { origin, id: entry.id, client: reply.client_id });
    expect(calls).toHaveLength(2);
    const order = await page.locator(".transcript-content > article").evaluateAll(nodes => nodes.map(node => node.textContent));
    expect(order.findIndex(text => text?.includes("Offsite venue"))).toBeLessThan(order.findIndex(text => text?.includes("Use the second option")));
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    await card().scrollIntoViewIfNeeded();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: "test-results/chat-updates-compact.png" });
  } finally {
    await app.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1, checkboxChecked: false }); }).catch(() => {});
    await closeDesktop(app); server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); await rm(root, { recursive: true, force: true });
  }
});
