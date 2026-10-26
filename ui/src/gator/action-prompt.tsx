import { createSignal } from "solid-js"
import { DialogPrompt } from "../ui/dialog-prompt"
import { useDialog } from "../ui/dialog"
import { useClient } from "../context/client"
import { literal } from "./text"

export function ActionPrompt(props: { token: symbol; title: string; value: string; description?: string; save(text: string): unknown }) {
  const client = useClient(), dialog = useDialog(), [busy, setBusy] = createSignal(false), [issue, setIssue] = createSignal("")
  const active = () => dialog.key === props.token
  async function confirm(text: string) {
    if (busy()) return
    setBusy(true); setIssue("")
    try {
      await props.save(text)
      // A synchronous menu callback or successful file reopen can replace this
      // prompt. Do not clear the new screen when its old request completes.
      if (active()) dialog.clear()
    } catch (error) {
      client.fail(error)
      if (active()) setIssue(error instanceof Error ? error.message : String(error))
    } finally { if (active()) setBusy(false) }
  }
  return <DialogPrompt title={props.title} value={props.value} busy={busy()} busyText="Waiting for Gator's acknowledgement…"
    subtitle={literal([props.description, issue()].filter(Boolean).join("\n"))} onConfirm={text => { void confirm(text) }} onCancel={() => dialog.clear()} />
}
