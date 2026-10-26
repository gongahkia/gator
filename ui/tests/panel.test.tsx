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
    expect(host.sent.at(-1)?.key).toBe("ctrl+r")
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

test("loaded filters keep original record indices and empty results cannot activate a hidden item", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-loaded-filter-"))
  const host = new FixtureHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } })
  host.current.panel = { id: "models", kind: "models", title: "Models", selected: 0, rows: [{ label: "First provider" }, { label: "Second provider" }, { label: "Anthropic fixture" }] }
  host.dispatch = async frame => { if (frame.panel?.action === "select" && host.current.panel) host.change({ ...host.current, panel: { ...host.current.panel, selected: frame.panel.row } }) }
  const client = new GatorClient(host)
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 32 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  try {
    await client.start(); await flush(); setup.mockInput.pressKey("f", { ctrl: true }); await flush()
    await setup.mockInput.typeText("Anthropic"); setup.mockInput.pressEnter(); await flush()
    expect(host.sent.at(-1)?.panel?.row).toBe(2); expect(host.current.panel?.selected).toBe(2)
    expect(setup.captureCharFrame()).toContain("Anthropic fixture"); expect(setup.captureCharFrame()).not.toContain("First provider")
    setup.mockInput.pressEnter(); await flush(); expect(host.sent.at(-1)?.panel?.action).toBe("key")
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); await setup.mockInput.typeText("has no matches"); setup.mockInput.pressEnter(); await flush()
    expect(setup.captureCharFrame()).toContain("No matching loaded items")
    const count = host.sent.length; setup.mockInput.pressEnter(); await flush(); expect(host.sent).toHaveLength(count)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
})
