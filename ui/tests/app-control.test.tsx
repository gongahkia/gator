import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

type Message = { role: string; content?: string; tool_call_id?: string }
test("chat app controls preserve reviews for limits retention and deletion and persist future model and connection choices", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-app-control-")), config = join(root, "config/gator"), calls: { model: string; messages: Message[] }[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as { model: string; messages: Message[] }; calls.push(payload)
    const question = payload.messages.findLast(message => message.role === "user")?.content ?? "", last = payload.messages.at(-1)
    let setting = "", value: unknown, topic = "usage"
    if (question.includes("rebind")) { topic = "shortcuts"; setting = "shortcuts"; value = { action: "command_search", shortcut: "Mod+P" } }
    else if (question.includes("model-two")) { topic = "models"; setting = "model"; value = { provider: "fixture", model: "model-two" } }
    else if (question.includes("daily requests")) { setting = "usage_limits"; value = { daily_requests: question.includes("2000") ? 2000 : 1000 } }
    else if (question.includes("history for 30")) { topic = "data"; setting = "retention"; value = { history_days: 30 } }
    else if (question.includes("Reports")) { topic = "connections"; setting = "connections"; value = { ids: ["reports"] } }
    else if (question.includes("delete saved knowledge")) { topic = "data"; setting = "data_action"; value = { action: "delete", scope: "knowledge" } }
    const tool = last?.role !== "tool" ? { name: "gator_app_inspect", arguments: JSON.stringify({ topic }), id: "inspect-" + calls.length } : last.tool_call_id?.startsWith("inspect-") && setting ? { name: "gator_app_update", arguments: JSON.stringify({ setting, value }), id: "update-" + calls.length } : undefined
    const message = tool ? { role: "assistant", tool_calls: [{ id: tool.id, type: "function", function: { name: tool.name, arguments: tool.arguments } }] } : { role: "assistant", content: "The app result is ready for review." }
    return Response.json({ choices: [{ message, finish_reason: tool ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 } })
  } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "model-one" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["model-one", "model-two"], default_model: "model-one" }], connectors: [{ version: 1, id: "reports", name: "Reports", kind: "http_json", resource: "https://example.com/reports.json", authentication: "none" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_UI_FRONTEND: "terminal", GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + " " + JSON.stringify(client.state?.pending_approval)); await flush() } }
  const ask = async (text: string, approval?: boolean) => {
    await client.edit(text); await flush(); setup.mockInput.pressEnter()
    if (approval !== undefined) {
      await until(() => !!client.state?.pending_approval, "app change review"); await flush()
      if (approval) { setup.mockInput.pressKey("y", { ctrl: true }); await flush(); expect(setup.captureCharFrame()).toContain("Approve this exact action?"); setup.mockInput.pressEnter() }
      else setup.mockInput.pressKey("d", { ctrl: true })
    }
    await until(() => !client.state?.running && !!client.state?.messages.some(message => message.role === "You" && message.text === text), "completed app request")
    expect(client.state?.messages.filter(message => message.result?.title === "Work failed")).toHaveLength(0)
  }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" })
    await ask("Please rebind command search to Mod+P"); await until(() => client.state?.desktop?.shortcuts.command_search === "Mod+P", "saved shortcut")
    setup.mockInput.pressKey("p", { ctrl: true }); await flush(); expect(setup.captureCharFrame()).toContain("Gator commands"); setup.mockInput.pressEscape(); await Bun.sleep(80); await flush()
    await ask("Please switch to model-two"); await until(() => client.state?.model === "model-two", "future model")
    await ask("What is my usage?"); expect(calls.at(-1)?.model).toBe("model-two")
    await ask("Limit me to 1000 daily requests", true)
    const policy = join(root, "state/gator/usage/policy.json"); expect(JSON.parse(await readFile(policy, "utf8")).daily_requests).toBe(1000)
    await ask("Limit me to 2000 daily requests", false); expect(JSON.parse(await readFile(policy, "utf8")).daily_requests).toBe(1000)
    await ask("Keep completed history for 30 days", true); expect(JSON.parse(await readFile(join(root, "state/gator/data-control/retention.json"), "utf8")).HistoryDays).toBe(30)
    await ask("Select Reports for future messages"); await until(() => client.state?.work_details?.connector_ids.join(",") === "reports", "future connection")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start()
    expect(client.state?.desktop?.shortcuts.command_search).toBe("Mod+P"); expect(client.state?.work_details?.connector_ids).toEqual(["reports"])
    const knowledge = join(root, "state/gator/knowledge"); await mkdir(knowledge, { recursive: true }); const kept = join(knowledge, "kept.md"); await writeFile(kept, "Keep until final review.")
    await client.send({ type: "submit", text: "/mode inspect" }); await ask("Help me delete saved knowledge")
    await until(() => client.state?.panel?.title === "Review private data removal", "canonical deletion review")
    expect(await readFile(kept, "utf8")).toBe("Keep until final review.")
    await flush(); setup.mockInput.pressEscape(); await Bun.sleep(80); await flush(); expect(await readFile(kept, "utf8")).toBe("Keep until final review.")
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 90000)
