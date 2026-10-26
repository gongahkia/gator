import { For, Show, createEffect, createSignal } from "solid-js"
import { useKeyboard } from "@opentui/solid"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { useDialog } from "../ui/dialog"
import { DialogPrompt } from "../ui/dialog-prompt"
import { DialogSelect } from "../ui/dialog-select"
import type { BackendPanel, PanelAction } from "@gator/application-host/contracts/protocol"
import { literal } from "./text"

export function Panel(props: { panel: BackendPanel }) {
  const client = useClient(), theme = useTheme(), dialog = useDialog()
  const [fields, setFields] = createSignal<Record<number, string>>({}), [busy, setBusy] = createSignal(false)
  let identity = ""
  createEffect(() => { const next = props.panel.kind + props.panel.title + props.panel.rows?.map(r => r.label).join("|"); if (next !== identity) { identity = next; setFields({}) } })
  async function action(value: Omit<PanelAction, "id">, key?: string) {
    if (!props.panel.id || busy()) return
    setBusy(true)
    try { await client.send({ type: "panel", key, panel: { id: props.panel.id, fields: fields(), ...value } }) }
    catch (error) { client.fail(error) } finally { setBusy(false) }
  }
  function edit(index: number) {
    const row = props.panel.rows?.[index], input = row?.input
    if (!row || !input || input.read_only) return
    const change = (value: string) => { setFields(values => ({ ...values, [index]: value })); dialog.clear() }
    if (input.options?.length) {
      dialog.replace(() => <DialogSelect title={literal(row.label)} options={input.options!.map(option => ({ title: literal(option.label), value: option.value,
        onSelect: () => {
          if (input.kind !== "multi") change(option.value)
          else { const previous: unknown = JSON.parse(fields()[index] ?? input.value ?? "[]"); const values = Array.isArray(previous) ? previous.filter((v): v is string => typeof v === "string") : []; change(JSON.stringify(values.includes(option.value) ? values.filter(v => v !== option.value) : [...values, option.value])) }
        } }))} />)
      return
    }
    if (input.secret) {
      // The native edit buffer never paints credential bytes in terminal output.
      let value = ""
      dialog.replace(() => <box paddingX={3} paddingBottom={1} gap={1}><text fg={theme.text.base}>{literal(row.label)}</text>
        <text fg={theme.text.muted}>{literal(input.help) || "Leave empty to retain the stored credential. Enter saves; Esc cancels."}</text>
        <input focused maxLength={input.limit} textColor={theme.background.raised.high} focusedTextColor={theme.background.raised.high}
          backgroundColor={theme.background.raised.high} focusedBackgroundColor={theme.background.raised.high}
          onInput={text => { value = text }} onSubmit={() => change(value)} />
      </box>)
      return
    }
    dialog.replace(() => <DialogPrompt title={literal(row.label)} subtitle={literal(input.help)} value={fields()[index] ?? input.value ?? ""}
      onConfirm={value => { if (value.length > input.limit) { client.fail(new Error("This value exceeds the field limit.")); return }; change(value) }} />)
  }
  useKeyboard(event => {
    if (dialog.stack.length || busy()) return
    const key = event.ctrl ? `ctrl+${event.name}` : event.shift && event.name === "tab" ? "shift+tab" : ({ escape: "esc", return: "enter" }[event.name] ?? event.name)
    if (key === "enter" && props.panel.rows?.[props.panel.selected ?? 0]?.input) { event.preventDefault(); edit(props.panel.selected ?? 0); return }
    if (key === "ctrl+f" || key === "/") {
      event.preventDefault(); dialog.replace(() => <DialogPrompt title="Search" value={props.panel.query} onConfirm={text => { dialog.clear(); void action({ action: "query", text }) }} />); return
    }
    const allowed = new Set(["up", "down", "left", "right", "tab", "shift+tab", "enter", "esc", "space", "ctrl+r", "ctrl+u", "f1", ...(props.panel.hints ?? []).flatMap(h => h.shortcut.toLowerCase().replaceAll(" ", "").split("/"))])
    if (allowed.has(key)) { event.preventDefault(); void action({ action: "key", fields: key === "esc" ? undefined : fields() }, key) }
  })
  return <box flexGrow={1} minHeight={0} paddingX={2} gap={1}>
    <text fg={theme.accent}><b>{literal(props.panel.title)}</b>{busy() ? " · Saving…" : ""}</text>
    <Show when={props.panel.subtitle}><text fg={theme.text.muted}>{literal(props.panel.subtitle)}</text></Show>
    <scrollbox flexGrow={1} minHeight={0}>
      <For each={props.panel.summary}>{field => <text>{literal(field.label)}: {literal(field.value)}</text>}</For>
      <For each={props.panel.rows}>{(row, index) => <box backgroundColor={index() === props.panel.selected ? theme.background.raised.high : undefined} paddingY={0} onMouseUp={() => row.input ? edit(index()) : void action({ action: "activate", row: index() })}>
        <text fg={index() === props.panel.selected ? theme.accent : theme.text.base}>{index() === props.panel.selected ? "› " : "  "}{literal(row.label)}{row.state ? ` · ${literal(row.state_label || row.state)}` : ""}{row.input ? `: ${row.input.secret ? "[credential]" : literal(fields()[index()] ?? row.input.value)}` : ""}</text>
        <Show when={row.detail}><text fg={theme.text.muted}>  {literal(row.detail)}</text></Show>
        <For each={row.metadata}>{field => <text fg={theme.text.muted}>  {literal(field.label)}: {literal(field.value)}</text>}</For>
        <Show when={row.input?.help}><text fg={theme.text.muted}>  {literal(row.input?.help)}</text></Show>
      </box>}</For>
      <For each={props.panel.blocks}>{block => <box marginTop={1}><text fg={theme.accent}>{literal(block.title)}</text><For each={block.fields}>{f => <text>{literal(f.label)}: {literal(f.value)}</text>}</For><For each={block.lines ?? []}>{line => <text>{literal(line)}</text>}</For></box>}</For>
      <For each={props.panel.links}>{link => <text fg={theme.accent}>{literal(link.label)}: {literal(link.url)}</text>}</For>
      <Show when={props.panel.notice}><text fg={props.panel.notice_tone === "error" ? theme.text.feedback.error.base : theme.text.muted}>{literal(props.panel.notice)}</text></Show>
      <Show when={props.panel.usage}><text>{JSON.stringify(props.panel.usage, null, 2)}</text></Show>
    </scrollbox>
    <text fg={theme.text.muted}>{literal(props.panel.footer) || "↑/↓ select · Enter open/edit · Esc back · Ctrl+F search"}</text>
    <text fg={theme.text.muted}>{props.panel.hints?.map(h => `${literal(h.shortcut)} ${literal(h.action)}`).join(" · ")}</text>
  </box>
}
