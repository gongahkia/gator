import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("model and effort keyboard choices preserve drafts and respect configured identity and active work", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-models-")), config = join(root, "config/gator/config.json")
  let release = () => {}; const gate = new Promise<void>(resolve => { release = resolve }), calls: { model: string }[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) { const body = await request.json() as { model: string }; calls.push(body); await gate; return Response.json({ choices: [{ message: { role: "assistant", content: "The selected model replied." }, finish_reason: "stop" }] }) } })
  await mkdir(join(root, "config/gator"), { recursive: true })
  await writeFile(config, JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "one" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["one", "two"], default_model: "one" }, { id: "locked", base_url: "http://127.0.0.1:9/chat", api_key_env: "GATOR_MISSING_MODEL_FIXTURE_KEY", models: ["unavailable"], default_model: "unavailable" }], model_aliases: { "fixture:two": "Careful model" } }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_MISSING_MODEL_FIXTURE_KEY: "" } }
  const client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 36 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue); await flush() } }
  const menu = async () => { setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await setup.mockInput.typeText("models"); await flush(); setup.mockInput.pressEnter(); await until(() => setup.captureCharFrame().includes("Model and effort for future messages"), "model menu") }
  const choose = async (label: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.edit("Keep the model selection draft."); const context = client.context
    await menu(); await choose("Effort: high"); await until(() => client.state?.composer?.effort === "high", "high effort")
    await menu(); await choose("Effort: standard"); await until(() => client.state?.composer?.effort === "standard", "reset effort")
    await menu(); await choose("Careful model"); await until(() => client.state?.model === "two", "configured model identity")
    expect(JSON.parse(await readFile(config, "utf8")).defaults.model).toBe("two")
    expect(client.draft.text).toBe("Keep the model selection draft."); expect(client.context).toBe(context); expect(calls).toHaveLength(0)
    expect(client.state?.composer?.models.find(model => model.provider === "locked")?.unavailable).toBeTruthy()
    await menu(); await choose("unavailable"); expect(client.state?.model).toBe("two")
    setup.mockInput.pressEscape(); await Bun.sleep(80); await flush()
    await client.send({ type: "submit", text: "/mode inspect" }); await client.submit(); await until(() => calls.length === 1 && !!client.state?.running, "active model request")
    expect(calls[0].model).toBe("two"); await client.edit("Independent unsent message")
    await menu(); expect(setup.captureCharFrame()).toContain("Available after")
    await choose("Effort: high"); expect(client.state?.composer?.effort).toBe("standard"); expect(client.draft.text).toBe("Independent unsent message")
    release(); await until(() => !client.state?.running, "completed work")
    // The open picker must become usable when Go releases the work restriction.
    await choose("Effort: high"); await until(() => client.state?.composer?.effort === "high", "live released model controls")
    expect(client.draft.text).toBe("Independent unsent message"); expect(calls).toHaveLength(1)
  } finally { release(); await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)

test("first task retains its attachment and draft through model setup and restart until explicit send", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-first-task-")), config = join(root, "config/gator"), path = join(root, "meeting.txt")
  let calls = 0, received = ""
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) { received = await request.text(); calls++; return Response.json({ choices: [{ message: { role: "assistant", content: "Confirm the venue Friday." }, finish_reason: "stop" }] }) } })
  await mkdir(config, { recursive: true }); await writeFile(path, "Meeting note: confirm the venue on Friday.")
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: {}, custom_providers: [{ id: "starter", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 100, height: 32 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + JSON.stringify(client.state?.panel)); await flush() } }
  try {
    await client.start(); await client.edit("Help me understand this file."); await client.attach([path]); await client.submit()
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === "Cloud API key"), "first setup")
    expect(calls).toBe(0); expect(client.draft.uploads).toHaveLength(1); expect(client.draft.text).toBe("Help me understand this file.")
    await flush(); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Cloud models", "cloud models"); await flush()
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await setup.mockInput.typeText("starter"); setup.mockInput.pressEnter(); await flush()
    await until(() => !!client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label.includes("starter"), "original model row")
    setup.mockInput.pressEnter(); await flush(); await client.navigate("/chat"); await until(() => client.state?.provider === "starter", "first model")
    expect(calls).toBe(0); const upload = client.draft.uploads![0].id
    await writeFile(path, "Changed original after private import.")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start()
    expect(client.draft.text).toBe("Help me understand this file."); expect(client.draft.uploads?.[0].id).toBe(upload); expect(calls).toBe(0)
    await client.send({ type: "submit", text: "/mode inspect" }); await flush(); setup.mockInput.pressEnter()
    await until(() => calls === 1 && !client.state?.running && !!client.state?.messages.some(message => message.text === "Confirm the venue Friday."), "explicit file task")
    expect(received).toContain("confirm the venue on Friday"); expect(received).not.toContain("Changed original")
    expect(client.draft.text).toBe(""); expect(client.draft.uploads ?? []).toHaveLength(0)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
