import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("keyboard export reviews replacements, cancels safely and refuses a changed destination", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-export-")), source = join(root, "notes.txt"), destination = join(root, "copy.txt")
  await writeFile(source, "Exact retained bytes"); await writeFile(destination, "Existing copy")
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const review = async () => {
    await client.navigate("/files"); await flush(); setup.mockInput.pressEnter(); await flush()
    setup.mockInput.pressKey("F2"); await flush(); setup.mockInput.pressEnter(); await flush()
    await setup.mockInput.typeText("Export exact"); await flush(); setup.mockInput.pressEnter(); await flush()
    setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(destination); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("Replace existing export?")
    expect(setup.captureCharFrame()).toContain(destination)
  }
  const until = async (predicate: () => Promise<boolean>) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error("Export did not settle"); await Bun.sleep(20) } }
  try {
    await client.start(); await client.attach([source]); await client.edit("Keep my draft")
    await review(); setup.mockInput.pressEscape(); await Bun.sleep(75); await flush()
    expect(await readFile(destination, "utf8")).toBe("Existing copy")
    await review(); await writeFile(destination, "Concurrent edit")
    setup.mockInput.pressEnter()
    await until(async () => client.issue.includes("changed after review"))
    expect(await readFile(destination, "utf8")).toBe("Concurrent edit")
    await review(); setup.mockInput.pressEnter()
    await until(async () => await readFile(destination, "utf8") === "Exact retained bytes")
    expect(await readFile(source, "utf8")).toBe("Exact retained bytes")
    expect(client.draft.text).toBe("Keep my draft"); expect(client.state?.messages).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 30000)
