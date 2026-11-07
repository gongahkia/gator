import { createSignal, onCleanup } from "solid-js"
import { useKeyboard, usePaste } from "@opentui/solid"
import { useTheme } from "../context/theme"
import { literal } from "./text"

// Credential bytes never enter a renderable text/edit buffer. Coloring an input
// like its background still writes the secret to the terminal/selection buffer.
export function SecretField(props: { title: string; help?: string; limit: number; save(value: string): void }) {
  const theme = useTheme(), [length, setLength] = createSignal(0)
  let value = ""
  const replace = (next: string) => { value = Array.from(next.replace(/[\x00-\x1f\x7f]/g, "")).slice(0, props.limit).join(""); setLength(Array.from(value).length) }
  useKeyboard(event => {
    if (event.eventType === "release" || event.name === "escape") return
    event.preventDefault(); event.stopPropagation()
    if (event.name === "return") { props.save(value); return }
    if (event.name === "backspace") { replace(Array.from(value).slice(0, -1).join("")); return }
    if (event.ctrl && event.name === "u") { replace(""); return }
    if (!event.ctrl && !event.meta && !event.super && !event.sequence.includes("\x1b")) replace(value + event.sequence)
  })
  usePaste(event => { event.preventDefault(); event.stopPropagation(); replace(value + new TextDecoder().decode(event.bytes)) })
  onCleanup(() => { value = "" })
  return <box paddingX={3} paddingBottom={1} gap={1}><text fg={theme.text.base}>{literal(props.title)}</text>
    <text fg={theme.text.muted}>{literal(props.help) || "Leave empty to retain the stored credential."}</text>
    <text id="credential-mask" fg={theme.accent}>{length() ? "•".repeat(Math.min(64, length())) : "[empty]"}</text>
    <text fg={theme.text.muted}>Type or paste · Backspace removes the last character · Ctrl+U clears · Enter keeps field · Esc cancels</text>
  </box>
}
