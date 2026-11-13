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

for (const [width,height] of [[110,34],[80,24],[60,20],[42,14]]) test(`ordinary correction during initial preparation retires the old proposal and keeps exact review at ${width}×${height}`, async () => {
  let releaseInitial = () => {}, releaseCorrection = () => {}
  const initial = new Promise<void>(resolve => { releaseInitial = resolve }), revised = new Promise<void>(resolve => { releaseCorrection = resolve })
  const proposals: any[] = [], executions: string[] = []
  const f = await fixture(async body => {
    if (system(body).includes("You handle the conversation with Gator")) {
      const meta = metadata(body)
      return JSON.stringify(meta.message === "Hello during the revision" ? { kind:"reply",text:"We can keep talking while the corrected plan is prepared." } : { kind:"revise_plan",target:meta.tasks.find((task:any) => task.can_revise_plan).id,instruction:meta.message })
    }
    if (system(body).includes("Propose a plan")) {
      const meta = metadata(body); proposals.push(meta)
      await (meta.correction ? revised : initial)
      return JSON.stringify(meta.correction ? correctedPlan : originalPlan)
    }
    executions.push(JSON.stringify(body)); return "The reviewed comparison uses the corrected budget."
  })
  try {
    await f.client.start(); f.setup.resize(width,height); await f.flush(); await f.client.send({ type:"submit",text:"/mode inspect" })
    const input = await f.plan(), owner = f.client.state!.conversation_id!
    await until(() => proposals.length === 1 && f.client.state?.task_plans?.[0]?.phase === "preparing", "initial proposal running")
    const original = f.client.state!.task_plans![0]!
    await f.send(correction)
    await until(() => proposals.length === 2 && f.client.state?.task_plans?.[0]?.phase === "revising", "correction replaces the still-held initial proposal")
    expect(f.client.state?.task_plans?.[0]?.id).toBe(original.id); expect(executions).toHaveLength(0)
    expect(proposals[1].correction).toBe(correction); expect(proposals[1].previous_plan).toBeUndefined()
    expect(proposals[1].work_limits).toEqual(proposals[0].work_limits); expect(proposals[1].selected_source_ids).toEqual(proposals[0].selected_source_ids)
    expect(JSON.stringify(proposals)).not.toContain("silver otter")
    await f.send("Hello during the revision"); expect(f.client.state?.dialogue?.at(-1)?.state).toBe("answered"); expect(f.client.state?.running).toBe(true)
    const future = join(f.root,"future-initial-note.txt"); await writeFile(future,"Future draft only: amber finch"); await f.client.attach([future])
    const futureInput = f.client.draft.uploads![0]!.id, draft = "Keep my next question\nAfter correcting the initial plan"
    await f.client.edit(draft); await f.client.setPlanFirst(true); await f.flush()
    expect(f.setup.renderer.root.findDescendantById("transcript")!.height).toBeGreaterThanOrEqual(4)
    releaseCorrection()
    await until(() => !f.client.state?.running && f.client.state?.task_plans?.[0]?.revision === 2 && f.client.state?.dialogue?.[0]?.state === "guided", "corrected initial plan saved")
    releaseInitial(); await f.flush()
    expect(f.client.state?.task_plans?.[0]?.outcome).toBe(correctedPlan.outcome); expect(executions).toHaveLength(0)
    const request_id = randomUUID()
    await expect(f.client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:original.id,revision:original.revision,digest:original.digest,action:"approve",request_id}},request_id,state=>state.task_plan_result)).rejects.toThrow()
    await f.review(); await f.choose("Read exact proposed plan")
    // Exact content remains inspectable at a small viewport through native scroll.
    expect(f.setup.captureCharFrame()).toContain("Proposed plan")
    f.setup.mockInput.pressEscape(); await f.flush(); await f.review(); await f.choose("Approve plan and start"); f.setup.mockInput.pressEnter()
    await until(() => !f.client.state?.running && f.client.state?.task_plans?.[0]?.state === "completed", "only the corrected plan executes")
    expect(executions).toHaveLength(1); expect(executions[0]).toContain("Retained user correction"); expect(executions[0]).toContain("silver otter"); expect(executions[0]).not.toContain("amber finch")
    const journal = JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR,"gator/conversations",owner,"requests.json"),"utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].id).toBe(original.id); expect(journal.requests[0].options.Uploads).toEqual([input]); expect(proposals).toHaveLength(2)
    expect(f.client.draft.text).toBe(draft); expect(f.client.draft.uploads?.map(file=>file.id)).toEqual([futureInput]); expect(f.client.draft.plan_first).toBe(true)
    await f.client.close(); await f.setup.dispose(); const count = f.requests.length, restarted = new GatorClient(new Application(f.options))
    try { await restarted.start(); expect(restarted.state?.task_plans?.[0]?.id).toBe(original.id); expect(restarted.state?.dialogue?.[0]?.state).toBe("guided"); expect(restarted.draft.text).toBe(draft); expect(restarted.draft.uploads?.map(file=>file.id)).toEqual([futureInput]); expect(restarted.draft.plan_first).toBe(true); expect(f.requests).toHaveLength(count) }
    finally { await restarted.close() }
  } finally { releaseInitial(); releaseCorrection(); await f.close() }
},30000)

for (const [width,height] of [[110,34],[80,24],[60,20],[42,14]]) for (const scenario of ["cancel","failure and retry"] as const) test(`initial correction ${scenario} retains original task through restart at ${width}×${height}`, async () => {
  let releaseInitial = () => {}, releaseCorrection = () => {}, revisionCalls = 0, executions = 0
  const initial = new Promise<void>(resolve=>{releaseInitial=resolve}), revised = new Promise<void>(resolve=>{releaseCorrection=resolve})
  const f = await fixture(async body => {
    if (system(body).includes("You handle the conversation with Gator")) {
      const meta = metadata(body)
      return JSON.stringify(meta.message === "Stop the venue plan" ? {kind:"stop",target:meta.tasks.find((task:any)=>task.can_stop).id} : {kind:"revise_plan",target:meta.tasks.find((task:any)=>task.can_revise_plan).id,instruction:meta.message})
    }
    if (system(body).includes("Propose a plan")) {
      if (!metadata(body).correction) { await initial; return JSON.stringify(originalPlan) }
      revisionCalls++
      if (scenario === "failure and retry" && revisionCalls === 1) return new Response("Controlled initial revision unavailable",{status:503})
      if (scenario === "cancel") await revised
      return JSON.stringify(correctedPlan)
    }
    executions++; return "Unexpected unreviewed execution"
  })
  let restarted: GatorClient | undefined, setup: Awaited<ReturnType<typeof testRender>> | undefined
  try {
    await f.client.start(); f.setup.resize(width,height); await f.flush(); await f.client.send({type:"submit",text:"/mode inspect"})
    const input = await f.plan(), owner = f.client.state!.conversation_id!
    await until(()=>f.client.state?.task_plans?.[0]?.phase === "preparing","initial preparation started")
    const task = f.client.state!.task_plans![0]!
    await f.client.edit(correction); await f.client.submit()
    await until(()=>revisionCalls === 1,"initial correction started")
    if (scenario === "cancel") { await f.send("Stop the venue plan"); await until(()=>!f.client.state?.running && f.client.state?.dialogue?.[0]?.state === "interrupted","initial correction cancelled") }
    else { await until(()=>!f.client.state?.running && f.client.state?.dialogue?.[0]?.state === "failed","initial correction failure retained"); expect(f.client.state?.dialogue?.[0]?.issue).toContain("No proposal was saved") }
    await f.client.edit("Keep my correction recovery\ndraft",[],undefined,true)
    releaseInitial(); releaseCorrection(); await f.flush(); expect(revisionCalls).toBe(1); expect(executions).toBe(0)
    await f.client.close(); await f.setup.dispose(); const count = f.requests.length
    const client = restarted = new GatorClient(new Application(f.options))
    setup = await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>undefined,write:async()=>{}}}/>,{width,height})
    await client.start(); await setup.flush(); expect(client.state?.task_plans?.[0]?.id).toBe(task.id); expect(f.requests).toHaveLength(count)
    expect(client.draft.text).toBe("Keep my correction recovery\ndraft"); expect(client.draft.plan_first).toBe(true)
    if (scenario === "failure and retry") {
      await client.edit("Retry that original venue plan",[],undefined,false); await setup.flush(); await Bun.sleep(25); await setup.flush(); setup.mockInput.pressEnter()
      await until(()=>client.state?.dialogue?.at(-1)?.state === "guided" && !client.state?.running,"explicit retry uses same task")
      expect(revisionCalls).toBe(2); expect(client.state?.task_plans?.[0]?.outcome).toBe(correctedPlan.outcome)
    } else expect(client.state?.task_plans?.[0]?.state).toBe("discarded")
    const journal = JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR,"gator/conversations",owner,"requests.json"),"utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].id).toBe(task.id); expect(journal.requests[0].options.Uploads).toEqual([input]); expect(executions).toBe(0)
  } finally { releaseInitial();releaseCorrection();if(restarted)await restarted.close();if(setup&&!setup.renderer.isDestroyed)await setup.dispose();await f.close() }
},30000)

for (const [width,height] of [[110,34],[80,24],[60,20],[42,14]]) test.skipIf(process.platform === "win32")(`crashed initial correction stays interrupted until explicit retry at ${width}×${height}`, async () => {
  let releaseInitial=()=>{},releaseRevision=()=>{},revisionCalls=0,executions=0
  const initial=new Promise<void>(resolve=>{releaseInitial=resolve}),revision=new Promise<void>(resolve=>{releaseRevision=resolve})
  const f=await fixture(async body=>{
    if(system(body).includes("You handle the conversation with Gator")){const meta=metadata(body);return JSON.stringify({kind:"revise_plan",target:meta.tasks.find((task:any)=>task.can_revise_plan).id,instruction:meta.message})}
    if(system(body).includes("Propose a plan")){
      if(!metadata(body).correction){await initial;return JSON.stringify(originalPlan)}
      revisionCalls++;if(revisionCalls===1)await revision;return JSON.stringify(correctedPlan)
    }
    executions++;return "Unexpected execution"
  })
  let restarted:GatorClient|undefined,setup:Awaited<ReturnType<typeof testRender>>|undefined
  try{
    await f.client.start();f.setup.resize(width,height);await f.flush();await f.client.send({type:"submit",text:"/mode inspect"})
    const input=await f.plan(),owner=f.client.state!.conversation_id!
    await until(()=>f.client.state?.task_plans?.[0]?.phase==="preparing","initial proposal started")
    const task=f.client.state!.task_plans![0]!
    await f.send(correction);await until(()=>revisionCalls===1&&f.client.state?.task_plans?.[0]?.phase==="revising","correction request held")
    await f.client.edit("Draft kept after an interrupted\ninitial correction",[],undefined,true)
    const children=execFileSync("ps",["-axo","pid=,ppid=,command="],{encoding:"utf8"}).split("\n").map(line=>line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/)).filter(match=>match&&Number(match[2])===process.pid&&match[3].includes(f.options.binary)&&match[3].includes("ui-backend"))
    expect(children).toHaveLength(1);process.kill(Number(children[0]![1]),"SIGKILL")
    await until(()=>f.client.snapshot.connection==="disconnected","exact isolated runtime crashed")
    releaseInitial();releaseRevision();await f.client.close();await f.setup.dispose();const count=f.requests.length
    const client=restarted=new GatorClient(new Application(f.options))
    setup=await testRender(()=><App client={client} quit={()=>client.close()} clipboard={{read:async()=>undefined,write:async()=>{}}}/>,{width,height})
    await client.start();await setup.flush()
    expect(client.state?.task_plans?.[0]?.id).toBe(task.id);expect(client.state?.task_plans?.[0]?.phase).toBe("interrupted");expect(client.state?.task_plans?.[0]?.state).toBe("interrupted")
    expect(client.state?.dialogue?.[0]?.state).toBe("failed");expect(client.state?.dialogue?.[0]?.issue).toContain("will not replay automatically")
    expect(client.draft.text).toBe("Draft kept after an interrupted\ninitial correction");expect(client.draft.plan_first).toBe(true);expect(f.requests).toHaveLength(count);expect(executions).toBe(0)
    await client.edit("Retry that original venue plan",[],undefined,false);await setup.flush();await Bun.sleep(25);await setup.flush();setup.mockInput.pressEnter()
    await until(()=>client.state?.dialogue?.at(-1)?.state==="guided"&&!client.state?.running,"explicit retry after crash")
    expect(revisionCalls).toBe(2);expect(client.state?.task_plans?.[0]?.id).toBe(task.id);expect(client.state?.task_plans?.[0]?.outcome).toBe(correctedPlan.outcome)
    const journal=JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR,"gator/conversations",owner,"requests.json"),"utf8"))
    expect(journal.requests).toHaveLength(1);expect(journal.requests[0].options.Uploads).toEqual([input]);expect(executions).toBe(0)
  }finally{releaseInitial();releaseRevision();if(restarted)await restarted.close();if(setup&&!setup.renderer.isDestroyed)await setup.dispose();await f.close()}
},30000)

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

for (const phase of ["preparing","review"] as const) test(`a delegated ${phase} plan correction returns its real result to the requesting chat without disturbing other work`, async () => {
  let releaseMain!: () => void, releaseRevision!: () => void
	let releaseInitial = () => {}
	const initialGate = new Promise<void>(resolve => { releaseInitial = resolve })
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
      if (phase === "preparing") await initialGate
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
    await until(() => f.client.state?.dialogue?.[0]?.state === "delegated" && !!f.client.state?.dialogue_peers?.some(peer => peer.can_revise_plan && peer.state === (phase === "preparing" ? "running" : "review")), "child plan " + phase)
    const child = f.client.state!.dialogue![0]!.task_chat!
    await f.send(correction)
    await until(() => proposals.some(plan => plan.correction) && !f.client.state?.dialogue_busy, "child revision admitted")
    releaseInitial()
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
    expect(journal.requests[0].plan.edit.base_revision).toBe(phase === "preparing" ? 0 : 1)
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
  } finally { releaseInitial(); releaseMain(); releaseRevision(); await f.close() }
}, 60000)

test("a second ordinary correction replaces an active revision in the same real Go owner", async () => {
  let releaseFirst = () => {}, releaseLatest = () => {}
  const first = new Promise<void>(resolve => { releaseFirst = resolve }), latest = new Promise<void>(resolve => { releaseLatest = resolve })
  const proposals:any[] = []; let work = 0
  const f = await fixture(async body => {
    if (system(body).includes("You handle the conversation with Gator")) {
      const context = metadata(body)
      return JSON.stringify(context.message === "Hello during the revision" ? {kind:"reply",text:"We can keep talking."} : {kind:"revise_plan",target:context.tasks.find((task:any)=>task.can_revise_plan).id,instruction:context.message})
    }
    if (system(body).includes("Propose a plan")) {
      const meta = metadata(body); proposals.push(meta)
      if (!meta.correction) return JSON.stringify(originalPlan)
      await (meta.prior_corrections?.length ? latest : first)
      return JSON.stringify(correctedPlan)
    }
    work++; return "Freshly reviewed work completed."
  })
  try {
    await f.client.start(); await f.client.send({type:"submit",text:"/mode inspect"})
    const input = await f.plan(); await until(()=>f.client.state?.task_plans?.[0]?.phase==="review","original plan")
    const original = f.client.state!.task_plans![0]!, owner = f.client.state!.conversation_id
    await f.send(correction); await until(()=>proposals.length===2,"first revision held")
    await f.send("Also preserve the original selected sources")
    await until(()=>proposals.length===3,"latest revision starts after obsolete call stops")
    expect(proposals[2].prior_corrections).toEqual([correction])
    expect(proposals[2].correction).toBe("Also preserve the original selected sources")
    expect(proposals[2].work_limits).toEqual(proposals[0].work_limits)
    expect(proposals[2].selected_source_ids).toEqual(proposals[0].selected_source_ids)
    expect(f.client.state?.conversation_id).toBe(owner); expect(work).toBe(0)
    await f.send("Hello during the revision")
    expect(f.client.state?.dialogue?.at(-1)?.state).toBe("answered")
    releaseFirst(); releaseLatest()
    await until(()=>f.client.state?.task_plans?.[0]?.phase==="review"&&!f.client.state?.running,"latest proposal saved")
    const journal = JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR,"gator/conversations",owner!,"requests.json"),"utf8"))
    expect(journal.requests).toHaveLength(1); expect(journal.requests[0].id).toBe(original.id)
    expect(journal.requests[0].options.Uploads).toEqual([input]); expect(journal.requests[0].plan.approval).toBeUndefined()
    expect(journal.requests[0].plan.edit.prior_corrections[0].instruction).toBe(correction)
    expect(f.client.state?.dialogue?.find(turn=>turn.text===correction)?.state).toBe("interrupted")
    expect(f.client.state?.dialogue?.find(turn=>turn.text==="Also preserve the original selected sources")?.state).toBe("guided")
    expect(work).toBe(0)
  } finally {releaseFirst(); releaseLatest(); await f.close()}
},45000)

test("typed input removal retains originals and executes only the freshly reviewed selection",async()=>{
 const executions:string[]=[]
 const f=await fixture(body=>{
  if(system(body).includes("Propose a plan"))return JSON.stringify(originalPlan)
  executions.push(JSON.stringify(body));return "The narrowed input selection was processed."
 })
 try{
  await f.client.start();await f.client.send({type:"submit",text:"/mode inspect"})
  const removed=join(f.root,"remove.txt");await writeFile(removed,"Withdraw this selected input: amber finch");await f.client.attach([removed]);const removedID=f.client.draft.uploads![0]!.id
  await f.plan();await until(()=>f.client.state?.task_plans?.[0]?.phase==="review","original two-file review")
  const before=f.client.state!.task_plans![0]!,owner=f.client.state!.conversation_id!
  await f.client.navigate("/task-plans");await f.flush();await f.choose(originalPlan.outcome)
  await f.choose("Remove selected inputs");await f.choose("remove.txt");f.setup.mockInput.pressEnter();await f.flush()
  await until(()=>(f.client.state?.task_plans?.[0]?.revision??0)>before.revision,"narrowed exact review")
  const saved=JSON.parse(await readFile(join(f.env.GATOR_STATE_DIR,"gator/conversations",owner,"requests.json"),"utf8")).requests[0]
  expect(saved.plan.approval).toBeUndefined();expect(saved.options.Uploads).not.toContain(removedID);expect(saved.plan.input_changes[0].before_options.Uploads).toContain(removedID)
  expect(executions).toHaveLength(0)
  await expect(f.client.result({type:"task_plan",task_plan:{conversation_id:owner,task_id:before.id,revision:before.revision,digest:before.digest,action:"approve",request_id:"stale-input-selection"}},"stale-input-selection",state=>state.task_plan_result)).rejects.toThrow()
  await f.choose("Read exact proposed plan");expect(f.setup.captureCharFrame()).toContain("withdrawn");f.setup.mockInput.pressEscape();await f.flush();f.setup.mockInput.pressEscape();await f.flush();await f.client.navigate("/task-plans");await f.flush();await f.choose(originalPlan.outcome)
  await f.choose("Approve plan and start");f.setup.mockInput.pressEnter();await until(()=>!f.client.state?.running&&executions.length===1,"fresh narrowed execution").catch(error=>{throw Error(String(error)+"\n"+JSON.stringify({plan:f.client.state?.task_plan_result,tasks:f.client.state?.task_plans,issue:f.client.issue})+"\n"+f.setup.captureCharFrame())})
  expect(executions[0]).toContain("silver otter");expect(executions[0]).not.toContain("amber finch")
 }finally{await f.close()}
},45000)
