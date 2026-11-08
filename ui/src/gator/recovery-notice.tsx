import { Show } from "solid-js"
import { Keymap } from "../context/keymap"
import { useData } from "../context/data"
import { useDialog } from "../ui/dialog"
import { useTheme } from "../context/theme"
import { useKeyHints } from "./key-hints"
import { useTerminalDimensions } from "@opentui/solid"

export function RecoveryNotice(props: { open(): void }) {
  const data = useData(), dialog = useDialog(), theme = useTheme(), hint = useKeyHints(), dimensions = useTerminalDimensions()
  const disconnected = () => data.snapshot.connection === "disconnected" || !!data.snapshot.error
  const visible = () => !!(disconnected() || data.issue || data.snapshot.error || data.snapshot.sessionIssue)
  const shortcutActive = () => visible() && data.route === "chat" && !data.state?.panel && dialog.stack.length === 0
  Keymap.createLayer(() => ({ mode: "global", priority: 5, enabled: shortcutActive(), commands: [
    { id: "gator.recover", title: "Review recovery options", bind: "ctrl+r", palette: true, run: props.open },
  ] }))
  return <Show when={visible()}><text id="recovery-notice" height={1} wrapMode="none" truncate fg={theme.text.feedback.warning.base} onMouseUp={props.open}>
    {disconnected() ? "Connection lost" : "Action needs attention"} · {shortcutActive() ? `${hint("gator.recover")} recovery` : "Open recovery"}{dimensions().width >= 60 && (data.draft.text || data.draft.uploads?.length) ? " · Draft kept" : ""}
  </text></Show>
}
