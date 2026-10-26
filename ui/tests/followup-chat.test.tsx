import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

type Message = { role: string; content: string; tool_call_id?: string }
test("chat creates edits and pauses an origin-linked recurring check only after exact live reviews", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-followup-chat-")), config = join(root, "config/gator"), expires = new Date(Date.now() + 48 * 3600000).toISOString()
  let requests = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as { messages: Message[] }; requests++; const last = payload.messages.at(-1)!, user = payload.messages.findLast(message => message.role === "user")?.content ?? ""
    const tool = (name: string, args: unknown) => ({ role: "assistant", tool_calls: [{ id: name, type: "function", function: { name, arguments: JSON.stringify(args) } }] })
    let message
    if (last.role !== "tool") message = tool("gator_app_inspect", { topic: "followups" })
    else if (last.tool_call_id === "gator_app_inspect") {
      const current = JSON.parse(last.content).checks?.[0]
      const value = user.includes("Only") ? { action: "edit", id: current.id, expected_digest: current.expected_digest, alert_when: "Only a price changes" } : user.includes("Pause") ? { action: "pause", id: current.id, expected_digest: current.expected_digest } : { action: "create", title: "Venue replies", outcome: "Track venue availability", alert_when: "A price or availability changes", interval_minutes: 15, ends_at: expires, mode: "inspect" }
      message = tool("gator_app_update", { setting: "followup", value })
    } else message = { role: "assistant", content: "The reviewed result is available. The local background engine is offline." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 12000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const directory = join(root, "state/gator/jobs/definitions")
  const saved = async () => Promise.all((await readdir(directory).catch(() => [])).filter(name => name.endsWith(".json")).map(async name => JSON.parse(await readFile(join(directory, name), "utf8"))))
  const ask = async (text: string) => { await client.edit(text); await flush(); setup.mockInput.pressEnter(); await until(() => !!client.state?.pending_approval, "live agreement review"); await flush() }
  const approve = async () => { setup.mockInput.pressKey("y", { ctrl: true }); await flush(); expect(setup.captureCharFrame()).toContain("Approve this exact action?"); setup.mockInput.pressEnter(); await until(() => !client.state?.running, "approved completion"); await flush() }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); const origin = client.state!.conversation_id!
    await ask("Watch venue replies until the agreed end date.")
    const preview = JSON.stringify(client.state?.messages)
    expect(preview).toContain("Venue replies"); expect(preview).toContain("Offline"); expect(preview).toContain("Ends"); expect(await saved()).toHaveLength(0)
    setup.mockInput.pressKey("d", { ctrl: true }); await until(() => !client.state?.running, "declined agreement"); expect(await saved()).toHaveLength(0)
    await ask("Watch venue replies until the agreed end date."); await approve()
    const original = (await saved())[0]; expect(original.origin_conversation_id).toBe(origin); expect(new Date(original.ends_at).getTime()).toBe(new Date(expires).getTime()); expect(original.mode).toBe("inspect")
    await ask("Only bother me if the price changes."); expect((await saved())[0].alert_when).toBe("A price or availability changes"); await approve()
    expect((await saved())[0].alert_when).toBe("Only a price changes"); expect((await saved())[0].origin_conversation_id).toBe(origin)
    await ask("Pause that follow-up."); expect((await saved())[0].enabled).toBe(true); await approve()
    expect(await saved()).toHaveLength(1); expect((await saved())[0].enabled).toBe(false)
    await client.edit("Keep the recurring-check draft"); await client.navigate("/followup"); await flush()
    expect(client.state?.panel?.rows?.find(row => row.label === "Venue replies")?.state).toBe("paused")
    const count = requests; await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await client.navigate("/followup"); await flush()
    expect(client.state?.panel?.rows?.find(row => row.label === "Venue replies")?.state).toBe("paused"); expect(client.draft.text).toBe("Keep the recurring-check draft"); expect(requests).toBe(count)
    expect(client.state?.conversation_id).toBe(origin)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
