import { expect, test } from "bun:test"
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises"
import { basename, join, resolve } from "node:path"
import { tmpdir } from "node:os"
import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { Application } from "@gator/application-host/application"
import { GatorClient } from "../src/gator/client"

const binary = process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/" + (process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"))
test("existing desktop profile and draft formats reopen idempotently with files, versions and per-profile isolation", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-profile-continuity-")), userData = join(root, "existing-electron-data")
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config") }
  const options = { binary, env, userData, workspace: join(root, "workspace") }
  const cli = (args: string[]) => execFileSync(binary, args, { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, encoding: "utf8" })
  cli(["note", "add", "--title", "Existing desktop knowledge", "--body", "This private note predates the frontend switch."])
  const original = new GatorClient(new Application(options))
  let context = "", fileID = "", revision = ""
  try {
    await original.start(); context = original.context
    const source = join(root, "original.txt"); await writeFile(source, "Immutable original")
    await original.attach([source]); fileID = original.draft.uploads![0].id
    const request = { action: "open" as const, context, ref: { kind: "upload" as const, id: fileID } }
    const opened = await original.host.file(request)
    const saved = await original.host.file({ ...request, action: "save", expected: opened.current, edit: { text: "Existing working version" } }); revision = saved.current
    await original.host.file({ ...request, action: "draft", expected: saved.current, edit: { text: "Uncommitted file edit" } })
    await original.close()
    // Seed the exact existing Electron on-disk record, including disk-only metadata.
    const draftFile = join(userData, "drafts", createHash("sha256").update(options.env.GATOR_STATE_DIR).digest("hex") + ".json")
    await mkdir(join(userData, "drafts"), { recursive: true })
    const record = { version: 1, enabled: true, drafts: [{ context, title: "Existing chat", source: original.state?.source ?? "", text: "Existing unsent desktop draft", uploads: original.draft.uploads, updated: Date.now() }] }
    await writeFile(draftFile, JSON.stringify(record), { mode: 0o600 }); const bytes = await readFile(draftFile, "utf8")
    for (let pass = 0; pass < 2; pass++) {
      const client = new GatorClient(new Application(options))
      try {
        await client.start(); expect(client.context).toBe(context); expect(client.draft.text).toBe("Existing unsent desktop draft")
        expect(client.draft.uploads?.[0].id).toBe(fileID)
        const restored = await client.host.file(request)
        expect(restored.current).toBe(revision); expect(restored.versions).toHaveLength(2); expect(restored.draft?.text).toBe("Uncommitted file edit")
        expect(await readFile(source, "utf8")).toBe("Immutable original")
        expect(await readFile(draftFile, "utf8")).toBe(bytes)
        await client.send({ type: "submit", text: "/note list" }); expect(JSON.stringify(client.state?.messages)).toContain("Existing desktop knowledge")
      } finally { await client.close() }
    }
    const isolated = new GatorClient(new Application({ ...options, env: { ...env, GATOR_STATE_DIR: join(root, "other-state"), GATOR_CONFIG_DIR: join(root, "other-config") } }))
    try { await isolated.start(); expect(isolated.draft.text).toBe(""); expect(isolated.draft.uploads ?? []).toHaveLength(0); expect(isolated.state?.messages).toHaveLength(0); expect(await readFile(draftFile, "utf8")).toBe(bytes) }
    finally { await isolated.close() }
    const disabled = new GatorClient(new Application(options))
    try { await disabled.start(); await disabled.host.drafts.request({ action: "configure", enabled: false }); await disabled.edit("Only in this process"); expect(disabled.draft.text).toBe("Only in this process"); expect((await disabled.host.drafts.request({ action: "list" })).drafts).toHaveLength(0) }
    finally { await disabled.close() }
    const restart = new GatorClient(new Application(options))
    try { await restart.start(); expect(restart.draft.text).toBe(""); expect((await restart.host.drafts.request({ action: "list" })).enabled).toBe(false) }
    finally { await restart.close() }
    expect(cli(["note", "list"])).toContain("Existing desktop knowledge")
  } finally { await original.close(); await rm(root, { recursive: true, force: true }) }
}, 30000)

test("relative profile overrides keep the existing Unix/Windows backend interpretation", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-relative-terminal-")), workspace = join(root, "workspace")
  await mkdir(workspace)
  const env = { ...process.env, GATOR_STATE_DIR: basename(root) + "-state", GATOR_CONFIG_DIR: basename(root) + "-config" }
  const host = new Application({ binary, env, workspace, userData: join(root, "app") })
  // Windows resolves against the launching host. Unix retains workspace-relative
  // Go behavior, matching the retained desktop rather than silently relocating it.
  const stateRoot = process.platform === "win32" ? resolve(env.GATOR_STATE_DIR) : join(workspace, env.GATOR_STATE_DIR)
  const configRoot = process.platform === "win32" ? resolve(env.GATOR_CONFIG_DIR) : join(workspace, env.GATOR_CONFIG_DIR)
  execFileSync(binary, ["note", "add", "--title", "Kept relative profile", "--body", "Existing profile remains selected."], { env: { ...env, GATOR_STATE_DIR: stateRoot, GATOR_CONFIG_DIR: configRoot, GATOR_INTERNAL_COMMANDS: "1" } })
  const client = new GatorClient(host)
  try { await client.start(); await client.send({ type: "submit", text: "/note list" }); expect(JSON.stringify(client.state?.messages)).toContain("Kept relative profile") }
  finally { await client.close(); await rm(root, { recursive: true, force: true }); if (process.platform === "win32") { await rm(stateRoot, { recursive: true, force: true }); await rm(configRoot, { recursive: true, force: true }) } }
})
