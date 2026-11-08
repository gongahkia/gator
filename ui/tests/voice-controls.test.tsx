import { expect, test } from "bun:test"
import { testRender } from "@opentui/solid"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { VoiceSession } from "@gator/application-host/voice-session"
import { App } from "../src/app"
import { GatorClient } from "../src/gator/client"
import { FixtureHost, state } from "./fixture"

test("dictation shortcut reviews its route once per chat/session, stages editable text, and cancels directly", async () => {
  const root = await mkdtemp(join(tmpdir(), "gator-voice-controls-"))
  class VoiceHost extends FixtureHost { mutable() {} }
  const host = new VoiceHost({ binary: "/unused", workspace: root, userData: root, env: { GATOR_STATE_DIR: root } }), client = new GatorClient(host)
  let captures = 0, transcriptions = 0, cancelled = 0, route = "local-route", provider = "whisper-local", held = false
  const voice = new VoiceSession(host, {
    capture: () => { captures++; let finish!: (data: Uint8Array) => void; const audio = new Promise<Uint8Array>(resolve => { finish = resolve }); return { audio, stop: () => finish(new Uint8Array([1, 2])), cancel: () => { cancelled++; finish(new Uint8Array()) } } },
    call: async (_binary, _workspace, _env, request, signal) => {
      const status = { ready: true, provider, model: "base", route, preferences: { spoken_replies: false }, max_seconds: 120, max_audio_bytes: 1024 }
      if (request.action === "transcribe") {
        transcriptions++
        if (held) await new Promise((_, reject) => signal!.addEventListener("abort", () => reject(new Error("Cancelled")), { once: true }))
        return { ...status, text: "Editable spoken words." }
      }
      return status
    }, playback: async () => {},
  })
  let quits = 0
  const setup = await testRender(() => <App client={client} voice={voice} quit={async () => { quits++ }} clipboard={{ read: async () => ({ data: "", mime: "text/plain" }), write: async () => {} }} />, { width: 110, height: 34 })
  const flush = async () => { await setup.flush(); await Bun.sleep(25); await setup.flush() }
  const invoke = async () => { setup.mockInput.pressKey("v", { ctrl: true }); await flush() }
  try {
    await client.start(); await client.edit("Keep the draft.")
    await invoke(); expect(setup.captureCharFrame()).toContain("Start microphone recording?")
    expect(captures).toBe(0); expect(setup.captureCharFrame()).toContain("transcribed locally")
    for (const [width, height] of [[110, 34], [80, 24], [60, 20], [42, 14]]) { setup.resize(width!, height!); await flush(); expect(setup.captureCharFrame()).toContain("confirm enter") }
    setup.mockInput.pressEnter(); await flush(); expect(captures).toBe(1)
    expect(setup.captureCharFrame()).toContain("Local Whisper"); expect(setup.captureCharFrame()).toContain("Ctrl+V finish")
    await invoke(); expect(transcriptions).toBe(1); expect(client.draft.text).toBe("Keep the draft.\nEditable spoken words.")
    expect(host.sent).toHaveLength(0)
    setup.resize(110, 34); await invoke(); expect(captures).toBe(2)
    expect(setup.captureCharFrame()).not.toContain("Start microphone recording?")
    setup.mockInput.pressKey("c", { ctrl: true }); await flush(); expect(cancelled).toBe(1); expect(voice.phase).toBe("idle"); expect(quits).toBe(0)
    await invoke(); held = true; await invoke(); expect(voice.phase).toBe("transcribing")
    const before = client.draft.text
    setup.mockInput.pressKey("c", { ctrl: true }); await flush(); expect(voice.phase).toBe("idle"); expect(client.draft.text).toBe(before); expect(client.issue).toBe("")
    route = "cloud-route"; provider = "openai"
    await invoke(); expect(captures).toBe(3); expect(setup.captureCharFrame()).toContain("Audio is sent to openai")
    route = "changed-again"; setup.mockInput.pressEnter(); await flush()
    expect(captures).toBe(3); expect(client.issue).toContain("voice destination changed")
    await invoke(); setup.mockInput.pressEscape(); await flush(); expect(captures).toBe(3)
    host.change(state("second")); await flush(); await invoke()
    expect(setup.captureCharFrame()).toContain("Start microphone recording?"); expect(captures).toBe(3)
    expect(host.sent).toHaveLength(0)
  } finally { voice.cancel(); await client.close(); await setup.dispose(); await rm(root, { recursive: true, force: true }) }
}, 20000)
