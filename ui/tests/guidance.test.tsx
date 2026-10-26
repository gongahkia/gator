import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("saved guidance uses typed choices and full text, retains scope on edit, filters status and survives restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-guidance-"))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 40 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const field = async (label: string) => { setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressEnter(); await flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const filter = async (status: string) => { setup.mockInput.pressKey("l", { ctrl: true }); await flush(); await choose(status) }
  const body = "Review venue capacity before proposing a shortlist.\nPreserve original evidence and explicitly flag unknowns."
  try {
    await client.start(); await client.edit("Keep my unsent guidance draft"); const context = client.context
    await client.navigate("/learnings"); await flush(); setup.mockInput.pressKey("n"); await until(() => client.state?.panel?.kind === "learning-form", "new guidance"); await flush()
    await field("Type"); await choose("Procedure"); await field("Scope"); await choose("This workspace")
    await field("Key"); await choose("offsite-review")
    await field("Text"); await setup.mockInput.pasteBracketedText(body); setup.mockInput.pressKey("s", { ctrl: true }); await flush()
    setup.mockInput.pressKey("F2"); await until(() => client.state?.panel?.title === "Guidance details", "saved full guidance"); await flush()
    expect(JSON.stringify(client.state?.panel)).toContain("procedure"); expect(JSON.stringify(client.state?.panel)).toContain("offsite-review")
    expect(client.state?.panel?.blocks?.find(block => block.title === "Guidance")?.lines).toEqual([body])
    const scope = client.state?.panel?.summary?.find(field => field.label === "Scope")?.value
    setup.mockInput.pressKey("e"); await until(() => client.state?.panel?.kind === "learning-form", "edit saved guidance"); await flush()
    expect(client.state?.panel?.rows?.[0].input?.read_only).toBe(true); expect(client.state?.panel?.rows?.[1].input?.read_only).toBe(true)
    const updated = body + "\nReport capacity before cost."
    await field("Text"); setup.mockInput.pressKey("\x1b[F"); await setup.mockInput.pasteBracketedText("\nReport capacity before cost."); setup.mockInput.pressKey("s", { ctrl: true }); await flush(); setup.mockInput.pressKey("F2")
    await until(() => client.state?.panel?.title === "Guidance details", "edited guidance"); await flush()
    expect(client.state?.panel?.summary?.find(field => field.label === "Scope")?.value).toBe(scope)
    // Verify complete content after editing, including retained line breaks.
    expect(client.state?.panel?.blocks?.find(block => block.title === "Guidance")?.lines).toEqual([updated])
    setup.mockInput.pressKey("d"); await until(() => client.state?.panel?.summary?.some(field => field.label === "Status" && field.value === "disabled") === true, "disabled guidance")
    await client.navigate("/learnings"); await flush(); await filter("Active"); await until(() => !client.state?.panel?.rows?.length, "empty active filter")
    await filter("Disabled"); await until(() => client.state?.panel?.rows?.length === 1, "disabled filter"); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressKey("a")
    await until(() => client.state?.panel?.summary?.some(field => field.label === "Status" && field.value === "active") === true, "re-enabled guidance")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await client.navigate("/learnings"); await flush()
    expect(client.state?.panel?.rows).toHaveLength(1); setup.mockInput.pressEnter(); await flush()
    expect(client.state?.panel?.blocks?.find(block => block.title === "Guidance")?.lines).toEqual([updated])
    expect(client.context).toBe(context); expect(client.draft.text).toBe("Keep my unsent guidance draft"); expect(client.state?.messages).toHaveLength(0)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 60000)
