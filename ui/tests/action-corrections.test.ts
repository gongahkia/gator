import { test, expect } from "bun:test"
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"
import type { BackendState } from "@gator/application-host/contracts/protocol"
import type { FrontendFrame } from "@gator/application-host/contracts/frontend"

async function until(predicate: () => boolean, label: string) { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) } }

for (const delegated of [false, true]) test(`${delegated ? "delegated" : "local"} pending-action correction preserves its owner and requires a fresh exact review`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-action-correction-"))
  let effects = 0, modelCalls = 0, holding = false
  let release!: () => void
  const hold = new Promise<void>(resolve => { release = resolve })
  const routes: any[] = [], sent: string[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const body = await request.json() as any
    if (new URL(request.url).pathname.includes("/gmail/")) { effects++; sent.push(JSON.stringify(body)); return Response.json({ id: "corrected-send" }) }
    const system = body.messages.filter((message:any) => message.role === "system").map((message:any) => message.content).join("\n")
    let message: any
    if (system.includes("You handle the conversation with Gator")) {
      const context = JSON.parse(body.messages.at(-1).content); routes.push(context)
      message = { role: "assistant", content: JSON.stringify(context.message === "Send the venue update" ? { kind: "task" } : context.message === "Hello during review" ? { kind: "reply", text: "You can keep talking while the action waits." } : { kind: "revise_action", target: context.tasks.find((task:any) => task.can_correct_action).id, instruction: context.message }) }
    } else if (body.messages.some((message:any) => message.role === "user" && message.content === "Hold another task")) {
      holding = true; await hold; message = { role: "assistant", content: "The unrelated task finished." }
    } else {
      modelCalls++
      message = modelCalls <= 2 ? { role: "assistant", tool_calls: [{ id: "send-" + modelCalls, type: "function", function: { name: "connector_google_gmail_send", arguments: JSON.stringify({ payload: { from: "me@example.com", to: "venue@example.com", subject: modelCalls === 1 ? "Original subject" : "Corrected subject", body: "Preserved body" } }) } }] } : { role: "assistant", content: "The corrected update was sent. Inspect the receipt." }
      if (modelCalls === 2) expect(JSON.stringify(body.messages)).toContain("Use Corrected subject")
    }
    return Response.json({ choices: [{ message, finish_reason: message.tool_calls ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } })
  } })
  const base = `http://127.0.0.1:${server.port}`
  await mkdir(join(root, "config/gator"), { recursive: true })
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base + "/chat", models: ["fixture"], default_model: "fixture" }], connectors: [{ version: 1, id: "google", name: "Local fixture", kind: "google_workspace", resource: base, authentication: "none" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime"), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  try {
    await client.start()
    await client.send({ type: "submit", text: "/mode act" }); await client.send({ type: "submit", text: "/connector add google" })
    await until(() => !!client.state?.work_details?.connector_ids.includes("google"), "selected scope")
    const parent = client.state!.conversation_id
    if (delegated) {
      await client.edit("Hold another task"); await client.submit(); await until(() => holding, "independent original work")
    }
    await client.edit("Send the venue update"); await client.submit()
    const ownedWorkers = () => (client.host.sessions as unknown as { workers: Set<{ closed: boolean; state?: BackendState; apply(frame: FrontendFrame): Promise<unknown> }> }).workers
    const actionOwner = () => [...ownedWorkers()].find(worker => !worker.closed && (delegated ? worker.state?.conversation_id !== parent : worker.state?.conversation_id === parent))!
    await until(() => !!actionOwner()?.state?.pending_approval && (!delegated || !!client.state?.dialogue_peers?.some(peer => peer.can_correct_action)), "first proposal")
    const original = actionOwner().state!.pending_approval!, owner = client.state!.conversation_id
    await client.edit("Hello during review"); await client.submit()
    await until(() => client.state?.dialogue?.at(-1)?.state === "answered", "unrelated reply")
    expect(actionOwner().state?.pending_approval?.token).toBe(original.token)
    await client.edit("Use Corrected subject; preserve recipient and body"); await client.submit()
    await until(() => !!actionOwner().state?.pending_approval && actionOwner().state!.pending_approval!.token !== original.token && client.state?.dialogue?.at(-1)?.state === "guided", "replacement proposal")
    const fresh = actionOwner().state!.pending_approval!
    expect(client.state?.dialogue?.at(-1)?.state).toBe("guided")
    expect(client.state?.dialogue?.at(-1)?.response).toContain("fresh exact review")
    expect(client.state?.conversation_id).toBe(owner); expect(effects).toBe(0)
    expect(routes.at(-1).tasks.find((task:any) => task.can_correct_action).id).toBe(routes.find(route => route.message === "Hello during review").tasks.find((task:any) => task.can_correct_action).id)
    if (delegated) expect(client.state?.running).toBe(true)
    expect(JSON.stringify(routes)).not.toContain(original.token)
    await client.edit("An unrelated draft to retain")
    await actionOwner().apply({ type: "approval", interaction_id: original.id, approval_token: original.token, approved: true })
    expect(effects).toBe(0); expect(actionOwner().state?.pending_approval?.token).toBe(fresh.token)
    await actionOwner().apply({ type: "approval", interaction_id: fresh.id, approval_token: fresh.token, approved: true })
    await until(() => !actionOwner().state?.running && effects === 1, "fresh reviewed send")
    expect(modelCalls).toBe(3); expect(sent).toHaveLength(1)
    const actions = actionOwner().state?.messages.flatMap(message => message.bundle?.Actions ?? []) ?? []
    expect(actions.some(action => action.Status === "denied")).toBe(true)
    expect(actions.some(action => action.Status === "executed")).toBe(true)
    release(); await until(() => !client.snapshot.allChatsBusy, "all original owners settled")
    await client.close()
    client = new GatorClient(new Application(options)); await client.start(owner)
    expect(client.draft.text).toBe("An unrelated draft to retain")
    expect(client.state?.running).toBe(false); expect(client.state?.pending_approval).toBeFalsy()
    expect(effects).toBe(1); expect(modelCalls).toBe(3)
  } finally { release(); await client.close(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
