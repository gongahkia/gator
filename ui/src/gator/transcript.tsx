import { For, Show, createEffect, createMemo, createSignal, onCleanup } from "solid-js"
import type { BackendResult } from "@gator/application-host/contracts/protocol"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { literal } from "./text"
import { TextAttributes } from "@opentui/core"
import type { ScrollBoxRenderable, Renderable } from "@opentui/core"
import { useRenderer } from "@opentui/solid"
import { useDialog } from "../ui/dialog"
import { chatTimeline, createTimelineReconciler, updateStatus, updateWarning } from "./timeline"
import { Activity, activityKey } from "./activity"
import { Markdown } from "./markdown"
import { displayDate } from "./display"
import { Spinner } from "../component/spinner"
import { WorkingStatus } from "./working-status"
import { ConversationTurn } from "./conversation-turn"
import { useKeyHints } from "./key-hints"
import { useTranscriptReader } from "./transcript-reader"
import { useTranscriptWindow } from "./transcript-window"

function Result(props: { value: BackendResult }) {
  const theme = useTheme()
  return <box gap={1}><text fg={theme.text.base}><b>{literal(props.value.title)}</b></text><text fg={theme.text.base}>{literal(props.value.summary)}</text>
    <For each={props.value.fields}>{f => <text fg={theme.text.base}>{literal(f.label)}: {literal(f.value)}</text>}</For>
    <For each={props.value.items}>{item => <text fg={theme.text.base}>{literal(item.label)} · {literal(item.state)} {literal(item.detail)}</text>}</For>
    <For each={props.value.lines}>{line => <text fg={theme.text.base}>{literal(line)}</text>}</For><Show when={props.value.markdown}><Markdown text={props.value.markdown} /></Show>
    <Show when={props.value.preview !== undefined}><box><text fg={theme.text.base}><b>Exact action details</b></text><text fg={theme.text.base}>{literal(JSON.stringify(props.value.preview, null, 2))}</text></box></Show>
    <For each={props.value.sections}>{section => <box><text fg={theme.text.base}><b>{literal(section.title)}</b></text><text fg={theme.text.base}>{literal(section.summary)}</text>
      <For each={section.fields}>{f => <text fg={theme.text.base}>{literal(f.label)}: {literal(f.value)}</text>}</For>
      <For each={section.items}>{item => <text fg={theme.text.base}>{literal(item.label)} {literal(item.detail)} {literal(item.state)}</text>}</For>
      <For each={section.lines}>{line => <text fg={theme.text.base}>{literal(line)}</text>}</For><Show when={section.markdown}><Markdown text={section.markdown} /></Show>
    </box>}</For><text fg={theme.text.base}>{literal(props.value.footer)}</text>
  </box>
}
export function Transcript() {
  const data = useData(), client = useClient(), theme = useTheme(), renderer = useRenderer(), dialog = useDialog()
  const hint = useKeyHints()
  const reconcile = createTimelineReconciler()
  const timeline = createMemo(() => reconcile(data.context, chatTimeline(data.state?.messages ?? [], data.state?.chat_updates ?? [], data.state?.dialogue ?? []).filter(item => item.kind !== "message" || item.message.role !== "Deliverables" || !!(item.message.text || item.message.result || item.message.activity || item.message.bundle?.Artifacts?.length))))
  const cards = new Map<string, Renderable>(), delivered = new Set<string>()
  let scroll: ScrollBoxRenderable | undefined
  const window = useTranscriptWindow(timeline, () => scroll)
  const reading = useTranscriptReader(timeline, () => scroll, window)
  let revealAfterLayout: (() => void) | undefined
  let revealedScrollTop: number | undefined
  const [revealed, setRevealed] = createSignal(-1)
  createEffect(() => { data.context; setRevealed(-1); revealedScrollTop = undefined })
  const reveal = (index: number) => {
    setRevealed(index)
    const position = timeline().findIndex(item => item.kind === "message" && item.index === index)
    if (position >= 0) window.ensure(position)
    if (revealAfterLayout) renderer.off("frame", revealAfterLayout)
    revealAfterLayout = () => {
      if (!window.ready()) { renderer.once("frame", revealAfterLayout!); return }
      const node = scroll?.content.getChildren().find(node => node.id === `message-${index}`)
      // Explicit reveal aligns the requested item's start. Native nearest-edge
      // scrolling currently does nothing when item and viewport heights match.
      if (scroll && node && !node.isDestroyed) scroll.scrollBy(node.screenY - scroll.viewport.screenY)
      revealedScrollTop = scroll?.scrollTop; reading.moved(); revealAfterLayout = undefined
    }
    renderer.once("frame", revealAfterLayout)
  }
  const acknowledgeVisible = () => {
    if (!scroll || scroll.isDestroyed || reading.restoring() || revealAfterLayout || dialog.stack.length || client.route !== "chat" || client.state?.panel || client.state?.external || client.snapshot.terminalBusy || client.snapshot.sessionSwitching || client.snapshot.connection !== "connected") return
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
  const visibleActions = () => {
    const viewport = scroll?.viewport
    // Message boxes are direct transcript children. Index them once instead
    // of recursively searching the full native tree for every older message.
    const nodes = new Map((scroll?.content.getChildren() ?? []).map(node => [node.id, node]))
    // Explicit search/reveal chooses a particular message. Keep that target
    // until reading moves, rather than choosing a neighbouring footer/notice.
    const selected = scroll?.scrollTop === revealedScrollTop ? timeline().find(item => item.kind === "message" && item.index === revealed()) : undefined
    for (const item of [...(selected ? [selected] : []), ...timeline().toReversed()]) {
      const node = item.kind === "message" ? nodes.get(`message-${item.index}`) : item.kind === "update" ? cards.get(item.update.id) : nodes.get(`dialogue-${item.turn.id}`)
      if (!viewport || !node || node.isDestroyed || node.height < 1 || node.screenY >= viewport.screenY + viewport.height || node.screenY + node.height <= viewport.screenY) continue
      if (item.kind === "message") { client.emit("item-actions", { kind: "message", context: client.context, message: item.message, index: item.index }); return }
      if (item.kind === "update") { client.emit("item-actions", { kind: "update", context: client.context, update: item.update }); return }
      if (item.kind === "dialogue") { client.emit("item-actions", { kind: "dialogue", context: client.context, turn: item.turn, peer: client.state?.dialogue_peers?.find(peer => peer.conversation_id === item.turn.task_chat && peer.turn_id === item.turn.id) }); return }
    }
    client.emit("item-actions")
  }
  client.on("request-item-actions", visibleActions)
  renderer.on("frame", acknowledgeVisible)
  onCleanup(() => { if (revealAfterLayout) renderer.off("frame", revealAfterLayout); client.off("reveal-message", reveal); client.off("request-item-actions", visibleActions); renderer.off("frame", acknowledgeVisible) })
  Keymap.createLayer(() => ({ commands: [
    { bind: "ctrl+home", title: "Jump to earliest retained content", run: () => { scroll?.scrollTo(0); reading.moved() } },
    { bind: "pageup", title: "Scroll earlier", run: () => { scroll?.scrollBy(-10); reading.moved() } },
    { bind: "pagedown", title: "Scroll later", run: () => { scroll?.scrollBy(10); reading.moved() } },
    { bind: "ctrl+end", title: "Jump to latest", run: reading.latest },
  ] }))
  return <scrollbox id="transcript" ref={node => { scroll = node }} flexGrow={1} minHeight={0} stickyScroll stickyStart="bottom" paddingX={2}>
    <Show when={data.state?.branch}><text fg={theme.text.muted} onMouseUp={() => void client.navigate("/original-chat")}>Branched chat · /original-chat returns to its source</text></Show>
    <Show when={!data.state?.messages?.length && !data.state?.dialogue?.length}><box paddingY={3} paddingX={3} gap={1}><text fg={theme.text.base}><b>What would you like to get done?</b></text>
      <text fg={theme.text.muted}>Ask a question, work with files, or follow an ongoing task.</text><Show when={!data.state?.provider || data.state?.composer?.readiness?.state !== undefined && data.state.composer.readiness.state !== "configured"} fallback={<text fg={theme.text.muted}>/request-context shows processing, sources and work limits.</text>}><text fg={theme.text.muted}>Write your first request. I’ll help with the missing model setup and keep your draft.</text></Show></box></Show>
    <box id="transcript-before" height={window.before()} minHeight={0} flexShrink={0} />
    <For each={window.visible()}>{item => { const context = client.context; return <Show when={item.kind === "message" ? item : undefined} fallback={item.kind === "update" ? <box id={`update-${item.update.id}`} ref={node => { const id = item.update.id; cards.set(id, node); onCleanup(() => { if (cards.get(id) === node) cards.delete(id) }) }} paddingY={1}>
      <text fg={theme.accent}><b>{literal(item.update.title)}</b> · {literal(updateStatus(item.update.status))} · {item.update.read ? "read" : "unread"} · {item.update.resolved ? "done" : "open"}</text>
      <text fg={theme.text.base}>{literal(item.update.summary)}</text>
      <Show when={updateWarning(item.update.status)}>{warning => <text fg={theme.text.feedback.warning.base}>{warning()} Earlier results don’t confirm that nothing changed.</text>}</Show>
      <Show when={item.update.rating}><text fg={theme.text.base}>Feedback: {item.update.rating}</text></Show>
      <Show when={data.updateFeedback?.id === item.update.id}><Show when={data.updateFeedback?.saving} fallback={<text fg={theme.text.muted}>{literal(data.updateFeedback?.message)}</text>}><Spinner>{literal(data.updateFeedback?.message)}</Spinner></Show></Show>
      <text fg={theme.text.muted} onMouseUp={() => { if (!renderer.getSelection()?.getSelectedText()) client.emit("item-actions", { kind: "update", context, update: item.update }) }}>{hint("gator.context-actions")} actions · /actions chooses an item</text>
    </box> : item.kind === "dialogue" ? <ConversationTurn turn={item.turn} task={data.state?.dialogue_peers?.find(peer=>peer.conversation_id===item.turn.task_chat)} busy={data.state?.dialogue_busy} /> : undefined}>{messageItem => { const { message, index } = messageItem(); return <box id={`message-${index}`} marginY={1} paddingY={message.role === "You" ? 1 : 0} paddingLeft={message.role === "You" || revealed() === index ? 2 : 3} paddingRight={2} gap={1}
      backgroundColor={message.role === "You" ? theme.background.raised.base : undefined} border={message.role === "You" || revealed() === index ? ["left"] : false} borderColor={message.role === "You" || revealed() === index ? theme.accent : undefined}>
      <text fg={message.role === "You" ? theme.accent : theme.text.muted} onMouseUp={() => { if (!renderer.getSelection()?.getSelectedText()) client.emit("item-actions", { kind: "message", context, message, index }) }}><b>{literal(message.role)}</b><span style={{ attributes: TextAttributes.DIM }}>  {displayDate(message.created_at)} · {hint("gator.context-actions")} actions</span></text>
      <Show when={message.activity}>{trace => <Activity trace={trace()} identity={activityKey(client.context, message, index)} expanded={data.activityExpanded} />}</Show>
      <Show when={message.text}><Show when={message.role !== "You"} fallback={<text fg={theme.text.base}>{literal(message.text)}</text>}><Markdown text={message.text} /></Show></Show>
      <Show when={message.result}>{result => <Result value={result()} />}</Show>
      <Show when={message.inputs?.length}><text fg={theme.text.muted}>Inputs: {message.inputs?.map(literal).join(" · ")}</text></Show>
      <For each={message.bundle?.Artifacts}>{artifact => <text fg={theme.accent} onMouseUp={() => { if (!renderer.getSelection()?.getSelectedText()) client.emit("item-actions", { kind: "file", context, bundle: message.bundle, file: artifact }) }}>File: {literal(artifact.Path)} · {literal(artifact.MediaType)}</text>}</For>
    </box> }}</Show> }}</For>
    <box id="transcript-after" height={window.after()} minHeight={0} flexShrink={0} />
    <For each={data.state?.request_recoveries?.slice(0,1)}>{task => <box paddingY={1}>
      <text fg={theme.text.base}>{literal(task.recovery.summary)}</text>
      <text fg={theme.accent} onMouseUp={() => void client.navigate("/request-recovery")}>/request-recovery · inspect setup and retained task</text>
    </box>}</For>
    <For each={data.state?.task_plans}>{plan => <box paddingY={1}>
      <text fg={theme.accent} onMouseUp={() => void client.navigate("/task-plans")}>{literal(plan.outcome || plan.task)}</text>
      <text fg={theme.text.muted}>{literal(plan.state)} · {literal(plan.phase)}{plan.phase === "review" ? " · /task-plans to review before work starts" : " · /task-plans for details"}</text>
    </box>}</For>
    <For each={data.state?.specialists}>{task => <text fg={theme.text.muted} onMouseUp={() => void client.navigate("/specialists")}>{literal(task.name)} · {literal(task.status)} · /specialists</text>}</For>
    <Show when={data.state?.activity}>{trace => <Activity live trace={trace()} identity={client.context + ":live:" + trace().started_at} expanded={data.activityExpanded} />}</Show>
    <Show when={data.state?.running}><WorkingStatus status={data.state?.status} elapsed={data.state?.elapsed} attention={!!data.state?.pending_approval} cancelling={!!data.state?.cancelling} /></Show>
    
    <Show when={data.state?.dialogue_more}><text fg={theme.text.muted}>Earlier conversation messages are retained · /earlier-messages</text></Show>
    <For each={[data.state?.notice, data.state?.dialogue_issue, data.state?.requests_issue, data.state?.action_recovery_issue, data.state?.chat_checks_issue, data.state?.chat_updates_issue, data.state?.chat_memory_issue].filter(Boolean)}>{issue => <text fg={theme.text.feedback.warning.base}>{literal(issue)}</text>}</For>
    <Show when={data.state?.pending_approval}><text fg={theme.text.feedback.warning.base}>Review the exact request above. Ctrl+Y opens the approval decision; Ctrl+D declines.</text></Show>
    <Show when={data.state?.requests_recovered}><text fg={theme.text.feedback.warning.base}>Recovered requests are paused. Review the queue before resuming.</text></Show>
    <Show when={data.state?.action_recoveries?.length}><text fg={theme.text.feedback.warning.base}>Saved request receipts · {data.state?.action_recoveries?.length}{data.state?.action_recovery_truncated ? "+" : ""} · /receipts inspects them. Reopening does not retry an external action.</text></Show>
  </scrollbox>
}
