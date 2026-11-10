import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { frontendDefaults } from "@gator/application-host/preferences"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`conversational and manual terminal settings share persisted outcomes at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-terminal-settings-")), config = join(root, "config/gator/config.json"), legacyPath = join(root, "app/frontend-preferences.json")
    const calls: any[] = [], changes: any[] = []
    const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
      const body = await request.json(); calls.push(body)
      if (body.messages.some((message: any) => message.role === "system" && message.content?.includes("You handle the conversation with Gator"))) {
        const context = JSON.parse(body.messages.at(-1).content)
        return Response.json({ choices: [{ message: { role: "assistant", content: JSON.stringify({ kind: "task", instruction: context.message }) }, finish_reason: "stop" }] })
      }
      const question = body.messages.findLast((message: any) => message.role === "user")?.content || "", last = body.messages.at(-1)
      let tool: any
      if (body.tools?.some((tool: any) => tool.function?.name === "gator_app_inspect")) {
        if (last.role !== "tool") tool = { id: "inspect-" + calls.length, name: "gator_app_inspect", arguments: JSON.stringify({ topic: "terminal" }) }
        else if (last.tool_call_id?.startsWith("inspect-") && question.includes("Hide navigation")) {
          const value = { theme: "terminal", sidebarCollapsed: true, groupChatsByFolder: false }; changes.push(value)
          tool = { id: "update-" + calls.length, name: "gator_app_update", arguments: JSON.stringify({ setting: "terminal", value }) }
        }
      }
      let content = "The terminal settings task finished."
      if (last.role === "tool") {
        const result = JSON.parse(last.content), values = result.result || result.preferences
        content = values ? `Terminal settings: theme=${values.theme}; sidebarCollapsed=${values.sidebarCollapsed}.` : `The setting needs attention: ${last.content}`
      }
      const message = tool ? { role: "assistant", tool_calls: [{ id: tool.id, type: "function", function: { name: tool.name, arguments: tool.arguments } }] } : { role: "assistant", content }
      return Response.json({ choices: [{ message, finish_reason: tool ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } })
    } })
    await mkdir(join(root, "config/gator"), { recursive: true }); await mkdir(join(root, "app"))
    await writeFile(config, JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
    const legacy = { ...frontendDefaults(), theme: "contrast" as const, sidebarCollapsed: false, groupChatsByFolder: true, workDetailsSections: { ...frontendDefaults().workDetailsSections, context: true } }
    await writeFile(legacyPath, JSON.stringify({ version: 1, legacyImport: "absent", preferences: legacy }))
    const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
    let client = new GatorClient(new Application(options))
    const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height })
    let setup = await render()
    const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
    const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame() + "\n" + JSON.stringify(client.state?.dialogue)); await flush() } }
    const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
    const settings = async () => JSON.parse(await readFile(config, "utf8"))
    try {
      await client.start(); await flush()
      expect(client.preferences).toEqual(legacy); expect(client.state?.terminal_preferences?.initialized).toBe(true)
      const original = client.state!.conversation_id!, originalOwner = (client.host as any).backend, importedBytes = await readFile(legacyPath, "utf8")
      expect((await settings()).theme).toBe("contrast"); expect((await settings()).tui.preferences.groupChatsByFolder).toBe(true)
      await client.edit("Keep this unrelated draft"); await client.navigate("/appearance"); await flush(); await choose("Light")
      await until(() => client.preferences.theme === "light", "manual saved appearance")
      expect((await settings()).theme).toBe("light"); expect(client.draft.text).toBe("Keep this unrelated draft")
      await client.edit("What terminal settings am I using?"); await client.submit()
      await until(() => !client.snapshot.allChatsBusy && !!client.state?.messages.some(message => message.text.includes("theme=light")), "read actual manual preference conversationally")
      await client.edit("Hide navigation and follow my terminal colors"); await client.submit()
      await until(() => !client.snapshot.allChatsBusy && client.preferences.theme === "terminal" && client.preferences.sidebarCollapsed, "conversational terminal change")
      expect(changes).toHaveLength(1); expect(client.state?.conversation_id).toBe(original)
      expect((await settings()).theme).toBe("terminal"); expect((await settings()).tui.preferences.sidebarCollapsed).toBe(true)
      expect((await settings()).tui.preferences.groupChatsByFolder).toBe(false)
      expect(client.preferences.workDetailsSections.context).toBe(true)
      expect(await readFile(legacyPath, "utf8")).toBe(importedBytes)
      // Cached owners share the new values; returning cannot write an old
      // chat's theme back over the acknowledged global selection.
      await client.navigate("/new"); await flush(); await client.configurePreferences({ theme: "mono" })
      await client.navigate("/conversations"); await until(() => !!client.state?.panel?.rows?.some(row => row.identity?.startsWith(original + ":")), "original conversation in canonical history")
      const index = client.state!.panel!.rows!.findIndex(row => row.identity?.startsWith(original + ":"))
      while ((client.state!.panel!.selected ?? 0) !== index) { setup.mockInput.pressArrow((client.state!.panel!.selected ?? 0) < index ? "down" : "up"); await flush() }
      setup.mockInput.pressEnter(); await until(() => client.state?.conversation_id === original && !client.state.panel, "cached owner restored")
      expect((client.host as any).backend).toBe(originalOwner); expect(client.preferences.theme).toBe("mono")
      expect(client.state?.terminal_preferences?.values.theme).toBe("mono")
      await client.edit("Keep my offline multiline draft\nSecond line")
      const persisted = await readFile(config, "utf8")
      await (client.host as any).backend.stop(); await until(() => client.snapshot.connection === "disconnected", "disconnected owner")
      setup.mockInput.pressKey("b", { ctrl: true }); await until(() => !!client.issue, "rejected offline preference change")
      expect(client.preferences.sidebarCollapsed).toBe(true); expect(await readFile(config, "utf8")).toBe(persisted)
      await client.reconnect(); await flush(); expect(client.draft.text).toBe("Keep my offline multiline draft\nSecond line")
      setup.mockInput.pressKey("b", { ctrl: true }); await until(() => !client.preferences.sidebarCollapsed, "reconnected preference save")
      for (const [w, h] of [[110, 34], [80, 24], [60, 20], [42, 14]]) { setup.resize(w, h); await flush(); const input = setup.renderer.root.findDescendantById("message-input")!; expect(input.screenY + input.height).toBeLessThanOrEqual(h) }
      await client.close(); await setup.dispose()
      // An old frontend file is now an inert import source, even if another
      // old installation edits it while this terminal is closed.
      await writeFile(legacyPath, JSON.stringify({ version: 1, preferences: frontendDefaults() }))
      client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await flush()
      expect(client.preferences.theme).toBe("mono"); expect(client.preferences.sidebarCollapsed).toBe(false)
      expect(client.preferences.workDetailsSections.context).toBe(true)
      expect(client.draft.text).toBe("Keep my offline multiline draft\nSecond line")
    } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
  }, 60000)
}

test("unreadable retained display settings recover through a cancelled and then confirmed default import", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-preference-recovery-")), legacyPath = join(root, "app/frontend-preferences.json")
  await mkdir(join(root, "app")); await writeFile(legacyPath, "{unreadable retained settings")
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  const client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 42, height: 14 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await flush(); expect(client.issue).toContain("/terminal-settings")
    expect(client.state?.terminal_preferences?.initialized).toBe(false)
    await client.edit("Keep the original draft"); await client.navigate("/terminal-settings"); await flush(); await choose("Use default display")
    expect(setup.captureCharFrame()).toContain("default display")
    setup.mockInput.pressEscape(); await flush(); expect(client.state?.terminal_preferences?.initialized).toBe(false)
    expect(await readFile(legacyPath, "utf8")).toBe("{unreadable retained settings")
    await client.navigate("/terminal-settings"); await flush(); await choose("Use default display"); setup.mockInput.pressEnter()
    const end = Date.now() + 10000
    while (!client.state?.terminal_preferences?.initialized) { if (Date.now() > end) throw new Error(client.issue + "\n" + setup.captureCharFrame()); await flush() }
    expect(client.preferences).toEqual(frontendDefaults()); expect(client.issue).toBe("")
    expect(client.draft.text).toBe("Keep the original draft")
    expect(await readFile(legacyPath, "utf8")).toBe("{unreadable retained settings")
    await client.reconnect(); expect(client.state?.terminal_preferences?.initialized).toBe(true)
    expect(client.preferences).toEqual(frontendDefaults()); expect(client.draft.text).toBe("Keep the original draft")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 30000)

test("manual terminal changes preserve an executing owner's identity, scope and newer draft", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-live-settings-")), config = join(root, "config/gator")
  let release!: () => void, calls = 0
  const gate = new Promise<void>(resolve => { release = resolve })
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch() { calls++; await gate; return Response.json({ choices: [{ message: { role: "assistant", content: "The original task finished with its original scope." }, finish_reason: "stop" }] }) } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 80, height: 24 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const until = async (predicate: () => boolean) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  try {
    await client.start(); await client.send({type:"submit",text:"/mode inspect"}); await client.edit("Inspect the current task"); await client.submit(); await until(() => calls > 0 && !!client.state?.running)
    const owner = client.state!.conversation_id, control = client.state!.guidance_id, scope = client.state!.work_details
    await client.edit("Keep this next question\nSecond line"); setup.mockInput.pressKey("b", { ctrl: true }); await until(() => !client.preferences.sidebarCollapsed)
    await client.navigate("/appearance"); await flush(); await setup.mockInput.typeText("Light"); setup.mockInput.pressEnter(); await until(() => client.preferences.theme === "light")
    expect(client.state?.running).toBe(true); expect(client.state?.conversation_id).toBe(owner); expect(client.state?.guidance_id).toBe(control)
    expect(client.state?.work_details).toEqual(scope); expect(client.draft.text).toBe("Keep this next question\nSecond line")
    release(); await until(() => !client.state?.running && !!client.state?.messages.some(message => message.text === "The original task finished with its original scope."))
    expect(client.draft.text).toBe("Keep this next question\nSecond line"); expect(client.preferences.theme).toBe("light")
  } finally { release(); await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 30000)
