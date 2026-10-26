import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ImageRenderable } from "@opentui/core"
import { mkdtemp, writeFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { createHash } from "node:crypto"
import { Application } from "@gator/application-host/application"
import { pdfFixture, pngFixture } from "../../app-host/tests/pdf-fixture.mjs"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean, label: string) { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) } }
test("native terminal decodes image pixels and distinct PDF pages without Electron or remote requests", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-pixels-")), imagePath = join(root, "picture.png"), pdfPath = join(root, "pages.pdf")
  await writeFile(pdfPath, pdfFixture())
  await writeFile(imagePath, pngFixture())
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 36 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const visual = () => { const node = setup.renderer.root.findDescendantById("file-visual-preview"); return node instanceof ImageRenderable ? node : undefined }
  const hash = () => { const bytes = visual()?.source; return bytes instanceof Uint8Array ? createHash("sha256").update(bytes).digest("hex") : "" }
  try {
    await client.start(); await client.edit("Keep unsent preview draft"); await client.attach([imagePath, pdfPath])
    const open = async (name: string) => { await client.navigate("/files"); await flush(); await setup.mockInput.typeText(name); await flush(); setup.mockInput.pressEnter(); await until(() => !!visual()?.image, name + " decoded").catch(error => { throw new Error(`${error.message}: ${visual()?.loadError}\n${setup.captureCharFrame()}`) }); await flush() }
    await open("picture.png"); expect(visual()?.image?.info().width).toBe(32); expect(visual()?.effectiveProtocol).toBe("blocks"); expect(visual()?.loadError).toBeNull()
    setup.mockInput.pressEscape(); await flush(); await open("pages.pdf")
    expect(visual()?.image?.width).toBeGreaterThan(100); expect(setup.captureCharFrame()).toContain("PDF page 1 of 2")
    const first = hash(); expect(first).not.toBe("")
    setup.mockInput.pressArrow("right", { ctrl: true }); await until(() => hash() !== first, "second rendered page"); await flush()
    expect(setup.captureCharFrame()).toContain("PDF page 2 of 2"); expect(visual()?.image?.width).toBeGreaterThan(100)
    setup.mockInput.pressArrow("left", { ctrl: true }); await until(() => hash() === first, "original rendered page"); await flush()
    setup.mockInput.pressEscape(); await flush()
    expect(client.draft.text).toBe("Keep unsent preview draft"); expect(client.draft.uploads).toHaveLength(2); expect(client.state?.messages).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
