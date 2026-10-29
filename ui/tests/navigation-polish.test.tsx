import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("rail removes repeated branding, supports pointer resizing and keeps chat space", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-rail-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const rail = () => setup.renderer.root.findDescendantById("navigation-rail")!
  try {
    await client.start(); await flush()
    expect(setup.captureCharFrame()).not.toContain("GATOR"); expect(setup.captureCharFrame()).not.toContain("⌕"); expect(setup.captureCharFrame()).toContain("Search")
    const handle = setup.renderer.root.findDescendantById("sidebar-resize")!
    await setup.mockMouse.drag(handle.screenX, handle.screenY + 4, 35, handle.screenY + 4); await flush()
    expect(rail().width).toBe(36)
    const wider = setup.renderer.root.findDescendantById("sidebar-resize")!
    await setup.mockMouse.drag(wider.screenX, wider.screenY + 4, 100, wider.screenY + 4); await flush(); expect(rail().width).toBe(48)
    setup.resize(90, 32); await flush(); expect(rail().width).toBe(46)
    setup.mockInput.pressArrow("left", { ctrl: true, shift: true }); await flush(); expect(rail().width).toBe(44)
    setup.resize(60, 24); await flush(); expect(setup.renderer.root.findDescendantById("navigation-rail")).toBeUndefined()
    setup.resize(110, 32); await flush(); expect(rail().width).toBe(44)
    setup.mockInput.pressKey("p", { ctrl: true }); await flush(); expect(setup.captureCharFrame()).toContain("CHATS")
    await setup.mockInput.typeText("catalog"); await flush(); expect(setup.captureCharFrame()).toContain("CATALOG ACTIONS")
    expect(setup.captureCharFrame()).not.toContain("catalog-actions")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("slash exit and quit use application shutdown, preserve cancelled reviews, and never reach Go", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-exit-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  let closed = 0, busy = false
  const snapshot = host.snapshot.bind(host); host.snapshot = () => ({ ...snapshot(), allChatsBusy: busy })
  const setup = await testRender(() => <App client={client} quit={async () => { closed++ }} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  try {
    await client.start(); await client.edit("/exit"); client.navigationGuard = async () => false
    await client.submit(); expect(closed).toBe(0); expect(client.draft.text).toBe("/exit")
    client.navigationGuard = undefined; busy = true; host.change(host.current)
    await client.submit(); await flush(); expect(setup.captureCharFrame()).toContain("Stop work and close Gator?")
    setup.mockInput.pressEscape(); await flush(); expect(closed).toBe(0); expect(client.draft.text).toBe("/exit")
    busy = false; host.change(host.current); await client.submit(); expect(closed).toBe(1); expect(client.draft.text).toBe("")
    await client.edit("/quit"); await client.submit(); expect(closed).toBe(2); expect(host.sent).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("resume arguments accept one chat ID and reject shell fragments or extra arguments", async () => {
  const { resumeID, resumeCommand } = await import("../src/gator/resume")
  expect(resumeID([])).toBeUndefined()
  expect(resumeID(["resume", "work-resume-fixture"])).toBe("work-resume-fixture")
  for (const args of [["resume"], ["resume", "../other"], ["resume", "work-id;echo"], ["resume", "work-one", "work-two"]]) expect(() => resumeID(args)).toThrow("Usage")
  expect(resumeCommand("work-one", "/Apps/Gator/bin/gator", "/bin/bun", true, false)).toBe("'/Apps/Gator/bin/gator' resume work-one")
  expect(resumeCommand("bad;id", "/bin/gator", "/bin/bun", true)).toBeUndefined()
})
