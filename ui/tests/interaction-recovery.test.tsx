import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import type { FrontendFrame } from "@gator/application-host/contracts/frontend"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("contextual recovery retries the original admission once and reconnects without replaying work", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-interaction-recovery-"))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime")
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  let calls = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    await request.json(); calls++
    return Response.json({ choices: [{ message: { role: "assistant", content: "One retained result." }, finish_reason: "stop" }], usage: { total_tokens: 20 } })
  } })
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "recovery-fixture", "--base-url", `http://127.0.0.1:${server.port}/chat`, "--model", "fixture"])
  cli(["config", "set", "default-provider", "recovery-fixture"])
  const host = new Application({ binary, env, workspace: join(root, "workspace"), userData: join(root, "app") }), client = new GatorClient(host)
  // Test-only fault injection; backend transport remains outside the public UI contract.
  const runtime = host as Application & { backend: { submitUpload(frame: FrontendFrame): Promise<void>; stop(): Promise<void> } }
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" })
    const owner = runtime.backend, original = owner.submitUpload.bind(owner)
    let loseAcknowledgement = true
    owner.submitUpload = async frame => { await original(frame); if (loseAcknowledgement) { loseAcknowledgement = false; throw new Error("Acknowledgement lost after admission") } }
    await client.edit("Count this request once"); await flush(); setup.mockInput.pressEnter()
    await until(() => !!client.issue && !client.state?.running && calls === 1, "lost acknowledgement after completed work")
    expect(client.canRetrySubmission).toBe(true); expect(client.draft.text).toBe("Count this request once")
    expect(setup.captureCharFrame()).toContain("Ctrl+R recovery")
    setup.mockInput.pressKey("r", { ctrl: true }); await flush()
    await setup.mockInput.typeText("Retry this message acknowledgement"); await flush(); setup.mockInput.pressEnter()
    await until(() => client.draft.text === "", "original admission acknowledged")
    expect(client.canRetrySubmission).toBe(false); expect(calls).toBe(1)
    const conversation = client.state!.conversation_id!
    const journal = JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/conversations", conversation, "requests.json"), "utf8"))
    expect(journal.requests).toHaveLength(1)
    await client.edit("Keep this offline draft"); await owner.stop()
    await until(() => client.snapshot.connection === "disconnected", "connection lost"); await flush()
    expect(setup.captureCharFrame()).toContain("Reconnect before sending")
    setup.mockInput.pressKey("r", { ctrl: true }); await flush()
    expect(setup.captureCharFrame()).toContain("Reconnect and review")
    expect(setup.captureCharFrame()).not.toContain("Retry this message acknowledgement")
    setup.mockInput.pressEnter()
    await until(() => runtime.backend !== owner && client.snapshot.connection === "connected", "reconnected owner"); await flush()
    expect(client.context).toBe("conversation:" + conversation)
    expect(client.draft.text).toBe("Keep this offline draft"); expect(calls).toBe(1)
    expect(client.issue).toBe("")
  } finally { await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 30000)
