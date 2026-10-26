import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { TextareaRenderable } from "@opentui/core"
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("file comparison and restoration keep full versions while stale recovered edits never overwrite a newer save", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-file-recovery-")), source = join(root, "long-notes.md"), original = "# Notes\n" + "Complete context retained.\n".repeat(300) + "Original final paragraph."
  await writeFile(source, original)
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const choose = async (label: string) => { await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const editor = () => { const value = setup.renderer.root.findDescendantById("file-text-editor"); if (!(value instanceof TextareaRenderable)) throw new Error("File editor is not open"); return value }
  const open = async () => { await client.navigate("/files"); await flush(); await choose("long-notes.md"); await until(() => !!setup.renderer.root.findDescendantById("file-text-editor"), "complete text editor"); await flush() }
  const versions = async (version: number, action: string) => { setup.mockInput.pressKey("F2"); await flush(); await choose(`Version ${version}`); await choose(action) }
  try {
    await client.start(); await client.attach([source]); await client.edit("Keep independent composer"); const ref = { kind: "upload" as const, id: client.draft.uploads![0].id }, context = client.context
    const view = () => client.host.file({ action: "open", context, ref })
    await open(); expect(editor().plainText).toBe(original); setup.mockInput.pressKey("\x1b[F"); await setup.mockInput.pasteBracketedText("\nSaved user suffix."); setup.mockInput.pressKey("s", { ctrl: true })
    await until(async () => (await view()).versions.length === 2, "saved second version"); await flush()
    expect((await view()).text).toBe(original + "\nSaved user suffix.")
    await versions(1, "Compare with current"); expect(setup.captureCharFrame()).toContain("Saved version comparison"); expect(setup.captureCharFrame()).toContain("Saved user suffix.")
    setup.mockInput.pressEscape(); await flush(); await open(); await versions(1, "Restore as a new working version"); await until(async () => (await view()).versions.length === 3, "restored new version"); await flush()
    expect((await view()).text).toBe(original); expect(await readFile(source, "utf8")).toBe(original)
    setup.mockInput.pressKey("\x1b[F"); await setup.mockInput.pasteBracketedText("\nUncommitted recovery text."); await until(async () => !!(await view()).draft?.text?.includes("Uncommitted recovery text."), "retained file edit draft")
    setup.mockInput.pressEscape(); await flush(); expect(setup.captureCharFrame()).toContain("Unsaved file edits"); await choose("Keep edit draft and close")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await open()
    expect(editor().plainText).toBe(original + "\nUncommitted recovery text."); expect(client.draft.text).toBe("Keep independent composer")
    const before = await view(); await client.host.file({ action: "save", context, ref, expected: before.current, edit: { text: "Concurrent latest saved version." } })
    setup.mockInput.pressKey("s", { ctrl: true }); await until(() => !!client.issue, "stale file save refusal"); await flush()
    expect(editor().plainText).toBe(original + "\nUncommitted recovery text."); expect((await view()).text).toBe("Concurrent latest saved version."); expect((await view()).versions).toHaveLength(4)
    setup.mockInput.pressKey("F4"); await until(() => editor().plainText === "Concurrent latest saved version.", "explicit discard reloads latest")
    expect((await view()).draft).toBeFalsy(); expect(client.draft.text).toBe("Keep independent composer"); expect(client.state?.messages).toHaveLength(0); expect(await readFile(source, "utf8")).toBe(original)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
