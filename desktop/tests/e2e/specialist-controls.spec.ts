import { expect, test } from "@playwright/test";
import { createServer, type ServerResponse } from "node:http";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, launchDesktop, temporaryDirectory } from "./lifecycle";

test("guide one live specialist and stop another without stopping their parent", async ({}, testInfo) => {
  test.setTimeout(90000);
  const root = await temporaryDirectory(join(tmpdir(), "gator-specialist-controls-"));
  const workspace = join(root, "workspace"), configRoot = join(root, "config/gator");
  await mkdir(workspace); await mkdir(configRoot, { recursive: true });
  await writeFile(join(workspace, "facts.md"), "The primary deadline is 18 November.\n");
  type Message = { role: string; content?: string; tool_call_id?: string };
  const call = (name: string, id: string, args: unknown) => ({ role: "assistant", tool_calls: [{ id, type: "function", function: { name, arguments: JSON.stringify(args) } }] });
  const final = (content: string) => ({ role: "assistant", content });
  const reply = (response: ServerResponse, message: ReturnType<typeof call> | ReturnType<typeof final>) => {
    if (response.writableEnded || response.destroyed) return;
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  };
  let releaseResearch: (() => void) | undefined, releaseParent: (() => void) | undefined;
  let guidanceSeen = false, peerDisconnected = false;
  const server = createServer(async (request, response) => {
    let body = ""; for await (const chunk of request) body += chunk;
    const { messages } = JSON.parse(body) as { messages: Message[] };
    const result = (id: string) => messages.find(m => m.role === "tool" && m.tool_call_id === id);
    if (messages.some(m => m.role === "system" && m.content?.includes("LIVE_RESEARCH_MARKER"))) {
      guidanceSeen ||= messages.some(m => m.content?.includes("Prioritize the 18 November primary deadline"));
      if (!result("read-evidence") && !guidanceSeen) releaseResearch = () => reply(response, call("read_file", "read-evidence", { path: "source/facts.md" }));
      // Guidance arriving in flight supersedes that response. The runner asks
      // for a fresh turn with the new instruction before it executes tools.
      else if (!result("read-evidence")) reply(response, call("read_file", "read-evidence", { path: "source/facts.md" }));
      else {
        reply(response, final("The primary deadline is 18 November [source/facts.md:1]."));
      }
    } else if (messages.some(m => m.role === "system" && m.content?.includes("LIVE_PEER_MARKER"))) {
      response.on("close", () => { if (!response.writableEnded) peerDisconnected = true; });
      // Remain in a real in-flight model request until the native Stop control
      // cancels this worker's context. The other worker must remain alive.
    } else if (!result("create-research")) {
      reply(response, call("create_agent", "create-research", { name: "deadline_researcher", base: "source_researcher", description: "Check the primary deadline", instructions: "LIVE_RESEARCH_MARKER: cite the primary evidence.", task: "Read facts.md and check the deadline." }));
    } else if (!result("create-peer")) {
      reply(response, call("create_agent", "create-peer", { name: "independent_verifier", base: "source_researcher", description: "Independently verify the dates", instructions: "LIVE_PEER_MARKER: verify the dated claims.", task: "Independently verify the deadline." }));
    } else if (!result("wait-research")) {
      reply(response, call("await_agent", "wait-research", { id: JSON.parse(result("create-research")!.content!).task.id, timeout_seconds: 30 }));
    } else if (!result("wait-peer")) {
      reply(response, call("await_agent", "wait-peer", { id: JSON.parse(result("create-peer")!.content!).task.id, timeout_seconds: 30 }));
    } else releaseParent = () => reply(response, final("Research completed with the primary deadline. The independent verifier was stopped."));
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  const base = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
  await writeFile(join(configRoot, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base, models: ["fixture"], default_model: "fixture" }] }), { mode: 0o600 });
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await launchDesktop({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env: { ...process.env, GATOR_INTERNAL_COMMANDS: "", GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace } });
  const page = await app.firstWindow();
  const snapshot = () => page.evaluate(async () => (await window.gator!.snapshot()).state!);
  try {
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    await composer.fill("/mode inspect"); await composer.press("Enter"); await expect(composer).toHaveValue("");
    await composer.fill("Create two research specialists and compare their evidence about the deadline."); await composer.press("Enter");
    const research = page.getByRole("article", { name: "deadline researcher specialist", exact: true });
    const peer = page.getByRole("article", { name: "independent verifier specialist", exact: true });
    await expect(research).toContainText("Working"); await expect(peer).toContainText("Working");
    await research.getByText("Assignment", { exact: true }).click(); await expect(research).toContainText("Read facts.md");
    const oldCards = (await snapshot()).specialists!;
    await research.getByRole("button", { name: "Guide", exact: true }).click();
    await research.getByRole("textbox", { name: "Guidance for deadline researcher" }).fill("Prioritize the 18 November primary deadline");
    await expect(research.getByRole("button", { name: "Send guidance", exact: true })).toBeInViewport();
    await page.screenshot({ path: testInfo.outputPath("specialists-guidance.png") });
    await research.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(research.getByRole("button", { name: "Guide", exact: true })).toBeFocused();
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/theme light" }));
    await expect(page.locator(".app-shell")).toHaveAttribute("data-theme", "light");
    await page.setViewportSize({ width: 820, height: 740 });
    await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "light" });
    await research.getByRole("button", { name: "Guide", exact: true }).click();
    const guidance = research.getByRole("textbox", { name: "Guidance for deadline researcher" });
    await expect(guidance).toHaveValue("Prioritize the 18 November primary deadline");
    await expect.poll(async () => {
      const form = await research.locator("form").boundingBox(), scroll = await page.locator(".conversation-scroll").boundingBox();
      return !!form && !!scroll && form.y >= scroll.y && form.y + form.height <= scroll.y + scroll.height;
    }).toBe(true);
    await guidance.press("Tab");
    const submitGuidance = research.getByRole("button", { name: "Send guidance", exact: true });
    await expect(submitGuidance).toBeFocused(); await expect(submitGuidance).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("specialists-guidance-light-narrow.png") });
    await submitGuidance.press("Enter");
    await expect(research).toContainText("Guidance queued for the next model step");
    await peer.getByRole("button", { name: "Guide", exact: true }).click();
    await peer.getByRole("textbox", { name: "Guidance for independent verifier" }).fill("Check which timezone applies to the deadline.");
    await peer.getByRole("button", { name: "Stop", exact: true }).click();
    await expect(peer).toContainText("Stopped");
    await expect(page.getByText("Part of this work needs review", { exact: true })).toHaveCount(0);
    const retained = page.getByRole("article", { name: "Retained guidance for independent verifier", exact: true });
    await expect(retained).toContainText("Check which timezone applies");
    await expect.poll(() => peerDisconnected).toBe(true);
    expect((await snapshot()).running).toBe(true);
    await expect(research).toContainText("Working");
    releaseResearch!();
    await expect.poll(() => guidanceSeen).toBe(true);
    await expect(research).toContainText("Finished");
    await expect.poll(() => !!releaseParent).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("specialists-results.png") });
    releaseParent!();
    await expect.poll(async () => (await snapshot()).running).toBe(false);
    await expect(page.getByRole("article", { name: "Gator message", exact: true }).last()).toContainText("independent verifier was stopped");
    await expect(retained).toContainText("Check which timezone applies");
    await retained.getByRole("button", { name: "Move to composer", exact: true }).click();
    await expect(composer).toHaveValue("Check which timezone applies to the deadline.");
    await expect(retained).toHaveCount(0);
    expect((await snapshot()).running).toBe(false);
    // Even the same task ID cannot reuse a finished run's native control.
    await page.evaluate(async card => window.gator!.send({ type: "specialist", specialist: { id: card.id, token: card.token!, request_id: "stale-stop", action: "stop" } }), oldCards[0]);
    await expect.poll(async () => (await snapshot()).specialist_result?.error).toContain("no longer available");
    const taskRoot = join(root, "state/gator/tasks"), run = (await readdir(taskRoot))[0];
    const first = JSON.parse(await readFile(join(taskRoot, run, "subagent-001.json"), "utf8"));
    const second = JSON.parse(await readFile(join(taskRoot, run, "subagent-002.json"), "utf8"));
    expect(first).toMatchObject({ role: "deadline_researcher", status: "completed", steering_requests: ["Prioritize the 18 November primary deadline"] });
    expect(first.result.InspectedPaths).toContain("source/facts.md");
    expect(second).toMatchObject({ role: "independent_verifier", status: "cancelled" });
  } finally {
    // Always release fixture requests before shutdown, including assertion failures.
    releaseResearch?.(); releaseParent?.(); server.closeAllConnections();
    try {
      if ((await snapshot()).running) {
        await page.evaluate(() => window.gator!.send({ type: "submit", text: "/cancel" }));
        await expect.poll(async () => (await snapshot()).running, { timeout: 5000 }).toBe(false);
      }
    } catch { /* Preserve the original test failure if the fixture disconnected. */ }
    await closeDesktop(app);
    await new Promise<void>(done => server.close(() => done()));
    await rm(root, { recursive: true, force: true });
  }
});
