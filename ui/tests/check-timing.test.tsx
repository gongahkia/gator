import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { ScrollBoxRenderable } from "@opentui/core"
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) {
  test(`saved legacy check → typed timing → computed review → stale rejection → restart at ${width}×${height}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "gator-check-timing-"))
    const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
    const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }, workspace = join(root, "workspace")
    let requests = 0, routingRequests = 0, release = () => {}
    const gate = new Promise<void>(resolve => { release = resolve })
    const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
      const body = await request.json() as { messages?: { role: string; content: string }[] }
      if (body.messages?.some(message => message.role === "system" && message.content.startsWith("Classify the user's immediate request"))) {
        routingRequests++
        return Response.json({ choices: [{ message: { role: "assistant", content: JSON.stringify({ mode: "inspect" }) }, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 4 } })
      }
      requests++; await gate
      return Response.json({ choices: [{ message: { role: "assistant", content: "Original task completed after timing edits." }, finish_reason: "stop" }], usage: { prompt_tokens: 20, completion_tokens: 10 } })
    } })
    await mkdir(workspace, { recursive: true }); await mkdir(join(env.GATOR_CONFIG_DIR, "gator"), { recursive: true })
    await writeFile(join(workspace, "brief.txt"), "Keep this original source.\n")
    await writeFile(join(env.GATOR_CONFIG_DIR, "gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
    const cli = (...args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
    const add = () => cli("job", "add", "Same check title", "--schedule", "15 8 * * *", "--timezone", "UTC", "--source", workspace, "--provider", "retained", "--model", "retained-model", "--max-model-requests", "7", "--artifact", "report.md", "--", "Review the original workspace").match(/job-[a-zA-Z0-9-]+/)![0]
    const id = add(), peer = add(); cli("job", "disable", id)
    const file = join(env.GATOR_STATE_DIR, "gator/jobs/definitions", id + ".json"), peerFile = join(env.GATOR_STATE_DIR, "gator/jobs/definitions", peer + ".json")
    const saved = async () => JSON.parse(await readFile(file, "utf8"))
    const original = await saved(), peerBefore = await readFile(peerFile, "utf8")
    const options = { binary, env, workspace, userData: join(root, "app") }
    let client = new GatorClient(new Application(options))
    const mount = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => undefined, write: async () => {} }} />, { width, height, kittyKeyboard: true })
    let setup = await mount()
    const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
    const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = performance.now() + 7000; while (!await predicate()) { if (performance.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
    const choose = async (label: string) => { setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush() }
    const select = async (label: string) => {
      const target = client.state?.panel?.rows?.findIndex(row => row.label === label) ?? -1
      expect(target).toBeGreaterThanOrEqual(0)
      while ((client.state?.panel?.selected ?? 0) !== target) { setup.mockInput.pressArrow((client.state?.panel?.selected ?? 0) < target ? "down" : "up"); await flush() }
      setup.mockInput.pressEnter(); await flush()
    }
    const editField = async (label: string, value: string) => { await select(label); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(value); setup.mockInput.pressEnter(); await flush() }
    const open = async () => {
      await client.navigate("/following"); await flush()
      const previous = client.state?.check_result?.request_id
      setup.mockInput.pressKey("r", { ctrl: true })
      await until(() => !!client.state?.check_result && client.state.check_result.request_id !== previous && client.route === "following", "explicit fresh check agreements")
      await flush()
      const index = client.state?.checks?.findIndex(check => check.id === id) ?? -1
      expect(index).toBeGreaterThanOrEqual(0)
      for (let n = 0; n < index; n++) { setup.mockInput.pressArrow("down"); await flush() }
      setup.mockInput.pressEnter(); await flush(); await choose("Edit check agreement")
      await until(() => client.state?.panel?.title === "Check timing", "canonical timing controls")
      await flush()
    }
    try {
      await client.start(); await client.edit("Keep the multiline\nschedule draft", [], undefined, true)
      const owner = client.context
      await open(); expect(client.state?.checks?.find(check => check.id === id)?.editable).toBe(true)
      await select("Daily at a local time")
      await editField("Local time (24-hour)", "25:00")
      setup.mockInput.pressKey("r", { ctrl: true })
      await until(() => !!client.state?.panel?.notice?.includes("24-hour"), "invalid time recovery")
      expect(client.state?.panel?.kind).toBe("settings-form"); expect((await saved()).schedule).toBe(original.schedule)
      await editField("Local time (24-hour)", "09:30"); await editField("Timezone", "Asia/Singapore")
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.panel?.title === "Review check timing", "computed schedule review"); await flush()
      expect((await saved()).schedule).toBe(original.schedule)
      expect(client.state?.panel?.blocks?.[0].lines?.join("\n")).toContain("30 9 * * * · Asia/Singapore")
      const scroll = setup.renderer.root.findDescendantById("backend-panel-content") as ScrollBoxRenderable
      expect(scroll.viewport.height).toBeGreaterThanOrEqual(3)
      setup.mockInput.pressArrow("down"); await flush(); expect(scroll.scrollTop).toBeGreaterThan(0)
      setup.mockInput.pressKey("END", { ctrl: true }); await flush()
      expect(setup.captureCharFrame()).toContain("approval.")
      setup.mockInput.pressEscape(); await until(() => client.state?.panel?.kind === "settings-form", "Back to entered values")
      expect(client.state?.panel?.rows?.map(row => row.input?.value)).toEqual(["09:30", "Asia/Singapore"])
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.panel?.title === "Review check timing", "second exact review")
      cli("job", "edit", id, "--task", "Corrected from another terminal")
      setup.mockInput.pressEnter(); await until(() => !!client.state?.panel?.notice?.includes("changed"), "stale schedule rejection")
      expect((await saved()).schedule).toBe(original.schedule); expect((await saved()).objective).toBe("Corrected from another terminal")
      await open(); await select("Weekdays at a local time"); await editField("Local time (24-hour)", "10:05"); await editField("Timezone", "Asia/Singapore")
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.panel?.title === "Review check timing", "fresh weekdays review")
      setup.mockInput.pressEnter(); await until(() => !!client.state?.panel?.notice?.startsWith("Timing saved."), "persisted schedule outcome")
      const updated = await saved()
      expect(updated.schedule).toBe("5 10 * * 1-5"); expect(updated.timezone).toBe("Asia/Singapore"); expect(updated.enabled).toBe(false)
      await until(() => client.state?.checks?.find(check => check.id === id)?.schedule === updated.schedule, "Following reflects the saved schedule")
      for (const key of ["id", "name", "source_path", "limits", "contract", "provider", "model", "code_policy", "connector_ids", "max_steps", "missed"]) expect(updated[key]).toEqual(original[key])
      expect(await readFile(peerFile, "utf8")).toBe(peerBefore)
      await select("Missed-check behavior"); await select("When this computer returns"); await choose("Run one catch-up")
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.panel?.title === "Review check timing", "catch-up review")
      setup.mockInput.pressEnter(); await until(async () => (await saved()).missed === "run_once" && !!client.state?.panel?.notice?.startsWith("Timing saved."), "catch-up save")
      expect(client.context).toBe(owner); expect(client.draft.text).toBe("Keep the multiline\nschedule draft"); expect(client.draft.plan_first).toBe(true)
      await client.navigate("/chat"); await client.edit("Hold the original independent task", [], undefined, false); await client.submit()
      await until(() => requests === 1 && !!client.state?.running, "controlled foreground request")
      const originalOwner = client.state!.conversation_id!
      await client.edit("Keep the multiline\nschedule draft", [], undefined, true)
      await open(); await select("Hourly at a chosen minute"); await editField("Minute of the hour (0–59)", "45")
      setup.mockInput.pressKey("r", { ctrl: true }); await until(() => client.state?.panel?.title === "Review check timing", "review while independent work runs")
      setup.mockInput.pressEnter(); await until(async () => (await saved()).schedule === "45 * * * *" && !!client.state?.panel?.notice?.startsWith("Timing saved."), "busy schedule save")
      expect(client.context).not.toBe(owner); expect(client.snapshot.chatActivity.some(task => task.id === originalOwner && task.running)).toBe(true)
      expect(client.drafts.get(owner)?.text).toBe("Keep the multiline\nschedule draft"); expect(client.drafts.get(owner)?.plan_first).toBe(true); expect(requests).toBe(1); expect(routingRequests).toBe(1)
      release(); await until(() => !client.snapshot.chatActivity.some(task => task.id === originalOwner && task.running), "original task finishes in original owner")
      await client.host.sessions.open({ conversation_id: originalOwner }); await client.loadDraft(); await client.navigate("/chat"); await flush()
      expect(client.state?.messages.some(message => message.text.includes("Original task completed after timing edits"))).toBe(true)
      await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await mount(); await client.start(); await open()
      expect(client.state?.panel?.summary?.find(field => field.label === "Schedule")?.value).toContain("45 * * * * · Asia/Singapore")
      expect(client.state?.panel?.summary?.find(field => field.label === "Missed checks")?.value).toContain("One catch-up")
      expect(client.draft.text).toBe("Keep the multiline\nschedule draft"); expect(client.draft.plan_first).toBe(true)
      expect((await saved()).enabled).toBe(false); expect(await readFile(peerFile, "utf8")).toBe(peerBefore)
      setup.mockInput.pressEscape(); await until(() => client.state?.panel?.title === "Follow-up details", "Back to the original check details")
      expect(client.state?.panel?.subtitle).toBe("Same check title")
      expect(client.draft.text).toBe("Keep the multiline\nschedule draft"); expect(client.draft.plan_first).toBe(true)
      expect(requests).toBe(1); expect(routingRequests).toBe(1)
    } finally { release(); await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
  }, 25000)
}
