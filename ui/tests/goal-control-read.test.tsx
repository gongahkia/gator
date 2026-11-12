import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { join } from "node:path"
import { tmpdir } from "node:os"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`closing goal inspection retires late success/failure without stopping work at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-goal-read-"))
    const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
    const goal = { id: "goal-" + "a".repeat(24), title: "Venue planning", status: "paused", outcome: "Keep the venue outcome", controllable: true, digest: "b".repeat(64), source: root }
    host.current = { ...state(), running: true, following_goals: [goal] }
    const client = new GatorClient(host), setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height })
    const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
    let release = () => {}
    try {
      await client.start(); await client.edit("Keep the independent draft")
      client.issue = "Unrelated status kept"; client.emit("change")
      for (const failure of [false, true]) {
        const gate = new Promise<void>(resolve => { release = resolve })
        host.dispatch = async frame => {
          if (frame.type !== "goal") throw new Error("Goal inspection must not send another operation")
          const request = frame.goal!
          host.change({ ...host.current, goal_busy: true })
          await gate
          host.change({ ...host.current, goal_busy: false, goal_result: { request_id: request.request_id, ...(failure ? { error: "Delayed goal read failure" } : { review: { id: goal.id, digest: goal.digest, title: goal.title, status: goal.status, enabled_checks: 0, lines: ["Late retained goal details"] } }) } })
        }
        await client.navigate("/following"); await flush(); setup.mockInput.pressEnter(); await flush()
        await setup.mockInput.typeText("Inspect goal and retained plan"); await flush(); setup.mockInput.pressEnter(); await flush()
        expect(setup.renderer.root.findDescendantById("item-read-view")).toBeDefined()
        expect(host.current.goal_busy).toBe(true)
        if (failure) setup.mockInput.pressKey("c", { ctrl: true }); else setup.mockInput.pressEscape()
        await flush(); await client.navigate("/settings"); await flush()
        release(); for (let n = 0; n < 4; n++) await flush()
        expect(client.route).toBe("settings")
        expect(setup.renderer.root.findDescendantById("item-read-view")).toBeUndefined()
        expect(setup.captureCharFrame()).not.toContain("Late retained goal details")
        expect(setup.captureCharFrame()).not.toContain("Delayed goal read failure")
        expect(client.issue).toBe("Unrelated status kept")
        expect(client.draft.text).toBe("Keep the independent draft")
        expect(client.state?.running).toBe(true)
        expect(host.sent.every(frame => frame.type === "goal" && frame.goal?.action === "inspect")).toBe(true)
      }
      expect(host.sent).toHaveLength(2)
    } finally { release(); await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
  })
}
