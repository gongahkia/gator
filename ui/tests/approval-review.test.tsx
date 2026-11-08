import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("approval reviews show exact content, scroll at all sizes and refuse changed or missing previews", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-exact-review-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
  const pending = { id: 7, token: "current-token", message_index: 0 }
  try {
    await client.start()
    host.change({ ...host.current, pending_approval: pending, messages: [{ role: "Approval", text: "", result: {
      kind: "approval", title: "Send venue update", fields: [{ label: "Account", value: "me@example.com" }],
      preview: { to: "venue@example.com", subject: "Reservation update", body: "Exact body to review\n" + "Long paragraph\n".repeat(60) + "FINAL REVIEW LINE" },
      footer: "Sends one email from the selected account. Approval does not confirm delivery.",
    } }] })
    setup.mockInput.pressKey("y", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).toContain("venue@example.com")
    expect(setup.captureCharFrame()).toContain("Exact body to review")
    for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
      setup.resize(width!, height!); await flush()
      const review = setup.renderer.root.findDescendantById("approval-review")!
      expect(review.screenY + review.height).toBeLessThanOrEqual(height!)
      expect(setup.captureCharFrame()).toContain("approve")
      setup.mockInput.pressKey("END", { ctrl: true }); await flush()
      expect(setup.captureCharFrame()).toContain("Sends one email")
      setup.mockInput.pressKey("HOME", { ctrl: true }); await flush()
    }
    setup.resize(110, 34); setup.mockInput.pressKey("r", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).toContain('"kind": "approval"')
    host.change({ ...host.current, pending_approval: { ...pending, token: "new-token" } }); await flush()
    expect(setup.captureCharFrame()).toContain("Request changed")
    setup.mockInput.pressEnter(); setup.mockInput.pressKey("d", { ctrl: true }); await flush()
    expect(host.sent).toHaveLength(0)
    setup.mockInput.pressEscape(); await flush()
    setup.mockInput.pressKey("y", { ctrl: true }); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(host.sent).toEqual([{ type: "approval", interaction_id: 7, approval_token: "new-token", approved: true }])
    host.sent.length = 0
    host.change({ ...host.current, pending_approval: { ...pending, message_index: -1 } }); await flush()
    setup.mockInput.pressKey("y", { ctrl: true }); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Approval is disabled")
    expect(host.sent).toHaveLength(0)
    setup.mockInput.pressKey("d", { ctrl: true }); await flush()
    expect(host.sent.at(-1)).toEqual({ type: "approval", interaction_id: 7, approval_token: pending.token, approved: false })
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
