import { test, expect } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"
import { App } from "../src/app"

async function until(predicate: () => boolean, label = "state", timeout = 15000) {
  const end = Date.now() + timeout
  while (!predicate()) { if (Date.now() > end) throw new Error(`Timed out waiting for ${label}`); await Bun.sleep(20) }
}
test("real Go chat admits pasted Unicode only on send, restores drafts and closes cleanly", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-terminal-app-"))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../../desktop/runtime/gator-runtime")
  const env = { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config") }
  let requests = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    await request.json(); requests++
    return Response.json({ id: "terminal-fixture", choices: [{ message: { role: "assistant", content: "Terminal answer 🐊" }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 } })
  } })
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "terminal-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture-model"])
  cli(["config", "set", "default-provider", "terminal-fixture"])
  const options = { binary, env, workspace: join(directory, "workspace"), userData: join(directory, "app") }
  const client = new GatorClient(new Application(options))
  let copy = ""
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: copy, mime: "text/plain" }), write: async text => { copy = text } }} />, { width: 110, height: 32 })
  try {
    await client.start(); await setup.flush()
    await setup.mockInput.pasteBracketedText("Literal /model 🐊\nDo not send on paste")
    await until(() => client.draft.text.includes("Do not send"), "pasted draft"); expect(requests).toBe(0)
    setup.mockInput.pressEnter()
    await until(() => !!client.state?.messages.some(m => m.text.includes("Terminal answer")), "reply: " + JSON.stringify(client.snapshot))
    await until(() => !client.state?.running, "completed work"); const completedRequests = requests
    expect(completedRequests).toBeGreaterThan(0); expect(client.state?.messages.filter(m => m.role === "You").length).toBe(1); expect(client.draft.text).toBe("")
    await setup.mockInput.typeText("Keep this unsent draft")
    await until(() => client.draft.text.includes("Keep this"), "typed draft")
    await client.host.drafts.flush()
    setup.mockInput.pressKey("p", { ctrl: true }); await setup.flush()
    expect(setup.captureCharFrame()).toContain("Gator commands")
    setup.mockInput.pressEscape(); await setup.flush()
    expect(client.draft.text).toContain("Keep this")
    setup.resize(50, 20); await setup.flush(); expect(setup.captureCharFrame()).toContain("Gator")
    await client.close(); await setup.dispose()
    const restarted = new GatorClient(new Application(options))
    try {
      await restarted.start(); await until(() => restarted.draft.text.includes("Keep this"), "restart draft")
      expect(requests).toBe(completedRequests); expect(restarted.state?.running).toBe(false)
    } finally { await restarted.close() }
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(directory, { recursive: true, force: true }) }
}, 30000)
