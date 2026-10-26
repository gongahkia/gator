import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("revision navigation preserves ancestry and drafts and reopening from another directory keeps original source access", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-revisions-")), workspace = join(root, "original"), other = join(root, "other"), config = join(root, "config/gator")
  await mkdir(workspace); await mkdir(other); await mkdir(config, { recursive: true })
  await writeFile(join(workspace, "note.md"), "Original workspace evidence"); await writeFile(join(other, "note.md"), "Launch directory secret must not enter restored chat")
  const requests: { messages: { role: string; content: string }[] }[] = []
  let answers = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as typeof requests[number]; requests.push(payload)
    const read = payload.messages.at(-1)?.role === "tool"
    const message = read ? { role: "assistant", content: `Grounded result ${++answers}.` } : { role: "assistant", tool_calls: [{ id: "read-source", type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/note.md" }) } }] }
    return Response.json({ choices: [{ message, finish_reason: read ? "stop" : "tool_calls" }], usage: { total_tokens: 20 } })
  } })
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace, env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const command = async (group: string, label: string) => { setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await setup.mockInput.typeText(group); await flush(); setup.mockInput.pressEnter(); await flush(); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Read the original note."); await client.submit(); await until(() => !client.state?.running && answers === 1, "first revision")
    const id = client.state!.conversation_id!, snapshot = client.state?.work_details?.snapshot_id
    const selected = async () => JSON.parse(await readFile(join(root, "state/gator/conversations", id, "conversation.json"), "utf8")).head_revision_id as string
    const first = await selected(); expect(first).toBeTruthy()
    expect(JSON.stringify(requests)).toContain("Original workspace evidence")
    await writeFile(join(workspace, "note.md"), "Explicitly refreshed source evidence")
    await client.edit("Keep draft during source refresh"); await command("Files", "Refresh workspace snapshot next turn"); expect(client.draft.text).toBe("Keep draft during source refresh")
    await client.edit("Read the updated note."); await client.submit(); await until(() => !client.state?.running && answers === 2, "second revision")
    const second = await selected(); expect(second).not.toBe(first); expect(client.state?.work_details?.snapshot_id).not.toBe(snapshot); expect(JSON.stringify(requests)).toContain("Explicitly refreshed source evidence")
    await client.edit("Keep revision navigation draft"); const count = requests.length
    await command("Chats", "Move to the previous conversation revision"); await until(() => JSON.stringify(client.state?.messages).includes("Grounded result 1.") && !JSON.stringify(client.state?.messages).includes("Grounded result 2."), "previous revision")
    expect(JSON.stringify(client.state?.messages)).toContain("Grounded result 1."); expect(JSON.stringify(client.state?.messages)).not.toContain("Grounded result 2."); expect(client.draft.text).toBe("Keep revision navigation draft"); expect(await selected()).toBe(first)
    await command("Chats", "Move to the next conversation revision"); await until(() => JSON.stringify(client.state?.messages).includes("Grounded result 2."), "next revision"); expect(JSON.stringify(client.state?.messages)).toContain("Grounded result 2."); expect(requests).toHaveLength(count)
    await client.close(); await setup.dispose(); client = new GatorClient(new Application({ ...options, workspace: other })); setup = await render(); await client.start(); await flush()
    expect(client.state?.conversation_id).toBe(id); expect(client.state?.source).toBe(workspace); expect(await selected()).toBe(second); expect(client.draft.text).toBe("Keep revision navigation draft"); expect(requests).toHaveLength(count)
    expect(JSON.stringify(requests)).not.toContain("Launch directory secret")
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
