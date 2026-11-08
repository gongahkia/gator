// Run from ui/: bun --preload ./scripts/preload.ts scripts/bench-transcript.tsx
// Native renderer only, synthetic history, isolated profile; never calls a model.
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdtemp, rm } from "node:fs/promises"
import { join } from "node:path"
import { tmpdir } from "node:os"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "../tests/fixture"

const counts = process.argv.slice(2).length ? process.argv.slice(2).map(Number) : [100, 1000, 5000]
if (counts.some(count => !Number.isInteger(count) || count < 1 || count > 10000)) throw new Error("Counts must be whole numbers from 1 to 10000.")
const measurements: Record<string, number>[] = []
for (const count of counts) {
  const root = await mkdtemp(join(tmpdir(), "gator-transcript-benchmark-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current.messages = Array.from({ length: count }, (_, index) => ({ role: index % 2 ? "Gator" : "You", text: `Message ${index}. ${index % 2 ? "A retained answer with **a clear conclusion**, a [source](https://example.test), and a short explanation." : "Please check the current plan and explain the next useful step."}`, created_at: new Date(1700000000000 + index * 60000).toISOString() }))
  const client = new GatorClient(host), started = performance.now()
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const frame = async () => { await setup.flush(); await Bun.sleep(10); await setup.flush() }
  try {
    await client.start(); await frame()
    // Markdown parsing is asynchronous. Startup ends when the user can read
    // the latest reply, rather than merely when its empty container is mounted.
    const deadline = performance.now() + 30000
    while (!setup.captureCharFrame().includes(`Message ${count - 1}.`) && performance.now() < deadline) await frame()
    const startup = performance.now() - started
    if (!setup.captureCharFrame().includes(`Message ${count - 1}.`)) throw new Error(`Latest message is not visible after initial layout (${count} messages).\n${setup.captureCharFrame()}`)
    const transcript = setup.renderer.root.findDescendantById("transcript") as ScrollBoxRenderable
    const scrollStart = performance.now()
    setup.mockInput.pressKey("\x1b[5~"); await frame()
    const scrolling = performance.now() - scrollStart, position = transcript.scrollTop
    const appendStart = performance.now()
    host.change({ ...host.current, messages: [...host.current.messages, { role: "You", text: "New message while reading earlier messages." }] }); await frame()
    const appending = performance.now() - appendStart
    if (transcript.scrollTop !== position) throw new Error("Appending moved the current reading position.")
    const resizeStart = performance.now()
    setup.resize(80, 24); await frame()
    const resizing = performance.now() - resizeStart
    measurements.push({ messages: count, startup_ms: Math.round(startup), scroll_ms: Math.round(scrolling), append_ms: Math.round(appending), resize_ms: Math.round(resizing), js_heap_mib: Math.round(process.memoryUsage().heapUsed / 1024 / 1024), rss_mib: Math.round(process.memoryUsage().rss / 1024 / 1024) })
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}
process.stdout.write(JSON.stringify({ runtime: `Bun ${Bun.version}`, platform: `${process.platform}/${process.arch}`, viewport: "110x34 then 80x24", frame_settle_ms: 10, measurements }, null, 2) + "\n")
