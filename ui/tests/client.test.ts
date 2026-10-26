import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { DraftSnapshot, UploadedFile } from "@gator/application-host/contracts/frontend"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

const roots: string[] = [], clients: GatorClient[] = []
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), "gator-client-")); roots.push(root)
  const host = new FixtureHost({ binary: "/unused", workspace: join(root, "workspace"), userData: join(root, "app"), env: { GATOR_STATE_DIR: join(root, "state") } })
  const client = new GatorClient(host); clients.push(client)
  return { host, client }
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done }); return { resolve, promise } }
afterEach(async () => { for (const client of clients.splice(0)) { if (client.host instanceof FixtureHost) client.host.closeIssue = ""; await client.close() }; for (const root of roots.splice(0)) await rm(root, { recursive: true, force: true }) })

test("draft loads keep their original chat identity and never overwrite newer input", async () => {
  const { host, client } = await fixture(), first = deferred<DraftSnapshot>(), second = deferred<DraftSnapshot>()
  let reads = 0
  host.drafts = { request: async () => (++reads === 1 ? first : second).promise, flush: async () => {} }
  const load = client.loadDraft()
  host.change(state("second"))
  second.resolve({ enabled: true, drafts: [] }); await Bun.sleep(5)
  first.resolve({ enabled: true, drafts: [] }); await load
  expect(client.drafts.get("conversation:first")?.title).toBe("first")
  expect(client.drafts.get("conversation:second")?.title).toBe("second")
  expect(reads).toBe(2)
})

test("retained desktop draft metadata remains editable and attachment intake merges concurrent typing", async () => {
  const { host, client } = await fixture()
  await host.drafts.request({ action: "save", draft: { context: client.context, title: "first", source: "topic:first", text: "Before restart", uploads: [] } })
  await client.start(); await client.edit("Editable after restart")
  const intake = deferred<UploadedFile[]>()
  host.importFiles = async () => intake.promise
  const attaching = client.attach(["/fixture.txt"])
  await client.edit("Typed while importing")
  intake.resolve([{ id: "a".repeat(32) + "/fixture.txt", name: "fixture.txt", size: 1 }]); await attaching
  expect(client.draft.text).toBe("Typed while importing")
  expect(client.draft.uploads).toHaveLength(1)
  expect((await host.drafts.request({ action: "list" })).drafts[0].text).toBe("Typed while importing")
})

test("admission clears only the submitted snapshot; rejection preserves the draft", async () => {
  const { host, client } = await fixture(); await client.start(); await client.edit("Submitted snapshot")
  const admission = deferred<void>(); host.dispatch = async () => admission.promise
  const submitted = client.submit(); await client.edit("Newer typing")
  admission.resolve(); await submitted
  expect(client.draft.text).toBe("Newer typing")
  host.dispatch = async () => { throw new Error("Admission rejected") }
  await expect(client.submit()).rejects.toThrow("Admission rejected")
  expect(client.draft.text).toBe("Newer typing"); expect(client.pending.size).toBe(0)
})

test("action results match nonce and owner, ignore duplicates and clean up on close", async () => {
  const { host, client } = await fixture(); await client.start()
  const baseline = client.listenerCount("change")
  const result = client.result({ type: "key", key: "escape" }, "wanted", s => s.queue_result)
  host.change({ ...host.current, queue_result: { request_id: "old" } })
  expect(client.listenerCount("change")).toBe(baseline + 1)
  host.change({ ...host.current, queue_result: { request_id: "wanted" } })
  expect((await result).request_id).toBe("wanted")
  host.change({ ...host.current, queue_result: { request_id: "wanted", error: "duplicate" } })
  expect(client.listenerCount("change")).toBe(baseline)
  const switched = client.result({ type: "key", key: "escape" }, "later", s => s.queue_result).catch(error => error)
  host.change(state("second")); expect((await switched).message).toContain("original chat")
  const closing = client.result({ type: "key", key: "escape" }, "closing", s => s.queue_result).catch(error => error)
  await client.close(); expect((await closing).message).toContain("closed")
  expect(host.listenerCount("change")).toBe(0)
  expect(client.listenerCount("change")).toBe(0)
})

test("a refused close keeps subscriptions and allows a later clean shutdown", async () => {
  const { host, client } = await fixture(); await client.start(); host.closeIssue = "Finish the external step"
  await expect(client.close()).rejects.toThrow("external step")
  host.change(state("still-connected")); expect(client.state?.title).toBe("still-connected")
  expect(host.listenerCount("change")).toBe(1)
  host.closeIssue = ""; await Promise.all([client.close(), client.close()])
  expect(host.closed).toBe(true); expect(host.listenerCount("change")).toBe(0)
})

test("uncertain admissions reuse their identity and guide acknowledges the live task before clearing", async () => {
  const { host, client } = await fixture(); await client.start(); await client.edit("Unconfirmed input")
  host.dispatch = async () => { throw new Error("Lost acknowledgement") }
  await expect(client.submit()).rejects.toThrow("Lost acknowledgement")
  await expect(client.submit()).rejects.toThrow("Lost acknowledgement")
  expect(host.sent[0].upload?.request_id).toBe(host.sent[1].upload?.request_id)
  expect(client.draft.text).toBe("Unconfirmed input")
  host.dispatch = async frame => { if (frame.queue) host.change({ ...host.current, queue_result: { request_id: frame.queue.request_id } }) }
  host.change({ ...host.current, running: true, guidance_id: "a".repeat(64) })
  const sent = host.sent.length
  for (const phase of ["recording", "transcribing"]) {
    client.voicePhase = phase
    await expect(client.guide()).rejects.toThrow("Finish or cancel dictation")
    expect(host.sent).toHaveLength(sent)
    expect(client.draft.text).toBe("Unconfirmed input")
  }
  client.voicePhase = "idle"
  await client.guide()
  expect(host.sent.at(-1)?.queue?.action).toBe("guide"); expect(host.sent.at(-1)?.queue?.id).toBe(host.current.guidance_id)
  expect(client.draft.text).toBe("")
  await client.edit("Keep linked reply", [], "update-1")
  await expect(client.guide()).rejects.toThrow("text only")
  await client.edit(client.draft.text, [], null); expect(client.draft.reply_to_update_id).toBeUndefined()
})

test("first model setup and local hub navigation preserve drafts and do not submit work", async () => {
  const { host, client } = await fixture(); await client.start(); host.change({ ...host.current, provider: undefined })
  await client.edit("First task draft"); await client.submit()
  expect(host.sent).toEqual([{ type: "navigate", text: "/model" }]); expect(client.draft.text).toBe("First task draft")
  await client.navigate("/saved"); await client.navigate("/chat")
  expect(host.sent).toHaveLength(1); expect(client.route).toBe("chat")
})
