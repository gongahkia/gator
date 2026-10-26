import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, readFile, readdir, writeFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

type Request = { messages: { role: string; content: string }[] }
async function until(predicate: () => boolean | Promise<boolean>, label: string) { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) } }
test("personal memory keyboard corrections retain stale edits, preserve concurrent facts and survive restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-memory-")), quote = "I prefer concise answers with bullet points."
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }, calls: Request[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const input = await request.json() as Request; calls.push(input); const last = input.messages.at(-1)
    const message = last?.role !== "tool" && last?.content.includes(quote) ? { role: "assistant", content: null, tool_calls: [{ id: "remember-" + calls.length, type: "function", function: { name: "memory_remember", arguments: JSON.stringify({ subject: "preference.response_length", kind: "preference", quote, scope: "personal", explicit: true }) } }] } : { role: "assistant", content: "Your grounded preference is available." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } })
  } })
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "memory-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture"]); cli(["config", "set", "default-provider", "memory-fixture"])
  const options = { binary, env, workspace: join(root, "workspace"), userData: join(root, "app") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 38 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const memoryPath = join(env.GATOR_STATE_DIR, "gator/memory")
  const memories = async () => Promise.all((await readdir(memoryPath).catch(() => [])).filter(name => name.endsWith(".json")).map(async name => JSON.parse(await readFile(join(memoryPath, name), "utf8"))))
  const edit = async () => { await client.navigate("/memories"); await flush(); setup.mockInput.pressEnter(); await flush(); await setup.mockInput.typeText("Correct"); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Remember: " + quote); await client.submit()
    await until(() => !client.state?.running && !!client.state?.chat_memories?.length, "memory receipt")
    const original = (await memories())[0], path = join(memoryPath, original.id + ".json")
    expect(original.text).toBe(quote); expect(original.origin.conversation_id).toBe(client.state?.conversation_id)
    await edit(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("Keep my unsaved correction")
    const concurrent = "I prefer longer explanations for technical work."
    await writeFile(path, JSON.stringify({ ...original, text: concurrent, updated_at: new Date(Date.now() + 1000).toISOString() }), { mode: 0o600 })
    setup.mockInput.pressEnter(); await until(() => !!client.issue, "guarded conflict"); await flush()
    expect(setup.captureCharFrame()).toContain("Keep my unsaved correction"); expect((await memories())[0].text).toBe(concurrent)
    setup.mockInput.pressEscape(); await flush(); await until(() => !!client.state?.chat_memories?.some(memory => memory.text === concurrent), "updated receipt")
    await edit(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("I prefer detailed proposal reviews."); setup.mockInput.pressEnter()
    await until(async () => (await memories())[0]?.text === "I prefer detailed proposal reviews.", "saved correction")
    const count = calls.length; await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start()
    await until(() => !!client.state?.chat_memories?.some(memory => memory.text === "I prefer detailed proposal reviews."), "restart receipt")
    expect(calls).toHaveLength(count)
    await client.navigate("/memories"); await flush(); setup.mockInput.pressEnter(); await flush(); await setup.mockInput.typeText("Forget"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(await memories()).toHaveLength(1); setup.mockInput.pressEnter(); await until(async () => (await memories()).length === 0, "explicit forget")
    expect(calls).toHaveLength(count)
    await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Remember: " + quote); await client.submit()
    await until(() => !client.state?.running && !!client.state?.chat_memories?.some(memory => memory.can_undo), "new undoable memory")
    await client.navigate("/memories"); await flush(); setup.mockInput.pressEnter(); await flush(); await setup.mockInput.typeText("Undo original"); await flush(); setup.mockInput.pressEnter()
    await until(async () => (await memories()).length === 0, "explicit original save undo")
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
