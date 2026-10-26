import { For, Show, createEffect, createMemo, createSignal, onCleanup } from "solid-js"
import type { BackendResult } from "@gator/application-host/contracts/protocol"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { literal } from "./text"
import type { ScrollBoxRenderable, Renderable } from "@opentui/core"
import { useRenderer } from "@opentui/solid"
import { useDialog } from "../ui/dialog"
import { chatTimeline, updateStatus, updateWarning } from "./timeline"
import { Activity, activityKey } from "./activity"

function Result(props: { value: BackendResult }) {
  return <box gap={1}><text><b>{literal(props.value.title)}</b></text><text>{literal(props.value.summary)}</text>
    <For each={props.value.fields}>{f => <text>{literal(f.label)}: {literal(f.value)}</text>}</For>
    <For each={props.value.items}>{item => <text>{literal(item.label)} · {literal(item.state)} {literal(item.detail)}</text>}</For>
    <For each={props.value.lines}>{line => <text>{literal(line)}</text>}</For><text>{literal(props.value.markdown)}</text>
    <Show when={props.value.preview !== undefined}><box><text><b>Exact action details</b></text><text>{literal(JSON.stringify(props.value.preview, null, 2))}</text></box></Show>
    <For each={props.value.sections}>{section => <box><text><b>{literal(section.title)}</b></text><text>{literal(section.summary)}</text>
      <For each={section.fields}>{f => <text>{literal(f.label)}: {literal(f.value)}</text>}</For>
      <For each={section.items}>{item => <text>{literal(item.label)} {literal(item.detail)} {literal(item.state)}</text>}</For>
      <For each={section.lines}>{line => <text>{literal(line)}</text>}</For><text>{literal(section.markdown)}</text>
    </box>}</For><text>{literal(props.value.footer)}</text>
  </box>
}
export function Transcript() {
  const data = useData(), client = useClient(), theme = useTheme(), renderer = useRenderer(), dialog = useDialog()
  const timeline = createMemo(() => chatTimeline(data.state?.messages ?? [], data.state?.chat_updates ?? []))
  const cards = new Map<string, Renderable>(), delivered = new Set<string>()
  let scroll: ScrollBoxRenderable | undefined
  let previous = ""
  let restore: (() => void) | undefined
  let revealAfterLayout: (() => void) | undefined
  const [revealed, setRevealed] = createSignal(-1)
  const remember = () => { if (previous && scroll) client.readingPositions.set(previous, scroll.scrollTop) }
  createEffect(() => {
    const id = data.state?.conversation_id ?? data.state?.source ?? ""
    if (id === previous) return
    remember(); previous = id; setRevealed(-1); if (restore) renderer.off("frame", restore)
    const saved = client.readingPositions.get(id)
    // Native layout establishes the scroll range during the first frame. A timer
    // can run earlier and clamp a retained position to zero.
    restore = () => { if (scroll && !scroll.isDestroyed) scroll.scrollTo(saved ?? scroll.scrollHeight); restore = undefined }
    renderer.once("frame", restore)
  })
  const reveal = (index: number) => {
    setRevealed(index)
    if (revealAfterLayout) renderer.off("frame", revealAfterLayout)
    revealAfterLayout = () => { scroll?.scrollChildIntoView(`message-${index}`); remember(); revealAfterLayout = undefined }
    renderer.once("frame", revealAfterLayout)
  }
  const acknowledgeVisible = () => {
    if (!scroll || scroll.isDestroyed || restore || revealAfterLayout || dialog.stack.length || client.route !== "chat" || client.state?.panel || client.state?.external || client.snapshot.terminalBusy || client.snapshot.sessionSwitching || client.snapshot.connection !== "connected") return
    const conversation_id = client.state?.conversation_id, viewport = scroll.viewport
    if (!conversation_id || viewport.height < 1) return
    for (const update of client.state?.chat_updates ?? []) {
      const node = cards.get(update.id), key = `${conversation_id}:${update.id}`
      if (update.delivered || delivered.has(key) || !node || node.isDestroyed || node.height < 1 || node.screenY >= viewport.screenY + viewport.height || node.screenY + node.height <= viewport.screenY) continue
      delivered.add(key)
      void client.send({ type: "update", update: { conversation_id, id: update.id, version: update.version, action: "delivered" } }).catch(error => client.fail(error))
    }
  }
  client.on("reveal-message", reveal)
  renderer.on("frame", acknowledgeVisible)
  onCleanup(() => { remember(); if (restore) renderer.off("frame", restore); if (revealAfterLayout) renderer.off("frame", revealAfterLayout); client.off("reveal-message", reveal); renderer.off("frame", acknowledgeVisible) })
  Keymap.createLayer(() => ({ commands: [
    { bind: "pageup", title: "Scroll earlier", run: () => scroll?.scrollBy(-10) },
    { bind: "pagedown", title: "Scroll later", run: () => scroll?.scrollBy(10) },
    { bind: "ctrl+end", title: "Jump to latest", run: () => scroll?.scrollTo(scroll.scrollHeight) },
  ] }))
  return <scrollbox id="transcript" ref={node => { scroll = node }} flexGrow={1} minHeight={0} stickyScroll stickyStart="bottom" paddingX={2}>
    <Show when={!data.state?.messages?.length}><box paddingY={2} gap={1}><text fg={theme.accent}><b>What would you like to get done?</b></text>
      <text fg={theme.text.muted}>Choose a model with /model. Select sources with /source.</text><text fg={theme.text.muted}>Ctrl+P opens commands, Following, Saved and Settings.</text></box></Show>
    <For each={timeline()}>{item => <Show when={item.kind === "message" ? item : undefined} fallback={item.kind === "update" ? <box id={`update-${item.update.id}`} ref={node => { const id = item.update.id; cards.set(id, node); onCleanup(() => { if (cards.get(id) === node) cards.delete(id) }) }} paddingY={1}>
      <text fg={theme.accent}><b>{literal(item.update.title)}</b> · {literal(updateStatus(item.update.status))} · {item.update.read ? "read" : "unread"} · {item.update.resolved ? "done" : "open"}</text>
      <text>{literal(item.update.summary)}</text>
      <Show when={updateWarning(item.update.status)}>{warning => <text fg={theme.text.feedback.warning.base}>{warning()} Earlier results don’t confirm that nothing changed.</text>}</Show>
      <Show when={item.update.rating}><text>Feedback: {item.update.rating}</text></Show>
      <Show when={data.updateFeedback?.id === item.update.id}><text fg={theme.text.muted}>{literal(data.updateFeedback?.message)}</text></Show>
      <text fg={theme.text.muted}>Use “updates” in Ctrl+P to inspect evidence, read, reply, mark done or give feedback.</text>
    </box> : undefined}>{messageItem => { const { message, index } = messageItem(); return <box id={`message-${index}`} paddingY={1} gap={0} border={revealed() === index ? ["left"] : false} borderColor={revealed() === index ? theme.accent : undefined}>
      <text fg={message.role === "You" ? theme.accent : theme.text.muted}><b>{literal(message.role)}</b> {literal(message.created_at)}</text>
      <Show when={message.activity}>{trace => <Activity trace={trace()} identity={activityKey(client.context, message, index)} expanded={data.activityExpanded} />}</Show>
      <Show when={message.text}><text fg={theme.text.base}>{literal(message.text)}</text></Show>
      <Show when={message.result}>{result => <Result value={result()} />}</Show>
      <Show when={message.inputs?.length}><text fg={theme.text.muted}>Inputs: {message.inputs?.map(literal).join(" · ")}</text></Show>
      <For each={message.bundle?.Artifacts}>{artifact => <text fg={theme.accent}>File: {literal(artifact.Path)} · {literal(artifact.MediaType)}</text>}</For>
    </box> }}</Show>}</For>
    <Show when={data.state?.activity}>{trace => <Activity live trace={trace()} identity={client.context + ":live:" + trace().started_at} expanded={data.activityExpanded} />}</Show>
    <Show when={data.state?.running}><text fg={theme.accent}>{data.state?.cancelling ? "Stopping…" : literal(data.state?.status) || "Gator is working…"} {literal(data.state?.elapsed)}</text></Show>
    <Show when={data.state?.chat_checks?.length}><text fg={theme.text.muted}>{data.state?.chat_checks?.length} configured checks · “check-health” in Ctrl+P shows current timing, failures and observed gaps.</text></Show>
    <For each={[data.state?.notice, data.state?.requests_issue, data.state?.action_recovery_issue, data.state?.chat_checks_issue, data.state?.chat_updates_issue, data.state?.chat_memory_issue].filter(Boolean)}>{issue => <text fg={theme.text.feedback.warning.base}>{literal(issue)}</text>}</For>
    <Show when={data.state?.pending_approval}><text fg={theme.text.feedback.warning.base}>Review the exact request above. Ctrl+Y opens the approval decision; Ctrl+D declines.</text></Show>
    <Show when={data.state?.requests_recovered}><text fg={theme.text.feedback.warning.base}>Recovered requests are paused. Review the queue before resuming.</text></Show>
    <Show when={data.state?.action_recoveries?.length}><text fg={theme.text.feedback.warning.base}>Saved request receipts · {data.state?.action_recoveries?.length}{data.state?.action_recovery_truncated ? "+" : ""} · /receipts inspects them. Reopening does not retry an external action.</text></Show>
  </scrollbox>
}
