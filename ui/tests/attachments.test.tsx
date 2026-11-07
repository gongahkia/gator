import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve, delimiter } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { pngFixture } from "../../app-host/tests/pdf-fixture.mjs"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

// The clipboard helper is a local process fixture. Native desktop clipboard
// permissions and macOS/Windows integration still require their own host checks.
test.skipIf(process.platform !== "linux")("file selection and native image paste preserve drafts, reject limits, and submit immutable files only explicitly", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-attachments-")), helpers = join(root, "helpers"), config = join(root, "config/gator"), oldPath = process.env.PATH
  const document = join(root, "brief.txt"), image = join(root, "clipboard.png"), invalid = join(root, "program.exe"), large = join(root, "large.txt"), largeImage = join(root, "large.png")
  await mkdir(helpers); await mkdir(config, { recursive: true })
  await writeFile(document, "Immutable brief: violet kiwi."); await writeFile(image, pngFixture()); await writeFile(invalid, "unsupported executable")
  await writeFile(large, Buffer.alloc(8 * 1024 * 1024 + 1, 65)); await writeFile(largeImage, Buffer.alloc(4 * 1024 * 1024 + 1, 65))
  for (const name of ["xclip", "wl-paste"]) await writeFile(join(helpers, name), `#!${process.execPath}\nimport {readFileSync} from 'node:fs'; process.stdout.write(readFileSync(${JSON.stringify(image)}));\n`, { mode: 0o700 })
  process.env.PATH = helpers + delimiter + oldPath
  const calls: string[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) { calls.push(await request.text()); return Response.json({ choices: [{ message: { role: "assistant", content: "The selected files arrived." }, finish_reason: "stop" }], usage: { total_tokens: 20 } }) } })
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime"), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 36 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 12000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const action = async (text: string) => { setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await choose(text) }
  const pick = async (path: string) => { await client.navigate("/attach-file"); await flush(); setup.mockInput.pressKey("l", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await choose(path) }
  try {
    await client.start(); await client.edit("Keep this request"); const original = client.context
    await client.navigate("/attach-file"); await flush(); setup.mockInput.pressEscape(); await flush()
    expect(client.draft.text).toBe("Keep this request"); expect(client.draft.uploads ?? []).toHaveLength(0)
    for (const [file, issue] of [[invalid, "isn’t supported"], [large, "8 MiB"], [largeImage, "4 MiB"]]) {
      await pick(file); await until(() => client.issue.includes(issue), "visible intake rejection")
      expect(setup.captureCharFrame()).toContain(issue); expect(client.draft.text).toBe("Keep this request"); expect(client.draft.uploads ?? []).toHaveLength(0)
    }
    await pick(document); await until(() => client.draft.uploads?.length === 1, "selected file")
    await action("attachments"); await choose("brief.txt"); await choose("Remove from draft")
    expect(client.draft.uploads).toHaveLength(0); expect(client.draft.text).toBe("Keep this request")
    const paths = Array.from({ length: 12 }, (_, i) => join(root, `input-${i}.txt`)); await Promise.all(paths.map((path, i) => writeFile(path, `Selected file ${i}`)))
    await client.attach(paths); await pick(document); await until(() => client.issue.includes("limits"), "thirteenth attachment rejection")
    expect(client.draft.uploads).toHaveLength(12); expect(calls).toHaveLength(0)
    await client.edit(client.draft.text, []); await writeFile(large, Buffer.alloc(8 * 1024 * 1024, 65))
    await client.attach([large, large]); expect(client.draft.uploads).toHaveLength(2)
    await pick(document); await until(() => client.issue.includes("limits"), "combined attachment limit")
    expect(client.draft.uploads).toHaveLength(2); await client.edit(client.draft.text, [])
    await pick(document); await action("paste-image"); await until(() => client.draft.uploads?.length === 2, "clipboard image intake")
    const files = client.draft.uploads!.map(file => ({ ...file })); expect(files[1].name.endsWith(".png")).toBe(true); expect(calls).toHaveLength(0)
    await client.navigate("/new"); await until(() => client.context !== original && !client.snapshot.sessionSwitching, "independent chat")
    expect(client.draft.uploads ?? []).toHaveLength(0); await client.edit("Separate draft")
    await client.host.sessions.open({ conversation_id: original.slice(13) }); await until(() => client.context === original, "original draft")
    expect(client.draft.text).toBe("Keep this request"); expect(client.draft.uploads).toEqual(files)
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await flush()
    expect(client.context).toBe(original); expect(client.draft.uploads).toEqual(files); expect(calls).toHaveLength(0)
    await writeFile(document, "Changed original after selection."); await writeFile(image, "Clipboard changed.")
    await client.send({ type: "submit", text: "/mode inspect" }); await client.edit(""); await flush(); setup.mockInput.pressEnter()
    await until(() => calls.length === 1 && !client.state?.running, "files-only explicit send")
    expect(calls[0]).toContain("violet kiwi"); expect(calls[0]).not.toContain("Changed original"); expect(calls[0]).toContain("data:image/png;base64,")
    expect(client.draft.uploads).toHaveLength(0); expect(await readFile(document, "utf8")).toBe("Changed original after selection.")
    expect(client.state?.messages.find(message => message.role === "You")?.inputs).toHaveLength(2)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); process.env.PATH = oldPath; server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 90000)
