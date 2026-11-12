import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean | Promise<boolean>, label: string) { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error("Timed out: " + label); await Bun.sleep(20) } }
async function fixture(holdSend = false, width = 120, height = 42) {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-receipts-")), requests: string[] = []
  let releaseSend = () => {}, releaseRead = () => {}, messageID = "", holdRead = false
  const readGate = new Promise<void>(resolve => { releaseRead = resolve })
  const gate = new Promise<void>(resolve => { releaseSend = resolve })
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const body = await request.text(), url = new URL(request.url)
    if (url.pathname.startsWith("/other/")) { requests.push("WRONG ACCOUNT " + url.pathname); return Response.json({}, { status: 403 }) }
    if (url.pathname.startsWith("/gmail/")) {
      requests.push(request.method + " " + url.pathname)
      if (request.method === "POST" && url.pathname.endsWith("/send")) {
        messageID = Buffer.from(JSON.parse(body).raw, "base64url").toString().match(/Message-ID: (.+)\r\n/)![1]
        if (holdSend) await gate
        return Response.json({})
      }
      if (url.pathname === "/gmail/v1/users/me/messages") { expect(url.searchParams.get("q")).toBe(`in:sent rfc822msgid:${messageID}`); if (holdRead) await readGate; return Response.json({ messages: [{ id: "sent-one" }] }) }
      if (url.pathname === "/gmail/v1/users/me/messages/sent-one") return Response.json({ id: "sent-one", labelIds: ["SENT"], payload: { headers: [{ name: "Message-ID", value: messageID }] } })
      return Response.json({}, { status: 404 })
    }
    const payload = JSON.parse(body), send = payload.tools?.some((tool: { function: { name: string } }) => tool.function.name === "connector_google_gmail_send") && !payload.messages.some((message: { role: string }) => message.role === "tool")
    const message = send ? { role: "assistant", tool_calls: [{ id: "send-one", type: "function", function: { name: "connector_google_gmail_send", arguments: JSON.stringify({ payload: { from: "me@example.com", to: "venue@example.com", subject: "Venue update", body: "Please review the new date." } }) } }] } : { role: "assistant", content: "The send outcome is not confirmed. Verify Sent before trying again." }
    return Response.json({ choices: [{ message, finish_reason: send ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } })
  } })
  await mkdir(join(root, "config/gator"), { recursive: true })
  const base = `http://127.0.0.1:${server.port}`
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base + "/chat", models: ["fixture"], default_model: "fixture" }], connectors: [{ version: 1, id: "google", name: "Local fixture", kind: "google_workspace", resource: base, authentication: "none" }] }))
  const configPath = join(root, "config/gator/config.json"), settings = JSON.parse(await readFile(configPath, "utf8"))
  settings.connectors.push({ ...settings.connectors[0], id: "google-other", name: "Other fixture account", resource: base + "/other" }); await writeFile(configPath, JSON.stringify(settings))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), options = { binary, userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  return {
    root, binary, requests, releaseSend, get client() { return client }, get setup() { return setup }, flush, choose,
    holdRead() { holdRead = true }, releaseRead,
    async denyRead(deny: boolean) { const current = JSON.parse(await readFile(configPath, "utf8")); current.connector_permissions = deny ? [{ connector_id: "google", operation: "gmail_get", read: "deny" }] : []; await writeFile(configPath, JSON.stringify(current)) },
    async start() { await client.start() },
    async restart() { await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start() },
    async prepare() {
      await client.send({ type: "submit", text: "/mode act" }); await client.send({ type: "submit", text: "/connector add google" })
      await until(() => !!client.state?.work_details?.connector_ids.includes("google"), "connector scope")
      await client.edit("Send the reviewed venue update"); await client.submit(); await until(() => !!client.state?.pending_approval, "approval")
      setup.mockInput.pressKey("y", { ctrl: true }); await flush(); setup.mockInput.pressEnter(); await until(() => requests.length === 1, "approved dispatch")
    },
    async receipt() { await client.navigate("/receipts"); await flush(); await choose("gmail_send") },
    async close() { releaseSend(); releaseRead(); await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) },
  }
}

test("uncertain send receipt checks are read only, immutable and retained across restart", async () => {
  const f = await fixture()
  try {
    await f.start(); await f.prepare(); await until(() => !f.client.state?.running, "unknown outcome")
    const origin = f.client.state!.conversation_id!, bundle = f.client.state!.messages.find(message => message.bundle?.Actions?.length)!.bundle!, manifest = join(bundle.Path, "manifest.json"), before = await readFile(manifest, "utf8")
    expect(bundle.Actions?.[0].Status).toBe("unknown"); expect(f.requests).toHaveLength(1)
    await f.receipt(); await f.choose("Inspect saved Sent check"); expect(f.setup.captureCharFrame()).toContain("No saved Sent check"); expect(f.requests).toHaveLength(1)
    f.setup.mockInput.pressEscape(); await Bun.sleep(75); await f.flush(); await f.receipt(); await f.choose("Check Sent (read only)")
    await until(() => f.client.state?.receipt_result?.check?.status === "found", "Sent match"); await f.flush()
    expect(f.setup.captureCharFrame()).toContain("not recipient delivery")
    expect(f.requests).toEqual(["POST /gmail/v1/users/me/messages/send", "GET /gmail/v1/users/me/messages", "GET /gmail/v1/users/me/messages/sent-one"])
    expect(await readFile(manifest, "utf8")).toBe(before); expect(f.client.state?.action_recoveries ?? []).toHaveLength(0)
    f.setup.mockInput.pressEscape(); await Bun.sleep(75); await f.client.navigate("/new"); await f.client.navigate("/receipts"); await f.flush()
    expect(f.setup.captureCharFrame()).not.toContain("gmail_send")
    f.setup.mockInput.pressEscape(); await Bun.sleep(75); await f.client.openConversation(origin, f.client.state!.recent_id!); await f.restart()
    await f.receipt(); await f.choose("Inspect saved Sent check"); await f.flush()
    expect(f.setup.captureCharFrame()).toContain("Found in Sent"); expect(f.requests).toHaveLength(3); expect(await readFile(manifest, "utf8")).toBe(before)
  } finally { await f.close() }
}, 45000)

test.skipIf(process.platform === "win32")("a crashed approved dispatch reopens only its owned receipt and never retries the send", async () => {
  const f = await fixture(true)
  try {
    await f.start(); await f.prepare(); const owner = f.client.state!.conversation_id!
    await f.client.edit("Unsent after dispatch")
    const root = join(f.root, "state/gator/external-actions"), runs = await readdir(root), run = runs.find(name => !name.startsWith("."))!, files = await readdir(join(root, run)), receipt = JSON.parse(await readFile(join(root, run, files.find(name => name.endsWith(".json"))!), "utf8"))
    expect(receipt.conversation_id).toBe(owner); expect(receipt.phase).toBe("executing"); expect(receipt.action.status).toBe("unknown")
    const children = execFileSync("ps", ["-axo", "pid=,ppid=,command="], { encoding: "utf8" }).split("\n").map(line => line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/)).filter(match => match && Number(match[2]) === process.pid && match[3].includes(f.binary) && match[3].includes("ui-backend"))
    expect(children).toHaveLength(1); process.kill(Number(children[0]![1]), "SIGKILL")
    await until(() => f.client.snapshot.connection === "disconnected", "owned runtime crash"); f.releaseSend(); await f.restart()
    await until(() => !!f.client.state?.action_recoveries?.length, "retained unknown action"); await f.flush()
    expect(f.client.state?.conversation_id).toBe(owner); expect(f.client.state?.action_recoveries?.[0].live).toBe(false)
    expect(f.client.draft.text).toBe("Unsent after dispatch"); expect(f.setup.captureCharFrame()).toContain("Reopening does not retry")
    await f.receipt(); expect(f.setup.captureCharFrame()).not.toContain("Check Sent (read only)")
    await f.choose("Inspect recorded request"); expect(f.setup.captureCharFrame()).toContain("venue@example.com")
    f.setup.mockInput.pressEscape(); await Bun.sleep(75); await f.client.navigate("/new"); expect(f.client.state?.action_recoveries ?? []).toHaveLength(0)
    await f.client.openConversation(owner, f.client.state!.recent_id!); await until(() => !!f.client.state?.action_recoveries?.length, "return to owner")
    expect(f.requests).toHaveLength(1)
  } finally { await f.close() }
}, 45000)

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) test(`contextual receipts retain original account, permission checks and cancelled-read outcomes at ${width}×${height}`, async () => {
  const f = await fixture(false, width, height)
  const open = async () => {
    await f.client.navigate("/chat")
    const bundle = f.client.state!.messages.find(message => message.bundle?.Actions?.length)!.bundle!
    const index = f.client.state!.messages.findIndex(message => message.run_id === bundle.RunID)
    expect(index).toBeGreaterThanOrEqual(0)
    f.client.emit("reveal-message", index); await f.flush(); f.setup.mockInput.pressKey("e", { ctrl: true }); await f.flush(); await f.choose("Request: gmail_send")
  }
  const readText = async (text: string) => {
    for (let page = 0; page < 30; page++) { await f.flush(); if (f.setup.captureCharFrame().replaceAll("│", " ").replace(/\s+/g, " ").includes(text)) return; f.setup.mockInput.pressKey("\x1b[6~") }
    throw new Error("Missing readable detail: " + text + "\n" + f.setup.captureCharFrame())
  }
  try {
    await f.start(); await f.prepare(); await until(() => !f.client.state?.running, "retained send outcome")
    const context = f.client.context, bundle = f.client.state!.messages.find(message => message.bundle?.Actions?.length)!.bundle!, path = join(bundle.Path, "manifest.json"), sealed = await readFile(path)
    await f.client.edit("Keep unrelated\neditable draft"); await f.client.setPlanFirst(true)
    // Future selection changes cannot substitute a different account for the
    // exact retained request. Neither the read nor its retry can send again.
    await f.client.send({ type: "submit", text: "/connector clear" }); await f.client.send({ type: "submit", text: "/connector add google-other" })
    expect(f.client.state?.work_details?.connector_ids).toEqual(["google-other"])
    await open(); await f.choose("Inspect recorded request"); await until(() => f.setup.captureCharFrame().includes("Read only"), "request details")
    await readText("Original account: google"); await readText("venue@example.com"); expect(f.requests).toHaveLength(1)
    await open(); await f.choose("Inspect saved Sent check"); await until(() => f.setup.captureCharFrame().includes("Read only"), "local saved check")
    await readText("No saved Sent check"); expect(f.requests).toHaveLength(1)
    f.holdRead(); await open(); await f.choose("Check Sent (read only)")
    await until(() => f.requests.length === 2 && !!f.client.state?.receipt_busy, "original mailbox read in flight")
    f.setup.mockInput.pressKey("c", { ctrl: true }); await f.flush(); await f.client.navigate("/settings")
    f.releaseRead(); await until(() => !f.client.state?.receipt_busy, "cancelled view check completes durably"); await f.flush()
    expect(f.client.route).toBe("settings"); expect(f.setup.renderer.root.findDescendantById("item-read-view")).toBeUndefined()
    expect(f.client.issue).toBe(""); expect(f.requests).toHaveLength(3)
    await f.client.navigate("/chat"); await open(); await f.choose("Inspect saved Sent check"); await until(() => f.setup.captureCharFrame().includes("Read only"), "saved completed read")
    await readText("not recipient delivery"); expect(f.requests).toHaveLength(3)
    await f.denyRead(true); await open(); await f.choose("Check Sent (read only)")
    await until(() => !!f.client.state?.receipt_result?.error, "revoked permission refused")
    await readText("Ctrl+R retry read"); expect(f.requests).toHaveLength(3)
    await f.denyRead(false); f.setup.mockInput.pressKey("r", { ctrl: true }); await f.flush()
    await until(() => !f.client.state?.receipt_busy && f.requests.length === 5, "explicit permission repair retry")
    await until(() => f.setup.captureCharFrame().includes("Read only"), "retry result visible")
    expect(f.requests.filter(request => request.startsWith("POST"))).toHaveLength(1)
    expect(f.requests.some(request => request.includes("WRONG ACCOUNT"))).toBe(false)
    expect(f.client.context).toBe(context); expect(f.client.draft.text).toBe("Keep unrelated\neditable draft"); expect(f.client.draft.plan_first).toBe(true)
    expect(await readFile(path)).toEqual(sealed)
    await f.restart(); await open(); await f.choose("Inspect saved Sent check"); await until(() => f.setup.captureCharFrame().includes("Read only"), "saved receipt after restart")
    await readText("Found in Sent"); expect(f.requests).toHaveLength(5); expect(f.client.draft.text).toBe("Keep unrelated\neditable draft")
  } finally { await f.close() }
}, 45000)
