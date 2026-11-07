import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm, realpath } from "node:fs/promises"
import { join, resolve, dirname } from "node:path"
import { tmpdir } from "node:os"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

async function until(predicate: () => boolean | Promise<boolean>, label: string) {
  const deadline = Date.now() + 15000
  while (!await predicate()) { if (Date.now() > deadline) throw new Error(`Timed out: ${label}`); await Bun.sleep(20) }
}

test.skipIf(process.platform !== "darwin")("login service repair requires review, rejects stale configuration and preserves the draft", async () => {
  const root = await realpath(await mkdtemp(join(tmpdir(), "gator-service-repair-")))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime")
  const home = join(root, "home"), bin = join(root, "bin"), log = join(root, "service-calls")
  const env = { ...process.env, HOME: home, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "configuration"), PATH: bin + ":" + process.env.PATH }
  const path = join(home, "Library/LaunchAgents/com.gator.work-supervisor.plist")
  await mkdir(dirname(path), { recursive: true }); await mkdir(bin)
  const escape = (value: string) => value.replaceAll("&", "&amp;").replaceAll("<", "&lt;")
  const original = `<?xml version="1.0"?><plist version="1.0"><dict><key>Label</key><string>com.gator.work-supervisor</string><key>ProgramArguments</key><array><string>/removed/gator</string><string>job</string><string>supervisor</string></array><key>EnvironmentVariables</key><dict><key>GATOR_STATE_DIR</key><string>${escape(env.GATOR_STATE_DIR)}</string></dict></dict></plist>`
  await writeFile(path, original)
  // Every service-manager invocation is intercepted; this test cannot install
  // or stop a real LaunchAgent, including the user's production Gator label.
  await writeFile(join(bin, "launchctl"), '#!/bin/sh\nprintf "%s\\n" "$*" >> "$GATOR_SERVICE_TEST_LOG"\n', { mode: 0o700 })
  const appEnv = { ...env, GATOR_SERVICE_TEST_LOG: log }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...appEnv, GATOR_INTERNAL_COMMANDS: "1" } })
  cli(["provider", "add", "service-fixture", "--base-url", "http://127.0.0.1:9/v1/chat/completions", "--model", "fixture"])
  cli(["config", "set", "default-provider", "service-fixture"])
  const client = new GatorClient(new Application({ binary, env: appEnv, userData: join(root, "app"), workspace: join(root, "workspace") }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const calls = () => readFile(log, "utf8").catch(() => "")
  const review = async () => {
    await flush(); setup.mockInput.pressKey("f")
    await until(() => client.state?.panel?.kind === "confirmation", "repair review"); await flush()
    expect(client.state?.panel?.title).toBe("Repair start at login")
    expect(client.state?.panel?.subtitle).toContain(binary)
    expect(client.state?.panel?.subtitle).toContain(env.GATOR_CONFIG_DIR)
  }
  try {
    await client.start(); await client.edit("Keep this unsent draft")
    await client.navigate("/recovery"); await until(() => client.state?.panel?.title === "Recovery", "recovery page")
    await review(); expect(await calls()).toBe(""); expect(await readFile(path, "utf8")).toBe(original)
    setup.mockInput.pressEscape(); await until(() => client.state?.panel?.title === "Recovery", "cancel repair")
    expect(await calls()).toBe("")
    await review(); await writeFile(path, original + "\n")
    setup.mockInput.pressEnter()
    await until(() => !!client.state?.panel?.notice?.includes("changed during review"), "stale repair refusal")
    expect(await calls()).toBe(""); expect(await readFile(path, "utf8")).toBe(original + "\n")
    await flush(); setup.mockInput.pressKey("r")
    await until(() => client.state?.panel?.title === "Recovery" && !client.state.panel.notice, "fresh recovery state")
    await review(); setup.mockInput.pressEnter()
    await until(async () => (await calls()).includes("bootstrap"), "reviewed repair")
    await until(() => !!client.state?.panel?.notice?.includes("Installed local LaunchAgent"), "repair result")
    const repaired = await readFile(path, "utf8")
    expect(repaired).toContain(escape(binary)); expect(repaired).toContain(escape(env.GATOR_STATE_DIR)); expect(repaired).toContain(escape(env.GATOR_CONFIG_DIR))
    expect(client.draft.text).toBe("Keep this unsent draft")
    expect(client.state?.messages.some(message => message.role === "You")).toBe(false)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 60000)
