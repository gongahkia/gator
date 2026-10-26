import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost } from "./fixture"

test("credential keyboard and paste buffers paint only masks and preserve explicit form submission", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-secret-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current.panel = { id: "provider-form", kind: "form", title: "Provider credentials", selected: 0,
    rows: [{ label: "API key", input: { secret: true, limit: 100 } }], hints: [{ shortcut: "ctrl+enter", action: "Review configuration" }] }
  const client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  try {
    await client.start(); await flush(); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("credential")
    await setup.mockInput.typeText("sk-keyboard-secret"); await flush()
    expect(setup.captureCharFrame()).not.toContain("sk-keyboard-secret")
    setup.mockInput.pressKey("u", { ctrl: true }); await flush()
    await setup.mockInput.pasteBracketedText("sk-pasted-secret🦎\n"); await flush()
    expect(setup.captureCharFrame()).not.toContain("sk-pasted-secret")
    expect(setup.captureCharFrame()).toContain("••••")
    expect(host.sent).toHaveLength(0)
    setup.mockInput.pressBackspace(); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).not.toContain("sk-pasted-secret")
    setup.mockInput.pressKey("F2"); await flush()
    expect(host.sent.at(-1)?.key).toBe("ctrl+enter")
    expect(host.sent.at(-1)?.panel?.fields?.[0]).toBe("sk-pasted-secret")
    expect(client.draft.text).toBe("")
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})

test("unsaved typed form fields survive cancelled hub navigation and leave only after discard", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-form-guard-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current.panel = { id: "retained-form", kind: "form", title: "Recurring check", selected: 0, rows: [{ label: "Title", input: { limit: 100, value: "" } }] }
  const client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  try {
    await client.start(); await flush(); setup.mockInput.pressEnter(); await flush()
    await setup.mockInput.typeText("Keep unsaved check title"); setup.mockInput.pressEnter(); await flush()
    const cancelled = client.navigate("/saved"); await flush()
    expect(setup.captureCharFrame()).toContain("Leave this unsaved form?")
    setup.mockInput.pressEscape(); await cancelled; await flush()
    expect(client.route).toBe("chat"); expect(setup.captureCharFrame()).toContain("Keep unsaved check title")
    const confirmed = client.navigate("/saved"); await flush(); setup.mockInput.pressEnter(); await confirmed; await flush()
    expect(client.route).toBe("saved"); expect(host.sent).toHaveLength(0)
    expect(client.navigationGuard).toBeUndefined()
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
