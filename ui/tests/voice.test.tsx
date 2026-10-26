import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises"
import { join, resolve, delimiter } from "node:path"
import { tmpdir } from "node:os"
import { Application } from "@gator/application-host/application"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"

// Exercise the real native process boundary with synthetic PCM; microphone and
// speaker hardware permissions still require native host acceptance.
test.skipIf(process.platform !== "linux")("keyboard dictation reviews audio in its original draft and cancels without submitting", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-terminal-voice-")), helpers = join(root, "helpers"), config = join(root, "config/gator"), oldPath = process.env.PATH
  let requests = 0, chats = 0, release = () => {}, gate: Promise<void> = Promise.resolve()
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, async fetch(request) {
    const payload = await request.json() as { contents: { parts: { inlineData: { data: string } }[] }[] }
    if (request.url.includes(":generateContent")) {
      requests++; const audio = Buffer.from(payload.contents[0].parts[0].inlineData.data, "base64")
      expect(audio.subarray(0, 4).toString()).toBe("RIFF"); expect(audio.length).toBeGreaterThan(44)
      await gate
      return Response.json({ candidates: [{ content: { parts: [{ text: "Review this spoken request." }] } }], usageMetadata: { totalTokenCount: 25 } })
    }
    chats++; return Response.json({ choices: [{ message: { role: "assistant", content: "A new reply." }, finish_reason: "stop" }] })
  } })
  await mkdir(helpers); await mkdir(config, { recursive: true })
  const denied = join(root, "denied"), captured = join(root, "capture-started")
  await writeFile(join(helpers, "ffmpeg"), `#!${process.execPath}\nimport {existsSync,writeFileSync} from 'node:fs'; if(existsSync(${JSON.stringify(denied)})){process.stderr.write('Microphone permission denied');process.exit(1)} writeFileSync(${JSON.stringify(captured)},'yes'); process.stdin.on('data',()=>{process.stdout.write(Buffer.alloc(3200),()=>process.exit(0))});\n`, { mode: 0o700 })
  process.env.PATH = helpers + delimiter + oldPath
  await writeFile(join(config, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "fixture", model: "fixture" }, custom_providers: [{ id: "fixture", base_url: `http://127.0.0.1:${server.port}/chat`, models: ["fixture"], default_model: "fixture" }], provider_endpoints: { gemini: `http://127.0.0.1:${server.port}/v1beta` }, desktop: { voice: { provider: "gemini", model: "voice-fixture", language: "en" } } }))
  const client = new GatorClient(new Application({ binary: process.env.GATOR_TEST_BINARY || resolve(import.meta.dir, "../runtime/gator-runtime"), userData: join(root, "app"), workspace: join(root, "workspace"), env: { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GEMINI_API_KEY: "local-fixture" } }))
  const setup = await testRender(() => <App client={client} quit={() => client.close()} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 35 })
  const flush = async () => { await setup.flush(); await Bun.sleep(30); await setup.flush() }
  const until = async (predicate: () => boolean | Promise<boolean>, label: string) => { const end = Date.now() + 10000; while (!await predicate()) { if (Date.now() > end) throw new Error(label + ": " + client.issue); await flush() } }
  const action = async (label: string) => {
    await client.navigate("/voice"); await until(() => setup.captureCharFrame().includes("Voice ·"), "voice controls")
    await setup.mockInput.typeText(label); await flush(); setup.mockInput.pressEnter(); await flush()
  }
  const start = async () => { await action("Start dictation"); await until(() => setup.captureCharFrame().includes("Start microphone recording?"), "explicit microphone review"); setup.mockInput.pressEnter(); await until(() => client.voicePhase === "recording", "capture"); await until(async () => (await readFile(captured, "utf8").catch(() => "")) === "yes", "helper ready") }
  try {
    await client.start(); await client.edit("Keep my draft."); const original = client.context
    await start(); expect(requests).toBe(0)
    await expect(client.submit()).rejects.toThrow("Finish or cancel dictation")
    gate = new Promise(resolve => { release = resolve })
    await action("Stop and transcribe"); await until(() => requests === 1, "voice request")
    expect(client.draft.text).toBe("Keep my draft."); expect(chats).toBe(0)
    release(); await until(() => client.draft.text.includes("Review this spoken request."), "reviewable transcription")
    expect(client.context).toBe(original); expect(client.state?.messages).toHaveLength(0)
    const usage = JSON.parse(await readFile(join(root, "state/gator/usage/days", new Date().toISOString().slice(0, 10) + ".json"), "utf8"))
    const charges = Object.values(usage.Charges) as { Category: string; Provider: string; Tokens: number; Failed: boolean }[]
    expect(charges).toHaveLength(1); expect(charges[0]).toMatchObject({ Category: "voice", Provider: "gemini/voice-fixture", Tokens: 25, Failed: false })
    expect(JSON.stringify(usage)).not.toContain("Review this spoken request.")
    const draft = client.draft.text
    await start(); await action("Cancel recording"); await until(() => client.voicePhase === "idle", "cancel capture")
    expect(requests).toBe(1); expect(client.draft.text).toBe(draft)
    await writeFile(denied, "yes"); await action("Start dictation"); setup.mockInput.pressEnter()
    await until(() => client.issue.includes("Microphone permission denied") && client.voicePhase === "idle", "permission error")
    expect(client.draft.text).toBe(draft); await rm(denied)
    await start(); gate = new Promise(resolve => { release = resolve }); await action("Stop and transcribe")
    await until(() => requests === 2, "pending transcription"); await action("Cancel recording / transcription"); release()
    await until(() => client.voicePhase === "idle", "cancel transcription"); expect(client.draft.text).toBe(draft)
    await start(); await client.navigate("/new"); await until(() => client.context !== original && client.voicePhase === "idle", "chat-bound cancellation")
    expect(client.draft.text).toBe(""); expect(client.drafts.get(original)?.text).toBe(draft); expect(chats).toBe(0)
  } finally { release(); await client.close(); await setup.dispose(); process.env.PATH = oldPath; server.stop(true); await rm(root, { recursive: true, force: true }) }
}, 45000)
