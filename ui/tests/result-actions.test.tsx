import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import type { ScrollBoxRenderable, TextareaRenderable } from "@opentui/core"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) test(`original result actions → retained evidence → editable file → update reply at ${width}×${height}`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-result-actions-")), workspace = join(root, "workspace"), original = "# Report\nOwner: Mina\nDeadline: Friday\n"
  const requests: string[] = [], copied: string[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const raw = await request.text(); requests.push(raw)
    const body = JSON.parse(raw), messages = body.messages as { role: string }[], last = messages.map(message => message.role).lastIndexOf("user")
    const message = messages.slice(last + 1).some(message => message.role === "tool") || raw.includes("selected_update_id")
      ? { role: "assistant", content: "Mina owns the Friday report. Retained evidence and report.md are available." }
      : { role: "assistant", tool_calls: [
        { id: "read-project", type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/project.md" }) } },
        { id: "write-report", type: "function", function: { name: "write_artifact", arguments: JSON.stringify({ path: "report.md", content: original }) } },
      ] }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  await mkdir(workspace); await mkdir(join(root, "config/gator"), { recursive: true }); await writeFile(join(workspace, "project.md"), original)
  const key = width === 42 ? "k" : "e"
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }], desktop: { shortcuts: { context_actions: `Ctrl+${key.toUpperCase()}` } } }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace, env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  const host = new Application(options), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async text => { copied.push(text) } }} />, { width, height })
  const flush = async () => { await setup.flush(); await Bun.sleep(35); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 20000; while (!await predicate()) { if (Date.now() > end) throw new Error(`${label}: ${client.issue}\n${setup.captureCharFrame()}`); await flush() } }
  const choose = async (label: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const escape = async () => { setup.mockInput.pressEscape(); await flush() }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode draft" }); await client.send({ type: "submit", text: "/artifact report.md" })
    await client.edit("Read project.md and write report.md with its owner and deadline."); await client.submit()
    await until(() => !client.state?.running && !!client.state?.messages.some(message => message.bundle?.Artifacts?.some(file => file.Path === "report.md")), "retained report")
    const context = client.context, conversation = client.state!.conversation_id!, bundle = client.state!.messages.find(message => message.bundle?.Artifacts?.some(file => file.Path === "report.md"))!.bundle!
    const index = client.state!.messages.findIndex(message => message.role === "Gator" && message.run_id === bundle.RunID), message = client.state!.messages[index]!
    expect(index).toBeGreaterThanOrEqual(0)
    const originalBytes = await readFile(join(bundle.Path, "output/report.md")), manifest = await readFile(join(bundle.Path, "manifest.json")), count = requests.length
    await client.edit("Keep my independent\nmultiline draft"); await client.setPlanFirst(true); await flush()
    client.emit("reveal-message", index); await flush()
    expect(setup.captureCharFrame()).toContain(`Ctrl+${key.toUpperCase()} actions`)
    setup.mockInput.pressKey(key, { ctrl: true }); await flush()
    await choose("Copy result text"); expect(copied.at(-1)).toContain("Mina owns")
    expect(client.draft.text).toBe("Keep my independent\nmultiline draft"); expect(client.draft.plan_first).toBe(true)
    client.emit("reveal-message", index); await flush(); setup.mockInput.pressKey(key, { ctrl: true }); await flush()
    await choose("Inspect source evidence"); await choose("project.md")
    await until(() => setup.captureCharFrame().includes("Read only"), "retained source details loaded")
    expect(setup.captureCharFrame()).toContain("project.md")
    for (let page = 0; page < 30 && !setup.captureCharFrame().includes("Mina"); page++) { setup.mockInput.pressKey("\x1b[6~"); await flush() }
    expect(setup.captureCharFrame()).toContain("Mina")
    expect(requests).toHaveLength(count)
    // Escape returns through evidence and the original item menu.
    await escape(); await until(() => setup.captureCharFrame().includes("Retained sources"), "back to exact evidence list")
    await escape(); await until(() => setup.captureCharFrame().includes("Inspect source evidence"), "back to filtered original result actions")
    await choose("File: report.md"); await choose("Open complete file")
    await until(() => !!setup.renderer.root.findDescendantById("file-text-editor"), "exact file editor")
    const editor = setup.renderer.root.findDescendantById("file-text-editor") as TextareaRenderable
    expect(editor.height).toBeGreaterThanOrEqual(2); expect(editor.screenY + editor.height).toBeLessThanOrEqual(height)
    expect(setup.captureCharFrame()).toContain("Ctrl+S save")
    await setup.mockInput.pasteBracketedText("Edited result: silver otter.\n"); await flush(); setup.mockInput.pressKey("s", { ctrl: true })
    const ref = { kind: "output" as const, bundle: bundle.Path, path: "report.md" }, file = () => host.file({ action: "open", context, ref })
    await until(async () => (await file()).versions.length === 2, "saved exact working version")
    const saved = await file(); expect(saved.text).toContain("silver otter"); expect(saved.text).toContain("Mina")
    await until(() => !!setup.renderer.root.findDescendantById(`file-editor-${saved.revision}`), "saved working version rendered")
    expect(await readFile(join(bundle.Path, "output/report.md"))).toEqual(originalBytes); expect(await readFile(join(bundle.Path, "manifest.json"))).toEqual(manifest)
    await escape(); expect(client.draft.text).toBe("Keep my independent\nmultiline draft"); expect(client.context).toBe(context)
    // The same title elsewhere must not gain this item's controls.
    await client.navigate("/new"); await client.edit("Other conversation draft")
    client.emit("item-actions", { kind: "message", context, index, message }); await flush()
    expect(client.issue).toContain("original conversation"); expect(client.draft.text).toBe("Other conversation draft"); expect(requests).toHaveLength(count)
    await client.navigate("/conversations"); await until(() => !!client.state?.panel?.rows?.some(row => row.identity?.startsWith(conversation + ":")), "versioned conversation picker")
    const panel = client.state!.panel!, row = panel.rows!.findIndex(row => row.identity?.startsWith(conversation + ":"))
    await client.send({ type: "panel", panel: { id: panel.id!, action: "activate", row } }); await until(() => client.context === context, "original owner restored")
    client.issue = ""; await client.send({ type: "submit", text: "/artifact clear" }); await flush()
    const inbox = join(options.env.GATOR_STATE_DIR, "gator/inbox"), path = join(inbox, "inbox-report.json")
    await mkdir(inbox, { recursive: true }); await writeFile(path, JSON.stringify({ version: 1, id: "inbox-report", kind: "job", title: "Report ready", summary: "Review Mina's report.", status: "completed", origin_conversation_id: conversation, job_id: "job-report", evidence: ["source/project.md:2"], created_at: new Date().toISOString() }))
    await until(() => client.state?.chat_updates?.some(update => update.id === "inbox-report") || false, "owned update")
    const scroll = setup.renderer.root.findDescendantById("transcript") as ScrollBoxRenderable
    scroll.scrollTo(scroll.scrollHeight); await flush(); setup.mockInput.pressKey(key, { ctrl: true }); await flush()
    const previousReceipt = client.state?.chat_update_result?.request_id
    await choose("Mark done"); await until(async () => Date.parse(JSON.parse(await readFile(path, "utf8")).resolved_at) > 0 && !!client.state?.chat_updates?.find(update => update.id === "inbox-report")?.resolved && !!client.state?.chat_update_result && client.state.chat_update_result.request_id !== previousReceipt, "acknowledged update decision")
    expect(client.state?.chat_update_result?.error).toBeUndefined(); expect(client.context).toBe(context)
    await client.navigate("/actions"); await flush(); await choose("Report ready"); await choose("Reply in original chat")
    await until(() => client.draft.reply_to_update_id === "inbox-report", "original update reply staged")
    expect(client.draft.reply_to_update_id).toBe("inbox-report"); expect(client.draft.text).toBe("Keep my independent\nmultiline draft"); expect(client.draft.plan_first).toBe(true)
    expect(requests).toHaveLength(count)
    await client.setPlanFirst(false); await client.edit("Thanks, inspect this update's retained result."); setup.mockInput.pressEnter()
    await until(() => requests.length > count && !client.state?.running, "explicit contextual reply")
    expect(requests.slice(count).join("\n")).toContain("inbox-report")
    const journal = JSON.parse(await readFile(join(options.env.GATOR_STATE_DIR, "gator/conversations", conversation, "requests.json"), "utf8"))
    expect(journal.requests.find((entry: { text: string }) => entry.text.startsWith("Thanks, inspect"))?.options.ReplyToUpdateID).toBe("inbox-report")
    expect(await readFile(join(bundle.Path, "output/report.md"))).toEqual(originalBytes)
  } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
