import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import type { TextareaRenderable } from "@opentui/core"
import { execFileSync } from "node:child_process"
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { createHash } from "node:crypto"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"
import { App } from "../src/app"

// Native input, real Go tools and persistence; controlled model, not a live account.
for (const [width, height] of [[110,34], [80,24], [60,20], [42,14]]) test(`dynamic knowledge → original answer → correction/forget → restart at ${width}×${height}`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-dynamic-context-")), calls: string[] = []
  let memoryID = "", noteID = ""
  const memory = "The ceramic delivery arrives Friday.", current = "The ceramic delivery arrives Tuesday.", corrected = "The ceramic delivery arrives Wednesday."
  const note = Array.from({ length: 38 }, (_, i) => `Ceramic dossier line ${i + 1}: bounded saved evidence.`).join("\n") + "\nTail marker: supplier needs a confirmed appointment."
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const raw = await request.text(); calls.push(raw)
    const messages = JSON.parse(raw).messages, last = messages.at(-1), followup = messages.findLast((message: { role: string }) => message.role === "user")?.content === "Continue from retained context."
    const tool = (id: string, name: string, args: unknown) => ({ id, type: "function", function: { name, arguments: JSON.stringify(args) } })
    const message = followup ? { role: "assistant", content: "Continuing from the retained references without reading saved knowledge again." } : last.role !== "tool" ? { role: "assistant", tool_calls: [tool("find", "knowledge_search", { query: "ceramic" })] }
      : last.tool_call_id === "find" ? { role: "assistant", tool_calls: [tool("memory", "knowledge_read", { id: memoryID }), tool("note", "knowledge_read", { id: noteID })] }
      : { role: "assistant", content: "The selected references are ready to inspect." }
    return Response.json({ choices: [{ message, finish_reason: message.tool_calls ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime"), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
  await mkdir(join(root, "config/gator"), { recursive: true }); await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  memoryID = cli(["memory", "add", "--kind", "fact", "--source", "Planning call", memory]).split(/\s/)[0]!
  noteID = cli(["note", "add", "--title", "Ceramic dossier", "--source-ref", "Supplier call", "--body", note]).split(/\s/)[0]!
  const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
  let host = new Application(options), client = new GatorClient(host)
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async () => {} }} />, { width, height })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (check: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 20000; while (!await check()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const block = (title: string) => client.state?.panel?.blocks?.find(block => block.title === title)?.lines?.join("\n")
  const choose = async (title: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(title); await flush(); setup.mockInput.pressEnter(); await flush() }
  const press = async (key: string, ctrl = false) => { if (key === "escape") setup.mockInput.pressEscape(); else setup.mockInput.pressKey(key, { ctrl }); await flush() }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Summarize."); await client.submit()
    await until(() => !client.state?.running && !!client.state?.messages.some(message => message.run_id && message.role === "Gator"), "completed real tool run")
    const context = client.context, run = client.state!.messages.find(message => message.role === "Gator" && message.run_id)!.run_id!
    let count = calls.length, shownRun = run
    const history = JSON.parse(await readFile(join(root, "state/gator/work-history", run + ".json"), "utf8"))
    expect(history.decision?.context_items?.some((item: { id: string }) => [memoryID, noteID].includes(item.id)) ?? false).toBe(false)
    const bundle = client.state!.messages.find(message => message.bundle?.RunID === run)!.bundle!, manifestBytes = await readFile(join(bundle.Path, "manifest.json"))
    const evidence = JSON.parse(manifestBytes.toString()).evidence.filter((entry: { locator: string }) => entry.locator.startsWith("knowledge:" + run + "/"))
    expect(evidence).toHaveLength(3)
    for (const entry of evidence) {
      const data = await readFile(join(bundle.Path, entry.snapshot_path)); expect(createHash("sha256").update(data).digest("hex")).toBe(entry.sha256)
      expect(calls.some(raw => JSON.parse(raw).messages.some((message: { role: string; content: string }) => message.role === "tool" && message.content === data.toString()))).toBe(true)
    }
    cli(["memory", "correct", memoryID, current]); await client.edit("Keep my dynamic context draft"); await client.setPlanFirst(true)
    const actions = async () => {
      await client.navigate("/chat"); await flush()
      const index = client.state!.messages.findIndex(message => message.role === "Gator" && message.run_id === shownRun)
      client.emit("reveal-message", index); await flush()
      setup.mockInput.pressKey("e", { ctrl: true }); await flush()
    }
    const inspect = async () => { await actions(); await choose("Inspect supplied memory"); await until(() => client.state?.panel?.kind === "context", "retained context list") }
    const openRead = async (label: string) => {
      const panel = client.state!.panel!, row = panel.rows!.findIndex(row => row.label.startsWith(label) && (row.label.includes("read during run") || row.label.includes("read carried from")))
      expect(row).toBeGreaterThanOrEqual(0)
      await client.send({ type: "panel", panel: { id: panel.id!, action: "select", row } }); await flush(); setup.mockInput.pressEnter()
      await until(() => !!block("Supplied excerpt"), "selected actual tool read"); await flush()
    }
    await inspect(); expect(client.state?.panel?.rows).toHaveLength(4); expect(client.state?.panel?.rows?.every(row => (row.detail?.length ?? 0) <= 161)).toBe(true)
    await press("f", true); await setup.mockInput.typeText("read during run"); setup.mockInput.pressEnter(); await flush()
    // A refresh can leave Go's selected row outside the local filter. Enter
    // must first select a visible item rather than activate that hidden row.
    const filtered = client.state!.panel!, hidden = filtered.rows!.findIndex(row => row.label.includes("search preview"))
    await client.send({ type: "panel", panel: { id: filtered.id!, action: "select", row: hidden } }); await flush(); setup.mockInput.pressEnter()
    await until(() => !!client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label.includes("read during run"), "visible selection after filtered refresh")
    expect(client.state?.panel?.rows).toHaveLength(4)
    await openRead("fact"); expect(block("Supplied excerpt")).toBe(memory); expect(block("Current saved text")).toContain(current); expect(JSON.stringify(client.state?.panel)).toContain("Planning call")
    expect(setup.captureCharFrame()).toContain("Supplied excerpt")
    await press("e", true); await until(() => client.state?.panel?.kind === "knowledge-form", "authoritative correction form")
    setup.mockInput.pressEnter(); await until(() => !!setup.renderer.root.findDescendantById("multiline-field-input"), "native correction editor")
    ;(setup.renderer.root.findDescendantById("multiline-field-input") as TextareaRenderable).setText(corrected)
    await press("s", true); await press("r", true); await until(() => block("Current saved text")?.includes(corrected) === true, "acknowledged correction")
    expect(block("Supplied excerpt")).toBe(memory); await press("escape"); await until(() => !!client.state?.panel?.rows?.length, "context list restored")
    expect(setup.captureCharFrame()).toContain("Filter loaded entries: read during run")
    await openRead("Ceramic dossier"); expect(block("Supplied excerpt")).toBe(note); expect(JSON.stringify(client.state?.panel)).toContain("Supplier call")
    for (let page = 0; page < 100 && !setup.captureCharFrame().includes("Tail marker:"); page++) await press("\x1b[6~")
    expect(setup.captureCharFrame()).toContain("Tail marker:")
    await press("x", true); await until(() => client.state?.panel?.kind === "confirmation", "exact note forget review"); setup.mockInput.pressEnter()
    await until(() => block("Saved state")?.includes("no longer saved") === true, "confirmed note forgetting")
    expect(block("Supplied excerpt")).toBe(note); expect(calls).toHaveLength(count)
    await actions(); await choose("Inspect source evidence"); await choose("Saved knowledge read")
    await until(() => setup.captureCharFrame().includes("Saved knowledge read"), "native retained response inspection")
    const catalogue = await host.sourceCatalog({ context, run }); expect(catalogue.references.filter(item => item.kind === "knowledge")).toHaveLength(3)
    const retained = await host.sourcePreview({ context, run, path: "evidence:" + evidence[0].id }); expect(retained.notice).toContain("Exact saved-knowledge tool response")
    expect(await readFile(join(bundle.Path, "manifest.json"))).toEqual(manifestBytes); expect(client.draft.text).toBe("Keep my dynamic context draft"); expect(client.draft.plan_first).toBe(true)
    await client.navigate("/chat"); await client.setPlanFirst(false); await client.edit("Continue from retained context."); await client.submit()
    await until(() => !client.state?.running && !!client.state?.messages.some(message => message.role === "Gator" && message.run_id && message.run_id !== run), "follow-up with carried knowledge")
    shownRun = client.state!.messages.findLast(message => message.role === "Gator" && message.run_id)!.run_id!
    expect(calls).toHaveLength(count + 1); count = calls.length
    const followupMessages = JSON.parse(calls.at(-1)!).messages as { role: string; content: string }[]
    const currentInput = followupMessages.filter(message => message.role === "system").map(message => message.content).join("\n")
    expect(currentInput).toContain(corrected); expect(currentInput).toContain("forgotten_or_missing"); expect(currentInput).not.toContain("Tail marker:")
    expect(followupMessages.some(message => message.role === "tool" && message.content.includes(memory))).toBe(true)
    await client.edit("Keep my dynamic context draft"); await client.setPlanFirst(true); await inspect()
    expect(client.state?.panel?.rows).toHaveLength(6)
    expect(client.state?.panel?.rows?.filter(row => row.label.includes("carried from " + run))).toHaveLength(4)
    expect(client.state?.panel?.rows?.filter(row => row.label.includes("refreshed for this run"))).toHaveLength(2)
    const openRefresh = async (label: string) => {
      const panel = client.state!.panel!, row = panel.rows!.findIndex(row => row.label.startsWith(label) && row.label.includes("refreshed for this run"))
      expect(row).toBeGreaterThanOrEqual(0)
      await client.send({ type: "panel", panel: { id: panel.id!, action: "select", row } }); await flush(); setup.mockInput.pressEnter()
      await until(() => !!block("Supplied excerpt"), "current request snapshot"); await flush()
    }
    await openRefresh("fact"); expect(block("Supplied excerpt")).toBe(corrected); expect(setup.captureCharFrame()).toContain("Supplied excerpt")
    const refreshedBundle = client.state!.messages.find(message => message.bundle?.RunID === shownRun)!.bundle!
    const refreshedManifest = JSON.parse(await readFile(join(refreshedBundle.Path, "manifest.json"), "utf8"))
    const refreshEvidence = refreshedManifest.evidence.find((entry: { locator: string }) => entry.locator.includes("knowledge:" + shownRun + "/knowledge_refresh/"))
    const refreshSource = await host.sourcePreview({ context, run: shownRun, path: "evidence:" + refreshEvidence.id })
    expect(refreshSource.notice).toContain("supplied before this request"); expect(refreshSource.text).toContain(corrected)
    await press("escape"); await openRefresh("Saved knowledge"); expect(block("Supplied excerpt")).toContain("No longer saved"); expect(block("Supplied excerpt")).not.toContain("Tail marker:")
    await press("escape")
    await openRead("Ceramic dossier"); expect(block("Supplied excerpt")).toBe(note); expect(block("Saved state")).toContain("no longer saved")
    await client.navigate("/chat"); await client.close(); await setup.dispose(); host = new Application(options); client = new GatorClient(host); setup = await render(); await client.start(); await flush()
    await inspect(); await openRead("Ceramic dossier"); expect(block("Supplied excerpt")).toBe(note); expect(block("Saved state")).toContain("no longer saved"); expect(calls).toHaveLength(count)
    expect(client.draft.text).toBe("Keep my dynamic context draft"); expect(client.draft.plan_first).toBe(true)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 80000)
