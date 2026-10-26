import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, writeFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("large table keyboard pages remain bounded and selecting a distant saved cell stages its actual value", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-grid-")), path = join(root, "large.csv")
  await writeFile(path, Array.from({ length: 125 }, (_, row) => Array.from({ length: 45 }, (_, col) => `r${row}c${col}`).join(",")).join("\n"))
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  try {
    await client.start(); await client.attach([path]); await client.navigate("/files"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(setup.renderer.root.findDescendantById("file-grid-page")?.getChildren().length).toBeLessThanOrEqual(55)
    setup.mockInput.pressArrow("down", { ctrl: true }); setup.mockInput.pressArrow("down", { ctrl: true }); await flush()
    setup.mockInput.pressArrow("right", { ctrl: true }); setup.mockInput.pressArrow("right", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).toContain("Rows 101–125 of 125"); expect(setup.captureCharFrame()).toContain("AO101=r100c40")
    setup.mockInput.pressKey("F6"); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("AS125"); setup.mockInput.pressEnter(); await flush()
    setup.mockInput.pressKey("F2"); await flush(); setup.mockInput.pressEnter(); await flush(); await setup.mockInput.typeText("Ask Gator"); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(client.draft.text).toContain("AS125: r124c44"); expect(client.draft.uploads).toHaveLength(2); expect(client.state?.messages).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 30000)
