import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("appearance colors follow keyboard settings across cached chats, compact dialogs and restart", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-appearance-")), config = join(root, "config/gator")
  let requests = 0
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    await request.text(); requests++
    return Response.json({ choices: [{ message: { role: "assistant", content: "A place to think. Your notes and plans stay together." }, finish_reason: "stop" }], usage: { total_tokens: 20 } })
  } })
  await mkdir(config, { recursive: true }); await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }] }))
  const options = { binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }
  let client = new GatorClient(new Application(options))
  const render = () => testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 36 })
  let setup = await render()
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean) => { const end = Date.now() + 10000; while (!predicate()) { if (Date.now() > end) throw new Error(client.issue + "\n" + setup.captureCharFrame()); await flush() } }
  const color = (label: string, expected: number[]) => {
    const spans = setup.captureSpans().lines.flatMap(line => line.spans), span = spans.find(span => span.text.includes(label))
    expect(span).toBeDefined(); expect(span!.bg.toInts().slice(0, 3)).toEqual(expected)
  }
  const choose = async (label: string, theme: string) => {
    await client.navigate("/appearance"); await flush()
    const target = client.state!.panel!.rows!.findIndex(row => row.label === label); expect(target).toBeGreaterThanOrEqual(0)
    for (let n = 0; (client.state!.panel!.selected ?? 0) !== target && n < 8; n++) { setup.mockInput.pressArrow((client.state!.panel!.selected ?? 0) < target ? "down" : "up"); await flush() }
    setup.mockInput.pressEnter(); await until(() => client.state?.theme === theme); await flush()
    expect(JSON.parse(await readFile(join(config, "config.json"), "utf8")).theme).toBe(theme)
  }
  try {
    await client.start(); await client.send({ type: "submit", text: "/mode inspect" }); await client.edit("Help me plan my week"); await client.submit(); await until(() => !client.state?.running && requests === 1)
    const original = client.state!.conversation_id!; await client.edit("Keep first draft"); await client.navigate("/new"); await client.edit("Keep second draft")
    await choose("Light", "light"); color("Appearance", [246, 248, 245]); color("Gator", [246, 248, 245])
    await client.navigate("/chats"); await flush(); await setup.mockInput.typeText("Help me plan my week"); await flush(); setup.mockInput.pressEnter(); await until(() => client.state?.conversation_id === original); await flush()
    expect(client.state?.theme).toBe("light"); expect(client.draft.text).toBe("Keep first draft"); color("A place to think", [246, 248, 245])
    setup.resize(60, 20); await flush(); setup.mockInput.pressKey("p", { ctrl: true }); await flush(); expect(setup.captureCharFrame()).toContain("Search Gator commands"); color("Search Gator commands", [228, 235, 229])
    setup.mockInput.pressEscape(); await flush(); expect(client.draft.text).toBe("Keep first draft")
    await client.close(); await setup.dispose(); client = new GatorClient(new Application(options)); setup = await render(); await client.start(); await flush()
    expect(client.state?.theme).toBe("light"); expect(client.draft.text).toBe("Keep first draft"); color("Gator", [246, 248, 245]); expect(requests).toBe(1)
    await choose("High contrast", "contrast"); color("Appearance", [0, 0, 0])
    await choose("Monochrome", "mono"); color("Appearance", [16, 22, 18])
    await choose("Dark", "gator"); color("Appearance", [16, 22, 18]); expect(client.draft.text).toBe("Keep first draft"); expect(requests).toBe(1)
  } finally { await client.close(); if (!setup.renderer.isDestroyed) await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
