import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean, label: string) { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) } }
test("keyboard indexed catalog pages, renames, pins and archives by original identity and retains stale title edits", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-catalog-")), workspace = join(root, "workspace"), config = join(root, "config/gator")
  await mkdir(workspace); await mkdir(config, { recursive: true })
  let calls = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch() { calls++; return new Response("Catalog must not call a model", { status: 500 }) } })
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}`, models: ["fixture"], default_model: "fixture" }] }))
  const at = Date.now(), oldID = "work-catalog-129", oldRoot = join(root, "state/gator/conversations", oldID)
  for (let index = 0; index < 130; index++) {
    const id = `work-catalog-${String(index).padStart(3, "0")}`, directory = join(root, "state/gator/conversations", id)
    await mkdir(join(directory, "revisions"), { recursive: true })
    await writeFile(join(directory, "conversation.json"), JSON.stringify({ version: 2, id, title: `Topic ${index}`, source_path: workspace, snapshot_id: "snap-catalog", head_revision_id: index === 129 ? "rev-old" : undefined, created_at: new Date(at - 864000000).toISOString(), updated_at: new Date(at - index * 60000).toISOString() }))
  }
  const revision = JSON.stringify({ version: 2, id: "rev-old", conversation_id: oldID, snapshot_id: "snap-catalog", objective: "Choose a direction", bundle_path: join(root, "missing-result"), status: "completed", final_text: "We chose the jade orchid decision for the garden.", created_at: new Date(at - 129 * 60000).toISOString() })
  await writeFile(join(oldRoot, "revisions/rev-old.json"), revision)
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace, userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 36 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const loaded = () => !!client.state?.panel?.catalog && !client.state.catalog_busy && !client.state.panel.catalog.loading
  const chooseAction = async (title: string) => { setup.mockInput.pressKey("F3"); await flush(); await setup.mockInput.typeText(title); await flush(); setup.mockInput.pressEnter(); await flush() }
  const filter = async (title: string) => { setup.mockInput.pressKey("l", { ctrl: true }); await flush(); await setup.mockInput.typeText(title); await flush(); setup.mockInput.pressEnter(); await until(loaded, "filter loaded"); await flush() }
  const search = async () => { setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await setup.mockInput.typeText("jade orchid"); setup.mockInput.pressEnter(); await until(() => loaded() && client.state?.panel?.rows?.length === 1, "indexed old-message result"); await flush() }
  try {
    await client.start(); const originalContext = client.context; await client.edit("Keep the catalog draft"); await client.navigate("/conversations")
    await until(() => loaded() && client.state?.panel?.rows?.length === 50, "first page")
    await flush(); expect(setup.captureCharFrame()).not.toContain(workspace)
    expect(setup.captureCharFrame()).toContain("F1 help")
    expect(setup.renderer.root.findDescendantById("panel-row-0")?.height).toBe(2)
    setup.resize(60, 24); await flush()
    expect(setup.renderer.root.findDescendantById("panel-row-0")?.height).toBe(2)
    setup.mockInput.pressKey("F1"); await flush(); expect(setup.captureCharFrame()).toContain("Chat shortcuts")
    setup.mockInput.pressEscape(); await flush(); setup.resize(110, 36); await flush()
    await chooseAction("Conversation details"); expect(setup.captureCharFrame()).toContain(workspace)
    setup.mockInput.pressEscape(); await flush()
    setup.mockInput.pressArrow("right", { ctrl: true }); await until(() => loaded() && client.state?.panel?.catalog?.page === 2, "second page")
    setup.mockInput.pressArrow("right", { ctrl: true }); await until(() => loaded() && client.state?.panel?.catalog?.page === 3, "third page")
    expect(client.state?.panel?.catalog?.has_next).toBeFalsy(); setup.mockInput.pressArrow("left", { ctrl: true }); await until(() => loaded() && client.state?.panel?.catalog?.page === 2, "previous page")
    await search(); expect(client.state?.panel?.rows?.[0].label).toBe("Topic 129")
    await chooseAction("Rename"); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("Garden plans"); setup.mockInput.pressEnter()
    await until(() => loaded() && client.state?.panel?.rows?.[0].label === "Garden plans", "rename"); await flush()
    await chooseAction("Pin conversation"); await until(() => loaded() && !!client.state?.panel?.rows?.[0].pinned, "pin")
    await chooseAction("Archive"); setup.mockInput.pressEnter(); await until(() => loaded() && !client.state?.panel?.rows?.length, "archive")
    await filter("Archived"); expect(client.state?.panel?.rows?.[0].label).toBe("Garden plans")
    await chooseAction("Restore"); setup.mockInput.pressEnter(); await until(() => loaded() && !client.state?.panel?.rows?.length, "restore"); await filter("Active")
    expect(await readFile(join(oldRoot, "revisions/rev-old.json"), "utf8")).toBe(revision)
    await chooseAction("Rename"); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("Keep this uncommitted title")
    const displayPath = join(oldRoot, "display.json"), display = JSON.parse(await readFile(displayPath, "utf8"))
    await writeFile(displayPath, JSON.stringify({ ...display, revision: display.revision + 1 }))
    setup.mockInput.pressEnter(); await until(() => !!client.issue, "stale title refusal"); await flush()
    expect(setup.captureCharFrame()).toContain("Keep this uncommitted title"); expect(JSON.parse(await readFile(displayPath, "utf8")).title).toBe("Garden plans")
    setup.mockInput.pressEscape(); await Bun.sleep(75); await flush()
    const stalePanel = client.state?.panel?.id
    setup.mockInput.pressKey("F5"); await until(() => loaded() && client.state?.panel?.id !== stalePanel, "acknowledged refresh after conflict"); await flush(); setup.mockInput.pressEnter()
    await until(() => client.state?.conversation_id === oldID && !client.snapshot.sessionSwitching, "open original retained chat")
    expect(JSON.stringify(client.state?.messages)).toContain("jade orchid decision"); expect(client.drafts.get(originalContext)?.text).toBe("Keep the catalog draft")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await client.navigate("/conversations"); await until(loaded, "restart catalog"); await search()
    expect(client.state?.panel?.rows?.[0].label).toBe("Garden plans"); expect(client.state?.panel?.rows?.[0].pinned).toBe(true)
    expect((await readFile(join(root, "state/gator/cache/conversations-v1.sqlite"))).subarray(0, 16).toString()).toBe("SQLite format 3\0"); expect(calls).toBe(0)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
