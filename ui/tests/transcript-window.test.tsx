import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable, TextRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`bounded history scrolls, reveals original actions, pins selection and restores at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-window-"))
    const options = { binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }
    let host = new FixtureHost(options)
    host.current = { ...state(), messages: Array.from({ length: 1000 }, (_, index) => ({ role: index % 2 ? "Gator" : "You", text: `Long history target ${index}.\nA readable explanation with café and 日本語.\nLine two remains selectable.`, created_at: new Date(1700000000000 + index * 60000).toISOString() })) }
    let disconnected = false
    const snapshot = host.snapshot.bind(host)
    host.snapshot = () => ({ ...snapshot(), connection: disconnected ? "disconnected" : "connected" })
    let client = new GatorClient(host)
    const mount = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height })
    let setup = await mount()
    const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
    const settle = async () => { for (let n = 0; n < 5; n++) await flush() }
    const scroll = () => setup.renderer.root.findDescendantById("transcript") as ScrollBoxRenderable
    const mounted = () => scroll().content.getChildren().filter(node => /^message-\d+$/.test(node.id))
    const search = async (text: string) => {
      setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await setup.mockInput.typeText(text); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressEnter(); await settle()
    }
    try {
      await client.start()
      // Native Markdown parsing uses an asynchronous worker. The harness's
      // synchronous frame-pass waiter does not wait for that worker's clock.
      const readableBy = performance.now() + 2000
      while (!setup.captureCharFrame().includes("Long history target 999.") && performance.now() < readableBy) await flush()
      await settle()
      await client.edit("Keep the unsent draft\nwith its next line", [], undefined, true); await flush()
      expect(setup.captureCharFrame()).toContain("Long history target 999.")
      expect(mounted().length).toBeLessThanOrEqual(80)
      setup.mockInput.pressKey("\x1b[1;5H"); await settle()
      expect(setup.captureCharFrame()).toContain("Long history target 0.")
      expect(mounted().length).toBeLessThanOrEqual(80)
      // Scrollbar jumps into unmeasured history remain usable too.
      scroll().scrollTo(Math.floor(scroll().scrollHeight / 3)); await settle()
      expect(client.readingAnchors.get("conversation:first")?.index).toBeGreaterThan(100)
      expect(client.readingAnchors.get("conversation:first")?.index).toBeLessThan(600)
      await search("Long history target 486.")
      expect(setup.captureCharFrame()).toContain("Long history target 486.")
      expect(mounted().length).toBeLessThanOrEqual(80)
      let original: { kind: string; index: number } | undefined
      const picked = (item: typeof original) => { original = item }
      client.on("item-actions", picked)
      setup.mockInput.pressKey("e", { ctrl: true }); await flush()
      expect(original?.kind).toBe("message"); expect(original?.index).toBe(486)
      setup.mockInput.pressEscape(); await flush(); client.off("item-actions", picked)
      const node = setup.renderer.root.findDescendantById("message-486")!
      const body = node.getChildren().find(child => child instanceof TextRenderable && child.plainText.includes("Long history target")) as TextRenderable
      expect(body).toBeDefined()
      setup.renderer.startSelection(body, body.screenX, body.screenY)
      setup.renderer.updateSelection(body, body.screenX + 8, body.screenY, { finishDragging: true }); await flush()
      const selected = setup.renderer.getSelection()?.getSelectedText()
      expect(selected?.length).toBeGreaterThan(0)
      scroll().scrollBy(-1600); await settle()
      expect(setup.renderer.root.findDescendantById("message-486")).toBe(node)
      expect(body.isDestroyed).toBe(false)
      expect(setup.renderer.getSelection()?.getSelectedText()).toBe(selected)
      setup.renderer.clearSelection(); await settle()
      expect(mounted().length).toBeLessThanOrEqual(80)
      expect(body.isDestroyed).toBe(true)
      await search("Long history target 486.")
      const saved = client.readingAnchors.get("conversation:first")!
      setup.resize(width === 42 ? 80 : 42, height); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      setup.resize(width, height); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      // An exact review can refer to an item outside the native content window.
      // This fixture checks rendering/navigation; real Go token enforcement is
      // covered by approvals.test.tsx, not simulated here.
      host.change({ ...host.current, running: true, pending_approval: { id: 17, token: "original-review", message_index: 17 }, messages: host.current.messages.map((message, index) => index === 17 ? { ...message, role: "Approval", result: { kind: "approval", title: "Send venue update", fields: [{ label: "Account", value: "me@example.com" }], preview: { to: "venue@example.com", body: "Exact reviewed message." }, footer: "Sends one email from the selected account." } } : message) }); await settle()
      expect(setup.renderer.root.findDescendantById("message-17")).toBeUndefined()
      setup.mockInput.pressKey("y", { ctrl: true }); await flush()
      const approval = setup.renderer.root.findDescendantById("approval-review")!
      expect(approval).toBeDefined(); expect(approval.screenY + approval.height).toBeLessThanOrEqual(height)
      setup.mockInput.pressKey("\x1b[1;5F"); await flush()
      expect(setup.captureCharFrame()).toContain("Sends one email")
      setup.mockInput.pressEscape(); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(client.draft.text).toBe("Keep the unsent draft\nwith its next line")
      host.change({ ...host.current, running: false, pending_approval: undefined }); await settle()
      disconnected = true; host.emit("change"); await settle()
      expect(setup.captureCharFrame()).toContain("Connection lost")
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      disconnected = false; host.emit("change"); await settle()
      // A paged snapshot can retain the reading item at a new backend index.
      host.change({ ...host.current, messages: host.current.messages.slice(100) }); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(mounted().length).toBeLessThanOrEqual(80)
      host.change({ ...host.current, messages: [...host.current.messages, { role: "Gator", text: "New answer beyond the rendered window.", created_at: "2026-11-12T00:00:00Z" }] }); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(setup.captureCharFrame()).toContain("1 new")
      const retained = structuredClone(host.current)
      await client.close(); await setup.dispose()
      host = new FixtureHost(options); host.current = retained; client = new GatorClient(host); setup = await mount()
      await client.start(); await settle()
      expect(client.readingAnchors.get("conversation:first")?.key).toBe(saved.key)
      expect(setup.captureCharFrame()).toContain("Long history target 486.")
      await client.navigate("/catch-up"); await settle()
      expect(setup.captureCharFrame()).toContain("New answer beyond")
      setup.mockInput.pressKey("l", { ctrl: true }); await settle()
      expect(client.readingAnchors.get("conversation:first")?.following).toBe(true)
      expect(client.draft.text).toBe("Keep the unsent draft\nwith its next line"); expect(client.draft.plan_first).toBe(true)
      expect(mounted().length).toBeLessThanOrEqual(80)
      expect(host.sent).toHaveLength(0)
    } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
  }, 20000)
}
