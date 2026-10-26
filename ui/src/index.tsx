import { render } from "@opentui/solid"
import { createCliRenderer } from "@opentui/core"
import { Application } from "@gator/application-host/application"
import { homedir } from "node:os"
import { join, resolve } from "node:path"
import { App } from "./app"
import { GatorClient } from "./gator/client"

const userData = process.env.GATOR_TERMINAL_USER_DATA || process.env.GATOR_DESKTOP_USER_DATA || (process.platform === "darwin" ? join(homedir(), "Library/Application Support/Gator") : process.platform === "win32" ? join(process.env.APPDATA || homedir(), "Gator") : join(process.env.XDG_CONFIG_HOME || join(homedir(), ".config"), "Gator"))
const host = new Application({ binary: process.env.GATOR_TERMINAL_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime")),
  userData, workspace: process.env.GATOR_TERMINAL_WORKSPACE || process.env.GATOR_DESKTOP_WORKSPACE || join(userData, "workspace") })
const client = new GatorClient(host)
const renderer = await createCliRenderer({ exitOnCtrlC: false, useKittyKeyboard: {}, openConsoleOnError: false, useMouse: true })
let closing = false
async function quit() {
  if (closing) return
  await client.close()
  closing = true; renderer.destroy()
}
process.once("SIGTERM", () => { void quit() }); process.once("SIGHUP", () => { void quit() })
renderer.once("destroy", () => { if (!closing) void client.close() })
await render(() => <App client={client} quit={quit} clipboard={{ read: async () => undefined, write: async text => { renderer.copyToClipboardOSC52(text) } }} />, renderer)
void client.start().catch(error => client.fail(error))
