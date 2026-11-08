import { draftLinks, markdownLink } from "./draft-links"
import { onCleanup } from "solid-js"
import { NativeImage } from "@opentui/core"
import { useRenderer } from "@opentui/solid"
import { openLink, clipboardRead, clipboardWrite, clipboardImage, speechVoices } from "@gator/application-host/native"
import { VoiceSession } from "@gator/application-host/voice-session"
import { useClient } from "../context/client"
import type { Choice } from "./actions"

export function useNative(controls: { actions: Record<string, () => unknown>; menu(title: string, choices: Choice[]): void; prompt(title: string, value: string, save: (text: string) => unknown, description?: string): void; confirm(title: string, message: string, run: () => unknown): void }, suppliedVoice?: VoiceSession) {
  const client = useClient(), renderer = useRenderer(), voice = suppliedVoice ?? new VoiceSession(client.host)
  if (voice.application !== client.host) throw new Error("Voice must belong to this application's current host.")
  const microphoneConsent = new Set<string>()
  const stopDictation = async () => {
    try {
      const result = await voice.stop()
      if (result.context !== client.context) throw new Error("The conversation changed during dictation.")
      await client.edit(client.draft.text + (client.draft.text ? "\n" : "") + result.text)
    } catch (error) {
      // Explicit cancellation also invalidates a pending transcription result.
      if (voice.phase === "idle" && !voice.context) return
      throw error
    }
  }
  const startDictation = async () => {
    const context = client.context, status = await voice.status()
    if (context !== client.context) throw new Error("The chat changed before microphone review.")
    if (!status.ready) return controls.actions.voice()
    if (!status.route) throw new Error("The voice destination could not be verified. Reconfigure voice before recording.")
    const key = context + ":" + status.route
    const begin = async () => {
      if (context !== client.context) throw new Error("The chat changed before recording. Start again in the intended chat.")
      voice.stopPlayback()
      client.voiceDestination = status.provider === "whisper-local" ? "Local Whisper" : `${status.provider} / ${status.model}`
      await voice.start(status.route)
      microphoneConsent.add(key)
    }
    if (microphoneConsent.has(key)) return begin()
    controls.confirm("Start microphone recording?", (status.provider === "whisper-local" ? "Audio is transcribed locally using your installed Whisper model." : `Audio is sent to ${status.provider} / ${status.model} when you finish recording.`) + " Allow explicit recordings in this chat for this session and destination. Each recording starts only when you invoke dictation. You can revoke consent in /voice. Review and edit the transcript before sending. OS microphone permission and ffmpeg are required.", begin)
  }
  controls.actions["toggle-dictation"] = () => voice.phase === "recording" ? stopDictation() : voice.phase === "transcribing" || voice.phase === "downloading" ? voice.cancel() : startDictation()
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
      { title: "Start dictation", description: status.issue || `${status.provider} / ${status.model}; explicit microphone capture, at most ${status.max_seconds} seconds`, disabled: !status.ready || voice.phase !== "idle", run: startDictation },
      { title: "Stop and transcribe into this draft", disabled: voice.phase !== "recording", run: stopDictation },
      { title: "Cancel recording / transcription", disabled: voice.phase === "idle", run: () => voice.cancel() },
      { title: "Revoke microphone consent for this session", description: "Stops current dictation and requires a new review next time.", run: () => { microphoneConsent.clear(); voice.cancel() } },
      { title: "Stop spoken reply", disabled: !voice.playing, run: () => voice.stopPlayback() },
      { title: status.preferences.spoken_replies ? "Disable spoken replies" : "Enable spoken replies", run: () => voice.configure({ spoken_replies: !status.preferences.spoken_replies }) },
      { title: "Voice provider", run: () => controls.menu("Dictation provider", ["whisper-local", "openai", "gemini"].map(provider => ({ title: provider, run: () => voice.configure({ provider }) }))) },
      ...(status.local?.models ?? []).map(model => ({ title: `${model.available ? "Select" : "Download"} local Whisper ${model.label}`, description: `${Math.ceil(model.bytes / 1024 / 1024)} MiB${status.local?.runtime_ready ? "" : " · Install whisper-cli to transcribe locally"}`, disabled: voice.phase !== "idle", run: async () => {
        if (model.available) await voice.configure({ provider: "whisper-local", model: model.id })
        else controls.confirm("Download local voice model?", `${model.label} · ${Math.ceil(model.bytes / 1024 / 1024)} MiB. Gator verifies the downloaded model. Audio stays local when using Whisper.`, async () => { await voice.download(model.id); await voice.configure({ provider: "whisper-local", model: model.id }) })
      } })),
      { title: "System voice", run: async () => controls.menu("Installed speech voices", [{ title: "Use system default", run: () => voice.configure({ system_voice: "" }) }, ...(await speechVoices()).map(item => ({ title: item.name, description: item.language, run: () => voice.configure({ system_voice: item.name }) }))]) },
      { title: "Voice model", description: status.model, run: () => controls.prompt("Voice model", status.model, model => voice.configure({ model })) },
      { title: "Language", description: status.preferences.language, run: () => controls.prompt("Voice language", status.preferences.language || "", language => voice.configure({ language })) },
      { title: "Speech rate", description: String(status.preferences.rate || 1), run: () => controls.prompt("Speech rate (0.5–2)", String(status.preferences.rate || 1), rate => voice.configure({ rate: Number(rate) })) },
      { title: "Configure provider credentials", run: () => client.navigate("/model") },
    ])
  }
  controls.actions["read-aloud"] = () => controls.menu("Read a reply aloud", (client.state?.messages ?? []).filter(message => message.role === "Gator" && message.text && !message.result).map(message => ({ title: message.text.slice(0, 100), run: () => voice.read(message.text.replace(/```[\s\S]*?```/g, " Code block omitted. ").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/[`#*>_]/g, " ").trim().slice(0, 12000)) })))
  controls.actions["stop-reading"] = () => voice.stopPlayback()
  controls.actions.links = () => {
    const context = client.context, draft = client.draft.text
    const groups = [{ text: draft, label: "Message draft", draft: true }, ...(client.state?.messages ?? []).map((message, index) => ({ text: message.text, label: `${message.role} · message ${index + 1}`, draft: false }))]
    controls.menu("Links", [
      ...(client.state?.panel?.links ?? []).map(link => ({ title: link.label, description: link.url, run: () => openLink(link.url) })),
      ...groups.flatMap(group => draftLinks(group.text).map(link => ({ title: link.label, description: group.label, run: () => {
        const stage = async (label: string, url: string) => {
          if (context !== client.context) throw new Error("Return to the original conversation before editing this link.")
          const value = markdownLink(label, url)
          if (group.draft) {
            if (client.draft.text !== group.text) throw new Error("The draft changed. Open /links again to edit its current text.")
            await client.edit(group.text.slice(0, link.start) + value + group.text.slice(link.end))
          } else await client.edit([client.draft.text, value].filter(Boolean).join("\n\n"))
        }
        controls.menu("Link actions", [
          { title: "Open in browser", description: link.url, run: () => openLink(link.url) },
          { title: "Copy link", run: () => clipboardWrite(link.url) },
          { title: "Edit address in draft", run: () => controls.prompt("Link address", link.url, url => stage(link.label, url)) },
          { title: "Edit label in draft", run: () => controls.prompt("Link label", link.label, label => stage(label, link.url)) },
        ])
      } }))),
    ])
  }

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
