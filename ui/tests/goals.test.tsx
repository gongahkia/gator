import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("saved goals resume their own workspace and plans retain deadlines, evidence and explicit removal without starting checks", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-goals-"))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const cli = (...args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
  const id = cli("goal", "add", "--context", "Keep the venue decision and next steps together.", "Venue planning").split(/\s/)[0]
  const options = { binary, env, userData: join(root, "app"), workspace: join(root, "workspace") }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean, label: string) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const send = async (command: string) => { await client.edit(command); await flush(); setup.mockInput.pressEnter(); await until(() => !client.draft.text, command); await flush() }
  const plan = () => cli("goal", "plan", id)
  const selectGoal = async () => { await client.navigate("/goals"); await flush(); expect(setup.captureCharFrame()).toContain("Venue planning"); setup.mockInput.pressEnter(); await until(() => client.state?.goal_id === id && !client.state?.panel, "selected goal"); await flush() }
  try {
    await client.start(); await client.edit("Keep ordinary chat draft"); const origin = client.context
    await client.navigate("/following"); await flush(); expect(setup.captureCharFrame()).toContain("No recurring check attached")
    await selectGoal(); expect(client.state?.source).not.toBe(options.workspace); expect(cli("goal", "show", id)).toContain(client.state!.source)
    await send("/plan outcome Choose an accessible venue"); expect(plan()).toContain("Choose an accessible venue")
    await send("/plan add Check capacity | 2030-10-25T18:30:00Z | planning-brief.md")
    let saved = plan(); const step = saved.match(/step-[a-f0-9]+/)![0]; expect(saved).toContain("2030-10-25T18:30:00Z"); expect(saved).toContain("planning-brief.md")
    await send(`/plan step ${step} blocked | Waiting for venue reply`); saved = plan(); expect(saved).toContain("blocked"); expect(saved).toContain("Waiting for venue reply")
    await send("/plan review-at 2030-10-26T18:30:00Z"); expect(plan()).toContain("2030-10-26T18:30:00Z")
    await send(`/plan step ${step} done | Confirmed by venue`); expect(plan()).toContain("done"); expect(plan()).toContain("Confirmed by venue")
    await send("/plan"); expect(setup.captureCharFrame()).toContain("Check capacity"); expect(JSON.stringify(client.state?.messages.at(-1)?.result)).toContain("Goal plan")
    await send(`/plan remove ${step}`); expect(plan()).toContain(step); expect(JSON.stringify(client.state?.messages)).toContain("--yes")
    await client.edit("Keep goal draft"); const goalContext = client.context; await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await selectGoal()
    expect(client.context).toBe(goalContext); expect(client.draft.text).toBe("Keep goal draft"); expect(plan()).toContain(step)
    await send(`/plan remove ${step} --yes`); expect(plan()).not.toContain(step)
    await client.navigate("/followup"); await flush(); expect(client.state?.panel?.rows ?? []).toHaveLength(0); expect(client.state?.running).toBe(false)
    const drafts = await client.host.drafts.request({ action: "list" }); expect(drafts.drafts.find(draft => draft.context === origin)?.text).toBe("Keep ordinary chat draft")
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
