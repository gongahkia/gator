import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { mascotFrame } from "../src/component/mascot"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("mascot follows actual work, offers still/hidden modes, and keeps narrow composers and drafts usable", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-mascot-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 100, height: 28 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  try {
    await client.start(); await client.edit("Keep this message"); await flush()
    expect(setup.renderer.root.findDescendantById("gator-mascot")).toBeDefined()
    expect(setup.captureCharFrame()).toContain(mascotFrame("idle", 0).split("\n")[1]!.trim())
    host.change({ ...host.current, running: true, status: "Working" }); await flush()
    if (!process.env.GATOR_REDUCED_MOTION) {
      await Bun.sleep(375); await setup.flush()
      expect(setup.captureCharFrame()).toContain(mascotFrame("working", 1).split("\n")[1]!.trim())
    }
    await client.configurePreferences({ mascot: "still" }); await flush(); await Bun.sleep(400); await setup.flush()
    expect(setup.captureCharFrame()).toContain(mascotFrame("working", 0).split("\n")[1]!.trim())
    host.change({ ...host.current, pending_approval: { id: 1, token: "live-token", message_index: 0 } }); await flush()
    expect(setup.captureCharFrame()).toContain(mascotFrame("attention", 0).split("\n")[1]!.trim())
    host.change({ ...host.current, running: false, pending_approval: undefined }); await client.navigate("/mascot"); await flush()
    await setup.mockInput.typeText("Hidden"); await flush(); setup.mockInput.pressEnter()
    const end = Date.now() + 3000
    while (client.preferences.mascot !== "hidden" && Date.now() < end) { await Bun.sleep(20); await setup.flush() }
    expect(client.preferences.mascot).toBe("hidden"); await flush()
    expect(setup.renderer.root.findDescendantById("gator-mascot")).toBeUndefined()
    expect((await host.preferences.read()).mascot).toBe("hidden")
    await client.configurePreferences({ mascot: "animated" }); setup.resize(48, 18); await flush()
    expect(setup.renderer.root.findDescendantById("gator-mascot")).toBeUndefined()
    expect(setup.captureCharFrame()).toContain("Keep this message"); expect(client.draft.text).toBe("Keep this message")
    expect(host.sent).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
