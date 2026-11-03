import { expect, test, type Page } from "@playwright/test";
import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { wordFixture } from "../office-fixtures.mjs";
import { closeDesktop, launchDesktop, temporaryDirectory } from "./lifecycle";

const state = (page: Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);

test("selected Office originals reach Gmail only after exact review and remain distinct from model text", async () => {
  test.setTimeout(90000);
  const root = await temporaryDirectory(join(tmpdir(), "gator-email-original-"));
  const workspace = join(root, "workspace"), configRoot = join(root, "config/gator"), selected = join(root, "report.docx");
  await mkdir(workspace); await mkdir(configRoot, { recursive: true });
  const original = wordFixture();
  await writeFile(selected, original);
  const modelRequests: unknown[] = [];
  const sent: Buffer[] = [];
  const server = createServer(async (request, response) => {
    let body = "";
    for await (const chunk of request) body += chunk;
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/gmail/v1/users/me/messages/send") {
      sent.push(Buffer.from(JSON.parse(body).raw, "base64url"));
      response.end('{"id":"sent-original"}');
      return;
    }
    if (request.url !== "/chat") { response.statusCode = 404; response.end("{}"); return; }
    const payload = JSON.parse(body);
    modelRequests.push(payload);
    const listing = payload.messages.find((message: { role: string; tool_call_id?: string }) => message.role === "tool" && message.tool_call_id === "selected-originals");
    const sentResult = payload.messages.some((message: { role: string; tool_call_id?: string }) => message.role === "tool" && message.tool_call_id === "send-original");
    let message;
    if (!listing) {
      message = { role: "assistant", tool_calls: [{ id: "selected-originals", type: "function", function: { name: "list_input_files", arguments: "{}" } }] };
    } else if (!sentResult) {
      const files = JSON.parse(listing.content).result.files;
      expect(files).toHaveLength(1);
      expect(files[0].filename).toBe("report.docx");
      message = { role: "assistant", tool_calls: [{ id: "send-original", type: "function", function: { name: "connector_google_gmail_send", arguments: JSON.stringify({ payload: { from: "me@example.com", to: "reviewer@example.com", subject: "Original report", body: "Please review the attached original.", attachments: [{ source: files[0].source }] } }) } }] };
    } else {
      message = { role: "assistant", content: "The original-file send result has been recorded." };
    }
    response.end(JSON.stringify({ choices: [{ message, finish_reason: message.tool_calls ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  const base = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
  await writeFile(join(configRoot, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base + "/chat", models: ["fixture"], default_model: "fixture" }], connectors: [{ version: 1, id: "google", name: "Gmail fixture", kind: "google_workspace", resource: base, authentication: "none" }] }), { mode: 0o600 });
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "", GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await launchDesktop({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  try {
    const page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    for (const command of ["/mode act", "/connector add google"]) {
      await composer.fill(command); await composer.press("Enter"); await expect(composer).toHaveValue("");
    }
    await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual(["google"]);
    await app.evaluate(({ dialog }, file) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [file] }); }, selected);
    await page.getByRole("button", { name: "Attach files", exact: true }).click();
    await expect(page.getByRole("button", { name: "Remove attachment report.docx", exact: true })).toBeVisible();
    expect(modelRequests).toHaveLength(0); expect(sent).toHaveLength(0);
    await composer.fill("Email the original attached report to reviewer@example.com for review.");
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    const approve = page.getByRole("button", { name: "Approve this action", exact: true });
    await expect(approve).toBeVisible();
    const snapshot = await state(page);
    const proposal = snapshot.messages[snapshot.pending_approval!.message_index!].result!.preview as { preview: string };
    const reviewed = JSON.parse(proposal.preview);
    expect(reviewed.attachments).toEqual([expect.objectContaining({ filename: "report.docx", bytes: original.length, sha256: createHash("sha256").update(original).digest("hex") })]);
    expect(JSON.stringify(proposal)).not.toContain(original.toString("base64"));
    expect(JSON.stringify(modelRequests)).toContain("Hello world.");
    expect(JSON.stringify(modelRequests)).not.toContain(original.toString("base64"));
    expect(sent).toHaveLength(0);
    // The file selected by the picker may change without changing the staged original.
    await writeFile(selected, Buffer.from("changed after selecting the attachment"));
    await approve.click();
    await expect.poll(async () => (await state(page)).running).toBe(false);
    expect(sent).toHaveLength(1);
    // Decode the binary MIME part independently of Gator's attachment builder.
    const mime = sent[0].toString("ascii");
    const part = mime.split(/--gator-[a-f0-9]+\r\n/).find(part => part.includes("Content-Disposition: attachment;"));
    expect(part).toBeTruthy();
    const encoded = part!.split("\r\n\r\n", 2)[1].split(/\r\n--gator-/)[0];
    expect(Buffer.from(encoded, "base64")).toEqual(original);
    await expect(page.locator(".connected-receipt > summary")).toContainText("Gmail accepted the message");
  } finally {
    await closeDesktop(app); server.closeAllConnections();
    await new Promise<void>(done => server.close(() => done()));
    await rm(root, { recursive: true, force: true });
  }
});
