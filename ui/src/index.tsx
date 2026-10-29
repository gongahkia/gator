import { render } from "@opentui/solid"
import { createCliRenderer, RGBA } from "@opentui/core"
import { Application } from "@gator/application-host/application"
import { homedir } from "node:os"
import { join, resolve } from "node:path"
import { App } from "./app"
import { GatorClient } from "./gator/client"
import { resumeID, resumeCommand } from "./gator/resume"
import { clipboardRead, clipboardWrite } from "@gator/application-host/native"

const requestedConversation = resumeID(process.argv.slice(2))
const userData = process.env.GATOR_TERMINAL_USER_DATA || process.env.GATOR_DESKTOP_USER_DATA || (process.platform === "darwin" ? join(homedir(), "Library/Application Support/Gator") : process.platform === "win32" ? join(process.env.APPDATA || homedir(), "Gator") : join(process.env.XDG_CONFIG_HOME || join(homedir(), ".config"), "Gator"))
const host = new Application({ binary: process.env.GATOR_TERMINAL_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")),
  env: { ...process.env, GATOR_UI_FRONTEND: "terminal" },
  userData, workspace: process.env.GATOR_TERMINAL_WORKSPACE || process.env.GATOR_DESKTOP_WORKSPACE || join(userData, "workspace") })
const client = new GatorClient(host)
const renderer = await createCliRenderer({ backgroundColor: RGBA.defaultBackground(), exitOnCtrlC: false, useKittyKeyboard: {}, openConsoleOnError: false, useMouse: true })
let closing = false
async function quit() {
  if (closing) return
  const id = client.state?.conversation_id
  await client.close()
  closing = true; renderer.destroy()
  await renderer.closed
  const launcher = process.env.GATOR_TERMINAL_LAUNCHER || resolve(import.meta.dir, "../launcher/gator.mjs")
  const command = resumeCommand(id, launcher, process.execPath, !!process.env.GATOR_TERMINAL_LAUNCHER && !launcher.endsWith(".mjs"))
  if (command) process.stdout.write(`\nResume this chat:\n  ${command}\n`)

}
process.once("SIGTERM", () => { void quit() }); process.once("SIGHUP", () => { void quit() })
renderer.once("destroy", () => { if (!closing) void client.close() })
await render(() => <App client={client} quit={quit} clipboard={{ read: async () => ({ data: await clipboardRead(), mime: "text/plain" }), write: async text => { try { await clipboardWrite(text) } catch (error) { if (!renderer.copyToClipboardOSC52(text)) throw error } } }} />, renderer)
void client.start(requestedConversation).catch(error => client.fail(error))
