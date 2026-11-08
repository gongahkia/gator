import { createSignal, Show } from "solid-js"
import type { ScrollBoxRenderable } from "@opentui/core"
import { useTerminalDimensions } from "@opentui/solid"
import type { BackendMessage, BackendResult, BackendState } from "@gator/application-host/contracts/protocol"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { DialogHeader, DialogFooter, useDialog } from "../ui/dialog"
import { literal } from "./text"

function describe(value: unknown, indent = ""): string[] {
  if (Array.isArray(value)) return value.flatMap((entry, index) => [`${indent}${index + 1}.`, ...describe(entry, indent + "  ")])
  if (value !== null && typeof value === "object") return Object.entries(value).flatMap(([key, entry]) => {
    const label = key.replace(/_/g, " ").replace(/([a-z])([A-Z])/g, "$1 $2")
    return entry !== null && typeof entry === "object" ? [`${indent}${label}:`, ...describe(entry, indent + "  ")] : [`${indent}${label}: ${String(entry ?? "None")}`]
  })
  return [indent + String(value ?? "None")]
}
function resultText(result: BackendResult): string {
  return [result.title, result.summary, ...(result.fields ?? []).map(field => `${field.label}: ${field.value}`),
    ...(result.items ?? []).map(item => [item.label, item.state, item.detail].filter(Boolean).join(" · ")),
    ...(result.lines ?? []), result.markdown,
    result.preview === undefined ? "" : describe(result.preview).join("\n"),
    ...(result.sections ?? []).flatMap(section => [section.title, section.summary,
      ...(section.fields ?? []).map(field => `${field.label}: ${field.value}`),
      ...(section.items ?? []).map(item => [item.label, item.state, item.detail].filter(Boolean).join(" · ")),
      ...(section.lines ?? []), section.markdown]), result.footer].filter(Boolean).join("\n\n")
}

// The displayed snapshot and the decision share one identity. Never silently
// replace an open review with a newer action or fall back to another message.
export function ApprovalReview(props: { pending: NonNullable<BackendState["pending_approval"]>; message?: BackendMessage; context: string }) {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog(), dimensions = useTerminalDimensions()
  const [raw, setRaw] = createSignal(false)
  let scroll: ScrollBoxRenderable | undefined
  const current = () => data.snapshot.connection === "connected" && client.context === props.context && data.state?.pending_approval?.id === props.pending.id && data.state?.pending_approval?.token === props.pending.token && data.state?.pending_approval?.message_index === props.pending.message_index
  const inspectable = () => props.message?.result?.kind === "approval"
  const decide = (approved: boolean) => {
    if (!current() || approved && !inspectable()) return
    void client.send({ type: "approval", interaction_id: props.pending.id, approval_token: props.pending.token, approved }).catch(error => client.fail(error))
    dialog.clear()
  }
  Keymap.createLayer(() => ({ mode: "modal", priority: 10, commands: [
    { bind: "return", title: "Approve displayed action", run: () => decide(true) },
    { bind: "ctrl+d", title: "Decline displayed action", run: () => decide(false) },
    { bind: "ctrl+r", title: "Toggle exact approval data", run: () => { setRaw(!raw()); scroll?.scrollTo(0) } },
    { bind: "up", title: "Earlier review line", run: () => scroll?.scrollBy(-1) },
    { bind: "down", title: "Later review line", run: () => scroll?.scrollBy(1) },
    { bind: "pageup", title: "Earlier review page", run: () => scroll?.scrollBy(-8) },
    { bind: "pagedown", title: "Later review page", run: () => scroll?.scrollBy(8) },
    { bind: "ctrl+home", title: "Start of review", run: () => scroll?.scrollTo(0) },
    { bind: "ctrl+end", title: "End of review", run: () => scroll?.scrollTo(scroll.scrollHeight) },
  ] }))
  return <box id="approval-review" paddingX={2} paddingBottom={1} gap={1}>
    <DialogHeader titleView={<text height={1} wrapMode="none" truncate flexShrink={1}>Approve this exact action?</text>} />
    <scrollbox id="approval-details" ref={node => { scroll = node }} height={Math.max(2, dimensions().height - (dimensions().height >= 24 ? Math.floor(dimensions().height / 4) : 0) - 8)}>
      <Show when={props.message} fallback={<text fg={theme.text.feedback.error.base}>The exact request details are unavailable. Approval is disabled. Close and reopen the review, or decline this request.</text>}>
        <text fg={theme.text.base}>{literal(raw() ? JSON.stringify(props.message?.result ?? props.message, null, 2) : props.message?.result ? resultText(props.message.result) : props.message?.text)}</text>
      </Show>
    </scrollbox>
    <text height={1} wrapMode="none" truncate fg={current() ? theme.text.muted : theme.text.feedback.warning.base}>{current() ? "↑/↓ scroll · Ctrl+R exact data · Esc close" : "Request changed or disconnected. Close and review again."}</text>
    <DialogFooter actions={{ primary: [
      { title: "decline", key: "Ctrl+D", disabled: !current(), onTrigger: () => decide(false) },
      { title: "approve", key: "Enter", disabled: !current() || !inspectable(), onTrigger: () => decide(true) },
    ] }} />
  </box>
}
