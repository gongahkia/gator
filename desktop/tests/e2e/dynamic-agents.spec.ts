import { expect, test, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, launchDesktop, temporaryDirectory } from "./lifecycle";

test("create a research specialist, run it, restart, reuse and delete its saved definition", async () => {
  test.setTimeout(120000);
  const root = await temporaryDirectory(join(tmpdir(), "gator-specialist-"));
  const workspace = join(root, "workspace"), configRoot = join(root, "config/gator");
  await mkdir(workspace); await mkdir(configRoot, { recursive: true });
  await writeFile(join(workspace, "facts.md"), "# Venue facts\nThe review deadline is 18 November.\n");
  type Message = { role: string; content?: string; tool_call_id?: string };
  let childCalls = 0, definitions = 0;
  const childTools = new Set<string>();
  const server = createServer(async (request, response) => {
    let body = "";
    for await (const chunk of request) body += chunk;
    const payload = JSON.parse(body), messages = payload.messages as Message[];
    const toolResult = (id: string) => messages.find(m => m.role === "tool" && m.tool_call_id === id);
    const call = (name: string, id: string, args: unknown) => ({ role: "assistant", tool_calls: [{ id, type: "function", function: { name, arguments: JSON.stringify(args) } }] });
    const final = (content: string) => ({ role: "assistant", content });
    let message;
    if (messages.some(m => m.role === "system" && m.content?.includes("ROLE_SCOUT_MARKER"))) {
      childCalls++;
      for (const tool of payload.tools ?? []) childTools.add(tool.function.name);
      message = toolResult("child-read") ? final("The review deadline is 18 November [source/facts.md:2].") : call("read_file", "child-read", { path: "source/facts.md" });
    } else if (messages.some(m => m.role === "user" && m.content?.includes("Delete the saved timeline_scout"))) {
      const listed = toolResult("saved-list");
      message = !listed ? call("agent_profiles", "saved-list", { action: "list" }) : !toolResult("saved-delete") ? call("agent_profiles", "saved-delete", { action: "delete", name: "timeline_scout", expected_saved_digest: JSON.parse(listed.content!).profiles[0].digest, user_quote: "Delete the saved timeline_scout specialist" }) : final("Deleted the saved specialist. Prior task history remains available.");
    } else if (messages.some(m => m.role === "user" && m.content?.includes("Use my saved timeline_scout"))) {
      message = !toolResult("available") ? call("list_agents", "available", {}) : !toolResult("reuse") ? call("start_agent", "reuse", { agent: "timeline_scout", task: "Read facts.md and identify the review deadline." }) : !toolResult("reuse-wait") ? call("await_agent", "reuse-wait", { id: JSON.parse(toolResult("reuse")!.content!).id, timeout_seconds: 5 }) : final("The saved specialist confirmed the 18 November deadline.");
    } else {
      if (!toolResult("created")) {
        definitions++;
        message = call("create_agent", "created", { name: "timeline_scout", base: "source_researcher", description: "Check dates in the selected evidence", instructions: "ROLE_SCOUT_MARKER: investigate dated claims and cite exact source paths.", max_steps: 3, save: true, user_quote: "save it for future chats", task: "Read facts.md and identify the review deadline." });
      } else if (!toolResult("created-wait")) {
        message = call("await_agent", "created-wait", { id: JSON.parse(toolResult("created")!.content!).task.id, timeout_seconds: 5 });
      } else message = final("Created and saved timeline_scout. It found the 18 November review deadline.");
    }
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } }));
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  const base = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
  await writeFile(join(configRoot, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base, models: ["fixture"], default_model: "fixture" }] }), { mode: 0o600 });
  const env = { ...process.env, GATOR_INTERNAL_COMMANDS: "", GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const launch = () => launchDesktop({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env });
  let app = await launch();
  const state = (page: Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);
  const send = async (page: Page, text: string) => {
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible();
    await composer.fill("/mode inspect"); await composer.press("Enter"); await expect(composer).toHaveValue("");
    await composer.fill(text); await page.getByRole("button", { name: "Send message", exact: true }).click();
  };
  const savedPath = join(root, "state/gator/agent-profiles/profiles.json");
  try {
    let page = await app.firstWindow();
    await send(page, "Create timeline_scout, a research agent for dated facts, save it for future chats, and investigate the selected facts.");
    await expect(page.getByRole("article", { name: "Gator message", exact: true }).last()).toContainText("18 November");
    await expect.poll(async () => (await state(page)).running).toBe(false);
    expect(JSON.parse(await readFile(savedPath, "utf8")).profiles).toHaveLength(1);
    const taskRoot = join(root, "state/gator/tasks");
    const run = (await readdir(taskRoot))[0];
    const checkpoint = JSON.parse(await readFile(join(taskRoot, run, "subagent-001.json"), "utf8"));
    expect(checkpoint).toMatchObject({ role: "timeline_scout", status: "completed", profile: { saved: true, definition: { base: "source_researcher" } } });
    expect(checkpoint.result.InspectedPaths).toContain("source/facts.md");
    await closeDesktop(app);
    app = await launch(); page = await app.firstWindow();
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await send(page, "Use my saved timeline_scout to investigate the selected facts.");
    await expect(page.getByRole("article", { name: "Gator message", exact: true }).last()).toContainText("confirmed the 18 November");
    await expect.poll(async () => (await state(page)).running).toBe(false);
    expect(definitions).toBe(1); expect(childCalls).toBe(4);
    expect(childTools.has("read_file")).toBe(true);
    for (const control of ["create_agent", "start_agent", "agent_profiles", "delegate_agents", "gator_app_update"]) expect(childTools.has(control)).toBe(false);
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await send(page, "Delete the saved timeline_scout specialist.");
    await expect(page.getByRole("article", { name: "Gator message", exact: true }).last()).toContainText("Deleted the saved specialist");
    await expect.poll(async () => (await state(page)).running).toBe(false);
    expect(JSON.parse(await readFile(savedPath, "utf8")).profiles).toEqual([]);
    expect(await readFile(join(taskRoot, run, "subagent-001.json"), "utf8")).toContain("ROLE_SCOUT_MARKER");
  } finally {
    await closeDesktop(app); server.closeAllConnections();
    await new Promise<void>(done => server.close(() => done()));
    await rm(root, { recursive: true, force: true });
  }
});
