import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, writeFile, readFile, rm, utimes } from "node:fs/promises"
import { dirname, join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean, label: string) { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) } }
test("restart leaves interrupted requests paused and keyboard resume uses the admitted immutable attachment", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-recovery-")), source = join(root, "brief.txt")
  await writeFile(source, "Retained reference: jade heron.")
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const calls: string[] = []; let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const index = calls.push(await request.text()); if (index === 1) await gate
    return Response.json({ id: "recovery-" + index, choices: [{ message: { role: "assistant", content: "The retained brief is available." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } })
  } })
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "recovery-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture"]); cli(["config", "set", "default-provider", "recovery-fixture"])
  const options = { binary, env, workspace: join(root, "workspace"), userData: join(root, "app") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 36 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Prepare report"); await client.submit(); await until(() => calls.length === 1, "first active request")
    await client.attach([source]); await client.edit("Read brief next"); await client.submit(); await client.edit("Summarize afterwards"); await client.submit()
    expect(client.state?.queue).toHaveLength(2)
    await client.edit("Keep the unsent recovery draft")
    const queueAction = async (message: string, action: string) => {
      await client.navigate("/queue"); await flush(); await setup.mockInput.typeText(message); await flush(); setup.mockInput.pressEnter(); await flush()
      await setup.mockInput.typeText(action); await flush(); setup.mockInput.pressEnter(); await flush()
    }
    await queueAction("Read brief next", "Edit message")
    setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.pasteBracketedText("Read the retained brief next"); await flush(); setup.mockInput.pressEnter()
    await until(() => client.state?.queue[0]?.text === "Read the retained brief next", "edited queued message")
    expect(client.state?.queue[0]?.inputs?.[0]).toBe("brief.txt")
    await queueAction("Read the retained brief next", "Move later")
    await until(() => client.state?.queue[0]?.text === "Summarize afterwards", "reordered queue")
    await queueAction("Read the retained brief next", "Move earlier")
    await until(() => client.state?.queue[0]?.text === "Read the retained brief next", "restored queue order")
    expect(calls).toHaveLength(1); expect(client.draft.text).toBe("Keep the unsent recovery draft")
    const conversation = client.state!.conversation_id!, journal = join(env.GATOR_STATE_DIR, "gator/conversations", conversation, "requests.json")
    const before = JSON.parse(await readFile(journal, "utf8")); expect(before.requests.map((item: { state: string }) => item.state)).toEqual(["running", "queued", "queued"])
    const upload = before.requests[1].options.Uploads[0]
    await client.edit("Keep the unsent recovery draft"); await client.close(); await setup.dispose()
    await rm(source); await utimes(dirname(join(env.GATOR_STATE_DIR, "gator/uploads", upload)), 0, 0)
    client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await flush()
    expect(client.state?.queue).toHaveLength(3); expect(client.state?.requests_recovered).toBe(true); expect(client.state?.running).toBe(false); expect(calls).toHaveLength(1)
    expect(client.draft.text).toBe("Keep the unsent recovery draft")
    await client.navigate("/queue"); await flush(); await setup.mockInput.typeText("Read the retained brief next"); await flush(); setup.mockInput.pressEnter(); await flush()
    await setup.mockInput.typeText("Run / resume"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Run this retained message?"); expect(calls).toHaveLength(1)
    setup.mockInput.pressEnter(); await until(() => calls.length === 2 && !client.state?.running, "explicit selected resume")
    expect(calls[1]).toContain("jade heron"); expect(calls[1]).toContain("Read the retained brief next"); expect(client.state?.queue).toHaveLength(2)
    expect(client.draft.text).toBe("Keep the unsent recovery draft")
    await client.navigate("/queue"); await flush(); await setup.mockInput.typeText("Prepare report"); await flush(); setup.mockInput.pressEnter(); await flush()
    await setup.mockInput.typeText("Remove"); await flush(); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressEnter()
    await until(() => client.state?.queue.length === 1, "remove interrupted request")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await flush()
    expect(client.state?.queue?.[0].text).toBe("Summarize afterwards"); expect(calls).toHaveLength(2); expect(client.state?.running).toBe(false)
  } finally { release(); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
