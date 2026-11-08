import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("combined draft context leaves input and transcript usable through narrow resizes", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-composer-budget-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
  try {
    await client.start()
    host.change({ ...host.current, running: true, messages: [{ role: "Gator", text: "Your itinerary stays readable while you compose." }] })
    client.planFirst.add(client.context)
    // Render-only file metadata: this test makes no claim about upload admission.
    client.drafts.set(client.context, { ...client.draft, text: "Keep the input visible\nSecond line", reply_to_update_id: "booking",
      uploads: Array.from({ length: 8 }, (_, index) => ({ id: `fixture-${index}`, name: `Reservation-confirmation-with-a-long-filename-${index}.pdf`, size: 1024, mime: "application/pdf" })) })
    client.emit("change")
    for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
      setup.resize(width!, height!); await flush()
      const input = setup.renderer.root.findDescendantById("message-input")!, transcript = setup.renderer.root.findDescendantById("transcript")!
      expect(input.screenY).toBeGreaterThanOrEqual(0)
      expect(input.screenY + input.height).toBeLessThanOrEqual(height!)
      expect(transcript.height).toBeGreaterThanOrEqual(4)
      expect(setup.captureCharFrame()).toContain("Keep the input visible")
      client.drafts.set(client.context, { ...client.draft, text: "/" }); client.emit("change"); await flush()
      const completing = setup.renderer.root.findDescendantById("message-input")!
      expect(completing.screenY + completing.height).toBeLessThanOrEqual(height!)
      expect(setup.renderer.root.findDescendantById("transcript")!.height).toBeGreaterThanOrEqual(4)
      expect(setup.captureCharFrame()).toContain("Tab complete")
      client.drafts.set(client.context, { ...client.draft, text: "Keep the input visible\nSecond line" }); client.emit("change")
    }
    setup.resize(110, 34); await flush(); setup.mockInput.pressKey("t", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).toContain("Draft details")
    expect(setup.captureCharFrame()).toContain("Attachments: 8")
    setup.mockInput.pressEscape(); await flush()
    expect(client.draft.uploads).toHaveLength(8); expect(client.draft.reply_to_update_id).toBe("booking")
    expect(host.sent).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("idle Ctrl+C preserves input; custom bindings drive help and composer hints", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-key-hints-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  let quits = 0
  const setup = await testRender(() => <App client={client} quit={async () => { quits++ }} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
  try {
    await client.start(); await client.edit("Unsent, keep this draft")
    host.change({ ...host.current, desktop: { shortcuts: { new_chat: "Ctrl+K", command_search: "Ctrl+L", send_message: "Ctrl+S", guide_work: "Ctrl+R", newline: "Ctrl+J" } } } as typeof host.current)
    await flush(); setup.mockInput.pressKey("c", { ctrl: true }); await flush()
    expect(quits).toBe(0); expect(client.draft.text).toBe("Unsent, keep this draft")
    expect(setup.captureCharFrame()).toContain("Ctrl+S send"); expect(setup.captureCharFrame()).toContain("Ctrl+L")
    await client.navigate("/help"); await flush()
    expect(setup.captureCharFrame()).toContain("Ctrl+K new independent chat")
    expect(setup.captureCharFrame()).toContain("Ctrl+R guides the live task")
    expect(setup.captureCharFrame()).not.toContain("Ctrl+N new independent chat")
    setup.mockInput.pressKey("c", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).not.toContain("Gator terminal help")
    expect(client.draft.text).toBe("Unsent, keep this draft"); expect(quits).toBe(0)
    setup.mockInput.pressKey("q", { ctrl: true }); await flush(); expect(quits).toBe(1)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
