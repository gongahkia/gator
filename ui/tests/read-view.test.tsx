import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"
import { App } from "../src/app"

function gate() { let release!: () => void; const wait = new Promise<void>(resolve => { release = resolve }); return { wait, release } }

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) test(`retained reads cancel, retire late responses and recover without stopping work at ${width}×${height}`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-read-view-")), config = join(root, "config/gator"), workspace = join(root, "workspace"), task = gate()
  let calls = 0, holdTask = false
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    await request.text(); calls++
    if (holdTask) await task.wait
    const message = calls === 1 ? { role: "assistant", tool_calls: [
      { id: "source", type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/project.md" }) } },
      { id: "output", type: "function", function: { name: "write_artifact", arguments: JSON.stringify({ path: "report.md", content: "Original sealed report.\n" }) } },
    ] } : { role: "assistant", content: "The retained report and source are ready." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  await mkdir(config, { recursive: true }); await mkdir(workspace); await writeFile(join(workspace, "project.md"), "Owner: Mina\n")
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const host = new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime"), userData: join(root, "app"), workspace, env: { ...process.env, GATOR_CONFIG_DIR: join(root, "config"), GATOR_STATE_DIR: join(root, "state") } }), client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async () => {} }} />, { width, height })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const choose = async (label: string) => { await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const catalog = host.sourceCatalog.bind(host), preview = host.sourcePreview.bind(host), file = host.file.bind(host), releases: (() => void)[] = []
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode draft" }); await client.send({ type: "submit", text: "/artifact report.md" }); await client.edit("Read project.md and create report.md"); await client.submit()
    await until(() => !client.state?.running && calls >= 2, "retained result")
    const bundle = client.state!.messages.find(message => message.bundle?.Artifacts?.length)!.bundle!, manifest = await readFile(join(bundle.Path, "manifest.json")), count = calls, context = client.context
    await client.edit("Independent\neditable draft"); await client.setPlanFirst(true)
    const openSources = async () => { await client.navigate("/evidence"); await flush(); await choose("retained report") }
    const readPending = async () => { await until(() => !!setup.renderer.root.findDescendantById("item-read-view"), "read view visible"); expect(setup.captureCharFrame()).toContain("Ctrl+C close") }
    // The real catalogue is read first; only delivery to the view is held.
    const delayed = gate(); releases.push(delayed.release)
    host.sourceCatalog = async request => { const value = await catalog(request); await delayed.wait; return value }
    await openSources(); await readPending(); setup.mockInput.pressKey("c", { ctrl: true }); await flush()
    await until(() => !client.operations.size, "cancelled view releases UI operation")
    delayed.release(); await flush(); expect(setup.renderer.root.findDescendantById("item-read-view")).toBeUndefined()
    expect(setup.captureCharFrame()).not.toContain("Retained sources"); expect(client.issue).toBe("")
    expect(client.draft.text).toBe("Independent\neditable draft"); expect(client.draft.plan_first).toBe(true)
    host.sourceCatalog = catalog
    // A delayed failure after navigation must not replace Settings or surface
    // an obsolete error. It remains a handled completion.
    const abandoned = gate(); releases.push(abandoned.release)
    host.sourceCatalog = async request => { await catalog(request); await abandoned.wait; throw new Error("Obsolete catalogue failure") }
    await openSources(); await readPending(); await client.navigate("/settings"); abandoned.release(); await flush()
    expect(client.route).toBe("settings"); expect(client.issue).toBe(""); expect(setup.captureCharFrame()).not.toContain("Obsolete")
    host.sourceCatalog = catalog; await client.navigate("/chat")
    await openSources(); await until(() => setup.captureCharFrame().includes("project.md"), "source choices")
    const heldPreview = gate(); releases.push(heldPreview.release)
    host.sourcePreview = async request => { const value = await preview(request); await heldPreview.wait; return value }
    await choose("project.md"); await readPending(); setup.mockInput.pressEscape(); await flush()
    await until(() => setup.captureCharFrame().includes("project.md") && !setup.renderer.root.findDescendantById("item-read-view"), "escape returns to source choices")
    heldPreview.release(); await flush(); expect(setup.captureCharFrame()).not.toContain("Read only")
    host.sourcePreview = preview
    // A rejected file read offers an explicit retry of the same bound item.
    let attempts = 0
    host.file = (async (request: Parameters<typeof host.file>[0]) => { if (request.action === "open" && ++attempts === 1) throw new Error("Temporary file read failure"); return file(request) }) as typeof host.file
    await client.navigate("/files"); await flush(); await choose("report.md")
    await until(() => setup.captureCharFrame().includes("Temporary file read"), "contextual file recovery")
    setup.mockInput.pressKey("r", { ctrl: true }); await flush()
    await until(() => !!setup.renderer.root.findDescendantById("file-text-editor"), "retry opened original file")
    expect(attempts).toBe(2); expect(calls).toBe(count); setup.mockInput.pressEscape(); await flush(); host.file = file
    const backFile = gate(); releases.push(backFile.release)
    host.file = (async (request: Parameters<typeof host.file>[0]) => { const value = await file(request); await backFile.wait; return value }) as typeof host.file
    await client.navigate("/outputs"); await flush(); await choose("report.md"); await choose("Open complete file"); await readPending()
    setup.mockInput.pressEscape(); await flush()
    await until(() => setup.captureCharFrame().includes("report.md") && setup.captureCharFrame().includes("Open complete"), "back to selected file actions")
    backFile.release(); await flush(); expect(setup.renderer.root.findDescendantById("file-text-editor")).toBeUndefined(); host.file = file
    // Close a file read while a real Go-owned task is executing. UI cancel
    // must not send the task's stop key or discard its independent draft.
    await client.setPlanFirst(false); await client.send({ type: "submit", text: "/artifact clear" }); holdTask = true
    await client.edit("Explain the next step"); await client.submit(); await until(() => !!client.state?.running && calls > count, "live task")
    await client.edit("Draft during work"); const guidance = client.state?.guidance_id
    const heldFile = gate(); releases.push(heldFile.release)
    host.file = (async (request: Parameters<typeof host.file>[0]) => { const value = await file(request); await heldFile.wait; return value }) as typeof host.file
    await client.navigate("/files"); await flush(); await choose("report.md"); await readPending()
    setup.mockInput.pressKey("c", { ctrl: true }); await flush(); heldFile.release(); await flush()
    expect(client.context).toBe(context); expect(client.state?.running).toBe(true); expect(client.state?.cancelling).toBe(false)
    expect(client.state?.guidance_id).toBe(guidance); expect(client.draft.text).toBe("Draft during work")
    expect(setup.renderer.root.findDescendantById("file-text-editor")).toBeUndefined(); expect(client.operations.size).toBe(0)
    expect(await readFile(join(bundle.Path, "manifest.json"))).toEqual(manifest)
    task.release(); await until(() => !client.state?.running, "original task finishes")
  } finally { releases.forEach(release => release()); task.release(); host.file = file; host.sourceCatalog = catalog; host.sourcePreview = preview; await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
