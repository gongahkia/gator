import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { randomUUID } from "node:crypto"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"
import { draftLinks, markdownLink } from "../src/gator/draft-links"

async function until(predicate: () => boolean, label: string) {
  const end = Date.now() + 20000
  while (!predicate()) { if (Date.now() > end) throw new Error("Timed out: " + label); await Bun.sleep(20) }
}

test("chat commands retain exact plan reviews, frozen evidence and independent branches with the current Go backend", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-port-")), workspace = join(root, "workspace")
  await mkdir(workspace); await mkdir(join(root, "config/gator"), { recursive: true })
  await writeFile(join(workspace, "project.md"), "Owner: Mina\nDeadline: Friday\n")
  let calls = 0
  const proposal = { outcome: "Explain the observed project", steps: [{ id: "read", text: "Read the project" }, { id: "explain", text: "Explain the observed facts" }], sources: [], effects: ["read"] }
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const body = await request.json() as { tools?: unknown[]; messages: { role: string }[] }; calls++
    const lastUser = body.messages.map(message => message.role).lastIndexOf("user")
    const message = !body.tools?.length ? { role: "assistant", content: JSON.stringify(proposal) }
      : body.messages.slice(lastUser + 1).some(message => message.role === "tool") ? { role: "assistant", content: "Mina owns the project. The deadline is Friday. Source: `source/project.md`." }
        : { role: "assistant", tool_calls: [{ id: randomUUID(), type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/project.md" }) } }] }
    return Response.json({ choices: [{ message, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } })
  } })
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace, userData: join(root, "app"), env: { ...process.env, GATOR_UI_FRONTEND: "terminal", GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  const client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const choose = async (text: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const review = async () => { await client.navigate("/task-plans"); await flush(); await choose("Explain the observed project") }
  try {
    await client.start(); await flush()
    expect(setup.renderer.root.findDescendantById("navigation-rail")).toBeUndefined()
    expect(setup.captureCharFrame()).toContain("Ask anything…")
    await client.send({ type: "submit", text: "/mode inspect" })
    await client.navigate("/plan-first"); await flush(); await choose("Prepare a plan")
    await client.edit("Read project.md and explain its owner and deadline"); await client.submit()
    await until(() => client.state?.task_plans?.[0]?.phase === "review" && !client.state?.running, "retained plan")
    expect(calls).toBe(1)
    const original = client.state!.task_plans![0]!, conversation_id = client.state!.conversation_id!
    await client.edit("Keep this independent draft")
    await review(); await choose("Edit plan"); await choose("Intended outcome")
    setup.mockInput.pressKey("a", { ctrl: true }); await setup.mockInput.typeText("Explain verified project facts"); setup.mockInput.pressEnter(); await flush()
    await choose("Save revision for review")
    await until(() => client.state?.task_plans?.[0]?.revision === 2, "saved plan revision")
    const stale = randomUUID()
    await expect(client.result({ type: "task_plan", task_plan: { conversation_id, task_id: original.id, revision: original.revision, digest: original.digest, action: "approve", request_id: stale } }, stale, state => state.task_plan_result)).rejects.toThrow()
    expect(calls).toBe(1)
    setup.mockInput.pressEscape(); await flush()
    await client.navigate("/task-plans"); await flush(); await choose("Explain verified project facts"); await choose("Approve plan and start")
    expect(setup.captureCharFrame()).toContain("Read the project")
    setup.mockInput.pressEnter()
    await until(() => client.state?.task_plans?.[0]?.state === "completed" && !client.state?.running, "approved implementation")
    expect(client.draft.text).toBe("Keep this independent draft")
    const beforeInspect = calls, bundle = client.state!.messages.find(message => message.bundle?.RunID)!.bundle!
    await writeFile(join(workspace, "project.md"), "Owner: Eve\nDeadline: Tuesday\n")
    const catalog = await client.host.sourceCatalog({ context: client.context, run: bundle.RunID! })
    const source = catalog.references.find(reference => reference.locator === "source/project.md")!
    expect(source).toBeDefined()
    const preview = await client.host.sourcePreview({ context: client.context, run: bundle.RunID!, path: "evidence:" + source.id })
    expect(preview.text).toContain("Mina"); expect(preview.text).not.toContain("Eve"); expect(preview.reads.length).toBeGreaterThan(0)
    await client.navigate("/evidence"); await flush(); await choose("Mina owns"); await choose("project.md")
    expect(setup.captureCharFrame()).toContain("Mina"); expect(calls).toBe(beforeInspect)
    setup.mockInput.pressEscape(); await flush()
    await client.navigate("/branch"); await flush(); await choose("Mina owns"); setup.mockInput.pressEnter()
    await until(() => !!client.state?.branch && client.state?.conversation_id !== conversation_id && !client.snapshot.sessionSwitching, "independent branch")
    expect(client.issue).toBe(""); expect(client.state?.branch?.conversation_id).toBe(conversation_id)
    expect(client.state?.messages.some(message => message.text.includes("Mina owns"))).toBe(true)
    expect(client.draft.text).toBe(""); expect(calls).toBe(beforeInspect)
    await client.navigate("/original-chat"); await until(() => client.state?.conversation_id === conversation_id, "original chat")
    expect(client.draft.text).toBe("Keep this independent draft")
  } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)

test("source choices and individual specialist controls use live IDs without overwriting the chat draft", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-controls-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  host.current.sources = { id: "source-generation", selected: [], choices: [{ id: "mail", label: "My Gmail", service: "Gmail", status: "Ready", access: "Read" }] }
  host.current.specialists = [{ id: "worker-one", token: "exact-live-token", name: "Deadline researcher", status: "Working", can_guide: true, can_stop: true }]
  host.dispatch = async frame => {
    if (frame.sources) { if (frame.sources.action === "select") host.current.sources!.selected = frame.sources.ids; host.current.sources_result = { request_id: frame.sources.request_id } }
    if (frame.specialist) host.current.specialist_result = { request_id: frame.specialist.request_id }
    host.change(host.current)
  }
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 90, height: 28 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const choose = async (text: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.edit("Keep my unsent message")
    await client.navigate("/sources"); await flush(); await choose("My Gmail")
    expect(host.current.sources.selected).toEqual(["mail"]); setup.mockInput.pressEscape(); await flush()
    await client.navigate("/specialists"); await flush(); await choose("Deadline researcher"); await choose("Guide this specialist")
    await setup.mockInput.typeText("Use the primary deadline"); setup.mockInput.pressEnter(); await flush()
    expect(host.sent.find(frame => frame.specialist)?.specialist).toMatchObject({ id: "worker-one", token: "exact-live-token", action: "guide", text: "Use the primary deadline" })
    expect(client.draft.text).toBe("Keep my unsent message")
    setup.resize(48, 18); await client.navigate("/specialists"); await flush(); expect(setup.captureCharFrame()).toContain("Deadline researcher")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("link edits preserve punctuation and reject non-web or credential-bearing URLs", () => {
  expect(draftLinks("See https://example.com/a. Then https://example.com/a_(b). ").map(link => link.url)).toEqual(["https://example.com/a", "https://example.com/a_(b)"])
  expect(draftLinks("javascript:alert(1) https://user:secret@example.com/")).toHaveLength(0)
  expect(markdownLink("Project", "https://example.com/project")).toBe("[Project](https://example.com/project)")
  expect(() => markdownLink("Unsafe", "javascript:alert(1)")).toThrow()
})
