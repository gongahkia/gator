import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import type { TextareaRenderable } from "@opentui/core"
import { execFileSync } from "node:child_process"
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

type Message = { role: string; content: string; tool_call_id?: string }
// Native rendering and authoritative Go reviews/storage, with controlled model
// responses and an inserted job-update fixture. No real scheduler/account run.
for (const [width, height] of [[110,34], [80,24], [60,20], [42,14]]) test(`noise feedback → scoped review → conversation → stale pause → restart at ${width}×${height}`, async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-noise-feedback-")), calls: string[] = []
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const raw = await request.text(); calls.push(raw)
    const payload = JSON.parse(raw) as { messages: Message[] }, last = payload.messages.at(-1)!
    const tool = (name: string, value: unknown) => ({ role: "assistant", tool_calls: [{ id: name, type: "function", function: { name, arguments: JSON.stringify(value) } }] })
    let message
    if (last.role !== "tool") message = tool("gator_app_inspect", { topic: "followups" })
    else if (last.tool_call_id === "gator_app_inspect") {
      const check = JSON.parse(last.content).checks.find((item: { title: string }) => item.title === "Venue replies")
      if (!check) throw new Error("Controlled provider expected the original check: " + last.content)
      message = tool("gator_app_update", { setting: "followup", value: { action: "edit", id: check.id, expected_digest: check.expected_digest, alert_when: "Only confirmed price changes", interval_minutes: 60 } })
    } else message = { role: "assistant", content: "Inspect the saved tool outcome in Following. This computer's checking service is offline." }
    return Response.json({ choices: [{ message, finish_reason: "tool_calls" in message ? "tool_calls" : "stop" }], usage: { total_tokens: 20 } })
  } })
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime"), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  await mkdir(join(root, "config/gator"), { recursive: true }); await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary, env, workspace: join(root, "workspace"), userData: join(root, "app") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async () => {} }} />, { width, height })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (check: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 20000; while (!await check()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const choose = async (label: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const directory = join(root, "state/gator/jobs/definitions"), definitions = async () => Promise.all((await readdir(directory).catch(() => [])).filter(name => name.endsWith(".json")).map(async name => JSON.parse(await readFile(join(directory, name), "utf8"))))
  try {
    await client.start(); const origin = client.state!.conversation_id!, context = client.context
    const returnOriginal = async () => {
      if (client.context !== context) {
        await client.navigate("/conversations"); await until(() => !!client.state?.panel?.rows?.some(row => row.identity?.startsWith(origin + ":")), "original chat picker")
        const panel = client.state!.panel!, row = panel.rows!.findIndex(row => row.identity?.startsWith(origin + ":"))
        await client.send({ type: "panel", panel: { id: panel.id!, action: "activate", row } }); await until(() => client.context === context, "original owner")
      }
      await client.navigate("/chat"); await flush()
    }
    for (const [index, title] of ["Venue replies", "Weather briefing"].entries()) {
      if (index) { await client.navigate("/new"); await until(() => client.context !== context && !client.snapshot.sessionSwitching, "independent commitment owner") }
      await client.navigate("/followup"); await flush(); setup.mockInput.pressKey("n"); await until(() => client.state?.panel?.kind === "followup-form", "canonical check setup")
      await client.send({ type: "panel", key: "ctrl+r", panel: { id: client.state!.panel!.id!, action: "key", fields: { 0: title, 1: `Track ${title.toLowerCase()}` } } })
      await until(() => client.state?.panel?.title === "Review ongoing check", "canonical check review"); setup.mockInput.pressEnter()
      await until(async () => (await definitions()).length === index + 1 && client.state?.panel?.title === "Follow-up checks" && !client.state?.pending_operation && !client.snapshot.sessionSwitching, "reviewed check save acknowledged"); await returnOriginal()
    }
    const saved = await definitions(), target = saved.find(item => item.name === "Venue replies"), other = saved.find(item => item.name === "Weather briefing")
    const targetPath = join(directory, target.id + ".json"), otherPath = join(directory, other.id + ".json"), otherBytes = await readFile(otherPath)
    const readTarget = async () => JSON.parse(await readFile(targetPath, "utf8"))
    await client.navigate("/following"); await flush(); setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.checks?.length === 2 && !client.state?.pending_operation, "refreshed check cards"); await returnOriginal()
    await client.edit("Keep my independent noise draft"); await client.setPlanFirst(true)
    const inbox = join(root, "state/gator/inbox"), updatePath = join(inbox, "inbox-venue-noise.json")
    await mkdir(inbox, { recursive: true }); await writeFile(updatePath, JSON.stringify({ version: 1, id: "inbox-venue-noise", kind: "job", title: "Venue update", summary: "Another venue update to review.", status: "completed", origin_conversation_id: origin, job_id: target.id, goal_id: target.goal_id, created_at: new Date().toISOString() }))
    await until(() => !!client.state?.chat_updates?.some(update => update.id === "inbox-venue-noise"), "original check update")
    const noise = async () => { await client.navigate("/actions"); await flush(); await choose("Venue update"); await choose("Feedback: noise"); await until(() => setup.captureCharFrame().includes("Reduce interruptions"), "acknowledged noise options") }
    await noise(); expect((await readTarget()).enabled).toBe(true); expect(JSON.parse(await readFile(updatePath, "utf8")).rating).toBe("noise"); expect(calls).toHaveLength(0)
    expect(client.draft.text).toBe("Keep my independent noise draft"); expect(client.draft.plan_first).toBe(true)
    await choose("Review notification conditions and timing"); await until(() => client.state?.panel?.kind === "followup-form", "original check edit form")
    expect(client.state?.panel?.rows?.[0].input?.value).toBe("Venue replies")
    await client.send({ type: "panel", panel: { id: client.state!.panel!.id!, action: "select", row: 7 } }); await flush(); setup.mockInput.pressEnter(); await until(() => !!setup.renderer.root.findDescendantById("multiline-field-input"), "notification condition editor")
    ;(setup.renderer.root.findDescendantById("multiline-field-input") as TextareaRenderable).setText("Only a venue becomes unavailable")
    setup.mockInput.pressKey("s", { ctrl: true }); await flush(); setup.mockInput.pressKey("r", { ctrl: true })
    await until(() => client.state?.panel?.title === "Review ongoing check", "scoped notification review")
    expect((await readTarget()).alert_when).toBe(target.alert_when); expect(JSON.stringify(client.state?.panel)).toContain("Only a venue becomes unavailable")
    setup.mockInput.pressEnter(); await until(async () => (await readTarget()).alert_when === "Only a venue becomes unavailable" && client.state?.panel?.title === "Follow-up checks" && !client.state?.pending_operation, "reviewed notification change acknowledged")
    expect(await readFile(otherPath)).toEqual(otherBytes); await returnOriginal()
    // Ordinary language follows the same authoritative follow-up controller.
    await client.setPlanFirst(false); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Venue replies is too noisy. Only alert on confirmed price changes, and check hourly."); await client.submit()
    await until(() => !!client.state?.pending_approval, "conversational exact agreement")
    const review = JSON.stringify(client.state?.messages)
    expect(review).toContain("Previous notification condition"); expect(review).toContain("Only a venue becomes unavailable"); expect(review).toContain("Only confirmed price changes"); expect(review).toContain("this check only")
    expect((await readTarget()).alert_when).toBe("Only a venue becomes unavailable")
    setup.mockInput.pressKey("y", { ctrl: true }); await flush(); expect(setup.captureCharFrame()).toContain("Approve this exact action?"); setup.mockInput.pressEnter()
    await until(async () => !client.state?.running && (await readTarget()).alert_when === "Only confirmed price changes", "conversational persisted change")
    expect((await readTarget()).poll_minutes).toBe(60); expect(await readFile(otherPath)).toEqual(otherBytes)
    await client.edit("Keep my independent noise draft"); const count = calls.length
    await noise(); await choose("Review pausing this check"); expect((await readTarget()).enabled).toBe(true)
    execFileSync(binary, ["job", "edit", target.id, "--task", "Changed in another terminal"], { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
    setup.mockInput.pressEnter(); await until(() => client.issue.includes("changed"), "stale check refusal")
    expect((await readTarget()).enabled).toBe(true); expect((await readTarget()).objective).toBe("Changed in another terminal")
    await noise(); await choose("Review pausing this check"); setup.mockInput.pressEnter()
    await until(async () => (await readTarget()).enabled === false && setup.captureCharFrame().includes("Check paused"), "confirmed scoped pause")
    expect(await readFile(otherPath)).toEqual(otherBytes); expect(client.draft.text).toBe("Keep my independent noise draft"); expect(calls).toHaveLength(count)
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await client.navigate("/following"); await flush()
    expect(client.state?.checks?.find(check => check.id === target.id)?.enabled).toBe(false); expect(await readFile(otherPath)).toEqual(otherBytes); expect(calls).toHaveLength(count)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 120000)
