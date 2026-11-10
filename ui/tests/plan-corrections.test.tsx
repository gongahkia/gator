import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { randomUUID } from "node:crypto"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

const correction = "Keep the venue comparison under $2,000"
const originalPlan = { outcome: "A reviewed venue comparison", steps: [{ id: "compare", text: "Compare venues within the original selected scope" }], sources: [], effects: ["read"] }
const correctedPlan = { ...originalPlan, outcome: "Venue comparison under $2,000", steps: [{ id: "compare", text: "Compare venues within the $2,000 budget and original selected scope" }] }
const system = (body: any) => body.messages.find((message: any) => message.role === "system")?.content || ""
const metadata = (body: any) => JSON.parse(body.messages.at(-1).content)
async function until(predicate: () => boolean, label: string) { const end = Date.now() + 15000; while (!predicate()) { if (Date.now() > end) throw new Error("Timed out: " + label); await Bun.sleep(15) } }

async function fixture(respond: (body: any) => string | Response | Promise<string | Response>) {
  const root = await mkdtemp(join(tmpdir(), "gator-plan-corrections-")), workspace = join(root, "workspace")
  await mkdir(workspace)
  const requests: any[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const body = await request.json(); requests.push(body)
    const content = await respond(body)
    if (content instanceof Response) return content
    return Response.json({ choices: [{ message: { role: "assistant", content }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 10 } })
  } })
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "plan-correction-fixture", "--base-url", `http://127.0.0.1:${server.port}/v1/chat/completions`, "--model", "fixture"])
  cli(["config", "set", "default-provider", "plan-correction-fixture"])
  const options = { binary, env, workspace, userData: join(root, "app") }, client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const choose = async (text: string) => { await setup.mockInput.typeText(text); await flush(); setup.mockInput.pressEnter(); await flush() }
  const send = async (text: string) => {
    await client.edit(text); await client.submit()
    await until(() => client.state?.dialogue?.at(-1)?.text === text && !client.state?.dialogue_busy, text)
    if (client.state?.dialogue?.at(-1)?.issue) throw new Error(client.state.dialogue.at(-1)!.issue)
    await flush()
  }
  const plan = async () => {
    const brief = join(root, "brief.txt"); await writeFile(brief, "Original private brief: silver otter")
    await client.attach([brief]); const input = client.draft.uploads![0]!.id
    await client.navigate("/plan-first"); await flush(); await choose("Prepare a plan")
    await client.edit("Compare venues"); await client.submit()
    return input
  }
  const review = async () => { await client.navigate("/task-plans"); await flush(); await choose(correctedPlan.outcome) }
  const close = async () => { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
  return { root, env, options, client, setup, requests, flush, choose, send, plan, review, close }
}

test("an empty plan-first draft survives restart and keyboard submission still prepares review", async () => {
  let proposals = 0, executions = 0
  const f = await fixture(body => {
    if (system(body).includes("Propose a plan")) { proposals++; return JSON.stringify(originalPlan) }
    executions++; return "Unexpected execution before review"
  })
  let restarted: GatorClient | undefined, setup: Awaited<ReturnType<typeof testRender>> | undefined
  try {
    await f.client.start(); await f.flush()
    const owner = f.client.state!.conversation_id!
    await f.client.navigate("/plan-first"); await f.flush(); await f.choose("Prepare a plan")
    expect(f.client.draft.text).toBe(""); expect(f.client.draft.plan_first).toBe(true)
    await f.client.close(); await f.setup.dispose()
    const client = restarted = new GatorClient(new Application(f.options))
    setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
    const flush = async () => { await setup!.flush(); await Bun.sleep(25); await setup!.flush() }
    await client.start(); await flush()
    expect(client.state?.conversation_id).toBe(owner); expect(client.draft.plan_first).toBe(true)
    expect(proposals).toBe(0); expect(executions).toBe(0)
    await setup.mockInput.typeText("Compare venues after restart"); await flush()
    for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
      setup.resize(width, height); await flush()
      expect(setup.captureCharFrame()).toContain("Plan first")
      expect(setup.renderer.root.findDescendantById("transcript")!.height).toBeGreaterThanOrEqual(4)
      expect(client.draft.text).toBe("Compare venues after restart")
    }
    setup.mockInput.pressEnter(); await flush()
    await until(() => client.state?.task_plans?.[0]?.phase === "review" && !client.state.running, "restored draft prepares review")
    expect(proposals).toBe(1); expect(executions).toBe(0)
    expect(client.state?.task_plans?.[0]?.task).toBe("Compare venues after restart")
    expect(client.draft.text).toBe(""); expect(client.draft.plan_first).toBeUndefined()
    expect((await client.host.drafts.request({ action: "list" })).drafts).toHaveLength(0)
  } finally { if (restarted) await restarted.close(); if (setup && !setup.renderer.isDestroyed) await setup.dispose(); await f.close() }
}, 60000)

test("ordinary corrections revise the same plan while conversation continues, then require a fresh keyboard review", async () => {
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  const proposals: any[] = [], executions: string[] = []
  const f = await fixture(async body => {
    if (system(body).includes("You handle the conversation with Gator")) {
      const context = metadata(body)
      return JSON.stringify(context.message === "Hello during the revision" ? { kind: "reply", text: "You can keep talking while I revise the plan." } : { kind: "revise_plan", target: context.tasks.find((task: any) => task.can_revise_plan).id, instruction: context.message })
    }
    if (system(body).includes("Propose a plan")) {
      const meta = metadata(body); proposals.push(meta)
      if (meta.correction) { await gate; return JSON.stringify(correctedPlan) }
      return JSON.stringify(originalPlan)
    }
    executions.push(JSON.stringify(body)); return "The reviewed venue comparison respects the $2,000 budget."
  })
  try {
    await f.client.start(); await f.flush(); await f.client.send({ type: "submit", text: "/mode inspect" })
    const owner = f.client.state!.conversation_id!, input = await f.plan()
    await until(() => f.client.state?.task_plans?.[0]?.phase === "review" && !f.client.state?.running, "original review")
    const original = f.client.state!.task_plans![0]!
    await f.send(correction)
    await until(() => proposals.some(plan => plan.correction === correction) && f.client.state?.task_plans?.[0]?.phase === "revising", "revision running")
    expect(f.client.state?.dialogue?.at(-1)?.state).toBe("editing_plan")
    expect(f.client.state?.task_plans?.[0]?.id).toBe(original.id); expect(executions).toHaveLength(0)
    await f.send("Hello during the revision")
    expect(f.client.state?.dialogue?.at(-1)?.state).toBe("answered"); expect(f.client.state?.running).toBe(true)
    const futureBrief = join(f.root, "future-note.txt"); await writeFile(futureBrief, "Future draft only: amber finch")
    await f.client.attach([futureBrief]); const futureInput = f.client.draft.uploads![0]!.id
    const nextDraft = "Keep my next question\nSecond line for the next task"
    await f.client.edit(nextDraft); await f.client.setPlanFirst(true)
    for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) { f.setup.resize(width, height); await f.flush(); expect(f.setup.captureCharFrame()).toContain("Keep my next question"); expect(f.setup.renderer.root.findDescendantById("transcript")!.height).toBeGreaterThanOrEqual(4) }
    f.setup.resize(110, 34); release()
    await until(() => !f.client.state?.running && f.client.state?.task_plans?.[0]?.revision === 3 && f.client.state?.dialogue?.[0]?.state === "guided", "revised proposal saved")
    expect(proposals).toHaveLength(2); expect(proposals[1].previous_plan.outcome).toBe(originalPlan.outcome)
    expect(proposals[1].work_limits).toEqual(proposals[0].work_limits); expect(proposals[1].selected_source_ids).toEqual(proposals[0].selected_source_ids)
    expect(executions).toHaveLength(0); expect(f.client.state?.conversation_id).toBe(owner)
    const stale = randomUUID()
    await expect(f.client.result({ type: "task_plan", task_plan: { conversation_id: owner, task_id: original.id, revision: original.revision, digest: original.digest, action: "approve", request_id: stale } }, stale, state => state.task_plan_result)).rejects.toThrow()
    expect(executions).toHaveLength(0); expect(f.client.draft.text).toBe(nextDraft)
    await f.review(); await f.choose("Read exact proposed plan")
    expect(f.setup.captureCharFrame()).toContain(correction); expect(f.setup.captureCharFrame()).toContain("brief.txt")
    f.setup.mockInput.pressEscape(); await f.flush()
    await f.review(); await f.choose("Approve plan and start"); f.setup.mockInput.pressEnter()
    await until(() => !f.client.state?.running && f.client.state?.task_plans?.[0]?.state === "completed", "freshly reviewed execution")
    expect(executions).toHaveLength(1); expect(executions[0]).toContain("Retained user correction"); expect(executions[0]).toContain("silver otter"); expect(executions[0]).not.toContain("amber finch")
    const journal = JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR, "gator/conversations", owner, "requests.json"), "utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].id).toBe(original.id)
    expect(journal.requests[0].options.Uploads).toEqual([input]); expect(journal.requests[0].text).toBe("Compare venues")
    expect(f.client.draft.text).toBe(nextDraft); expect(f.client.draft.uploads?.map(file => file.id)).toEqual([futureInput]); expect(f.client.draft.plan_first).toBe(true)
    await f.client.close(); await f.setup.dispose()
    const count = f.requests.length, restarted = new GatorClient(new Application(f.options))
    try { await restarted.start(); expect(restarted.state?.conversation_id).toBe(owner); expect(restarted.state?.dialogue?.[0]?.state).toBe("guided"); expect(restarted.draft.text).toBe(nextDraft); expect(restarted.draft.uploads?.map(file => file.id)).toEqual([futureInput]); expect(restarted.draft.plan_first).toBe(true); expect(f.requests).toHaveLength(count) }
    finally { await restarted.close() }
  } finally { release(); await f.close() }
}, 60000)

test("failed plan corrections retain the previous draft and retry only after another explicit message", async () => {
  let revisionCalls = 0, executions = 0
  const f = await fixture(body => {
    if (system(body).includes("You handle the conversation with Gator")) { const context = metadata(body); return JSON.stringify({ kind: "revise_plan", target: context.tasks.find((task: any) => task.can_revise_plan).id, instruction: context.message }) }
    if (system(body).includes("Propose a plan")) {
      if (metadata(body).correction && ++revisionCalls === 1) return new Response("Fixture provider is temporarily unavailable", { status: 503 })
      return JSON.stringify(metadata(body).correction ? correctedPlan : originalPlan)
    }
    executions++; return "Unexpected unreviewed execution"
  })
  try {
    await f.client.start(); await f.flush(); await f.client.send({ type: "submit", text: "/mode inspect" })
    const input = await f.plan()
    await until(() => f.client.state?.task_plans?.[0]?.phase === "review" && !f.client.state.running, "original plan")
    const original = f.client.state!.task_plans![0]!, owner = f.client.state!.conversation_id!
    await f.client.edit(correction); await f.client.submit()
    await until(() => f.client.state?.dialogue?.[0]?.state === "failed" && !f.client.state.running, "failed correction retained")
    expect(revisionCalls).toBe(1); expect(executions).toBe(0)
    expect(f.client.state?.dialogue?.[0]?.issue).toContain("previous plan is retained")
    expect(f.client.state?.task_plans?.[0]?.outcome).toBe(originalPlan.outcome)
    expect(f.client.state?.task_plans?.[0]?.phase).toBe("review")
    const stale = randomUUID()
    await expect(f.client.result({ type: "task_plan", task_plan: { conversation_id: owner, task_id: original.id, revision: original.revision, digest: original.digest, action: "approve", request_id: stale } }, stale, state => state.task_plan_result)).rejects.toThrow()
    await f.client.edit("Keep this draft after failure")
    await f.client.navigate("/task-plans"); await f.flush(); await f.choose(originalPlan.outcome); await f.choose("Read exact proposed plan")
    expect(f.setup.captureCharFrame()).toContain("Requested correction (not applied)")
    expect(f.setup.captureCharFrame()).toContain("HTTP 503")
    f.setup.mockInput.pressEscape(); await f.flush(); expect(revisionCalls).toBe(1)
    await f.send(correction)
    await until(() => f.client.state?.dialogue?.at(-1)?.state === "guided" && !f.client.state.running, "explicit new correction succeeded")
    expect(revisionCalls).toBe(2); expect(executions).toBe(0)
    expect(f.client.state?.dialogue?.[0]?.state).toBe("failed")
    expect(f.client.state?.task_plans?.[0]?.revision).toBe(4)
    const journal = JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR, "gator/conversations", owner, "requests.json"), "utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].id).toBe(original.id)
    expect(journal.requests[0].options.Uploads).toEqual([input]); expect(journal.requests[0].plan.approval).toBeUndefined()
  } finally { await f.close() }
}, 60000)

test("conversational cancellation stops an in-flight plan correction without reviving its proposal", async () => {
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  let revising = false, executions = 0
  const f = await fixture(async body => {
    if (system(body).includes("You handle the conversation with Gator")) {
      const context = metadata(body)
      return JSON.stringify(context.message === "Cancel this plan" ? { kind: "stop", target: context.tasks.find((task: any) => task.can_stop).id } : { kind: "revise_plan", target: context.tasks.find((task: any) => task.can_revise_plan).id, instruction: context.message })
    }
    if (system(body).includes("Propose a plan")) {
      if (metadata(body).correction) { revising = true; await gate; return JSON.stringify(correctedPlan) }
      return JSON.stringify(originalPlan)
    }
    executions++; return "Unexpected execution"
  })
  try {
    await f.client.start(); await f.flush(); await f.client.send({ type: "submit", text: "/mode inspect" }); await f.plan()
    await until(() => f.client.state?.task_plans?.[0]?.phase === "review" && !f.client.state.running, "original review")
    await f.send(correction); await until(() => revising && f.client.state?.task_plans?.[0]?.phase === "revising", "provider preparing correction")
    await f.send("Cancel this plan")
    await until(() => !f.client.state?.running && f.client.state?.dialogue?.[0]?.state === "interrupted", "revision cancellation retained")
    expect(f.client.state?.dialogue?.at(-1)?.state).toBe("stopped")
    expect(f.client.state?.task_plans?.[0]?.state).toBe("discarded")
    expect(f.client.state?.dialogue?.[0]?.issue).toContain("cancelled before implementation")
    release(); await f.flush(); expect(executions).toBe(0)
    expect(f.client.state?.task_plans?.[0]?.outcome).toBe(originalPlan.outcome)
    expect(f.client.state?.task_plans?.[0]?.state).toBe("discarded")
  } finally { release(); await f.close() }
}, 60000)

test("a delegated plan correction returns its real result to the requesting chat without disturbing other work", async () => {
  let releaseMain!: () => void, releaseRevision!: () => void
  const mainGate = new Promise<void>(resolve => { releaseMain = resolve }), revisionGate = new Promise<void>(resolve => { releaseRevision = resolve })
  let work = 0
  const proposals: any[] = []
  const f = await fixture(async body => {
    if (system(body).includes("You handle the conversation with Gator")) {
      const context = metadata(body)
      if (context.message === "Compare venues") return JSON.stringify({ kind: "task" })
      if (context.message === "Hello during the revision") return JSON.stringify({ kind: "reply", text: "I'm here while both tasks continue." })
      return JSON.stringify({ kind: "revise_plan", target: context.tasks.find((task: any) => task.can_revise_plan).id, instruction: context.message })
    }
    if (system(body).includes("Propose a plan")) {
      const meta = metadata(body); proposals.push(meta)
      if (meta.correction) { await revisionGate; return JSON.stringify(correctedPlan) }
      return JSON.stringify(originalPlan)
    }
    work++; if (work === 1) await mainGate
    return "The original independent task finished."
  })
  try {
    await f.client.start(); await f.flush(); await f.client.send({ type: "submit", text: "/mode inspect" })
    const parent = f.client.state!.conversation_id!
    await f.client.edit("Research another topic"); await f.client.submit(); await until(() => work === 1, "main work")
    const input = await f.plan()
    await until(() => f.client.state?.dialogue?.[0]?.state === "delegated" && !!f.client.state?.dialogue_peers?.some(peer => peer.can_revise_plan), "child plan review")
    const child = f.client.state!.dialogue![0]!.task_chat!
    await f.send(correction)
    await until(() => proposals.some(plan => plan.correction) && !f.client.state?.dialogue_busy, "child revision admitted")
    expect(f.client.state?.conversation_id).toBe(parent); expect(f.client.state?.running).toBe(true)
    await f.send("Hello during the revision")
    expect(f.client.state?.dialogue?.at(-1)?.state).toBe("answered"); expect(work).toBe(1)
    await f.client.edit("Keep the requesting chat draft"); releaseRevision()
    await until(() => f.client.state?.dialogue?.find(turn => turn.text === correction)?.state === "guided", "actual revised plan result returned")
    expect(f.client.state?.dialogue_peers?.find(peer => peer.conversation_id === child)?.summary).toContain("$2,000")
    expect(f.client.state?.conversation_id).toBe(parent); expect(f.client.state?.running).toBe(true); expect(work).toBe(1)
    const journal = JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR, "gator/conversations", child, "requests.json"), "utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].plan.phase).toBe("review")
    expect(journal.requests[0].plan.approval).toBeUndefined(); expect(journal.requests[0].options.Uploads).toEqual([input])
    expect(journal.requests[0].options.DialogueParent).toBe(parent); expect(journal.requests[0].plan.edit.instruction).toBe(correction)
    expect(f.client.draft.text).toBe("Keep the requesting chat draft")
    await f.client.navigate("/tasks"); await f.flush(); await f.choose("Compare venues")
    await until(() => f.client.state?.conversation_id === child && !f.client.snapshot.sessionSwitching, "open original task")
    expect(f.client.state?.task_plans?.[0]?.id).toBe(journal.requests[0].id)
    await f.review(); await f.choose("Read exact proposed plan")
    expect(f.setup.captureCharFrame()).toContain(correction); expect(f.setup.captureCharFrame()).toContain("brief.txt")
    f.setup.mockInput.pressEscape(); await f.flush(); await f.client.navigate("/tasks"); await f.flush(); f.setup.mockInput.pressEnter()
    await until(() => f.client.state?.conversation_id === parent && !f.client.snapshot.sessionSwitching, "return requesting chat")
    expect(f.client.draft.text).toBe("Keep the requesting chat draft")
    releaseMain(); await until(() => !f.client.snapshot.allChatsBusy, "other work finished")
    expect(work).toBe(1)
  } finally { releaseMain(); releaseRevision(); await f.close() }
}, 60000)
