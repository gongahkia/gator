import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

for (const [width, height, queueBinding, kittyKeyboard] of [[110, 34, "Tab", false], [80, 24, "Tab", false], [60, 20, "Tab", false], [42, 14, "Tab", false], [42, 14, "Ctrl+K", false], [42, 14, "Ctrl+Shift+K", true]] as const) {
  test(`queue preserves exact draft context, completion and modal navigation at ${width}×${height} with ${queueBinding}${kittyKeyboard ? " (Kitty protocol)" : ""}`, async () => {
    const custom = queueBinding !== "Tab"
    const root = await mkdtemp(join(tmpdir(), "gator-queue-key-"))
    const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
    host.current = { ...host.current, running: true, dialogue_active: true, desktop: { shortcuts: { queue_message: queueBinding } } }
    const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height, kittyKeyboard })
    const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
    try {
      await client.start(); await flush()
      await client.edit("/mo"); await flush(); setup.mockInput.pressTab(); await flush()
      expect(client.draft.text).toBe("/models "); expect(host.sent).toHaveLength(0)
      const input = "a".repeat(32) + "/brief.txt"
      await client.edit("Queue the next step", [{ id: input, name: "brief.txt", size: 8 }], "update-original")
      client.planFirst.add(client.context); client.emit("change"); await flush()
      expect(setup.captureCharFrame()).toContain(queueBinding + " queue")
      await client.navigate("/draft-context"); await flush(); setup.mockInput.pressTab(); await flush()
      expect(host.sent).toHaveLength(0); expect(client.draft.text).toBe("Queue the next step")
      setup.mockInput.pressEscape(); await flush()
      if (custom) { setup.mockInput.pressTab(); await flush(); expect(host.sent).toHaveLength(0); await client.edit("Queue the next step"); await flush(); setup.mockInput.pressKey("k", { ctrl: true, shift: kittyKeyboard }) }
      else setup.mockInput.pressTab()
      await flush()
      if (host.sent.length !== 1) throw new Error(JSON.stringify({ width, height, custom, issue: client.issue, draft: client.draft, focus: setup.renderer.currentFocusedEditor?.id }) + "\n" + setup.captureCharFrame())
      expect(host.sent).toHaveLength(1)
      expect(host.sent[0]).toMatchObject({ type: "upload_submit", upload: { context: client.context, text: "Queue the next step", files: [input], reply_to_update_id: "update-original", plan_first: true, queue_only: true } })
      expect(client.draft.text).toBe(""); expect(client.draft.uploads).toEqual([])
      host.change({ ...host.current, running: false }); await client.edit("Idle input stays editable"); await flush()
      setup.mockInput.pressTab(); await flush(); expect(host.sent).toHaveLength(1)
      expect(client.draft.text).toContain("Idle input stays editable")
    } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
  })
}
