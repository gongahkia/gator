import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, writeFile, readFile, rm, stat } from "node:fs/promises"
import { join, resolve, extname } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { inspectOffice } from "@gator/application-host/office"
import { wordFixture, workbookFixture, slidesFixture } from "../../app-host/tests/office-fixtures.mjs"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

const until = async (predicate: () => Promise<boolean>, label: string) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) } }
test("keyboard Office and CSV edits save and export real native content while originals stay intact", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-keyboard-office-"))
  const specimens = [
    { name: "word.docx", bytes: wordFixture(), key: "p:0", match: "Hello", value: "Revised Word paragraph" },
    { name: "slides.pptx", bytes: slidesFixture(), key: "s:0:p:0", match: "Quarterly", value: "Revised slide text" },
    { name: "sheet.xlsx", bytes: workbookFixture(), key: "0!C1", match: "", value: "=B1*5" },
    { name: "table.csv", bytes: Buffer.from('Name,Amount\r\n"Venue, hall",4\r\n'), key: "1:1", match: "", value: "12" },
  ]
  for (const item of specimens) await writeFile(join(root, item.name), item.bytes)
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const replacePrompt = async (value: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(value); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.attach(specimens.map(item => join(root, item.name)))
    for (const item of specimens) {
      const file = client.draft.uploads!.find(file => file.name === item.name)!, context = client.context
      const request = { action: "open" as const, context, ref: { kind: "upload" as const, id: file.id } }
      await client.navigate("/files"); await flush(); await setup.mockInput.typeText(item.name); await flush(); setup.mockInput.pressEnter(); await flush()
      setup.mockInput.pressKey("F3"); await flush()
      if (item.match) { await setup.mockInput.typeText(item.match); await flush(); setup.mockInput.pressEnter(); await flush() }
      else { if (item.name.endsWith("xlsx")) { setup.mockInput.pressEnter(); await flush() }; await replacePrompt(item.name.endsWith("xlsx") ? "C1" : "2:2") }
      await replacePrompt(item.value)
      await until(async () => { const view = await client.host.file(request); return (view.draft?.paragraphs?.[item.key] ?? view.draft?.cells?.[item.key]) === item.value }, item.name + " retained edit")
      setup.mockInput.pressKey("s", { ctrl: true }); await until(async () => (await client.host.file(request)).versions.length === 2, item.name + " saved version"); await flush()
      setup.mockInput.pressKey("F2"); await flush(); await setup.mockInput.typeText("Version 2"); await flush(); setup.mockInput.pressEnter(); await flush()
      await setup.mockInput.typeText("Export exact selected bytes"); await flush(); setup.mockInput.pressEnter(); await flush()
      const destination = join(root, "export-" + item.name); await replacePrompt(destination)
      await until(async () => !!await stat(destination).catch(() => undefined), item.name + " export")
      const exported = await readFile(destination)
      expect(await readFile(join(root, item.name))).toEqual(item.bytes)
      if (item.name.endsWith("csv")) expect(exported.toString()).toBe('Name,Amount\r\n"Venue, hall",12\r\n')
      else {
        const view = inspectOffice(extname(item.name).slice(1), exported)
        expect(JSON.stringify(view)).toContain(item.name.endsWith("xlsx") ? "B1*5" : item.value)
      }
      expect(client.state?.messages).toHaveLength(0)
    }
    expect(client.draft.uploads).toHaveLength(4)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 60000)
