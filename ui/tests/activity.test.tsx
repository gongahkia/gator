import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("real public tool activity folds before the answer, remains searchable when open, and survives restart without replay", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-activity-")), workspace = join(root, "workspace"), config = join(root, "config/gator")
  await mkdir(workspace); await mkdir(config, { recursive: true }); await writeFile(join(workspace, "note.md"), "Decision: use sage Tide. Internal fixture body must stay out of public activity.")
  let calls = 0, release = () => {}; const gate = new Promise<void>(resolve => { release = resolve })
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as { messages: { role: string }[] }; calls++
    const ready = payload.messages.at(-1)?.role === "tool"; if (ready) await gate
    const message = ready ? { role: "assistant", content: "Sage Tide is selected." } : { role: "assistant", content: "I will check your notes first.", tool_calls: [{ id: "read-note", type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/note.md" }) } }] }
    return Response.json({ choices: [{ message, finish_reason: ready ? "stop" : "tool_calls" }], usage: { total_tokens: 20 } })
  } })
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace, env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 12000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const activity = async (live = false) => { setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await choose("activity"); await choose(live ? "Inspect live" : "Worked") }
  const search = async () => { setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await choose("check your notes") }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Read the note and tell me the decision."); await client.submit()
    await until(() => calls === 2 && !!client.state?.activity?.entries.some(entry => entry.kind === "tool"), "public tool completion")
    await client.edit("Keep the next question"); await flush()
    expect(setup.captureCharFrame()).toContain("I will check your notes first.")
    expect(JSON.stringify(client.state?.activity)).not.toContain("Internal fixture body"); expect(JSON.stringify(client.state?.activity)).not.toContain("source/note.md")
    await activity(true); expect(setup.captureCharFrame()).toContain("read_file"); setup.mockInput.pressEscape(); await flush()
    release(); await until(() => !client.state?.running, "completed answer"); await flush()
    expect(setup.captureCharFrame()).toContain("Worked for"); expect(setup.captureCharFrame()).not.toContain("I will check your notes first.")
    expect(setup.captureCharFrame()).toContain("Sage Tide is selected.")
    await activity(); await choose("Expand in transcript")
    expect(setup.captureCharFrame()).toContain("I will check your notes first.")
    expect(setup.captureCharFrame().indexOf("I will check your notes first.")).toBeLessThan(setup.captureCharFrame().indexOf("Sage Tide is selected."))
    await search(); expect(setup.captureCharFrame()).toContain("Sage Tide is selected."); setup.mockInput.pressEscape(); await flush()
    await activity(); await choose("Collapse in transcript"); await search()
    expect(setup.captureCharFrame()).toContain("No items available"); setup.mockInput.pressEscape(); await flush()
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await flush()
    expect(calls).toBe(2); expect(client.draft.text).toBe("Keep the next question"); expect(setup.captureCharFrame()).not.toContain("I will check your notes first.")
    await activity(); await choose("Inspect all public entries"); expect(setup.captureCharFrame()).toContain("I will check your notes first."); expect(setup.captureCharFrame()).toContain("read_file"); expect(calls).toBe(2)
    expect(client.state?.messages.find(message => message.activity)?.activity?.entries.map(entry => entry.kind)).toEqual(["text", "tool"])
  } finally { release(); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
