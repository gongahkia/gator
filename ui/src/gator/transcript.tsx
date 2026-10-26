import { For, Show, createEffect } from "solid-js"
import type { BackendResult } from "@gator/application-host/contracts/protocol"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { literal } from "./text"
import type { ScrollBoxRenderable } from "@opentui/core"

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
  const data = useData(), client = useClient(), theme = useTheme()
  let scroll: ScrollBoxRenderable | undefined
  const positions = new Map<string, number>()
  let previous = ""
  createEffect(() => { const id = data.state?.conversation_id ?? data.state?.source ?? ""; if (id === previous) return; if (previous && scroll) positions.set(previous, scroll.scrollTop); previous = id; setTimeout(() => { if (scroll && !scroll.isDestroyed) scroll.scrollTo(positions.get(id) ?? scroll.scrollHeight) }, 1) })
  Keymap.createLayer(() => ({ commands: [
    { bind: "pageup", title: "Scroll earlier", run: () => scroll?.scrollBy(-10) },
    { bind: "pagedown", title: "Scroll later", run: () => scroll?.scrollBy(10) },
    { bind: "ctrl+end", title: "Jump to latest", run: () => scroll?.scrollTo(scroll.scrollHeight) },
  ] }))
  return <scrollbox ref={node => { scroll = node }} flexGrow={1} minHeight={0} stickyScroll stickyStart="bottom" paddingX={2}>
    <Show when={!data.state?.messages?.length}><box paddingY={2} gap={1}><text fg={theme.accent}><b>What would you like to get done?</b></text>
      <text fg={theme.text.muted}>Choose a model with /model. Select sources with /source.</text><text fg={theme.text.muted}>Ctrl+P opens commands, Following, Saved and Settings.</text></box></Show>
    <For each={data.state?.messages}>{message => <box paddingY={1} gap={0}>
      <text fg={message.role === "You" ? theme.accent : theme.text.muted}><b>{literal(message.role)}</b> {literal(message.created_at)}</text>
      <Show when={message.text}><text fg={theme.text.base}>{literal(message.text)}</text></Show>
      <Show when={message.result}>{result => <Result value={result()} />}</Show>
      <Show when={message.inputs?.length}><text fg={theme.text.muted}>Inputs: {message.inputs?.map(literal).join(" · ")}</text></Show>
      <For each={message.activity?.entries}>{entry => <text fg={theme.text.muted}>{literal(entry.tool || entry.text)} · {literal(entry.state)}</text>}</For>
      <For each={message.bundle?.Artifacts}>{artifact => <text fg={theme.accent}>File: {literal(artifact.Path)} · {literal(artifact.MediaType)}</text>}</For>
    </box>}</For>
    <For each={data.state?.activity?.entries}>{entry => <text fg={theme.text.muted}>{literal(entry.text || entry.tool)} · {literal(entry.state)}</text>}</For>
    <Show when={data.state?.running}><text fg={theme.accent}>{literal(data.state?.status) || "Gator is working…"} {literal(data.state?.elapsed)}</text></Show>
    <Show when={data.state?.pending_approval}><text fg={theme.text.feedback.warning.base}>Review the exact request above. Ctrl+Y opens the approval decision; Ctrl+D declines.</text></Show>
    <Show when={data.state?.requests_recovered}><text fg={theme.text.feedback.warning.base}>Recovered requests are paused. Review the queue before resuming.</text></Show>
  </scrollbox>
}
