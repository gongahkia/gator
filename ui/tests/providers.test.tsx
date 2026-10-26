import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, readFile, rm } from "node:fs/promises"
import { join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("provider forms can review and save explicit metadata and rename without trapping Enter in field editing", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-provider-")), config = join(root, "config/gator/config.json")
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), workspace: join(root, "workspace"), userData: join(root, "app"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), OPENAI_API_KEY: "" } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 38 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + JSON.stringify(client.state?.panel)); await flush() } }
  const select = async (label: string) => {
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === label), "available " + label); await flush()
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); setup.mockInput.pressEnter(); await flush()
    await until(() => client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label === label, "selected " + label)
  }
  const field = async (label: string, text: string, multiline = false) => { await select(label); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(text); if (multiline) setup.mockInput.pressKey("s", { ctrl: true }); else setup.mockInput.pressEnter(); await flush() }
  const configValue = async () => JSON.parse(await readFile(config, "utf8").catch(() => "{}"))
  try {
    await client.start(); await client.edit("Keep provider setup draft"); await client.navigate("/model")
    await select("Cloud API key"); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.title === "Cloud models", "cloud setup"); await flush()
    setup.mockInput.pressKey("n"); await until(() => client.state?.panel?.title === "Custom provider", "new provider")
    await field("Provider ID", "review-fixture"); await field("Chat Completions URL", "http://127.0.0.1:9/v1/chat/completions")
    await field("Model IDs", "one\ntwo", true); await field("Default model", "one")
    expect((await configValue()).custom_providers ?? []).toHaveLength(0)
    await select("Provider ID")
    setup.mockInput.pressKey("F2"); await until(() => client.state?.panel?.title === "Save custom provider?", "metadata review")
    expect((await configValue()).custom_providers ?? []).toHaveLength(0); expect(JSON.stringify(client.state?.panel)).toContain("127.0.0.1:9")
    setup.mockInput.pressEnter(); await until(async () => (await configValue()).custom_providers?.length === 1, "saved metadata")
    await until(() => client.state?.panel?.title === "Cloud models", "returned catalog"); await flush()
    const model = client.state!.panel!.rows!.find(row => row.label.includes("review-fixture") && row.label.includes("one"))!.label
    await select(model); setup.mockInput.pressKey("e"); await until(() => client.state?.panel?.title === "Rename model", "rename form")
    await field("Display name", "My private model")
    setup.mockInput.pressKey("F2"); await until(async () => (await configValue()).model_aliases?.["review-fixture:one"] === "My private model", "saved alias")
    await until(() => client.state?.panel?.title === "Cloud models", "renamed catalog")
    const openai = client.state!.panel!.rows!.find(row => row.label.startsWith("openai ·"))!.label
    await select(openai); setup.mockInput.pressKey("c"); await until(() => client.state?.panel?.title === "Configure openai", "credential form")
    await field("Endpoint override", "http://127.0.0.1:9/v1/chat/completions")
    await field("API key", "sk-terminal-private-fixture")
    expect(setup.captureCharFrame()).not.toContain("sk-terminal-private-fixture")
    expect(JSON.stringify(client.state)).not.toContain("sk-terminal-private-fixture")
    const auth = join(root, "state/gator/auth.json")
    expect(await readFile(auth, "utf8").catch(() => "")).not.toContain("sk-terminal-private-fixture")
    setup.mockInput.pressKey("F2"); await until(async () => (await readFile(auth, "utf8").catch(() => "")).includes("sk-terminal-private-fixture"), "saved private credential")
    expect(JSON.stringify(client.state)).not.toContain("sk-terminal-private-fixture")
    expect(await readFile(config, "utf8")).not.toContain("sk-terminal-private-fixture")
    await until(() => client.state?.panel?.title === "Cloud models", "credential catalog"); await select(openai)
    setup.mockInput.pressKey("d"); await until(() => client.state?.panel?.title === "Remove stored Gator credential?", "credential removal review"); await flush()
    setup.mockInput.pressEscape(); await Bun.sleep(80); await flush()
    expect(await readFile(auth, "utf8")).toContain("sk-terminal-private-fixture")
    await until(() => client.state?.panel?.title === "Cloud models", "cancelled credential removal"); await flush()
    setup.mockInput.pressKey("d"); await until(() => client.state?.panel?.title === "Remove stored Gator credential?", "new removal review"); await flush(); setup.mockInput.pressEnter()
    await until(async () => !(await readFile(auth, "utf8")).includes("sk-terminal-private-fixture"), "explicit credential removal")
    await client.navigate("/chat"); expect(client.draft.text).toBe("Keep provider setup draft"); expect(client.state?.messages).toHaveLength(0)
  } finally { await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 45000)
