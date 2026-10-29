import { Show, createMemo, createEffect, createSignal, onCleanup } from "solid-js"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"
import { Spinner } from "../component/spinner"
import { literal } from "./text"

// Only current operations qualify. Retained activity, paused work, sign-in and
// manual external handoffs are not evidence that Gator is doing background work.
export function Progress() {
  const data = useData(), theme = useTheme(), [visible, setVisible] = createSignal(false)
  const label = createMemo(() => {
    if (data.snapshot.terminalBusy || data.state?.external) return ""
    if (data.snapshot.sessionSwitching) return "Opening chat…"
    if (data.snapshot.connection === "starting") return "Connecting to Gator…"
    if (data.voicePhase === "transcribing") return "Transcribing audio…"
    if (data.voicePhase === "recording" || data.voicePhase === "speaking" || data.state?.pending_approval) return ""
    if (data.state?.pending_operation) return literal(data.state.pending_operation)
    if (data.state?.pending_connection) return ""
    if (data.state?.receipt_busy) return "Checking saved receipt…"
    if (data.state?.catalog_busy && !data.state?.panel?.catalog) return "Loading chats…"
    if (data.progress) return literal(data.progress)
    if (data.admitting && !data.state?.running) return "Waiting for acknowledgement…"
    return ""
  })
  createEffect(() => {
    const current = label(); setVisible(false)
    if (!current) return
    const timer = setTimeout(() => setVisible(true), 150)
    onCleanup(() => clearTimeout(timer))
  })
  return <Show when={visible() && label()}><box id="operation-progress" position="absolute" bottom={0} left={0} width="100%" height={1} paddingLeft={1} zIndex={4000} backgroundColor={theme.background.base}><Spinner>{label()}</Spinner></box></Show>
}
