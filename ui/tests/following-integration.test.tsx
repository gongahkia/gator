import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, readdir, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { randomUUID } from "node:crypto"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("commitment cards inspect, pause, resume and edit the real saved agreement without replaying work", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-following-integration-")), config = join(root, "config/gator")
  let release = () => {}, arrived = 0
  const gate = new Promise<void>(resolve => { release = resolve })
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    await request.json(); arrived++; await gate
    return Response.json({ choices: [{ message: { role: "assistant", content: "Task completed after the explicit gate." }, finish_reason: "stop" }], usage: { prompt_tokens: 20, completion_tokens: 10 } })
  } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const deadline = Date.now() + 12000; while (!await predicate()) { if (Date.now() > deadline) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const definitions = async () => Promise.all((await readdir(join(env.GATOR_STATE_DIR, "gator/jobs/definitions")).catch(() => [])).filter(name => name.endsWith(".json")).map(async name => JSON.parse(await readFile(join(env.GATOR_STATE_DIR, "gator/jobs/definitions", name), "utf8"))))
  const choose = async (label: string) => { await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
  const cardMenu = async () => { await client.navigate("/following"); await flush(); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); const origin = client.state!.conversation_id!, originalContext = client.context
    await client.edit("Keep original commitment draft"); await client.navigate("/followup"); await flush(); setup.mockInput.pressKey("n"); await until(() => client.state?.panel?.kind === "followup-form", "check setup")
    await client.send({ type: "panel", key: "ctrl+r", panel: { id: client.state!.panel!.id!, action: "key", fields: { 0: "Venue replies", 1: "Track useful venue changes" } } })
    await until(() => client.state?.panel?.title === "Review ongoing check", "agreement review"); expect(await definitions()).toHaveLength(0)
    await flush(); setup.mockInput.pressEnter(); await until(async () => (await definitions()).length === 1 && !client.snapshot.sessionSwitching, "reviewed save")
    await client.host.sessions.open({ conversation_id: origin }); await client.loadDraft(); await client.navigate("/following"); await flush()
    setup.mockInput.pressKey("r", { ctrl: true }); await until(() => !!client.state?.check_result, "fresh checks"); await flush()
    const saved = (await definitions())[0], id = saved.id
    expect(client.state?.checks?.find(check => check.id === id)?.editable).toBe(true)
    for (const text of ["Venue replies", "Track useful venue changes", "Service offline", "None recorded"]) expect(setup.captureCharFrame()).toContain(text)
    await cardMenu(); await choose("Inspect agreement"); await until(() => client.state?.panel?.title === "Follow-up details", "exact inspection"); expect(client.state?.panel?.subtitle).toBe("Venue replies")
    await cardMenu(); await choose("Pause this check"); expect((await definitions())[0].enabled).toBe(true); setup.mockInput.pressEnter(); await until(async () => (await definitions())[0].enabled === false, "persisted pause"); await flush(); expect(setup.captureCharFrame()).toContain("Check paused"); setup.mockInput.pressEscape(); await flush()
    await cardMenu(); await choose("Resume this check"); setup.mockInput.pressEnter(); await until(async () => (await definitions())[0].enabled === true, "persisted resume"); await flush(); expect(setup.captureCharFrame()).toContain("service is offline"); setup.mockInput.pressEscape(); await flush()
    const stale = client.state!.checks!.find(check => check.id === id)!
    execFileSync(binary, ["job", "edit", id, "--task", "Changed from another terminal"], { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" } })
    const request_id = randomUUID()
    await expect(client.result({ type: "check", check: { action: "pause", id, digest: stale.digest, request_id } }, request_id, state => state.check_result)).rejects.toThrow("changed")
    expect((await definitions())[0].enabled).toBe(true); expect((await definitions())[0].objective).toBe("Changed from another terminal")
    await cardMenu(); await choose("Edit check agreement"); await until(() => client.state?.panel?.kind === "followup-form", "canonical edit form")
    expect(client.state?.panel?.rows?.[1].input?.value).toBe("Changed from another terminal")
    await client.send({ type: "panel", key: "ctrl+r", panel: { id: client.state!.panel!.id!, action: "key", fields: { 1: "Track accessibility and useful venue changes" } } })
    await until(() => client.state?.panel?.title === "Review ongoing check", "edited review"); expect((await definitions())[0].objective).toBe("Changed from another terminal")
    await flush(); setup.mockInput.pressEnter(); await until(async () => (await definitions())[0].objective === "Track accessibility and useful venue changes", "reviewed edit save")
    expect((await definitions())[0].origin_conversation_id).toBe(origin); expect((await definitions())[0].source_path).toBe(saved.source_path)
    await client.host.sessions.open({ conversation_id: origin }); await client.loadDraft(); await client.navigate("/chat"); await client.edit("Hold this independent task"); await client.submit(); await until(() => arrived > 0 && !!client.state?.running, "foreground model gate")
    await client.edit("Keep draft while task runs"); await cardMenu(); await choose("Goal and plan")
    await until(() => client.state?.goal_id === saved.goal_id && !client.snapshot.sessionSwitching, "exact goal while original task runs")
    expect(client.snapshot.chatActivity.some(task => task.id === origin && task.running)).toBe(true)
    expect(client.drafts.get(originalContext)?.text).toBe("Keep draft while task runs")
    await client.host.sessions.open({conversation_id: origin}); await client.loadDraft(); await client.navigate("/following"); await flush(); setup.mockInput.pressKey("r", { ctrl: true })
    await until(() => client.context !== originalContext && !!client.state?.check_result && client.route === "following", "isolated check management owner")
    expect(client.snapshot.chatActivity.some(task => task.id === origin && task.running)).toBe(true); expect(client.drafts.get(originalContext)?.text).toBe("Keep draft while task runs"); expect(arrived).toBe(1)
    release(); await until(() => !client.snapshot.chatActivity.some(task => task.id === origin && task.running), "original task completion")
    await client.host.sessions.open({ conversation_id: origin }); await client.loadDraft(); expect(client.draft.text).toBe("Keep draft while task runs")
    const calls = arrived; await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await client.navigate("/following"); await flush()
    expect(client.state?.checks?.find(check => check.id === id)?.outcome).toBe("Track accessibility and useful venue changes"); expect(client.draft.text).toBe("Keep draft while task runs"); expect(arrived).toBe(calls)
  } finally { release(); await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
