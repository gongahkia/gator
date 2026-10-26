import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, readdir, stat, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("private data keyboard reviews export verify and restore into a separate paused profile", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-data-")), workspace = join(root, "workspace"), backup = join(root, "private.tar.gz"), restored = join(root, "restored")
  await mkdir(workspace); await writeFile(join(workspace, "original.txt"), "External original is kept.")
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (args: string[], state = env.GATOR_STATE_DIR) => execFileSync(binary, args, { env: { ...env, GATOR_STATE_DIR: state, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
  cli(["note", "add", "--title", "Retained private note", "--body", "The original knowledge survives review."])
  const client = new GatorClient(new Application({ binary, env, workspace, userData: join(root, "app") }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + JSON.stringify(client.state?.panel)); await flush() } }
  const choose = async (label: string) => {
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === label), "available " + label)
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); setup.mockInput.pressEnter(); await flush()
    await until(() => client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label === label, "selected " + label)
    setup.mockInput.pressEnter(); await flush()
  }
  const path = async (label: string, value: string) => { await choose(label); setup.mockInput.pressKey("l", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(value); setup.mockInput.pressEnter(); await flush() }
  const review = async () => { const id = client.state?.panel?.id; setup.mockInput.pressKey("F2"); await until(() => !!client.state?.panel && client.state.panel.id !== id, "review form"); await flush() }
  const result = (text: string) => until(() => JSON.stringify(client.state?.panel?.blocks).includes(text), text)
  const exists = async (path: string) => !!await stat(path).catch(() => undefined)
  try {
    await client.start(); await client.edit("Keep private data draft"); const context = client.context
    await client.navigate("/data"); await choose("Export private backup"); await path("New backup file", backup); await review()
    expect(await exists(backup)).toBe(false); expect(JSON.stringify(client.state?.panel)).toContain(backup)
    setup.mockInput.pressEnter(); await result("Exported"); expect(await exists(backup)).toBe(true)
    const bytes = await readFile(backup)
    await choose("Verify a backup"); await path("Existing backup file", backup); await review(); setup.mockInput.pressEnter(); await result("Verified")
    expect(await readFile(backup)).toEqual(bytes)
    await choose("Restore into a new profile"); await path("Existing backup archive", backup); await path("New profile directory", restored)
    await review(); expect(await exists(restored)).toBe(false)
    setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Review restore into a fresh profile", "restore plan")
    expect(await exists(restored)).toBe(false); expect(JSON.stringify(client.state?.panel)).toContain(restored)
    setup.mockInput.pressEnter(); await result("Restored a separate profile")
    expect(client.context).toBe(context); expect(client.draft.text).toBe("Keep private data draft")
    expect(JSON.parse(await readFile(join(restored, "gator/usage/policy.json"), "utf8")).paused).toBe(true)
    expect(cli(["note", "list"], restored)).toContain("Retained private note")
    expect(await readFile(join(workspace, "original.txt"), "utf8")).toBe("External original is kept.")
    await choose("Choose content to delete"); await choose("Saved knowledge and retrieval cache")
    await until(() => client.state?.panel?.title === "Review private data removal", "deletion review")
    expect(cli(["note", "list"])).toContain("Retained private note")
    setup.mockInput.pressEscape(); await Bun.sleep(80); await flush()
    expect(cli(["note", "list"])).toContain("Retained private note")
    await client.navigate("/data"); await choose("Edit retention"); await choose("Completed history days (0 keep)")
    setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText("30"); setup.mockInput.pressEnter(); await flush(); await review()
    expect(JSON.stringify(client.state?.panel)).toContain("30"); setup.mockInput.pressEnter(); await result("Retention saved")
    expect(JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/data-control/retention.json"), "utf8")).HistoryDays).toBe(30)
    await client.navigate("/chat"); expect(client.draft.text).toBe("Keep private data draft"); expect(client.state?.messages).toHaveLength(0)
    expect(await readFile(backup)).toEqual(bytes)
    expect((await readdir(join(restored, "gator/data-control")))).toContain("restored.json")
    const restoredClient = new GatorClient(new Application({ binary, workspace, userData: join(root, "restored-app"), env: { ...env, GATOR_STATE_DIR: restored, GATOR_CONFIG_DIR: join(restored, "config") } }))
    try {
      await restoredClient.start(); expect(restoredClient.state?.messages).toHaveLength(0)
      await restoredClient.navigate("/knowledge"); await restoredClient.waitFor(state => !!state.panel?.rows?.some(row => row.label.includes("Retained private note")))
      expect(restoredClient.draft.text).toBe(""); expect(restoredClient.state?.running).toBe(false)
    } finally { await restoredClient.close() }
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 60000)
