import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import type { TextareaRenderable } from "@opentui/core"
import { execFileSync } from "node:child_process"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

// Real Go context/history/persistence and native keyboard rendering; the model
// endpoint is controlled. This is not evidence of a live model or physical PTY.
for (const [width, height] of [[110,34], [80,24], [60,20], [42,14]]) test(`answer memory → exact excerpt → guarded correction → busy inspection → restart at ${width}×${height}`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-answer-context-")), calls: string[] = []
  const supplied = "Planning replies should use three short paragraphs.", current = "Planning replies should use six detailed paragraphs.", corrected = "Planning replies should use two clear paragraphs."
  let release: (() => void) | undefined
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const raw = await request.text(); calls.push(raw)
    if (raw.includes("WAIT_FOR_RELEASE")) await new Promise<void>(resolve => { release = resolve })
    return Response.json({ choices: [{ message: { role: "assistant", content: "Your planning reply is ready." }, finish_reason: "stop" }], usage: { total_tokens: 20 } })
  } })
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime")
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
  await mkdir(join(root, "config/gator"), { recursive: true })
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const id = cli(["memory", "add", "--kind", "preference", "--source", "planning call", supplied]).split(/\s/)[0]!
  const memoryPath = join(root, "state/gator/memory", id + ".json"), options = { binary, env, workspace: join(root, "workspace"), userData: join(root, "app") }
  let host = new Application(options), client = new GatorClient(host)
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async () => {} }} />, { width, height })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (check: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 20000; while (!await check()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const press = async (name: string, ctrl = false) => { setup.mockInput.pressKey(name, { ctrl }); await flush() }
  const block = (title: string) => client.state?.panel?.blocks?.find(block => block.title === title)?.lines?.join("\n")
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Prepare planning replies in paragraphs."); await client.submit()
    await until(() => !!client.state?.messages.some(message => message.role === "Gator" && message.run_id) && !client.state?.running, "retained answer")
    expect(calls[0]).toContain(supplied)
    const context = client.context, conversation = client.state!.conversation_id!, index = client.state!.messages.findIndex(message => message.role === "Gator" && message.run_id), message = client.state!.messages[index]!
    const open = async () => {
      // Wait for the current owner's initial native frame, including restart.
      await flush()
      // Select the currently projected exact run, including restored messages.
      // Old callback snapshots intentionally cannot authorize a fresh view.
      const selected = client.state!.messages.findIndex(item => item.role === message.role && item.run_id === message.run_id)
      client.emit("item-actions", { kind: "message", context, index: selected, message: client.state!.messages[selected]! }); await flush()
      await setup.mockInput.typeText("Inspect supplied memory"); await flush(); setup.mockInput.pressEnter()
      await until(() => client.state?.panel?.kind === "context", "answer-specific context")
      setup.mockInput.pressEnter(); await until(() => block("Supplied excerpt") === supplied, "original supplied excerpt")
    }
    cli(["memory", "correct", id, current]); await client.edit("Keep my multiline\ndraft"); await client.setPlanFirst(true)
    const count = calls.length
    await open(); expect(block("Current saved text")).toContain(current); expect(JSON.stringify(client.state?.panel?.blocks)).toContain("planning call")
    expect(setup.captureCharFrame()).toContain("Supplied excerpt")
    await press("e", true); await until(() => client.state?.panel?.kind === "knowledge-form", "correction form")
    setup.mockInput.pressEnter(); await until(() => !!setup.renderer.root.findDescendantById("multiline-field-input"), "editable correction")
    const field = setup.renderer.root.findDescendantById("multiline-field-input") as TextareaRenderable
    field.setText(corrected); await press("s", true); await press("r", true)
    await until(async () => JSON.parse(await readFile(memoryPath, "utf8")).text === corrected && client.state?.panel?.kind === "context" && block("Current saved text")?.includes(corrected) === true, "authoritative correction and displayed acknowledgement")
    expect(block("Supplied excerpt")).toBe(supplied); expect(block("Current saved text")).toContain(corrected); expect(calls).toHaveLength(count)
    await client.navigate("/chat"); expect(client.context).toBe(context); expect(client.draft.text).toBe("Keep my multiline\ndraft"); expect(client.draft.plan_first).toBe(true)
    await client.setPlanFirst(false); await client.edit("Prepare planning replies WAIT_FOR_RELEASE"); await client.submit(); await until(() => !!release && !!client.state?.running, "controlled ongoing work")
    await client.edit("Independent draft during work"); await open()
    expect(client.context).not.toBe(context); expect(client.snapshot.chatActivity.find(item => item.id === conversation)?.running).toBe(true)
    expect(client.state?.panel?.hints?.some(hint => /omit/.test(hint.action))).toBe(false)
    expect(block("Next-message recall")).toContain("original conversation")
    const before = calls.length; await press("x", true); await until(() => client.state?.panel?.kind === "confirmation", "forget review")
    expect(JSON.parse(await readFile(memoryPath, "utf8")).text).toBe(corrected)
    setup.mockInput.pressEnter(); await until(async () => { try { await readFile(memoryPath); return false } catch (error) { return (error as NodeJS.ErrnoException).code === "ENOENT" } }, "confirmed forget")
    expect(block("Supplied excerpt")).toBe(supplied); expect(calls).toHaveLength(before)
    await client.openSession(conversation); await until(() => client.context === context, "return to original executing owner")
    expect(client.draft.text).toBe("Independent draft during work"); release!(); await until(() => !client.state?.running, "original work complete")
    await client.close(); await setup.dispose(); host = new Application(options); client = new GatorClient(host); setup = await render(); await client.start()
    await until(() => client.context === context, "restored original owner"); await flush()
    client.emit("item-actions", { kind: "message", context, index, message }); await flush()
    expect(client.issue).toContain("changed or is unavailable"); client.issue = ""; client.emit("change")
    await open()
    expect(block("Supplied excerpt")).toBe(supplied); expect(block("Saved state")).toContain("no longer saved"); expect(calls).toHaveLength(before)
  } finally { release?.(); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 65000)
