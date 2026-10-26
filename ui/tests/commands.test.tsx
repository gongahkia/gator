import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("commands have nested menus and composer completion never sends on paste or Tab", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-command-menus-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current.messages = [{ role: "Gator", text: "Ready for menu navigation." }]
  host.current.commands = [
    { name: "mode", description: "Choose work permissions", needs_input: true, advanced: false },
    { name: "model", description: "Configure cloud or local models", needs_input: false, advanced: false },
    { name: "theme", description: "Choose a color theme", needs_input: true, advanced: false },
    { name: "status", description: "Inspect current work", needs_input: false, advanced: false },
  ]
  const client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 100, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  try {
    await client.start(); await client.edit("Keep this draft"); await flush()
    setup.mockInput.pressKey("p", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).toContain("Following"); expect(setup.captureCharFrame()).not.toContain("/mode")
    await setup.mockInput.typeText("Settings"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Gator › Settings"); expect(setup.captureCharFrame()).toContain("Choose a color theme")
    expect(setup.captureCharFrame()).not.toContain("/theme")
    setup.mockInput.pressEscape(); await flush(); expect(setup.captureCharFrame()).toContain("Open menu")
    setup.mockInput.pressEscape(); await flush(); expect(client.draft.text).toBe("Keep this draft")
    await client.edit(""); await flush(); await setup.mockInput.typeText("/mo"); await flush()
    expect(setup.captureCharFrame()).toContain("Tab complete"); expect(setup.captureCharFrame()).toContain("/mode")
    setup.mockInput.pressArrow("down"); await flush(); setup.mockInput.pressTab(); await flush()
    expect(client.draft.text).toBe("/model "); expect(host.sent).toHaveLength(0)
    expect(setup.captureCharFrame()).not.toContain("Tab complete")
    await client.edit("/mode"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(client.draft.text).toBe("/mode "); expect(host.sent).toHaveLength(0)
    await setup.mockInput.typeText("inspect"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(host.sent.at(-1)).toEqual({ type: "submit", text: "/mode inspect" })
    await setup.mockInput.pasteBracketedText("/status"); await flush(); expect(host.sent).toHaveLength(1)
    setup.mockInput.pressEscape(); await flush(); expect(client.draft.text).toBe("/status"); expect(setup.captureCharFrame()).not.toContain("Tab complete")
    await client.edit("/stat"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(host.sent.at(-1)).toEqual({ type: "submit", text: "/status " }); expect(client.draft.text).toBe("")
    await client.edit("/unknown"); await flush(); expect(setup.captureCharFrame()).not.toContain("Tab complete")
    await client.edit("ordinary /mo text"); await flush(); expect(setup.captureCharFrame()).not.toContain("Tab complete")
    await client.edit("/"); setup.resize(50, 18); await flush(); expect(setup.captureCharFrame()).toContain("Tab complete")
    setup.mockInput.pressArrow("up"); await flush(); expect(setup.captureCharFrame()).toContain("/voice")
    setup.mockInput.pressEscape(); await flush(); expect(client.draft.text).toBe("/"); expect(host.sent).toHaveLength(2)
    await client.edit("Keep menu argument draft"); setup.resize(100, 32); await flush()
    setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await setup.mockInput.typeText("Settings"); await flush(); setup.mockInput.pressEnter(); await flush()
    await setup.mockInput.typeText("Choose a color theme"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Arguments for /theme")
    await setup.mockInput.typeText("light"); setup.mockInput.pressEnter(); await flush()
    expect(host.sent.at(-1)).toEqual({ type: "submit", text: "/theme light" })
    expect(client.draft.text).toBe("Keep menu argument draft")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
