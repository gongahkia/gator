import { test, expect } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"
import { App } from "../src/app"

async function until(predicate: () => boolean, label: string) { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(`Timed out waiting for ${label}`); await Bun.sleep(20) } }
test("exact live approval is required before a connected send and stale tokens cannot approve", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-approval-")); let effects = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const url = new URL(request.url), payload = await request.json()
    if (url.pathname.includes("/gmail/")) { effects++; return Response.json({}) }
    const tools = Array.isArray(payload.tools) ? payload.tools : []
    const send = tools.some((tool: { function?: { name?: string } }) => tool.function?.name === "connector_google_gmail_send") && !payload.messages.some((message: { role: string }) => message.role === "tool")
    const message = send ? { role: "assistant", tool_calls: [{ id: "send-one", type: "function", function: { name: "connector_google_gmail_send", arguments: JSON.stringify({ payload: { from: "me@example.com", to: "venue@example.com", subject: "Venue update", body: "Exact reviewed body 🐊" } }) } }] } : { role: "assistant", content: "The send outcome is unconfirmed. Inspect Sent before trying again." }
    return Response.json({ choices: [{ message, finish_reason: send ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } })
  } })
  const base = `http://127.0.0.1:${server.port}`
  await mkdir(join(root, "config/gator"), { recursive: true })
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base + "/chat", models: ["fixture"], default_model: "fixture" }], connectors: [{ version: 1, id: "google", name: "Local fixture", kind: "google_workspace", resource: base, authentication: "none" }] }))
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  try {
    await client.start()
    await client.send({ type: "submit", text: "/mode act" }); await client.send({ type: "submit", text: "/connector add google" })
    await until(() => !!client.state?.work_details?.connector_ids.includes("google"), "connector scope")
    await client.edit("Send the reviewed venue update"); await client.submit()
    await until(() => !!client.state?.pending_approval, "live approval")
    const pending = client.state!.pending_approval!
    await setup.flush(); expect(effects).toBe(0)
    expect(setup.captureCharFrame()).toContain("Exact reviewed body")
    expect(setup.captureCharFrame()).toContain("venue@example.com")
    await client.send({ type: "approval", interaction_id: pending.id, approval_token: "f".repeat(64), approved: true })
    expect(effects).toBe(0); expect(client.state?.pending_approval?.token).toBe(pending.token)
    await setup.mockInput.pasteBracketedText("yes\n\u001b[13u")
    expect(effects).toBe(0)
    setup.mockInput.pressKey("y", { ctrl: true }); await setup.flush(); await Bun.sleep(25)
    expect(setup.captureCharFrame()).toContain("Approve this exact action?")
    setup.mockInput.pressEnter()
    await until(() => !client.state?.running && effects === 1, "owned external result")
    expect(client.state?.messages.some(message => message.bundle?.Actions?.some(receipt => receipt.Status === "unknown"))).toBe(true)
    expect(effects).toBe(1)
  } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 30000)
