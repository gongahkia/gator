import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { createHash } from "node:crypto"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("Go-generated outputs retain sealed bytes while edited versions survive restart and reach the provider only on send", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-generated-files-")), config = join(root, "config/gator"), original = "# A generated report\n\nOriginal sealed output."
  const requests: unknown[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    requests.push(await request.json())
    const message = requests.length === 1 ? { role: "assistant", tool_calls: [{ id: "write-report", type: "function", function: { name: "write_artifact", arguments: JSON.stringify({ path: "report.md", content: original }) } }] } : { role: "assistant", content: "I received the current file." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const choose = async (label: string) => { await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const open = async () => { await client.navigate("/files"); await flush(); await choose("report.md"); await until(() => !!setup.renderer.root.findDescendantById("file-text-editor"), "complete generated file") }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode draft" }); await client.send({ type: "submit", text: "/artifact report.md" }); await client.edit("Create report.md."); await client.submit()
    await until(() => !client.state?.running && requests.length >= 2, "generated artifact")
    const bundle = client.state!.messages.find(message => message.bundle?.Artifacts?.some(file => file.Path === "report.md"))!.bundle!
    const path = join(bundle.Path, "output/report.md"), manifest = join(bundle.Path, "manifest.json"), sealed = await readFile(path), sealedManifest = await readFile(manifest)
    expect(sealed.toString()).toBe(original); expect(bundle.Artifacts?.find(file => file.Path === "report.md")?.SHA256).toBe(createHash("sha256").update(sealed).digest("hex"))
    await client.edit("Please review my edits."); await open(); await flush()
    await setup.mockInput.pasteBracketedText("Edited output: silver otter.\n"); await flush(); setup.mockInput.pressKey("s", { ctrl: true })
    const request = { action: "open" as const, context: client.context, ref: { kind: "output" as const, bundle: bundle.Path, path: "report.md" } }
    await until(async () => (await client.host.file(request)).versions.length === 2, "immutable working version"); await flush()
    const saved = await client.host.file(request); expect(saved.text).toContain("Edited output: silver otter."); expect(saved.text).toContain(original)
    expect(await readFile(path)).toEqual(sealed); expect(await readFile(manifest)).toEqual(sealedManifest)
    const count = requests.length; setup.mockInput.pressEscape(); await flush(); await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await open(); await flush()
    expect(setup.captureCharFrame()).toContain("Edited output: silver otter."); expect(requests).toHaveLength(count); expect(client.draft.text).toBe("Please review my edits.")
    setup.mockInput.pressKey("F2"); await flush(); await choose("Version 2"); await choose("Ask Gator")
    await until(() => client.draft.uploads?.length === 1, "staged edited output"); expect(requests).toHaveLength(count)
    const attached = client.draft.uploads![0], view = await client.host.file({ action: "open", context: client.context, ref: { kind: "upload", id: attached.id } }); expect(view.text).toBe(saved.text)
    await client.send({ type: "submit", text: "/artifact clear" }); await client.send({ type: "submit", text: "/mode inspect" }); await flush(); setup.mockInput.pressEnter()
    await until(() => requests.length > count && !client.state?.running, "explicit send of edited version")
    expect(JSON.stringify(requests.slice(count))).toContain("Edited output: silver otter."); expect(client.draft.uploads).toHaveLength(0)
    expect(await readFile(path)).toEqual(sealed); expect(await readFile(manifest)).toEqual(sealedManifest)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
