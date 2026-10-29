import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

test("progress follows live current-chat operations, clears on errors and ignores human waiting states", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-progress-")), host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await Bun.sleep(190); await setup.flush() }
  let release = () => {}, reject = (_error: Error) => {}
  try {
    await client.start(); await setup.flush()
    const work = client.track("Importing attachment…", new Promise<void>(resolve => { release = resolve }))
    await setup.flush(); expect(setup.renderer.root.findDescendantById("operation-progress")).toBeUndefined()
    await flush(); expect(setup.captureCharFrame()).toContain("Importing attachment…")
    // Repeated unrelated snapshots must not restart the delay indefinitely.
    for (let n = 0; n < 5; n++) { host.change({ ...host.current }); await Bun.sleep(20) }
    await setup.flush(); expect(setup.captureCharFrame()).toContain("Importing attachment…")
    host.change(state("second")); await flush(); expect(setup.renderer.root.findDescendantById("operation-progress")).toBeUndefined()
    host.change(state()); await flush(); expect(setup.captureCharFrame()).toContain("Importing attachment…")
    const failed = client.track("Saving file…", new Promise<void>((_, fail) => { reject = fail })).catch(() => {})
    await flush(); expect(setup.captureCharFrame()).toContain("Saving file…")
    reject(new Error("fixture failure")); await failed; await flush(); expect(setup.captureCharFrame()).toContain("Importing attachment…")
    release(); await work; await flush(); expect(setup.renderer.root.findDescendantById("operation-progress")).toBeUndefined()
    expect(client.operations.size).toBe(0)
    host.change({ ...state(), pending_operation: "Installing local recall…" }); await flush(); expect(setup.captureCharFrame()).toContain("Installing local recall…")
    host.change({ ...state(), receipt_busy: true }); await flush(); expect(setup.captureCharFrame()).toContain("Checking saved receipt…")
    host.change({ ...state(), pending_connection: "browser-login" }); await flush(); expect(setup.renderer.root.findDescendantById("operation-progress")).toBeUndefined()
    host.change(state()); client.voicePhase = "transcribing"; client.emit("change"); await flush(); expect(setup.captureCharFrame()).toContain("Transcribing audio…")
    for (const phase of ["recording", "speaking", "idle"]) { client.voicePhase = phase; client.emit("change"); await flush(); expect(setup.renderer.root.findDescendantById("operation-progress")).toBeUndefined() }
    await client.track("Quick operation", Promise.resolve()); await flush(); expect(setup.renderer.root.findDescendantById("operation-progress")).toBeUndefined()
  } finally { release(); reject(new Error("cleanup")); await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
