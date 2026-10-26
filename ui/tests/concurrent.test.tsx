import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean, label: string) { const deadline = Date.now() + 15000; while (!predicate()) { if (Date.now() > deadline) throw new Error(`Timed out: ${label}`); await Bun.sleep(15) } }
test("independent real Go chats retain work, queues, live guidance and drafts across keyboard switching and restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-concurrent-"))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const calls: string[] = []; let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const body = await request.text(), index = calls.push(body)
    if (index === 1) await gate
    return Response.json({ id: `concurrent-${index}`, choices: [{ message: { role: "assistant", content: `Finished isolated request ${index}.` }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } })
  } })
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "concurrent-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture"])
  cli(["config", "set", "default-provider", "concurrent-fixture"])
  const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
  const client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  try {
    await client.start(); await flush(); const first = client.state!.conversation_id!
    await client.send({ type: "submit", text: "/mode inspect" })
    await setup.mockInput.typeText("First concurrent task"); setup.mockInput.pressEnter(); await until(() => calls.length === 1, "first running")
    await client.edit("First queued follow-up"); await client.submit(); expect(client.state?.queue).toHaveLength(1)
    await client.edit("Keep first chat draft")
    setup.mockInput.pressKey("n", { ctrl: true }); await until(() => !!client.state?.conversation_id && client.state.conversation_id !== first && !client.snapshot.sessionSwitching, "new independent chat")
    await client.loadDraft(); await flush(); const second = client.state!.conversation_id!
    expect(client.draft.text).toBe(""); expect(client.snapshot.allChatsBusy).toBe(true)
    await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Second independent task"); await client.submit()
    await until(() => !!client.state?.messages.some(m => m.text.includes("Finished isolated request 2")) && !client.state.running, "second complete")
    expect(calls[1]).not.toContain("First concurrent task"); await client.edit("Keep second chat draft")
    await client.navigate("/chats"); await flush(); await setup.mockInput.typeText("First concurrent task"); await flush(); setup.mockInput.pressEnter()
    await until(() => client.state?.conversation_id === first && !client.snapshot.sessionSwitching, "switch first")
    await client.loadDraft(); expect(client.draft.text).toBe("Keep first chat draft"); expect(client.state?.running).toBe(true); expect(client.state?.queue).toHaveLength(1)
    await client.edit("Focus the original request"); await flush(); setup.mockInput.pressKey("g", { ctrl: true })
    await until(() => client.draft.text === "", "live guidance acknowledgement")
    expect(client.state?.status).toContain("Steering submitted"); expect(client.state?.queue).toHaveLength(1)
    await client.edit("Keep first chat draft"); await client.navigate("/conversations"); await flush()
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await setup.mockInput.typeText("Second independent task"); setup.mockInput.pressEnter()
    await until(() => client.state?.panel?.query === "Second independent task" && !client.state.catalog_busy && !client.state.panel.catalog?.loading && client.state.panel.rows?.[client.state.panel.selected ?? 0]?.label === "Second independent task", "acknowledged catalog result"); await flush(); setup.mockInput.pressEnter()
    await until(() => client.state?.conversation_id === second && !client.snapshot.sessionSwitching, "catalog switch second").catch(error => { throw new Error(error.message + "\n" + JSON.stringify({ issue: client.issue, panel: client.state?.panel, current: client.state?.conversation_id, second }) + "\n" + setup.captureCharFrame()) })
    await client.loadDraft()
    release(); await until(() => !client.snapshot.allChatsBusy, "all work finished")
    expect(client.state?.conversation_id).toBe(second); expect(client.draft.text).toBe("Keep second chat draft")
    expect(calls).toHaveLength(4); expect(calls[2]).toContain("Focus the original request"); expect(calls[3]).toContain("First queued follow-up")
    for (const body of calls.slice(2)) expect(body).not.toContain("Second independent task")
    expect(client.snapshot.chatActivity.some(row => row.id === first && row.status === "Finished")).toBe(true)
    const journal = JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/conversations", first, "requests.json"), "utf8"))
    expect(journal.requests.map((item: { state: string }) => item.state)).toEqual(["completed", "completed"])
    await client.close(); await setup.dispose()
    const restarted = new GatorClient(new Application(options))
    try { await restarted.start(); expect(restarted.state?.running).toBe(false); expect(restarted.draft.text).toBe("Keep second chat draft"); expect(calls).toHaveLength(4) }
    finally { await restarted.close() }
  } finally { release(); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
