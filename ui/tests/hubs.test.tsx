import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { join } from "node:path"
import { tmpdir } from "node:os"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("compact hubs reveal selected destinations and reset selection when switching pages", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-hubs-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 50, height: 18 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  try {
    await client.start(); await client.edit("Keep compact-page draft"); await client.navigate("/settings"); await flush()
    expect(setup.captureCharFrame()).toContain("Models and providers"); expect(setup.captureCharFrame()).not.toContain("GATOR")
    for (let i = 0; i < 8; i++) { setup.mockInput.pressArrow("down"); await flush() }
    const scroll = setup.renderer.root.findDescendantById("hub-items") as ScrollBoxRenderable
    expect(scroll.scrollTop).toBeGreaterThan(0); expect(setup.captureCharFrame()).toContain("› About and updates")
    setup.resize(42, 14); await flush(); expect(setup.captureCharFrame()).toContain("› About and updates")
    setup.mockInput.pressEnter(); await flush(); expect(setup.captureCharFrame()).toContain("About Gator terminal"); setup.mockInput.pressEscape(); await flush()
    await client.navigate("/following"); await flush(); expect(setup.captureCharFrame()).toContain("› Ongoing goals")
    setup.mockInput.pressEnter(); await flush(); expect(host.sent.at(-1)).toEqual({ type: "navigate", text: "/goals" })
    await client.navigate("/saved"); await flush(); setup.resize(42, 14); await flush()
    for (let i = 0; i < 6; i++) { setup.mockInput.pressArrow("down"); await flush() }
    expect(setup.captureCharFrame()).toContain("› Connected request receipts")
    expect(client.draft.text).toBe("Keep compact-page draft"); expect(host.sent).toHaveLength(1)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
