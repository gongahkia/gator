import { For, Show, createEffect, createSignal, onCleanup } from "solid-js"
import type { BackendResult } from "@gator/application-host/contracts/protocol"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { literal } from "./text"
import type { ScrollBoxRenderable } from "@opentui/core"
import { useRenderer } from "@opentui/solid"

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
  const data = useData(), client = useClient(), theme = useTheme(), renderer = useRenderer()
  let scroll: ScrollBoxRenderable | undefined
  let previous = ""
  let restore: (() => void) | undefined
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
  const reveal = (index: number) => { setRevealed(index); scroll?.scrollChildIntoView(`message-${index}`); remember() }
  client.on("reveal-message", reveal)
  onCleanup(() => { remember(); if (restore) renderer.off("frame", restore); client.off("reveal-message", reveal) })
  Keymap.createLayer(() => ({ commands: [
    { bind: "pageup", title: "Scroll earlier", run: () => scroll?.scrollBy(-10) },
    { bind: "pagedown", title: "Scroll later", run: () => scroll?.scrollBy(10) },
    { bind: "ctrl+end", title: "Jump to latest", run: () => scroll?.scrollTo(scroll.scrollHeight) },
  ] }))
  return <scrollbox id="transcript" ref={node => { scroll = node }} flexGrow={1} minHeight={0} stickyScroll stickyStart="bottom" paddingX={2}>
    <Show when={!data.state?.messages?.length}><box paddingY={2} gap={1}><text fg={theme.accent}><b>What would you like to get done?</b></text>
      <text fg={theme.text.muted}>Choose a model with /model. Select sources with /source.</text><text fg={theme.text.muted}>Ctrl+P opens commands, Following, Saved and Settings.</text></box></Show>
    <For each={data.state?.messages}>{(message, index) => <box id={`message-${index()}`} paddingY={1} gap={0} border={revealed() === index() ? ["left"] : undefined} borderColor={theme.accent}>
      <text fg={message.role === "You" ? theme.accent : theme.text.muted}><b>{literal(message.role)}</b> {literal(message.created_at)}</text>
      <Show when={message.text}><text fg={theme.text.base}>{literal(message.text)}</text></Show>
      <Show when={message.result}>{result => <Result value={result()} />}</Show>
      <Show when={message.inputs?.length}><text fg={theme.text.muted}>Inputs: {message.inputs?.map(literal).join(" · ")}</text></Show>
      <For each={message.activity?.entries}>{entry => <text fg={theme.text.muted}>{literal(entry.tool || entry.text)} · {literal(entry.state)}</text>}</For>
      <For each={message.bundle?.Artifacts}>{artifact => <text fg={theme.accent}>File: {literal(artifact.Path)} · {literal(artifact.MediaType)}</text>}</For>
    </box>}</For>
    <For each={data.state?.activity?.entries}>{entry => <text fg={theme.text.muted}>{literal(entry.text || entry.tool)} · {literal(entry.state)}</text>}</For>
    <Show when={data.state?.running}><text fg={theme.accent}>{data.state?.cancelling ? "Stopping…" : literal(data.state?.status) || "Gator is working…"} {literal(data.state?.elapsed)}</text></Show>
    <For each={data.state?.chat_updates}>{update => <box paddingY={1}><text fg={theme.accent}><b>{literal(update.title)}</b> · {literal(update.status)} · {update.read ? "read" : "unread"} · {update.resolved ? "done" : "open"}</text><text>{literal(update.summary)}</text><For each={update.evidence}>{line => <text fg={theme.text.muted}>{literal(line)}</text>}</For><text fg={theme.text.muted}>Use “updates” in Ctrl+P to read, reply, mark done or give feedback.</text></box>}</For>
    <For each={data.state?.chat_checks}>{check => <text fg={theme.text.muted}>Check: {literal(check.title)} · {literal(check.state)} · {literal(check.issue)} {literal(check.next_action)}</text>}</For>
    <For each={[data.state?.notice, data.state?.requests_issue, data.state?.action_recovery_issue, data.state?.chat_checks_issue, data.state?.chat_updates_issue, data.state?.chat_memory_issue].filter(Boolean)}>{issue => <text fg={theme.text.feedback.warning.base}>{literal(issue)}</text>}</For>
    <Show when={data.state?.pending_approval}><text fg={theme.text.feedback.warning.base}>Review the exact request above. Ctrl+Y opens the approval decision; Ctrl+D declines.</text></Show>
    <Show when={data.state?.requests_recovered}><text fg={theme.text.feedback.warning.base}>Recovered requests are paused. Review the queue before resuming.</text></Show>
  </scrollbox>
}
