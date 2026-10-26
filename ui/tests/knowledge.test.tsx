import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("saved knowledge filters preserve identity and guarded corrections, omission and forgetting preserve unrelated notes", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-knowledge-"))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (...args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
  const note = cli("note", "add", "--title", "Offsite planning notes", "--body", "Venue shortlist: Sentosa or East Coast.", "--source-ref", "planning-brief.md").split(/\s/)[0]
  const other = cli("note", "add", "--title", "Communication preferences", "--body", "Prefer concise summaries.").split(/\s/)[0]
  const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const filter = async (value: string) => { setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(value); setup.mockInput.pressEnter(); await flush() }
  const open = async () => { await client.navigate("/knowledge"); await flush(); await filter("Offsite planning"); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Offsite planning notes", "selected note") }
  const edit = async (text: string) => { setup.mockInput.pressKey("e"); await until(() => client.state?.panel?.kind === "knowledge-form", "correction form"); await flush(); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressKey("\x1b[H"); setup.mockInput.pressKey("\x1b[1;2F"); await setup.mockInput.pasteBracketedText(text); setup.mockInput.pressKey("s", { ctrl: true }); await flush() }
  try {
    await client.start(); await client.edit("Keep knowledge draft"); await client.navigate("/knowledge"); await flush(); await filter("Sentosa")
    expect(setup.captureCharFrame()).toContain("1 of 2"); expect(setup.captureCharFrame()).not.toContain("Communication preferences")
    await filter("No matching knowledge"); setup.mockInput.pressEnter(); await flush(); expect(setup.captureCharFrame()).toContain("No matching loaded items"); expect(client.state?.panel?.title).toBe("Saved knowledge")
    await filter("Sentosa"); setup.mockInput.pressEnter(); await flush(); expect(client.state?.panel?.title).toBe("Offsite planning notes"); expect(JSON.stringify(client.state?.panel)).toContain("planning-brief.md")
    setup.mockInput.pressKey("o"); await flush(); expect(client.state?.panel?.notice).toContain("Omitted"); setup.mockInput.pressKey("o"); await flush(); expect(client.state?.panel?.notice).toContain("Included again")
    const stale = "Keep the correction after a concurrent note edit."
    await edit(stale); cli("note", "edit", note, "--body", "Concurrent saved note is authoritative."); setup.mockInput.pressKey("F2"); await until(() => !!client.state?.panel?.notice?.includes("changed"), "stale knowledge refusal"); await flush()
    expect(client.state?.panel?.kind).toBe("knowledge-form"); expect(client.state?.panel?.rows?.[0].input?.value).toBe(stale); expect(cli("note", "show", note)).toContain("Concurrent saved note is authoritative.")
    setup.mockInput.pressEscape(); await flush(); setup.mockInput.pressEnter(); await flush(); await open()
    await edit("Corrected shortlist: East Coast.\nConfirm capacity first."); setup.mockInput.pressKey("F2"); await until(() => client.state?.panel?.title === "Offsite planning notes", "saved correction")
    expect(cli("note", "show", note)).toContain("Corrected shortlist: East Coast.\nConfirm capacity first."); expect(cli("note", "show", note)).toContain("planning-brief.md")
    setup.mockInput.pressKey("f"); await until(() => client.state?.panel?.title === "Forget saved knowledge?", "explicit forget review"); setup.mockInput.pressEscape(); await flush(); expect(cli("note", "list")).toContain(note)
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await open(); await flush()
    expect(JSON.stringify(client.state?.panel)).toContain("Corrected shortlist: East Coast."); expect(client.draft.text).toBe("Keep knowledge draft")
    setup.mockInput.pressKey("f"); await flush(); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Saved knowledge", "forgotten selected note")
    expect(cli("note", "list")).not.toContain(note); expect(cli("note", "show", other)).toContain("Prefer concise summaries."); expect(client.state?.messages).toHaveLength(0)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
