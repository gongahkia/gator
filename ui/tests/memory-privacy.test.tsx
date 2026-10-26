import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

type Request = { messages: { role: string; content: string }[]; tools?: { function: { name: string } }[] }
test("personal memory crosses chats while earlier-chat retrieval and learning remain independent through restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-memory-privacy-")), config = join(root, "config/gator"), quote = "I prefer concise answers with bullet points.", calls: Request[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as Request; calls.push(payload); const last = payload.messages.at(-1)
    const message = last?.role === "user" && last.content.includes(quote) ? { role: "assistant", tool_calls: [{ id: "remember-" + calls.length, type: "function", function: { name: "memory_remember", arguments: JSON.stringify({ subject: "preference.response_length", kind: "preference", quote, scope: "personal", explicit: true }) } }] } : { role: "assistant", content: "The supplied preference is available." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 40 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const ask = async (text: string) => { await client.navigate("/chat"); await client.send({ type: "submit", text: "/mode inspect" }); const before = calls.length; await client.edit(text); await flush(); setup.mockInput.pressEnter(); await until(() => calls.length > before && !client.state?.running, "completed request"); return calls[before] }
  const setting = (label: string) => client.state?.panel?.summary?.find(field => field.label === label)?.value
  const memoryPage = async () => { await client.navigate("/memory"); await until(() => !!setting("Learn from chats"), "memory privacy page"); await flush() }
  try {
    await client.start(); await ask("Remember: " + quote); await until(() => !!client.state?.chat_memories?.length, "grounded memory")
    const origin = client.context, id = client.state!.chat_memories![0].id, path = join(root, "state/gator/memory", id + ".json")
    expect(JSON.parse(await readFile(path, "utf8")).origin.conversation_id).toBe(origin.slice(13))
    await client.edit("Keep original privacy draft"); await memoryPage()
    const learning = setting("Learn from chats"), beforeHistory = setting("Refer to earlier chats")
    setup.mockInput.pressKey("h"); await until(() => setting("Refer to earlier chats") !== beforeHistory, "disabled earlier-chat retrieval"); expect(setting("Learn from chats")).toBe(learning)
    await client.navigate("/new"); await until(() => client.context !== origin && !client.snapshot.sessionSwitching, "fresh chat")
    expect(client.state?.messages).toHaveLength(0); expect(client.draft.text).toBe("")
    const request = await ask("What do you know about my response preferences?")
    expect(request.tools?.map(tool => tool.function.name)).not.toContain("chat_search"); expect(request.tools?.map(tool => tool.function.name)).not.toContain("chat_read")
    expect(request.tools?.map(tool => tool.function.name)).toContain("memory_remember")
    expect(request.messages.filter(message => message.role === "system").map(message => message.content).join("\n")).toContain(quote)
    expect(client.drafts.get(origin)?.text).toBe("Keep original privacy draft")
    await client.edit("Keep second privacy draft"); const current = client.context, count = calls.length
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await memoryPage()
    expect(client.context).toBe(current); expect(client.draft.text).toBe("Keep second privacy draft"); expect(calls).toHaveLength(count)
    expect(setting("Refer to earlier chats")).not.toBe(beforeHistory); expect(setting("Learn from chats")).toBe(learning)
    setup.mockInput.pressKey("h"); await until(() => setting("Refer to earlier chats") === beforeHistory, "restored earlier-chat retrieval"); await flush()
    setup.mockInput.pressKey("a"); await until(() => setting("Learn from chats") !== learning, "disabled automatic learning"); expect(setting("Refer to earlier chats")).toBe(beforeHistory)
    const restored = await ask("Find our earlier preferences discussion.")
    expect(restored.tools?.map(tool => tool.function.name)).toContain("chat_search"); expect(restored.tools?.map(tool => tool.function.name)).toContain("chat_read")
    expect(restored.messages.filter(message => message.role === "system").map(message => message.content).join("\n")).toContain(quote)
    expect(JSON.parse(await readFile(path, "utf8")).text).toBe(quote)
    await client.host.sessions.open({ conversation_id: origin.slice(13) }); await client.loadDraft()
    expect(client.draft.text).toBe("Keep original privacy draft"); expect(client.state?.messages.filter(message => message.role === "You")).toHaveLength(1)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
