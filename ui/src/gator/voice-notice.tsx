import { Show } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"
import { useKeyHints } from "./key-hints"

export function VoiceNotice() {
  const data = useData(), theme = useTheme(), hint = useKeyHints(), dimensions = useTerminalDimensions()
  const recording = () => data.voicePhase === "recording", transcribing = () => data.voicePhase === "transcribing"
  return <Show when={data.voicePhase !== "idle"}><box id="voice-notice" flexShrink={0} height={recording() || transcribing() ? 2 : 1}>
    <text height={1} wrapMode="none" truncate fg={theme.text.feedback.warning.base}>
      {dimensions().width >= 60 || !recording() ? `${recording() ? "Recording" : transcribing() ? "Transcribing" : data.voicePhase} · ` : ""}{recording() ? `${hint("gator.voice")} finish · ` : ""}{hint("gator.cancel")} cancel
    </text>
    <Show when={recording() || transcribing()}><text height={1} wrapMode="none" truncate fg={theme.text.muted}>{dimensions().width < 60 ? `${recording() ? "Recording" : "Transcribing"} · ` : ""}{data.voiceDestination}{dimensions().width >= 60 ? " · Transcript goes into your editable draft" : ""}</text></Show>
  </box></Show>
}
