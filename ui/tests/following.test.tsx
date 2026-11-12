import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { FollowingCheck } from "@gator/application-host/contracts/protocol"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

const check = (id: string, goal_id?: string): FollowingCheck => ({ id, goal_id, digest: "a".repeat(64), title: id === "job-venue" ? "Venue check" : "Daily briefing", outcome: "Keep useful venue changes together", state: "offline", health: "No successful check yet", enabled: true, editable: true, source: "/work/venue", connectors: ["mail-account"], timezone: "Asia/Ho_Chi_Minh", schedule: "*/15 * * * *", next_action: "Review local service", last_success: "2026-11-08T01:00:00Z" })

test("unscheduled goals retain visible workspace, paused controls and exact selection at all four sizes", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-unscheduled-goals-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  const active = { id: "goal-" + "a".repeat(24), digest: "a".repeat(64), title: "Same goal title", status: "active", source: "/work/alpha", controllable: true }
  const paused = { ...active, id: "goal-" + "b".repeat(24), digest: "b".repeat(64), status: "paused", source: "/work/beta" }
  host.current = { ...state(), following_goals: [active, paused], checks: [], supervisor_running: true }
  host.dispatch = async frame => {
    if (frame.type !== "goal") return
    expect(frame.goal?.action).toBe("inspect"); expect(frame.goal?.id).toBe(paused.id)
    host.change({ ...host.current, goal_result: { request_id: frame.goal!.request_id, review: { id: paused.id, digest: paused.digest, title: paused.title, status: paused.status, enabled_checks: 0, lines: ["Workspace: " + paused.source, "No recurring check is enabled."] } } })
  }
  const client = new GatorClient(host), setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const choose = async (label: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.edit("Keep the goal\nconversation draft", [], undefined, true); await client.navigate("/following"); await flush()
    const first = setup.captureCharFrame()
    expect(first).toContain("Workspace: /work/alpha"); expect(first).toContain("Workspace: /work/beta")
    expect(first).toContain("Inspect the retained plan or resume this goal")
    expect(first).not.toContain("Next check:")
    setup.mockInput.pressArrow("down"); await flush()
    for (const [width, height] of [[110,34], [80,24], [60,20], [42,14]]) {
      setup.resize(width, height); await flush()
      const scroll = setup.renderer.root.findDescendantById("following-items") as ScrollBoxRenderable
      expect(scroll.viewport.height).toBeGreaterThanOrEqual(5)
      expect(setup.captureCharFrame()).toContain("› Same goal title")
      setup.mockInput.pressEnter(); await flush(); await choose("Resume this goal")
      expect(setup.captureCharFrame()).toContain("Resume goal:")
      setup.mockInput.pressEscape(); await flush(); setup.mockInput.pressEscape(); await flush()
      expect(host.sent).toHaveLength(0)
    }
    // A refreshed ordering cannot retarget the same-title paused goal's controls.
    host.change({ ...host.current, following_goals: [paused, active] }); await flush()
    setup.mockInput.pressEnter(); await flush(); await choose("Inspect goal and retained plan")
    for (let attempt = 0; attempt < 20 && !setup.captureCharFrame().includes("/work/beta"); attempt++) await flush()
    expect(setup.captureCharFrame()).toContain("/work/beta"); expect(host.sent).toHaveLength(1)
    setup.mockInput.pressEscape(); await flush(); setup.mockInput.pressEscape(); await flush()
    setup.resize(110,34); host.change({ ...host.current, checks_issue: "Unavailable check health" }); await flush()
    expect(setup.captureCharFrame()).toContain("Refresh to verify check coverage")
    expect(client.draft.text).toBe("Keep the goal\nconversation draft"); expect(client.draft.plan_first).toBe(true)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("Following retains the selected commitment identity after refreshed goals reorder", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-following-selection-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  const goals = ["Alpha", "Beta", "Gamma"].map(title => ({ id: "goal-" + title, title, status: "paused" }))
  host.current = { ...state(), following_goals: goals }
  const client = new GatorClient(host), setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 80, height: 24 })
  const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
  try {
    await client.start(); await client.navigate("/following"); await flush()
    setup.mockInput.pressArrow("down"); await flush(); expect(setup.captureCharFrame()).toContain("› Beta")
    host.change({ ...host.current, following_goals: [goals[2]!, goals[0]!, goals[1]!] }); await flush()
    expect(setup.captureCharFrame()).toContain("› Beta")
    setup.mockInput.pressEnter(); await flush(); expect(setup.captureCharFrame()).toContain("Beta")
    expect(host.sent).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("Following groups checks with commitments, reveals controls at four sizes, and explains actual service limits", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-following-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  const venue = check("job-venue", "goal-venue"), briefing = check("job-briefing")
  host.current = { ...state(), supervisor_running: false, following_goals: [{ id: "goal-venue", title: "Venue planning", status: "active", outcome: "Confirm accessible venue" }, { id: "goal-unscheduled", title: "Learn Vietnamese", status: "active" }], checks: [venue, briefing] }
  host.dispatch = async frame => {
    if (frame.type !== "check") return
    const value = frame.check!
    host.change({ ...host.current, check_result: { request_id: value.request_id }, checks: host.current.checks?.map(item => value.action === "pause" && item.id === value.id ? { ...item, enabled: false, state: "paused", digest: "b".repeat(64) } : item) })
  }
  const client = new GatorClient(host), setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
  const choose = async (text: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.edit("Keep commitment draft"); await client.navigate("/following"); await flush()
    for (const text of ["Venue planning", "Confirm accessible venue", "Review local service", "Last success:", "mail-account", "Asia/Ho_Chi_Minh", "No recurring check attached", "Daily briefing", "Service offline"]) expect(setup.captureCharFrame()).toContain(text)
    expect(setup.captureCharFrame().match(/Venue planning/g)).toHaveLength(1)
    for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
      setup.resize(width!, height!); await flush()
      const scroll = setup.renderer.root.findDescendantById("following-items") as ScrollBoxRenderable
      expect(scroll.height).toBeGreaterThanOrEqual(5); expect(scroll.screenY + scroll.height).toBeLessThan(height!)
      setup.mockInput.pressEnter(); await flush()
      expect(setup.captureCharFrame()).toContain("Venue check")
      await choose("Pause this check"); expect(host.sent).toHaveLength(0)
      // The complete confirmation scrolls at narrow heights.
      expect(setup.captureCharFrame()).toContain("Pause Venue check?")
      setup.mockInput.pressEscape(); await flush()
    }
    setup.resize(110, 34); await flush(); setup.mockInput.pressEnter(); await flush(); await choose("Pause this check"); setup.mockInput.pressEnter(); await flush()
    expect(host.sent).toHaveLength(1); expect(host.sent[0]).toMatchObject({ type: "check", check: { action: "pause", id: venue.id, digest: venue.digest } })
    expect(host.current.checks?.[0].enabled).toBe(false); expect(setup.captureCharFrame()).toContain("Check paused")
    setup.mockInput.pressEscape(); await flush(); setup.mockInput.pressArrow("down"); await flush(); expect(setup.captureCharFrame()).toContain("› Learn Vietnamese")
    setup.mockInput.pressEnter(); await flush(); expect(setup.captureCharFrame()).toContain("Not monitored automatically"); setup.mockInput.pressEscape(); await flush()
    setup.mockInput.pressArrow("up"); await flush()
    for (let i = 0; i < 8; i++) { setup.mockInput.pressArrow("down"); await flush() }
    expect(setup.captureCharFrame()).toContain("› How Following runs"); setup.mockInput.pressEnter(); await flush()
    for (const text of ["awake", "Sleep", "Closing Gator stops foreground", "service can continue"]) expect(setup.captureCharFrame().replace(/[│\s]+/g, " ")).toContain(text)
    setup.mockInput.pressEscape(); await flush(); expect(client.draft.text).toBe("Keep commitment draft")
    setup.mockInput.pressKey("r", { ctrl: true }); await flush(); expect(host.sent.at(-1)).toMatchObject({ type: "check", check: { action: "refresh" } })
    host.change({ ...host.current, checks_issue: "Check health is unavailable: unreadable store" }); await flush()
    expect(setup.captureCharFrame()).toContain("Refresh before making changes")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("a commitment with multiple checks opens each exact agreement once", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-following-multi-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current = { ...state(), checks: [check("job-venue", "goal-one"), check("job-briefing", "goal-one")], following_goals: [{ id: "goal-one", title: "Venue planning", status: "active" }] }
  const client = new GatorClient(host), setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 80, height: 24 })
  const flush = async () => { await setup.flush(); await Bun.sleep(20); await setup.flush() }
  try {
    await client.start(); await client.navigate("/following"); await flush(); expect(setup.captureCharFrame().match(/Venue planning/g)).toHaveLength(1)
    setup.mockInput.pressEnter(); await flush(); expect(setup.captureCharFrame()).toContain("Venue check"); expect(setup.captureCharFrame()).toContain("Daily briefing")
    await setup.mockInput.typeText("Daily briefing"); await flush(); setup.mockInput.pressEnter(); await flush(); expect(setup.captureCharFrame()).toContain("Daily briefing"); expect(host.sent).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
