import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, readFile, writeFile, rm, stat } from "node:fs/promises"
import { join, resolve, delimiter } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

test("connection forms review private tokens, verify access and retain browser sign-in across chats", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-connections-")), config = join(root, "config/gator"), originalPath = process.env.PATH
  let reads = 0, tokenRequests = 0
  const callbackReservation = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response() }), callbackPort = callbackReservation.port; callbackReservation.stop(true)
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const path = new URL(request.url).pathname
    if (path === "/resource") { reads++; expect(request.headers.get("authorization")).toBe("Bearer private-service-token"); return Response.json({ value: 1 }) }
    if (path === "/token") { tokenRequests++; return Response.json({ access_token: "private-oauth-access", refresh_token: "private-refresh", token_type: "Bearer", expires_in: 3600 }) }
    return Response.json({}, { status: 404 })
  } })
  await mkdir(config, { recursive: true })
  const base = `http://127.0.0.1:${server.port}`, configPath = join(config, "config.json")
  await writeFile(configPath, JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: base + "/chat", models: ["fixture"], default_model: "fixture" }], connectors: [{ version: 1, id: "google", name: "Personal Google", kind: "google_workspace", resource: base, authentication: "oauth", oauth_client_id: "terminal-client", oauth_authorize_url: base + "/authorize", oauth_token_url: base + "/token", oauth_redirect_url: `http://127.0.0.1:${callbackPort}/oauth/callback`, oauth_scopes: "openid https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/gmail.readonly" }] }))
  const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")), options = { binary, userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") } }, client = new GatorClient(new Application(options))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 120, height: 42 })
  const flush = async () => { await setup.flush(); await Bun.sleep(40); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 15000; while (!await predicate()) { if (Date.now() > end) throw new Error(`${label}: ${client.issue}; ${client.state?.panel?.title}; ${client.state?.panel?.rows?.map(row => row.label).join(", ")}`); await Bun.sleep(20) } }
  const choose = async (label: string) => {
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === label), "available " + label); await flush()
    setup.mockInput.pressKey("f", { ctrl: true }); await flush(); setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(label); setup.mockInput.pressEnter(); await flush()
    await until(() => client.state?.panel?.rows?.[client.state.panel.selected ?? 0]?.label === label, "selected " + label)
    setup.mockInput.pressEnter(); await flush()
  }
  const field = async (label: string, value: string, secret = false) => { await choose(label); if (!secret) setup.mockInput.pressKey("u", { ctrl: true }); await setup.mockInput.typeText(value); expect(setup.captureCharFrame()).not.toContain("private-service-token"); setup.mockInput.pressEnter(); await flush() }
  try {
    await client.start(); await client.edit("Keep connection setup draft"); const original = client.context
    await client.navigate("/connections"); await choose("Add a service token connection")
    await field("Connection ID", "metrics"); await field("Display name", "Team metrics")
    await choose("Service"); await setup.mockInput.typeText("JSON API"); await flush(); setup.mockInput.pressEnter(); await flush()
    await field("Resource URL", base + "/resource")
    setup.mockInput.pressKey("F2"); await flush(); expect(JSON.parse(await readFile(configPath, "utf8")).connectors).toHaveLength(1)
    setup.mockInput.pressEnter(); await until(() => !!client.state?.panel?.rows?.some(row => row.label === "Set access token"), "saved connection")
    await choose("Set access token"); await field("Access token", "private-service-token", true)
    setup.mockInput.pressKey("F2"); await flush(); expect(JSON.stringify(client.state)).not.toContain("private-service-token"); expect(reads).toBe(0)
    setup.mockInput.pressEnter(); await until(() => !!client.state?.panel?.rows?.some(row => row.label === "Check account & read access"), "saved credential")
    await choose("Check account & read access"); await until(() => reads === 1 && !!client.state?.panel?.notice?.includes("Read access verified"), "read access")
    await choose("Disconnect account"); setup.mockInput.pressEnter(); await until(() => client.state?.panel?.subtitle === "Not connected", "disconnect")
    expect(JSON.stringify(client.state)).not.toContain("private-service-token")
    setup.mockInput.pressEscape(); await Bun.sleep(75); await flush(); await choose("Personal Google"); await choose("Sign in")
    setup.mockInput.pressKey("F2"); await flush(); expect(tokenRequests).toBe(0); setup.mockInput.pressEnter()
    await until(() => !!client.state?.panel?.links?.length && !!client.state.pending_connection, "browser authorization")
    const url = client.state!.panel!.links![0].url
    if (process.platform === "linux") {
      const helpers = join(root, "helpers"), opened = join(root, "browser.json"); await mkdir(helpers)
      await writeFile(join(helpers, "xdg-open"), `#!${process.execPath}\nimport {writeFile} from 'node:fs/promises'; await writeFile(${JSON.stringify(opened)}, JSON.stringify(process.argv.slice(2)));\n`, { mode: 0o700 })
      process.env.PATH = helpers + delimiter + originalPath
      setup.mockInput.pressKey("p", { ctrl: true }); await flush(); await setup.mockInput.typeText("links"); await flush(); setup.mockInput.pressEnter(); await flush(); setup.mockInput.pressEnter()
      await until(async () => !!await stat(opened).catch(() => undefined), "literal browser helper")
      expect(JSON.parse(await readFile(opened, "utf8"))).toEqual([url])
    }
    await client.navigate("/chat"); await until(() => client.context !== original && !client.snapshot.sessionSwitching, "browse while signing in")
    await client.edit("Other chat draft"); await client.navigate("/connections")
    await until(() => client.context === original && client.state?.panel?.title === "Connections", "return to owned connections")
    await choose("Personal Google")
    await until(() => !!client.state?.panel?.rows?.some(row => row.label === "Cancel sign-in"), "retained sign-in")
    const authorize = new URL(url), callback = new URL(authorize.searchParams.get("redirect_uri")!); callback.searchParams.set("state", authorize.searchParams.get("state")!); callback.searchParams.set("code", "authorized-fixture")
    await fetch(callback).catch(() => {}); await until(() => !!client.state?.panel?.rows?.some(row => row.label === "Reconnect account"), "OAuth callback")
    expect(tokenRequests).toBe(1); expect(JSON.stringify(client.state)).not.toContain("private-oauth-access")
    expect(client.draft.text).toBe("Keep connection setup draft"); expect(reads).toBe(1)
  } finally { process.env.PATH = originalPath; await client.close(); await setup.dispose(); server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 60000)
