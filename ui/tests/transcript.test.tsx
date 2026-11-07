import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

test("long transcript keeps its reading position through hubs and activity; search reveals the selected message", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-transcript-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current = { ...state(), messages: Array.from({ length: 60 }, (_, n) => ({ role: "Gator", text: `Retained message ${n}: grounded answer`, created_at: `2026-10-26T00:${String(n).padStart(2, "0")}:00Z` })) }
  const client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const transcript = () => { const node = setup.renderer.root.findDescendantById("transcript"); if (!(node instanceof ScrollBoxRenderable)) throw new Error("Transcript not mounted"); return node }
  try {
    await client.start(); await flush()
    const subscriptions = client.listenerCount("change")
    host.change({ ...host.current, messages: host.current.messages.map(message => ({ ...message, activity: { state: "completed", started_at: "2026-10-26T00:00:00Z", finished_at: "2026-10-26T00:00:12Z", entries: [{ id: "public-note", kind: "text", text: "Retained public narration", state: "completed", started_at: "2026-10-26T00:00:00Z" }] } })) }); await flush()
    expect(client.listenerCount("change")).toBe(subscriptions)
    expect(transcript().scrollTop).toBeGreaterThan(10)
    const latest = transcript().scrollTop
    setup.mockInput.pressKey("\x1b[5~"); await flush()
    const position = transcript().scrollTop
    expect(position).toBeLessThan(latest)
    host.change({ ...host.current, messages: [...host.current.messages, { role: "Gator", text: "New activity while reading" }] }); await flush()
    expect(transcript().scrollTop).toBe(position)
    await client.navigate("/saved"); await flush(); expect(client.readingPositions.get("first")).toBe(position)
    await client.navigate("/chat"); await flush()
    expect(transcript().scrollTop).toBe(position)
    setup.mockInput.pressKey("f", { ctrl: true }); await flush()
    await setup.mockInput.typeText("Retained message 5:"); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Transcript matches")
    setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Retained message 5:")
    expect(transcript().scrollTop).toBeLessThan(position)
    expect(host.sent).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
