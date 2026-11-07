import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { copyFile, mkdir, mkdtemp, readFile, rm, symlink } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

type Request = { messages: { role: string; content: string }[]; tools?: { function: { name: string } }[] }
async function fixture(installed = false) {
  const root = await mkdtemp(join(tmpdir(), "gator-browser-ui-")), calls: Request[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as Request; calls.push(payload)
    const last = payload.messages.at(-1)
    const message = last?.role === "user" && last.content.includes("Inspect my approved browser")
      ? { role: "assistant", tool_calls: [{ id: "selected-tabs", type: "function", function: { name: "browser_tabs", arguments: "{}" } }] }
      : { role: "assistant", content: "Browser selection checked." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  const config = join(root, "config/gator"), state = join(root, "state")
  await mkdir(config, { recursive: true })
  await Bun.write(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  if (installed) {
    const runtime = join(state, "gator/browser/runtime/playwright-1.56.1"), source = process.env.GATOR_BROWSER_TEST_RUNTIME!
    await mkdir(runtime, { recursive: true })
    for (const file of ["package.json", "package-lock.json", "driver.mjs"]) await copyFile(join(source, file), join(runtime, file))
    for (const directory of ["node_modules", "browsers"]) await symlink(join(source, directory), join(runtime, directory), process.platform === "win32" ? "junction" : "dir")
  }
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: state, GATOR_CONFIG_DIR: join(root, "config") }
  const client = new GatorClient(new Application({ binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 44 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => {
    const end = Date.now() + 20000
    while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() }
  }
  const choose = async (label: string) => {
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === label), label); await flush()
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); setup.mockInput.pressEnter(); await flush()
    await until(() => client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label === label, "selected " + label)
    setup.mockInput.pressEnter(); await flush()
  }
  const confirm = async () => { await until(() => client.state?.panel?.kind === "confirmation", "review"); setup.mockInput.pressEnter(); await flush() }
  const sessions = async () => JSON.parse(await readFile(join(state, "gator/browser/sessions.json"), "utf8")).sessions as { id: string; state: string; paused: boolean; visual_capture: boolean }[]
  const close = async () => {
    await client.close(); await setup.dispose()
    for (const session of await sessions().catch(() => [])) if (session.state === "running") {
      const stop = Bun.spawn([binary, "browser", "stop", session.id], { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, stdout: "ignore", stderr: "ignore" }); await stop.exited
    }
    server.stop(true); await rm(root, { recursive: true, force: true })
  }
  return { root, client, setup, choose, confirm, until, flush, close, sessions, calls, origin: `http://127.0.0.1:${server.port}` }
}

test("browser setup is discoverable, reviewed and cancellable without downloading", async () => {
  const f = await fixture()
  try {
    await f.client.start(); await f.client.edit("Keep my browser task draft")
    await f.client.navigate("/browser"); await f.choose("Install browser")
    expect(f.client.state?.panel?.kind).toBe("confirmation")
    expect(f.setup.captureCharFrame()).toContain("Download")
    expect(f.client.state?.work_details?.browser_session).toBeUndefined()
    f.setup.mockInput.pressEscape(); await f.flush()
    await f.until(() => f.client.state?.panel?.title === "Browser", "cancelled install review")
    expect(f.client.state?.panel?.title).toBe("Browser")
    expect(f.client.draft.text).toBe("Keep my browser task draft")
    expect(await f.sessions().catch(() => [])).toEqual([])
    expect(f.calls).toHaveLength(0)
  } finally { await f.close() }
}, 30000)

const native = process.env.GATOR_BROWSER_TEST_RUNTIME ? test : test.skip
native("managed browser opens paused, grants one task, supports takeover and stops through terminal controls", async () => {
  const f = await fixture(true)
  try {
    await f.client.start(); await f.client.navigate("/browser")
    await f.choose("Open private browser"); await f.confirm()
    await f.until(() => !!f.client.state?.panel?.rows?.some(row => row.label === "Resume agent access"), "private window")
    const initial = (await f.sessions())[0]
    expect(initial.paused).toBe(true); expect(initial.visual_capture).toBe(false)
    expect(f.client.state?.work_details?.browser_session).toBeUndefined()
    await f.choose("Add allowed site"); await f.choose("Site URL")
    await f.setup.mockInput.typeText(f.origin); f.setup.mockInput.pressEnter(); await f.flush()
    f.setup.mockInput.pressKey("F2"); await f.flush(); await f.confirm()
    await f.choose("Resume agent access"); await f.confirm()
    await f.choose("Use for next task"); await f.confirm()
    await f.until(() => f.client.state?.work_details?.browser_session === initial.id, "next-task selection")
    await f.client.navigate("/chat"); await f.client.send({ type: "submit", text: "/mode inspect" })
    await f.client.edit("Inspect my approved browser"); f.setup.mockInput.pressEnter()
    await f.until(() => f.calls.length > 0 && !f.client.state?.running, "first task")
    expect(f.calls[0].tools?.map(tool => tool.function.name)).toContain("browser_tabs")
    const observed = f.calls.flatMap(call => call.messages).filter(message => message.role === "tool")
    expect(observed.some(message => message.content.includes("tab-1") && message.content.includes("about:blank"))).toBe(true)
    expect(f.client.state?.work_details?.browser_session).toBeUndefined()
    const before = f.calls.length
    await f.client.edit("Now a separate task"); f.setup.mockInput.pressEnter()
    await f.until(() => f.calls.length > before && !f.client.state?.running, "second task")
    expect(f.calls[before].tools?.map(tool => tool.function.name)).not.toContain("browser_tabs")
    await f.client.navigate("/browser"); await f.choose(initial.id); await f.choose("Pause for private takeover")
    await f.until(() => !!f.client.state?.panel?.rows?.some(row => row.label === "Resume agent access"), "takeover paused")
    expect((await f.sessions())[0].paused).toBe(true)
    await f.choose("Stop browser"); await f.confirm()
    await f.until(() => !!f.client.state?.panel?.rows?.some(row => row.label === "Open private browser"), "stopped")
    expect((await f.sessions())[0].state).toBe("stopped")
  } finally { await f.close() }
}, 120000)
