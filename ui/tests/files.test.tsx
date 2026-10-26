import { test, expect } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"
import { App } from "../src/app"
import type { FileEdit } from "@gator/application-host/contracts/fileProtocol"
import { wordFixture, workbookFixture, slidesFixture } from "../../app-host/tests/office-fixtures.mjs"

const until = async (predicate: () => boolean | Promise<boolean>, label = "file state") => { const deadline = Date.now() + 10000; while (!await predicate()) { if (Date.now() > deadline) throw new Error(`Timed out waiting for ${label}`); await Bun.sleep(20) } }

test("terminal files save exact versions, preserve originals and stage a selected version", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-files-")), path = join(root, "notes.txt")
  await writeFile(path, "Original file body")
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  try {
    await client.start(); await client.attach([path]); const original = client.draft.uploads![0]
    await client.navigate("/files"); await setup.flush(); await Bun.sleep(25); expect(setup.captureCharFrame()).toContain("notes.txt")
    setup.mockInput.pressEnter(); await until(() => !!setup.renderer.root.findDescendantById("file-text-editor"), "editor open"); await setup.flush(); await Bun.sleep(25)
    await setup.mockInput.pasteBracketedText("Added content ")
    setup.mockInput.pressKey("s", { ctrl: true })
    const request = { action: "open" as const, context: client.context, ref: { kind: "upload" as const, id: original.id } }
    await until(async () => (await client.host.file(request)).versions.length === 2, "saved version")
    const saved = await client.host.file(request)
    expect(saved.text).toContain("Added content"); expect(await readFile(path, "utf8")).toBe("Original file body")
    await setup.flush(); await Bun.sleep(25); setup.mockInput.pressKey("F2"); await setup.flush(); await Bun.sleep(25)
    await setup.mockInput.typeText("Version 2"); await setup.flush(); await Bun.sleep(25); setup.mockInput.pressEnter(); await setup.flush(); await Bun.sleep(25)
    await setup.mockInput.typeText("Ask Gator"); await setup.flush(); await Bun.sleep(25); setup.mockInput.pressEnter()
    await until(() => (client.draft.uploads?.length ?? 0) === 2, "staged version")
    const attached = client.draft.uploads![1]
    const selected = await client.host.file({ ...request, ref: { kind: "upload", id: attached.id } })
    expect(selected.text).toBe(saved.text); expect(client.state?.messages.length).toBe(0)
    expect(selected.versions.length).toBe(2)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 30000)

test("all native Office content remains accessible through the same file workflow", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-office-"))
  const paths = [join(root, "document.docx"), join(root, "budget.xlsx"), join(root, "slides.pptx")]
  await Promise.all(paths.map((path, index) => writeFile(path, [wordFixture(), workbookFixture(), slidesFixture()][index])))
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  try {
    await client.start(); await client.attach(paths)
    for (const name of ["document.docx", "budget.xlsx", "slides.pptx"]) {
      await client.navigate("/files"); await setup.flush(); await Bun.sleep(25); await setup.mockInput.typeText(name); await setup.flush(); await Bun.sleep(25); setup.mockInput.pressEnter(); await Bun.sleep(60); await setup.flush(); await Bun.sleep(25)
      expect(setup.captureCharFrame()).toContain(name)
      setup.mockInput.pressKey("F3"); await setup.flush(); await Bun.sleep(25)
      expect(setup.captureCharFrame()).toContain(name === "budget.xlsx" ? "Choose worksheet" : "Choose existing text")
      setup.mockInput.pressEscape(); await setup.flush(); await Bun.sleep(25)
    }
    for (const file of client.draft.uploads ?? []) {
      const request = { action: "open" as const, context: client.context, ref: { kind: "upload" as const, id: file.id } }
      const original = await client.host.file(request)
      const edit: FileEdit = original.kind === "word" ? { paragraphs: { "p:0": "Revised paragraph" } } : original.kind === "workbook" ? { cells: { "0!C1": "=B1*3" } } : { paragraphs: { "s:0:p:0": "Revised slide" } }
      const next = await client.host.file({ ...request, action: "save", expected: original.current, edit })
      expect(next.versions.length).toBe(2)
      const destination = join(root, "export-" + file.name)
      await client.host.exportFile({ ...request, action: "export", version: next.revision }, destination)
      expect((await readFile(destination)).length).toBeGreaterThan(0)
    }
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 30000)
