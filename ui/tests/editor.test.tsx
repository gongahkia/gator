import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, readFile, writeFile, stat, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

// The existing Go editor command uses POSIX sh, including in the Windows preview.
test.skipIf(process.platform === "win32")("external editor returns owned text and files after success, failure and a concurrent chat change", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-editor-")), script = join(root, "editor.mjs"), control = join(root, "control.json"), ready = join(root, "ready.json"), release = join(root, "release")
  await writeFile(script, `import { readFile, writeFile, access } from 'node:fs/promises';
    const command = JSON.parse(await readFile(process.env.GATOR_EDITOR_CONTROL, 'utf8'));
    const path = process.argv[2];
    await writeFile(process.env.GATOR_EDITOR_READY, JSON.stringify({path, original: await readFile(path, 'utf8')}));
    if (command.block) while (!await access(process.env.GATOR_EDITOR_RELEASE).then(() => true, () => false)) await new Promise(resolve => setTimeout(resolve, 20));
    await writeFile(path, command.text); process.exitCode = command.fail ? 3 : 0;
  `)
  const quote = (value: string) => "'" + value.replaceAll("'", "'\\''") + "'"
  const source = join(root, "notes.txt"); await writeFile(source, "Retained attachment")
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, VISUAL: "", EDITOR: `${quote(process.execPath)} ${quote(script)}`, GATOR_EDITOR_CONTROL: control, GATOR_EDITOR_READY: ready, GATOR_EDITOR_RELEASE: release, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 36 })
  let setup = await render()
  await setup.renderer.setupTerminal()
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}; ${client.issue}; ${client.state?.notice}; ${JSON.stringify(client.state?.external)}; draft=${client.draft.text}`); await Bun.sleep(20) } }
  const invoke = async (command: { text: string; fail?: boolean; block?: boolean }) => {
    await rm(ready, { force: true }); await writeFile(control, JSON.stringify(command))
    setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await setup.mockInput.typeText("external-editor"); await flush(); setup.mockInput.pressEnter()
  }
  try {
    await client.start(); await client.attach([source]); await client.edit("Before editor")
    const originalContext = client.context
    await invoke({ text: "Edited λ\nSecond line" }); await until(() => client.draft.text === "Edited λ\nSecond line" && !client.snapshot.terminalBusy, "successful import"); await flush()
    const first = JSON.parse(await readFile(ready, "utf8")); expect(first.original).toBe("Before editor"); expect(await stat(first.path).catch(() => undefined)).toBeUndefined()
    expect(client.draft.uploads).toHaveLength(1); expect(client.state?.messages).toHaveLength(0)
    await setup.mockInput.pasteBracketedText(" typing after return"); await until(() => client.draft.text.includes("typing after return"), "resumed input")
    await invoke({ text: "Saved despite editor exit", fail: true }); await until(() => client.draft.text === "Saved despite editor exit" && client.issue.includes("status 3"), "failed editor keeps saved content"); await flush()
    expect(client.draft.uploads).toHaveLength(1); expect(client.snapshot.connection).toBe("connected")
    const oversized = "x".repeat(128 * 1024 + 1)
    await invoke({ text: oversized }); await until(() => !client.snapshot.terminalBusy && client.issue.includes("128 KiB"), "oversized editor recovery"); await flush()
    expect(client.draft.text).toBe("Saved despite editor exit")
    expect(setup.renderer.currentFocusedEditor?.plainText).toBe(oversized)
    setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("Shortened returned editor text"); setup.mockInput.pressEnter()
    await until(() => client.draft.text === "Shortened returned editor text", "explicit shortened import"); await flush()
    await invoke({ text: "Original chat editor result", block: true }); await until(async () => !!await stat(ready).catch(() => undefined), "blocked editor")
    expect(client.snapshot.terminalBusy).toBe(true); await expect(client.close()).rejects.toThrow("external step")
    await client.edit("Concurrent original chat typing"); await client.host.sessions.open({ new: true }); await client.edit("Other chat draft")
    const otherContext = client.context; await writeFile(release, "continue")
    await until(() => !client.snapshot.terminalBusy && !!client.drafts.get(originalContext)?.text.includes("Original chat editor result"), "owned editor return"); await flush()
    expect(client.context).toBe(otherContext); expect(client.draft.text).toBe("Other chat draft")
    expect(client.drafts.get(originalContext)?.text).toBe("Concurrent original chat typing\n\nOriginal chat editor result")
    expect(client.drafts.get(originalContext)?.uploads).toHaveLength(1)
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start()
    const saved = await client.host.drafts.request({ action: "list" })
    expect(saved.drafts.find(draft => draft.context === originalContext)?.text).toContain("Original chat editor result")
    expect(client.draft.text).toBe("Other chat draft"); expect(client.state?.messages).toHaveLength(0)
    await client.host.prepareComposerEditor(client.context, client.draft.text)
    const prepared = client.state!.external!.args.at(-1)!
    expect((await stat(prepared)).isFile()).toBe(true)
    await client.close(); expect(await stat(prepared).catch(() => undefined)).toBeUndefined()
  } finally { await writeFile(release, "continue"); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
