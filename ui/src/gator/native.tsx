import { onCleanup } from "solid-js"
import { NativeImage } from "@opentui/core"
import { useRenderer } from "@opentui/solid"
import { openLink, clipboardRead, clipboardWrite, clipboardImage } from "@gator/application-host/native"
import { VoiceSession } from "@gator/application-host/voice-session"
import { useClient } from "../context/client"
import type { Choice } from "./actions"

export function useNative(controls: { actions: Record<string, () => unknown>; menu(title: string, choices: Choice[]): void; prompt(title: string, value: string, save: (text: string) => unknown, description?: string): void; confirm(title: string, message: string, run: () => unknown): void }) {
  const client = useClient(), renderer = useRenderer(), voice = new VoiceSession(client.host)
  onCleanup(() => voice.close())
  controls.actions["paste-image"] = async () => {
    const context = client.context
    const bytes = await clipboardImage()
    if (context !== client.context) throw new Error("Return to the intended chat before pasting an image.")
    const selected = await client.host.pasteImages([{ type: "image/png", data: new Uint8Array(bytes) }], data => {
      const image = NativeImage.decode(data)
      try { const info = image.info(); if (info.format !== "png" || info.width * info.height > 32000000) throw new Error("Clipboard intake requires a valid PNG up to 32 million pixels."); return data } finally { image.dispose() }
    })
    if (context !== client.context) throw new Error("The chat changed before image intake completed.")
    await client.appendFiles(selected, context)
  }
  controls.actions["paste-text"] = async () => { const context = client.context, text = await clipboardRead(); if (context !== client.context) throw new Error("The chat changed before paste."); await client.edit(client.draft.text + text) }
  controls.actions["copy-message"] = () => controls.menu("Copy message", (client.state?.messages ?? []).map(message => ({ title: message.text.slice(0, 100) || message.role, run: async () => {
    try { await clipboardWrite(message.text) } catch (error) { if (!renderer.copyToClipboardOSC52(message.text)) throw error }
  } })))
  controls.actions.voice = async () => {
    const status = await voice.status()
    controls.menu(`Voice · ${voice.phase}`, [
      { title: "Start dictation", description: status.issue || `${status.provider} / ${status.model}; explicit microphone capture, at most 120 seconds`, disabled: !status.ready || voice.phase !== "idle", run: () => controls.confirm("Start microphone recording?", "Audio is sent to your configured voice provider only when you stop and transcribe. Review the resulting draft before sending. OS microphone permission and ffmpeg are required.", () => voice.start()) },
      { title: "Stop and transcribe into this draft", disabled: voice.phase !== "recording", run: async () => { const result = await voice.stop(); if (result.context !== client.context) throw new Error("The conversation changed during dictation."); await client.edit(client.draft.text + (client.draft.text ? "\n" : "") + result.text) } },
      { title: "Cancel recording / transcription", disabled: voice.phase === "idle", run: () => voice.cancel() },
      { title: "Stop spoken reply", disabled: !voice.playing, run: () => voice.stopPlayback() },
      { title: status.preferences.spoken_replies ? "Disable spoken replies" : "Enable spoken replies", run: () => voice.configure({ spoken_replies: !status.preferences.spoken_replies }) },
      { title: "Voice provider", run: () => controls.menu("Dictation provider", ["openai", "gemini"].map(provider => ({ title: provider, run: () => voice.configure({ provider }) }))) },
      { title: "Voice model", description: status.model, run: () => controls.prompt("Voice model", status.model, model => voice.configure({ model })) },
      { title: "Language", description: status.preferences.language, run: () => controls.prompt("Voice language", status.preferences.language || "", language => voice.configure({ language })) },
      { title: "Speech rate", description: String(status.preferences.rate || 1), run: () => controls.prompt("Speech rate (0.5–2)", String(status.preferences.rate || 1), rate => voice.configure({ rate: Number(rate) })) },
      { title: "Configure provider credentials", run: () => client.navigate("/model") },
    ])
  }
  controls.actions.links = () => controls.menu("Links in the current page", (client.state?.panel?.links ?? []).map(link => ({ title: link.label, description: link.url, run: () => openLink(link.url) })))
  const continueExternal = async () => {
    const draft = client.draft
    await renderer.suspend()
    try {
      const result = await client.host.runExternal()
      if (result.kind === "editor" && result.context === draft.context && result.input !== undefined) {
        try { await client.restoreEditor(draft, result.input) }
        catch (error) {
          controls.prompt("Recover returned editor text", result.input, text => client.restoreEditor(draft, text), "The original draft is kept. Shorten this returned text to the composer limit, then explicitly import it.")
          throw error
        }
      }
      if (result.error) throw new Error(result.error)
    } finally { await renderer.resume() }
  }
  controls.actions["continue-external"] = continueExternal
  controls.actions["external-editor"] = async () => { const draft = client.draft; await client.host.prepareComposerEditor(draft.context, draft.text); await continueExternal() }
  const releases = () => { void openLink("https://github.com/gongahkia/gator/releases").catch(error => client.fail(error)) }
  client.on("open-releases", releases); onCleanup(() => client.off("open-releases", releases))
  const changed = () => { if (voice.issue) client.fail(new Error(voice.issue)); client.voicePhase = voice.phase === "idle" && voice.playing ? "speaking" : voice.phase; client.emit("change") }
  voice.on("change", changed); onCleanup(() => voice.off("change", changed))
  return voice
}
