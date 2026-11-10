import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`first task keeps its private draft through failed setup, repair and explicit execution at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-first-request-")), file = join(root, "meeting.txt"), config = join(root, "config/gator/config.json")
    await writeFile(file, "Meeting note: confirm the venue on Friday. Private original: silver otter.")
    const calls: string[] = []
    const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
      calls.push(await request.text())
      return Response.json({ choices: [{ message: { role: "assistant", content: "Confirm the venue on Friday." }, finish_reason: "stop" }] })
    } })
    const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), OPENAI_API_KEY: "" } }
    const client = new GatorClient(new Application(options))
    const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height })
    const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
    const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame() + "\n" + JSON.stringify(client.state?.panel)); await flush() } }
    const choose = async (label: string) => { await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
    const select = async (label: string) => {
      await until(() => !!client.state?.panel?.rows?.some(row => row.label === label), "available " + label)
      setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); setup.mockInput.pressEnter(); await flush()
      await until(() => client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label === label, "selected " + label)
    }
    const field = async (label: string, text: string, multiline = false) => {
      await select(label); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(text)
      if (multiline) {
        await flush()
        const input = setup.renderer.root.findDescendantById("multiline-field-input")!
        expect(input.screenY).toBeGreaterThanOrEqual(0); expect(input.screenY + input.height).toBeLessThanOrEqual(height)
        expect(setup.captureCharFrame()).toContain("Ctrl+S")
      }
      if (multiline) setup.mockInput.pressKey("s", { ctrl: true }); else setup.mockInput.pressEnter()
      await flush()
    }
    const saved = async () => JSON.parse(await readFile(config, "utf8").catch(() => "{}"))
    try {
      await client.start(); await flush()
      expect(client.state?.composer?.readiness?.state).toBe("needs_model")
      expect(setup.captureCharFrame()).toContain("Host connected")
      expect(setup.captureCharFrame()).toContain("Model setup needed")
      await client.attach([file]); await flush(); await setup.mockInput.typeText("Summarize this meeting note"); await flush()
      const context = client.context, upload = client.draft.uploads![0]!.id
      setup.mockInput.pressEnter(); await flush()
      expect(setup.captureCharFrame()).toContain("Set up a model")
      expect(calls).toHaveLength(0); expect(client.draft.text).toBe("Summarize this meeting note")
      await choose("Connect or repair")
      await select("Cloud API key"); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Cloud models", "provider setup")
      await flush(); setup.mockInput.pressKey("n"); await until(() => client.state?.panel?.title === "Custom provider", "new provider")
      await field("Provider ID", "first-request-fixture")
      await field("Chat Completions URL", "file:///invalid-model-endpoint")
      await field("Model IDs", "fixture", true); await field("Default model", "fixture")
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => !!client.state?.panel?.notice, "invalid setup explains failure")
      expect(client.state?.panel?.title).toBe("Custom provider")
      expect((await saved()).custom_providers ?? []).toHaveLength(0)
      expect(client.draft.text).toBe("Summarize this meeting note"); expect(client.draft.uploads?.[0]?.id).toBe(upload)
      expect(calls).toHaveLength(0)
      await field("Chat Completions URL", `http://127.0.0.1:${server.port}/v1/chat/completions`)
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.panel?.title === "Save custom provider?", "review repaired configuration")
      await flush(); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Cloud models", "saved repaired configuration")
      const label = client.state!.panel!.rows!.find(row => row.label.includes("first-request-fixture") && row.label.includes("fixture"))!.label
      await select(label); setup.mockInput.pressEnter(); await flush(); await client.navigate("/chat"); await flush()
      expect(client.state?.composer?.readiness?.state).toBe("configured")
      expect(client.state?.composer?.readiness?.processing.kind).toBe("local_endpoint")
      expect(client.state?.composer?.budget).toEqual({ work_steps: 24, code_steps: 16 })
      expect(client.context).toBe(context); expect(client.draft.uploads?.[0]?.id).toBe(upload); expect(calls).toHaveLength(0)
      setup.mockInput.pressKey("t", { ctrl: true }); await flush(); await choose("Request context and limits")
      let inspected = ""
      for (let page = 0; page < 60; page++) {
        const before = setup.captureCharFrame(); inspected += "\n" + before
        await setup.mockInput.pressKeys(["\u001b[6~"]); await flush()
        if (setup.captureCharFrame() === before) break
      }
      inspected = inspected.replace(/[│█▀▄]/g, " ").replace(/\s+/g, " ")
      expect(inspected).toContain("Work limit: 24 steps")
      expect(inspected).toContain("Connected accounts selected: None")
      expect(inspected).toContain("may forward them elsewhere")
      expect(inspected).toContain("not token or monetary spending caps")
      setup.mockInput.pressEscape(); await flush()
      await writeFile(file, "Changed source after private import")
      setup.mockInput.pressEnter(); await until(() => !client.state?.running && !!client.state?.messages.some(message => message.text === "Confirm the venue on Friday."), "first useful result")
      expect(calls.length).toBeGreaterThan(0); expect(calls[0]).toContain("silver otter"); expect(calls.join("\n")).not.toContain("Changed source")
      const journal = JSON.parse(await readFile(join(root, "state/gator/conversations", client.state!.conversation_id!, "requests.json"), "utf8"))
      expect(journal.requests).toHaveLength(1); expect(journal.requests[0].text).toBe("Summarize this meeting note")
      expect(client.draft.text).toBe(""); expect(client.draft.uploads).toEqual([])
      expect(client.state?.work_details?.connector_ids).toEqual([])
      expect(setup.renderer.root.findDescendantById("message-input")!.screenY).toBeLessThan(height)
    } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
  }, 60000)
}

test("missing and changed model credentials recover without losing or replaying the first request", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-first-credential-")), config = join(root, "config/gator")
  let calls = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch() { calls++; return Response.json({ choices: [{ message: { role: "assistant", content: "A useful first answer." }, finish_reason: "stop" }] }) } })
  await mkdir(config, { recursive: true })
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "locked", model: "fixture" }, custom_providers: [
    { id: "locked", base_url: `http://127.0.0.1:${server.port}/chat`, api_key_env: "GATOR_FIRST_TASK_MISSING_KEY", models: ["fixture"], default_model: "fixture" },
    { id: "available", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" },
  ] }))
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_FIRST_TASK_MISSING_KEY: "" } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 80, height: 24 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const until = async (predicate: () => boolean) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  try {
    await client.start(); await flush()
    expect(client.state?.provider).toBe("locked"); expect(client.state?.composer?.readiness?.state).toBe("needs_setup")
    await setup.mockInput.typeText("Help with my first task"); setup.mockInput.pressEnter(); await flush()
    await choose("Why setup is needed")
    expect(setup.captureCharFrame().replace(/\s+/g, " ")).toContain("GATOR_FIRST_TASK_MISSING_KEY")
    setup.mockInput.pressEscape(); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(calls).toBe(0); expect(client.draft.text).toBe("Help with my first task")
    await choose("Choose an already configured model"); await choose("Choose model"); await choose("fixture")
    await until(() => client.state?.provider === "available")
    expect(client.state?.composer?.readiness?.state).toBe("configured"); expect(calls).toBe(0)
    expect(client.draft.text).toBe("Help with my first task")
    // Change prerequisites after the UI read them. Go must reject admission,
    // retain the draft and publish the real issue instead of trusting the UI.
    const path = join(config, "config.json"), original = JSON.parse(await readFile(path, "utf8"))
    const changed = structuredClone(original)
    changed.custom_providers.find((provider: { id: string }) => provider.id === "available").api_key_env = "GATOR_FIRST_TASK_MISSING_KEY"
    await writeFile(path, JSON.stringify(changed))
    setup.mockInput.pressEnter(); await until(() => !!client.issue && !client.pending.size)
    expect(client.state?.composer?.readiness?.state).toBe("needs_setup")
    expect(calls).toBe(0); expect(client.draft.text).toBe("Help with my first task")
    await writeFile(path, JSON.stringify(original))
    setup.mockInput.pressKey("r", { ctrl: true }); await flush(); await choose("Repair model setup"); await choose("Connect or repair")
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === "Cloud API key")); await flush(); setup.mockInput.pressEnter()
    await until(() => client.state?.panel?.title === "Cloud models"); await flush()
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await setup.mockInput.typeText("available"); setup.mockInput.pressEnter(); await flush()
    setup.mockInput.pressEnter(); await flush(); await client.navigate("/chat"); await flush()
    expect(client.state?.composer?.readiness?.state).toBe("configured"); expect(calls).toBe(0)
    setup.mockInput.pressEnter(); await until(() => !client.state?.running && !!client.state?.messages.some(message => message.text === "A useful first answer."))
    expect(calls).toBeGreaterThan(0); expect(client.draft.text).toBe("")
    const journal = JSON.parse(await readFile(join(root, "state/gator/conversations", client.state!.conversation_id!, "requests.json"), "utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].text).toBe("Help with my first task")
  } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 30000)
