import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, cp, readFile, readdir, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { ClassicLevel } from "../../app-host/node_modules/classic-level"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("frontend preferences import into Bun, control navigation and grouping, and persist without changing Chromium data", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-preferences-")), userData = join(root, "app"), source = join(userData, "Local Storage/leveldb")
  if (process.env.GATOR_MIGRATION_PREFERENCES_FIXTURE) await cp(process.env.GATOR_MIGRATION_PREFERENCES_FIXTURE, userData, { recursive: true })
  else {
    const database = new ClassicLevel<Buffer, Buffer>(source, { keyEncoding: "buffer", valueEncoding: "buffer" })
    const put = (key: string, value: unknown) => database.put(Buffer.from("_gator://app\0\x01" + key, "latin1"), Buffer.from("\x01" + JSON.stringify(value), "latin1"))
    await put("gator:sidebar-collapsed", true); await put("gator:group-topics-by-folder", true)
    await put("gator:work-details-sections", { outputs: false, context: true, connections: false, workspace: false }); await database.close()
  }
  const digest = async () => Object.fromEntries(await Promise.all((await readdir(source)).map(async name => [name, createHash("sha256").update(await readFile(join(source, name))).digest("hex")])))
  const original = await digest(), calls: string[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) { calls.push(await request.text()); return Response.json({ choices: [{ message: { role: "assistant", content: "Retained first answer." }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }) } })
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "preference-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture"]); cli(["config", "set", "default-provider", "preference-fixture"])
  const options = { binary, env, userData, workspace: join(root, "workspace") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const until = async (predicate: () => boolean) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error("Preference action did not settle: " + client.issue); await Bun.sleep(20) } }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await flush()
    expect(client.issue).toBe(""); expect(client.preferences.sidebarCollapsed).toBe(true); expect(client.preferences.groupChatsByFolder).toBe(true)
    expect(client.preferences.workDetailsSections.context).toBe(true); expect(client.preferences.workDetailsSections.outputs).toBe(false)
    expect(setup.captureCharFrame()).not.toContain("GATOR")
    setup.mockInput.pressKey("b", { ctrl: true }); await until(() => !client.preferences.sidebarCollapsed); await flush()
    expect(setup.captureCharFrame()).toContain("GATOR")
    await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Original preference chat"); await client.submit(); await until(() => !client.state?.running && calls.length === 1)
    const origin = client.state!.conversation_id!; await client.edit("Keep first draft"); await client.navigate("/new"); await client.edit("Keep second draft")
    await client.navigate("/chats"); await flush(); expect(setup.captureCharFrame()).toContain("Chats grouped by folder")
    await choose("Show flat recent chats"); expect(client.preferences.groupChatsByFolder).toBe(false)
    expect(setup.captureCharFrame()).toContain("Open and recent chats")
    await choose("Original preference chat"); await until(() => client.state?.conversation_id === origin)
    expect(client.draft.text).toBe("Keep first draft"); expect(calls).toHaveLength(1)
    const saved = await readFile(join(userData, "frontend-preferences.json"), "utf8")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start()
    expect(client.preferences.sidebarCollapsed).toBe(false); expect(client.preferences.groupChatsByFolder).toBe(false); expect(client.draft.text).toBe("Keep first draft")
    expect(await readFile(join(userData, "frontend-preferences.json"), "utf8")).toBe(saved)
    expect(await digest()).toEqual(original); expect(calls).toHaveLength(1)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
