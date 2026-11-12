import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { readingItems, unreadStart } from "../src/gator/transcript-reading"
import { chatTimeline } from "../src/gator/timeline"
import { FixtureHost, state } from "./fixture"

test("reading identities follow original items across inserted history and delivery receipts are not new content", () => {
  const messages = [{ role: "Gator", text: "Retained answer", created_at: "2026-11-12T01:00:00Z" }]
  const before = readingItems(chatTimeline(messages, [])), tail = before.at(-1)!
  const inserted = readingItems(chatTimeline([{ role: "You", text: "Earlier", created_at: "2026-11-12T00:00:00Z" }, ...messages], []))
  expect(inserted.at(-1)?.key).toBe(tail.key)
  expect(unreadStart(inserted, { seen: tail.key, seen_version: tail.version })).toBe(2)
  const changed = readingItems(chatTimeline([{ ...messages[0], text: "Retained answer continued" }], []))
  expect(changed[0].key).toBe(tail.key)
  expect(unreadStart(changed, { seen: tail.key, seen_version: tail.version })).toBe(0)
  const running = readingItems(chatTimeline([{ ...messages[0], run_id: "owned-run", revision_id: "in-flight" }], []))[0]
  const finished = readingItems(chatTimeline([{ ...messages[0], run_id: "owned-run", revision_id: "retained-revision" }], []))[0]
  expect(finished.key).toBe(running.key)
  const update = { can_rate: true, rating_version: 0, id: "update-one", title: "A result", summary: "Exact summary", status: "completed", created_at: "2026-11-12T00:00:00Z", delivered: false, read: false, resolved: false, version: 1 }
  const original = readingItems(chatTimeline([], [update]))[0], receipt = readingItems(chatTimeline([], [{ ...update, delivered: true, read: true, resolved: true, version: 4 }]))[0]
  expect(original.key).toBe(receipt.key); expect(original.version).toBe(receipt.version)
})

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`reading anchor → new content → restart → catch up → configured latest at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-reading-journey-"))
    const options = { binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }
    let host = new FixtureHost(options)
    host.current = { ...state(), desktop: { shortcuts: { jump_latest: "Ctrl+Shift+L" } }, messages: Array.from({ length: 80 }, (_, index) => ({ role: "Gator", text: `Retained answer ${index}\n` + Array.from({ length: 8 }, (_, line) => `Line ${line}: a stable readable explanation.`).join("\n"), created_at: new Date(1700000000000 + index * 60000).toISOString() })) }
    let client = new GatorClient(host)
    const mount = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height, kittyKeyboard: true })
    let setup = await mount()
    const flush = async () => { await setup.flush(); await Bun.sleep(35); await setup.flush() }
    const settle = async () => { for (let i = 0; i < 4; i++) await flush() }
    const scroll = () => setup.renderer.root.findDescendantById("transcript") as ScrollBoxRenderable
    try {
      await client.start(); await settle()
      await client.edit("Keep my\nmultiline draft", [], undefined, true)
      for (let n = 0; n < 4; n++) { setup.mockInput.pressKey("\x1b[5~"); await flush() }
      await settle()
      let saved = client.readingAnchors.get("conversation:first")!
      expect(saved.following).toBe(false); expect(saved.index).toBeLessThan(79)
      expect(setup.captureCharFrame()).toContain("Ctrl+Shift+L latest")
      const text = "First unseen answer\n" + Array.from({ length: 12 }, (_, n) => `New detail ${n}`).join("\n")
      host.change({ ...host.current, messages: [...host.current.messages.map((message, index) => index ? message : { ...message, text: message.text + "\nInserted detail above the current reader.\nMore detail." }), { role: "Gator", text, created_at: "2026-11-12T00:00:00Z" }, { role: "Gator", text: "Newest answer kept intact", created_at: "2026-11-12T00:01:00Z" }] })
      await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(client.readingAnchors.get("conversation:first")?.offset).toBe(saved.offset)
      expect(setup.captureCharFrame()).toContain("2 new")
      // Loading earlier retained history changes every backend index. Keep the
      // original reading item instead of applying its old absolute row offset.
      host.change({ ...host.current, messages: [{ role: "You", text: "Previously unloaded history", created_at: "2020-01-01T00:00:00Z" }, ...host.current.messages] })
      await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(client.readingAnchors.get("conversation:first")?.offset).toBe(saved.offset)
      setup.resize(width === 42 ? 80 : 42, height); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      setup.resize(width, height); await settle()
      saved = client.readingAnchors.get("conversation:first")!
      const original = structuredClone(host.current)
      host.change({ ...state("second"), messages: [{ role: "Gator", text: "Different owner and history", created_at: "2026-11-12T01:00:00Z" }] }); await settle()
      host.change(original); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(client.readingAnchors.get("conversation:first")?.offset).toBe(saved.offset)
      expect(setup.captureCharFrame()).toContain("2 new")
      const position = scroll().scrollTop
      await client.navigate("/saved"); await flush(); await client.navigate("/chat"); await settle()
      expect(scroll().scrollTop).toBe(position)
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      const retainedState = structuredClone(host.current)
      await client.close(); await setup.dispose()
      host = new FixtureHost(options); host.current = retainedState; client = new GatorClient(host); setup = await mount()
      await client.start(); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(client.readingAnchors.get("conversation:first")?.offset).toBe(saved.offset)
      expect(setup.captureCharFrame()).toContain("2 new")
      expect(client.draft.text).toBe("Keep my\nmultiline draft"); expect(client.draft.plan_first).toBe(true)
      await client.navigate("/catch-up"); await settle()
      expect(setup.captureCharFrame()).toContain("First unseen answer")
      setup.mockInput.pressKey("l", { ctrl: true, shift: true }); await settle()
      expect(setup.captureCharFrame()).toContain("Newest answer kept intact")
      expect(setup.captureCharFrame()).not.toContain("2 new")
      expect(client.readingAnchors.get("conversation:first")?.following).toBe(true)
      expect(client.draft.text).toBe("Keep my\nmultiline draft"); expect(client.draft.plan_first).toBe(true)
      // Revisions of one streamed answer remain one new item, and growing
      // content below the reader must not pull them away from the old answer.
      for (let n = 0; n < 2; n++) { setup.mockInput.pressKey("\x1b[5~"); await flush() }
      const reading = client.readingAnchors.get("conversation:first")!
      expect(reading.following).toBe(false)
      for (const fragment of ["First streamed fragment", "Second streamed fragment"]) {
        host.change({ ...host.current, messages: host.current.messages.map((message, index, all) => index === all.length - 1 ? { ...message, text: message.text + "\n" + fragment } : message) }); await settle()
        expect(setup.captureCharFrame()).toContain("1 new")
        expect(client.readingAnchors.get("conversation:first")?.key).toBe(reading.key)
        expect(client.readingAnchors.get("conversation:first")?.offset).toBe(reading.offset)
      }
      expect(host.sent).toHaveLength(0)
    } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
  }, 15000)
}
