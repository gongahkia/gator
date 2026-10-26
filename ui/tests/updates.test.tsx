import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import type { ScrollBoxRenderable } from "@opentui/core"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

const zero = "0001-01-01T00:00:00Z"
const hasTime = (value?: string) => !!value && value !== zero
async function until(predicate: () => boolean | Promise<boolean>, label: string) { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error("Timed out: " + label); await Bun.sleep(20) } }
test("chat updates keep delivery, read, done, acknowledged feedback and retained replies independent", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-updates-")), calls: string[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) { calls.push(await request.text()); return Response.json({ choices: [{ message: { role: "assistant", content: `Retained reply ${calls.length}.` }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } }) } })
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }, options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "updates-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture"]); cli(["config", "set", "default-provider", "updates-fixture"])
  let host = new Application(options), client = new GatorClient(host)
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 40 })
  let setup = await render(), release = () => {}
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const action = async (title: string, action?: string) => { await client.navigate("/updates"); await flush(); await choose(title); if (action) await choose(action) }
  const inbox = join(env.GATOR_STATE_DIR, "gator/inbox"), entryPath = (id: string) => join(inbox, id + ".json"), read = async (id: string) => JSON.parse(await readFile(entryPath(id), "utf8"))
  try {
    await client.start(); const origin = client.state!.conversation_id!
    await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Plan the offsite"); await client.submit()
    await until(() => !client.state?.running && calls.length === 1, "first answer")
    await client.navigate("/new"); await client.edit("Other chat draft"); const other = client.state!.conversation_id!
    await mkdir(inbox, { recursive: true })
    const entries = [
      { id: "inbox-offsite", title: "Offsite venue", summary: "Choose the garden or the library.", status: "completed" },
      { id: "inbox-failed", title: "Calendar check", summary: "The calendar connection could not be reached.", status: "failed" },
      { id: "inbox-interrupted", title: "Venue check", summary: "The computer stopped before the check finished.", status: "interrupted" },
      { id: "inbox-private", title: "Private elsewhere", summary: "PRIVATE OTHER CHAT", status: "completed", origin_conversation_id: "work-private-other" },
    ]
    for (const [index, entry] of entries.entries()) await writeFile(entryPath(entry.id), JSON.stringify({ version: 1, kind: "job", origin_conversation_id: origin, job_id: "job-offsite", created_at: new Date(Date.now() + index).toISOString(), evidence: ["source/offsite.md:12"], ...entry }))
    await until(() => client.snapshot.chatActivity.some(row => row.id === origin), "owned attention")
    await client.navigate("/following"); await flush()
    expect(hasTime((await read("inbox-offsite")).delivered_at)).toBe(false); expect(client.draft.text).toBe("Other chat draft")
    await client.navigate("/chat"); await client.openSession(origin)
    await until(() => client.state?.chat_updates?.length === 3, "own updates"); await flush()
    const transcript = setup.renderer.root.findDescendantById("transcript") as ScrollBoxRenderable
    transcript.scrollTo(transcript.scrollHeight); await flush()
    await until(async () => hasTime((await read("inbox-interrupted")).delivered_at), "visible delivery")
    expect(hasTime((await read("inbox-interrupted")).read_at)).toBe(false)
    expect(hasTime((await read("inbox-interrupted")).resolved_at)).toBe(false)
    expect(setup.captureCharFrame()).toContain("Check interrupted")
    expect(setup.captureCharFrame()).not.toContain("PRIVATE OTHER CHAT")
    await action("Calendar check", "Mark read"); await action("Calendar check", "Mark done")
    let failed = await read("inbox-failed"); expect(failed.status).toBe("failed"); expect(hasTime(failed.read_at)).toBe(true); expect(hasTime(failed.resolved_at)).toBe(true)
    await action("Calendar check")
    await writeFile(entryPath("inbox-failed"), JSON.stringify({ ...failed, resolution_version: 3 }))
    await choose("Reopen"); await until(() => client.issue.includes("this update changed"), "stale resolution refusal")
    expect((await read("inbox-failed")).resolution_version).toBe(3)
    await action("Calendar check", "Inspect update"); await flush(); expect(setup.captureCharFrame()).toContain("source/offsite.md:12")
    setup.mockInput.pressEscape(); await Bun.sleep(75); await flush()
    await action("Offsite venue")
    // Hold only renderer notifications. The durable Go rating write completes;
    // success must wait for the matching delivered result, rather than send().
    const emit = host.emit.bind(host)
    host.emit = (name, ...args) => name === "change" && host.snapshot().state?.chat_update_result ? true : emit(name, ...args)
    release = () => { host.emit = emit; emit("change") }
    await choose("Feedback: useful")
    await until(async () => (await read("inbox-offsite")).rating === "useful", "durable feedback")
    expect(client.updateFeedback.get(client.context)?.saving).toBe(true)
    expect(client.state?.chat_updates?.find(update => update.id === "inbox-offsite")?.rating).toBeUndefined()
    expect(setup.captureCharFrame()).toContain("Saving feedback")
    await client.navigate("/new"); expect(client.state?.conversation_id).toBe(origin)
    release(); await until(() => client.updateFeedback.get(client.context)?.message === "Feedback saved.", "feedback acknowledgement")
    await action("Offsite venue")
    const entry = await read("inbox-offsite"); await writeFile(entryPath("inbox-offsite"), JSON.stringify({ ...entry, rating: "wrong", rating_version: 2 }))
    await choose("Feedback: noise"); await until(() => client.issue.includes("feedback changed"), "stale rating refusal")
    expect((await read("inbox-offsite")).rating).toBe("wrong")
    await action("Offsite venue", "Feedback: clear"); await action("Offsite venue", "Feedback: noise")
    expect((await read("inbox-offsite")).rating).toBe("noise")
    expect(hasTime((await read("inbox-offsite")).read_at)).toBe(false); expect(hasTime((await read("inbox-offsite")).resolved_at)).toBe(false)
    await action("Offsite venue", "Reply in original chat"); await client.edit("Use the second option")
    await client.openConversation(other, client.state!.recent_id!); expect(client.draft.reply_to_update_id).toBeUndefined(); expect(client.draft.text).toBe("Other chat draft")
    await client.openConversation(origin, client.state!.recent_id!); expect(client.draft.reply_to_update_id).toBe("inbox-offsite")
    await client.close(); await setup.dispose(); host = new Application(options); client = new GatorClient(host); setup = await render(); await client.start()
    expect(client.draft.text).toBe("Use the second option"); expect(client.draft.reply_to_update_id).toBe("inbox-offsite")
    expect(client.state?.chat_updates?.find(update => update.id === "inbox-offsite")?.rating).toBe("noise"); expect(calls).toHaveLength(1)
    await client.send({ type: "submit", text: "/mode inspect" }); await client.submit()
    await until(() => !client.state?.running && calls.length === 2, "explicit reply")
    expect(calls[1]).toContain("selected_update_id"); expect(calls[1]).toContain("inbox-offsite"); expect(calls[1]).not.toContain("PRIVATE OTHER CHAT")
    const journal = JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/conversations", origin, "requests.json"), "utf8")), reply = journal.requests.find((request: { text: string }) => request.text === "Use the second option")
    expect(reply.options.ReplyToUpdateID).toBe("inbox-offsite")
    await client.send({ type: "upload_submit", upload: { request_id: reply.client_id, context: "conversation:" + origin, text: reply.text, files: [], reply_to_update_id: "inbox-offsite" } }); expect(calls).toHaveLength(2)
    await flush()
    const order = (setup.renderer.root.findDescendantById("transcript") as ScrollBoxRenderable).content.getChildren().map(child => child.id)
    const replyIndex = client.state!.messages.findIndex(message => message.text === "Use the second option")
    expect(replyIndex).toBeGreaterThan(-1)
    expect(order.indexOf("update-inbox-offsite")).toBeGreaterThan(-1)
    expect(order.indexOf("update-inbox-offsite")).toBeLessThan(order.indexOf(`message-${replyIndex}`))
  } finally { release(); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)

test("update paging only acknowledges visible cards and check health stays with its owning chat", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-check-health-"))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }, client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); const origin = client.state!.conversation_id!
    await client.edit("Keep independent draft"); await client.navigate("/saved"); await flush()
    const inbox = join(env.GATOR_STATE_DIR, "gator/inbox"); await mkdir(inbox, { recursive: true })
    for (let index = 0; index < 53; index++) await writeFile(join(inbox, `inbox-${index}.json`), JSON.stringify({ version: 1, id: `inbox-${index}`, kind: "job", title: `Update ${index}`, summary: "Retained exact result " + index, status: "completed", origin_conversation_id: origin, job_id: "job-fixture", created_at: new Date(Date.now() - 60000 + index * 1000).toISOString() }))
    const added = execFileSync(binary, ["job", "add", "Project deadline", "--schedule", "*/15 * * * *", "--source", options.workspace, "--mode", "inspect", "--", "Check project deadline"], { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
    const id = added.match(/Added job (job-[a-zA-Z0-9-]+)/)![1], jobs = join(env.GATOR_STATE_DIR, "gator/jobs"), definitionPath = join(jobs, "definitions", id + ".json"), now = Date.now()
    const definition = JSON.parse(await readFile(definitionPath, "utf8")); await writeFile(definitionPath, JSON.stringify({ ...definition, origin_conversation_id: origin, created_at: new Date(now - 7200000).toISOString() }))
    await writeFile(join(jobs, "availability.json"), JSON.stringify({ LastSeen: new Date(now - 1200000).toISOString(), Gaps: [{ From: new Date(now - 3600000).toISOString(), To: new Date(now - 1200000).toISOString() }] }))
    await mkdir(join(jobs, "history", id), { recursive: true }); await writeFile(join(jobs, "history", id, "attempt-limited.json"), JSON.stringify({ version: 2, id: "attempt-limited", job_id: id, status: "limited", summary: "Daily model request limit reached", started_at: new Date(now - 1000).toISOString(), finished_at: new Date(now).toISOString() }))
    await until(() => client.state?.chat_updates_total === 53 && !!client.state.chat_checks?.length, "paged updates and health")
    const read = async (index: number) => JSON.parse(await readFile(join(inbox, `inbox-${index}.json`), "utf8"))
    expect(hasTime((await read(52)).delivered_at), "hub opening is not delivery").toBe(false)
    await client.navigate("/inbox"); await flush()
    expect(client.state?.panel).toBeDefined()
    expect(hasTime((await read(52)).read_at), "Inbox opening does not mark read").toBe(false)
    await client.navigate("/chat"); await flush()
    expect(client.state?.panel).toBeUndefined()
    // Returning can restore an earlier reading position; explicitly reveal the
    // latest cards before expecting delivery. Jumping does not expose the middle.
    setup.mockInput.pressKey("\x1b[1;5F"); await flush()
    await until(async () => { await flush(); return hasTime((await read(52)).delivered_at) }, "visible latest card")
    expect(hasTime((await read(25)).delivered_at), "offscreen middle page card stays undelivered").toBe(false); expect(hasTime((await read(52)).read_at), "delivery does not mark read").toBe(false)
    await client.navigate("/updates"); await flush(); await choose("older updates")
    expect(client.state?.chat_updates).toHaveLength(3); expect(!!client.state?.updates_have_older).toBe(false); expect(client.state?.updates_have_newer).toBe(true)
    await client.navigate("/updates"); await flush(); await choose("newer updates")
    expect(client.state?.chat_updates).toHaveLength(50)
    setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await choose("check-health")
    const screen = setup.captureCharFrame()
    expect(screen).toContain("offline"); expect(screen).toContain("No successful check recorded")
    expect(screen).toContain("Daily model request limit reached"); expect(screen).toContain("Observed supervisor gap")
    expect(screen).toMatch(/does not establish what\s+changed/)
    setup.mockInput.pressEscape(); await Bun.sleep(75); await client.navigate("/new"); await flush()
    expect(client.state?.chat_checks ?? []).toHaveLength(0); expect(client.state?.chat_updates ?? []).toHaveLength(0)
    expect(client.drafts.get("conversation:" + origin)?.text).toBe("Keep independent draft")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
