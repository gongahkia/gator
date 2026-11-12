import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`paused goal → inspect → reviewed resume → stale pause → restart at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-goal-control-"))
    const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
    const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
    let requests = 0, release = () => {}
    const gate = new Promise<void>(resolve => { release = resolve })
    const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
      await request.json(); requests++; await gate
      return Response.json({ choices: [{ message: { role: "assistant", content: "Original independent task finished after the controlled gate." }, finish_reason: "stop" }], usage: { prompt_tokens: 20, completion_tokens: 10 } })
    } })
    await mkdir(join(env.GATOR_CONFIG_DIR, "gator"), { recursive: true })
    await writeFile(join(env.GATOR_CONFIG_DIR, "gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
    const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
    const cli = (...args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
    const id = cli("goal", "add", "--context", "Keep the venue decision together", "Venue planning").split(/\s/)[0]!
    const peer = cli("goal", "add", "--context", "A separate same-title goal", "Venue planning").split(/\s/)[0]!
    cli("goal", "plan", id, "add", "--title", "Check capacity", "--due", "2030-11-12T10:00:00Z", "--evidence", "planning-brief.md")
    const watch = cli("goal", "watch", id), job = watch.match(/job-[a-zA-Z0-9-]+/)![0]
    cli("goal", "pause", id)
    const goalFile = join(env.GATOR_STATE_DIR, "gator/goals", id + ".json"), peerFile = join(env.GATOR_STATE_DIR, "gator/goals", peer + ".json"), jobFile = join(env.GATOR_STATE_DIR, "gator/jobs/definitions", job + ".json")
    const beforePeer = await readFile(peerFile, "utf8"), beforeJob = await readFile(jobFile, "utf8"), beforePlan = cli("goal", "plan", id)
    const savedGoal = async () => JSON.parse(await readFile(goalFile, "utf8"))
    let client = new GatorClient(new Application(options))
    const mount = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width, height })
    let setup = await mount()
    const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
    const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = performance.now() + 8000; while (!await predicate()) { if (performance.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
    const choose = async (label: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
    const open = async () => {
      await client.navigate("/following"); await flush()
      const position = client.state?.following_goals?.findIndex(goal => goal.id === id) ?? -1
      expect(position).toBeGreaterThanOrEqual(0)
      for (let n = 0; n < position; n++) { setup.mockInput.pressArrow("down"); await flush() }
      setup.mockInput.pressEnter(); await flush(); await choose("Goal controls and retained plan")
    }
    try {
      await client.start(); await client.edit("Keep the multiline\ngoal draft", [], undefined, true)
      await open(); await choose("Inspect goal and retained plan")
      await until(() => client.state?.goal_result?.review?.id === id, "exact goal inspection")
      expect(client.state?.goal_result?.review?.status).toBe("paused")
      const lines = client.state!.goal_result!.review!.lines.join("\n")
      expect(lines).toContain("Check capacity"); expect(lines).toContain("2030-11-12"); expect(lines).toContain("planning-brief.md")
      expect((await savedGoal()).status).toBe("paused")
      setup.mockInput.pressEscape(); await flush()
      await choose("Resume this goal")
      expect(setup.captureCharFrame()).toContain("Resume goal:")
      expect((await savedGoal()).status).toBe("paused")
      setup.mockInput.pressEscape(); await flush()
      expect(setup.captureCharFrame()).toContain("Resume this goal")
      setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressEnter()
      await until(async () => (await savedGoal()).status === "active" && !!client.state?.goal_result?.review && client.state.goal_result.review.status === "active", "persisted resume")
      await flush(); expect(setup.captureCharFrame()).toContain("Goal active")
      setup.mockInput.pressEscape(); await flush()
      await open(); await choose("Pause this goal")
      cli("goal", "context", id, "A corrected venue outcome from another terminal")
      setup.mockInput.pressEnter()
      await until(() => !!client.state?.goal_result?.error, "stale pause rejection")
      expect(client.state?.goal_result?.error).toContain("changed")
      expect((await savedGoal()).status).toBe("active")
      expect((await savedGoal()).context).toContain("corrected venue")
      await open(); await choose("Pause this goal"); setup.mockInput.pressEnter()
      await until(async () => (await savedGoal()).status === "paused" && client.state?.goal_result?.review?.status === "paused", "fresh reviewed pause")
      await flush(); expect(client.issue).toBe("")
      expect(await readFile(peerFile, "utf8")).toBe(beforePeer)
      expect(await readFile(jobFile, "utf8")).toBe(beforeJob)
      expect(cli("goal", "plan", id)).toBe(beforePlan)
      expect(client.draft.text).toBe("Keep the multiline\ngoal draft"); expect(client.draft.plan_first).toBe(true)
      await open(); await choose("Resume this goal"); setup.mockInput.pressEnter()
      await until(async () => (await savedGoal()).status === "active" && client.state?.goal_result?.review?.status === "active", "resume for busy controls")
      setup.mockInput.pressEscape(); await flush(); await client.navigate("/chat")
      const originalContext = client.context, originalOwner = client.state!.conversation_id!
      await client.edit("Hold this independent task", [], undefined, false); await client.submit()
      await until(() => requests === 1 && !!client.state?.running, "controlled foreground request")
      await client.edit("Keep the multiline\ngoal draft", [], undefined, true)
      await open(); await choose("Inspect goal and retained plan")
      await until(() => client.state?.goal_result?.review?.id === id && !client.state.goal_busy, "read while original work runs")
      expect(client.context).toBe(originalContext)
      expect(client.snapshot.chatActivity.some(task => task.id === originalOwner && task.running)).toBe(true)
      setup.mockInput.pressEscape(); await flush(); await choose("Pause this goal"); setup.mockInput.pressEnter()
      await until(async () => (await savedGoal()).status === "paused" && client.state?.goal_result?.review?.status === "paused", "busy reviewed goal pause")
      expect(client.context).not.toBe(originalContext)
      expect(client.snapshot.chatActivity.some(task => task.id === originalOwner && task.running)).toBe(true)
      expect(client.drafts.get(originalContext)?.text).toBe("Keep the multiline\ngoal draft")
      expect(client.drafts.get(originalContext)?.plan_first).toBe(true)
      expect(requests).toBe(1)
      expect(await readFile(peerFile, "utf8")).toBe(beforePeer); expect(await readFile(jobFile, "utf8")).toBe(beforeJob)
      release(); await until(() => !client.snapshot.chatActivity.some(task => task.id === originalOwner && task.running), "original task completion")
      await client.host.sessions.open({ conversation_id: originalOwner }); await client.loadDraft(); await client.navigate("/chat"); await flush()
      expect(client.context).toBe(originalContext)
      expect(client.state?.messages.some(message => message.text.includes("Original independent task finished"))).toBe(true)
      await client.close(); await setup.dispose()
      client = new GatorClient(new Application(options)); setup = await mount(); await client.start(); await open()
      await setup.mockInput.typeText("Resume this goal"); await flush()
      expect(setup.captureCharFrame()).toContain("Resume this goal")
      expect(client.draft.text).toBe("Keep the multiline\ngoal draft"); expect(client.draft.plan_first).toBe(true)
      expect((await savedGoal()).context).toContain("corrected venue")
    } finally { release(); await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
  }, 25000)
}
