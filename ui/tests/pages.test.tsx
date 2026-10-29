import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm, realpath } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean | Promise<boolean>, label: string) { const deadline = Date.now() + 15000; while (!await predicate()) { if (Date.now() > deadline) throw new Error(`Timed out: ${label}`); await Bun.sleep(15) } }
test("keyboard pages preserve the draft and recurring checks review exact typed sources, budgets and timing before save", async () => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "gator-terminal-pages-"))), picked = join(root, "chosen source")
  await mkdir(picked); await writeFile(join(picked, "project.md"), "A saved source, never an implicit workspace switch.")
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "pages-fixture", "--base-url", "http://127.0.0.1:9/v1/chat/completions", "--model", "fixture"]); cli(["config", "set", "default-provider", "pages-fixture"])
  const client = new GatorClient(new Application({ binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const select = async (label: string) => {
    const target = client.state?.panel?.rows?.findIndex(row => row.label === label) ?? -1
    expect(target).toBeGreaterThanOrEqual(0)
    for (let n = 0; (client.state?.panel?.selected ?? 0) !== target && n < 20; n++) { const previous = client.state?.panel?.selected ?? 0; setup.mockInput.pressTab(); await until(() => (client.state?.panel?.selected ?? 0) !== previous, "next field"); await flush() }
    expect(client.state?.panel?.selected ?? 0).toBe(target); setup.mockInput.pressEnter(); await flush()
  }
  const prompt = async (text: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(text); setup.mockInput.pressEnter(); await flush() }
  const definitions = async () => Promise.all((await readdir(join(env.GATOR_STATE_DIR, "gator/jobs/definitions")).catch(() => [])).filter(name => name.endsWith(".json")).map(async name => JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/jobs/definitions", name), "utf8"))))
  try {
    await client.start(); const original = client.state!.source, originalID = client.state!.conversation_id!, originalContext = client.context; await client.edit("Keep my unsent page draft")
    for (const route of ["/following", "/saved", "/settings"]) { await client.navigate(route); await flush(); expect(setup.captureCharFrame()).toContain(route.slice(1)[0].toUpperCase() + route.slice(2)); expect(client.draft.text).toBe("Keep my unsent page draft") }
    for (const route of ["/knowledge", "/memory", "/usage", "/data", "/recovery", "/connections", "/model", "/appearance"]) {
      await client.navigate(route); await until(() => !!client.state?.panel && !client.state.panel.title.startsWith("Loading"), route); await flush()
      expect(client.draft.text).toBe("Keep my unsent page draft"); expect(client.state?.running).toBe(false)
    }
    await client.navigate("/followup"); await flush(); setup.mockInput.pressKey("n"); await until(() => client.state?.panel?.kind === "followup-form", "new check form"); await flush()
    await select("Title"); await prompt("Watch the offsite plan")
    await select("Desired outcome"); await setup.mockInput.typeText("Review venue changes and report useful differences."); setup.mockInput.pressKey("s", { ctrl: true }); await flush()
    await select("Local workspace"); setup.mockInput.pressKey("l", { ctrl: true }); await flush(); await prompt(picked)
    await setup.mockInput.typeText("Choose this folder"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(client.state?.source).toBe(original)
    await select("Check frequency"); await setup.mockInput.typeText("Every 30 minutes"); await flush(); setup.mockInput.pressEnter(); await flush()
    await select("What Gator may do"); await setup.mockInput.typeText("Read and recommend"); await flush(); setup.mockInput.pressEnter(); await flush()
    await select("Limits for each attempt"); await setup.mockInput.typeText("Provider requests"); await flush(); setup.mockInput.pressEnter(); await flush(); await prompt("16")
    await select("Checks per UTC day"); await prompt("6")
    await select("End date (optional)"); await prompt("2030-10-25T18:30:00Z")
    expect(await definitions()).toHaveLength(0)
    setup.mockInput.pressKey("F2"); await until(() => client.state?.panel?.title === "Review ongoing check", "exact agreement review"); await flush()
    const review = JSON.stringify(client.state?.panel)
    for (const text of ["Watch the offsite plan", picked, "every 30 minutes", "6 Work checks/day", "16 requests", "Read and recommend"]) expect(review).toContain(JSON.stringify(text).slice(1, -1))
    expect(await definitions()).toHaveLength(0)
    setup.mockInput.pressEnter(); await until(async () => (await definitions()).length === 1, "explicit save")
    const saved = (await definitions())[0]; expect(saved.source_path).toBe(picked); expect(saved.max_checks_per_day).toBe(6)
    expect(saved.origin_conversation_id).toBe(originalID)
    expect(client.drafts.get(originalContext)?.text).toBe("Keep my unsent page draft")
    // Saving intentionally selects the newly created goal. Return to the origin
    // through the same session host to verify its workspace/draft still exist.
    await until(() => client.state?.source === picked && !client.snapshot.sessionSwitching, "saved check workspace selected")
    await client.host.sessions.open({ conversation_id: originalID })
    await until(() => client.state?.conversation_id === originalID && client.state?.source === original, "original chat restored"); await client.loadDraft()
    expect(client.state?.source).toBe(original)
    expect(client.draft.text).toBe("Keep my unsent page draft"); expect(client.state?.messages.some(message => message.role === "You")).toBe(false)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 60000)
