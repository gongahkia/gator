import { For, Show, createEffect, createSignal, onCleanup } from "solid-js"
import type { TextareaRenderable, ScrollBoxRenderable } from "@opentui/core"
import { Keymap } from "../context/keymap"
import { useKeyboard, useRenderer } from "@opentui/solid"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"
import { useDialog } from "../ui/dialog"
import { DialogPrompt } from "../ui/dialog-prompt"
import { DialogSelect } from "../ui/dialog-select"
import type { BackendPanel, PanelAction } from "@gator/application-host/contracts/protocol"
import { literal } from "./text"
import { PathPicker } from "./path-picker"
import { SecretField } from "./secret-field"
import { DialogConfirm } from "../ui/dialog-confirm"
import { Highlight } from "./highlight"

export function Panel(props: { panel: BackendPanel }) {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog(), renderer = useRenderer()
  const [fields, setFields] = createSignal<Record<number, string>>({}), [busy, setBusy] = createSignal(false), [query, setQuery] = createSignal("")
  const selectedRow = () => Math.max(0, Math.min(props.panel.selected ?? 0, (props.panel.rows?.length ?? 1) - 1))
  const footer = () => {
    const parts = (props.panel.footer ?? "").split("·").map(part => part.trim()).filter(Boolean)
    for (const hint of props.panel.hints ?? []) {
      if (!parts.some(part => part.toLowerCase().startsWith(hint.shortcut.toLowerCase() + " "))) parts.push(`${hint.shortcut} ${hint.action}`)
    }
    return literal(parts.join(" · ")) || "↑/↓ select · Enter open/edit · Esc back · Ctrl+F search"
  }
  const formSubmit = () => {
    if (props.panel.hints?.some(hint => /ctrl\+(r|enter)/.test(hint.shortcut.toLowerCase().replaceAll(" ", "")))) return { key: "ctrl+r", label: "review" }
    if (!props.panel.rows?.some(row => row.input)) return
    const hint = props.panel.hints?.find(hint => hint.shortcut.toLowerCase() === "enter" && /^(save|review)\b/i.test(hint.action))
    if (hint) return { key: "enter", label: hint.action.toLowerCase() }
  }
  const visible = () => (props.panel.rows ?? []).map((row, index) => ({ row, index })).filter(({ row }) => !query() || JSON.stringify([row.label, row.detail, row.metadata, row.source]).toLocaleLowerCase().includes(query().toLocaleLowerCase()))
  let identity = ""
  let scroll: ScrollBoxRenderable | undefined, reveal: (() => void) | undefined
  createEffect(() => {
    const selected = selectedRow(), filter = query()
    if (reveal) renderer.off("frame", reveal)
    reveal = () => { if (scroll && !scroll.isDestroyed && selected !== undefined) scroll.scrollChildIntoView(`panel-row-${selected}`); reveal = undefined }
    renderer.once("frame", reveal)
  })
  onCleanup(() => { if (reveal) renderer.off("frame", reveal) })
  createEffect(() => { const next = props.panel.kind + props.panel.title + props.panel.rows?.map(r => r.label).join("|"); if (next !== identity) { identity = next; setFields({}); setQuery("") } })
  const guard = async () => {
    if (!Object.keys(fields()).length) return true
    return new Promise<boolean>(resolve => {
      dialog.replace(() => <DialogConfirm title="Leave this unsaved form?" message="Edited form fields have not been submitted. Keep editing or explicitly discard them before navigating." label={{ confirm: "Discard form edits", cancel: "Keep editing" }} onConfirm={() => { setFields({}); resolve(true) }} onCancel={() => resolve(false)} />, () => resolve(false))
    })
  }
  client.navigationGuard = guard
  onCleanup(() => { if (client.navigationGuard === guard) client.navigationGuard = undefined })
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
    const value = () => fields()[index] ?? input.value ?? ""
    if (input.kind === "directory" || input.kind === "file" || input.kind === "save") {
      const kind = input.kind
      dialog.replace(() => <PathPicker kind={kind} choose={change} />)
      return
    }
    if (input.kind === "budget") {
      dialog.replace(() => <DialogSelect title={literal(row.label)} options={(input.parts ?? []).map((part, position) => ({ title: `${literal(part.label)}: ${value().split(",")[position] ?? ""}`, value: position,
        onSelect: () => dialog.replace(() => <DialogPrompt title={literal(part.label)} value={value().split(",")[position] ?? ""} subtitle={`Whole number, ${part.min}–${part.max}`} onConfirm={text => {
          if (!/^\d+$/.test(text) || Number(text) < part.min || Number(text) > part.max) { client.fail(new Error("Choose a whole number within the displayed bounds.")); return }
          const parts = value().split(","); parts[position] = text; change(parts.join(","))
        }} />),
      }))} />)
      return
    }
    if (input.kind === "mappings") {
      const parsed: unknown = JSON.parse(value() || "[]")
      const mappings: { from: string; to: string }[] = Array.isArray(parsed) ? parsed.filter((item): item is { from: string; to: string } => item && typeof item.from === "string" && typeof item.to === "string") : []
      const chooseTo = (from: string, position?: number) => dialog.replace(() => <DialogPrompt title="Replacement folder" value={position === undefined ? "" : mappings[position].to} onConfirm={to => { const next = [...mappings]; if (position === undefined) next.push({ from, to }); else next[position] = { from, to }; change(JSON.stringify(next)) }} />)
      dialog.replace(() => <DialogSelect title="Source folder mappings" options={[
        ...mappings.map((mapping, position) => ({ title: `${mapping.from} → ${mapping.to}`, value: String(position), onSelect: () => dialog.replace(() => <DialogSelect title="Mapping" options={[
          { title: "Change replacement", value: "edit", onSelect: () => chooseTo(mapping.from, position) },
          { title: "Remove mapping", value: "remove", onSelect: () => change(JSON.stringify(mappings.filter((_, at) => at !== position))) },
        ]} />) })),
        { title: "Add mapping", value: "add", disabled: mappings.length >= 32, onSelect: () => dialog.replace(() => <DialogPrompt title="Original folder" onConfirm={from => chooseTo(from)} />) },
      ]} />)
      return
    }
    if (input.options?.length) {
      dialog.replace(() => <DialogSelect title={literal(row.label)} options={input.options!.map(option => ({ title: literal(option.label), value: option.value,
        onSelect: () => {
          if (input.kind !== "multi") change(option.value)
          else { const previous: unknown = JSON.parse(fields()[index] ?? input.value ?? "[]"); const values = Array.isArray(previous) ? previous.filter((v): v is string => typeof v === "string") : []; change(JSON.stringify(values.includes(option.value) ? values.filter(v => v !== option.value) : [...values, option.value])) }
        } }))} />)
      return
    }
    if (input.secret) {
      dialog.replace(() => <SecretField title={row.label} help={input.help} limit={input.limit} save={change} />)
      return
    }
    if (input.multiline) {
      dialog.replace(() => <MultilineField title={literal(row.label)} value={value()} limit={input.limit} save={change} />)
      return
    }
    dialog.replace(() => <DialogPrompt title={literal(row.label)} subtitle={literal(input.help)} value={fields()[index] ?? input.value ?? ""}
      onConfirm={value => {
        if (value.length > input.limit) { client.fail(new Error("This value exceeds the field limit.")); return }
        if (input.kind === "number" && (!/^-?\d+$/.test(value) || Number(value) < (input.min ?? 0) || input.max !== undefined && Number(value) > input.max)) { client.fail(new Error("Choose a whole number within this field's bounds.")); return }
        if (input.kind === "datetime" && value && !Number.isFinite(Date.parse(value))) { client.fail(new Error("Enter an ISO date and time with a timezone.")); return }
        change(input.kind === "datetime" && value ? new Date(value).toISOString() : value)
      }} />)
  }
  useKeyboard(event => {
    if (dialog.stack.length || busy()) return
    const name = ({ escape: "esc", return: "enter" }[event.name] ?? event.name)
    const key = event.ctrl ? `ctrl+${name}` : event.shift && name === "tab" ? "shift+tab" : name
    if (key === "f3" && props.panel.catalog?.can_organize) { event.preventDefault(); client.emit("action", "catalog-actions"); return }
    if (["ctrl+r", "f5"].includes(key) && props.panel.catalog) { event.preventDefault(); void action({ action: "refresh" }); return }
    if (key === "esc" && Object.keys(fields()).length) { event.preventDefault(); void guard().then(leave => { if (leave) void action({ action: "key", fields: undefined }, "esc") }); return }
    const submit = formSubmit()
    if (["f2", "ctrl+enter", "ctrl+r"].includes(key) && submit) {
      event.preventDefault()
      // Match the reference host's form submission: Enter on the final field
      // submits the complete form instead of advancing to its next field.
      void action({ action: "key", row: submit.key === "enter" ? (props.panel.rows?.length ?? 1) - 1 : undefined }, submit.key)
      return
    }
    if (["pageup", "pagedown"].includes(key)) {
      event.preventDefault(); const rows = visible(), current = rows.findIndex(row => row.index === selectedRow())
      const selected = rows[Math.max(0, Math.min(rows.length - 1, current + (key === "pagedown" ? 10 : -10)))]
      if (selected) void action({ action: "select", row: selected.index }); return
    }
    if (query() && ["up", "down"].includes(key)) {
      event.preventDefault(); const rows = visible(), current = rows.findIndex(row => row.index === selectedRow())
      const selected = rows[(current + (key === "down" ? 1 : -1) + rows.length) % rows.length]
      if (selected) void action({ action: "select", row: selected.index }); return
    }
    if (key === "enter" && query() && !visible().length) { event.preventDefault(); return }
    if (key === "enter" && props.panel.rows?.[selectedRow()]?.input) { event.preventDefault(); edit(selectedRow()); return }
    if (key === "ctrl+f" || key === "/") {
      event.preventDefault(); dialog.replace(() => <DialogPrompt title={props.panel.catalog ? "Search retained conversations" : "Filter loaded entries"} value={props.panel.catalog ? props.panel.query : query()} onConfirm={text => {
        dialog.clear()
        if (props.panel.catalog) void action({ action: "query", text })
        else { setQuery(text); const row = visible()[0]; if (row) void action({ action: "select", row: row.index }) }
      }} />); return
    }
    if (key === "ctrl+l" && props.panel.filter) {
      dialog.replace(() => <DialogSelect title="Filter" options={props.panel.filter!.options.map(option => ({ title: option.label, value: option.value, onSelect: () => { dialog.clear(); void action({ action: "filter", text: option.value }) } }))} />); return
    }
    if (key === "ctrl+right" && props.panel.catalog?.has_next) { void action({ action: "next" }); return }
    if (key === "ctrl+left" && props.panel.catalog?.has_previous) { void action({ action: "previous" }); return }
    const allowed = new Set(["up", "down", "left", "right", "tab", "shift+tab", "enter", "esc", "space", "ctrl+r", "ctrl+u", "f1", ...(props.panel.hints ?? []).flatMap(h => h.shortcut.toLowerCase().replaceAll(" ", "").split("/"))])
    if (allowed.has(key)) { event.preventDefault(); void action({ action: "key", fields: key === "esc" ? undefined : fields() }, key) }
  })
  return <box flexGrow={1} minHeight={0} paddingX={2} gap={1}>
    <text fg={theme.accent}><b>{literal(props.panel.title)}</b>{busy() ? " · Saving…" : ""}</text>
    <Show when={props.panel.subtitle}><text fg={theme.text.muted}>{literal(props.panel.subtitle)}</text></Show>
    <Show when={props.panel.filter}><text fg={theme.text.muted}>Filter: {literal(props.panel.filter?.value)} · Ctrl+L changes</text></Show>
    <Show when={props.panel.catalog}><text fg={theme.text.muted}>Page {props.panel.catalog?.page} · Ctrl+←/→ pages · Ctrl+R/F5 refresh{props.panel.catalog?.can_organize ? " · F3 rename/pin/archive" : ""}</text></Show>
    <Show when={props.panel.catalog && props.panel.query}><text fg={theme.text.muted}>Search retained messages: {literal(props.panel.query)}</text></Show>
    <Show when={query()}><text fg={theme.text.muted}>Filter loaded entries: {literal(query())} · {visible().length} of {props.panel.rows?.length ?? 0} · Ctrl+F changes or clears</text></Show>
    <scrollbox ref={node => { scroll = node }} flexGrow={1} minHeight={0}>
      <For each={props.panel.summary}>{field => <text>{literal(field.label)}: {literal(field.value)}</text>}</For>
      <Show when={query() && !visible().length}><text fg={theme.text.muted}>No matching loaded items. Clear the filter to choose an entry.</text></Show>
      <For each={visible()}>{({ row, index }) => <box id={`panel-row-${index}`} backgroundColor={index === selectedRow() ? theme.background.raised.high : undefined} paddingY={0} onMouseUp={() => row.input ? edit(index) : void action({ action: "activate", row: index })}>
        <text fg={index === selectedRow() ? theme.accent : theme.text.base}>{index === selectedRow() ? "› " : "  "}{literal(row.label)}{row.state ? ` · ${literal(row.state_label || row.state)}` : ""}{row.input ? `: ${row.input.secret ? "[credential]" : literal(fields()[index] ?? row.input.value)}` : ""}</text>
        <Show when={row.detail}><text fg={theme.text.muted}>  <Highlight text={row.detail} query={props.panel.catalog ? props.panel.query : query()} /></text></Show>
        <Show when={row.source || row.updated_at || row.pinned || row.archived}><text fg={theme.text.muted}>  {[row.pinned ? "Pinned" : "", row.archived ? "Archived" : "", row.updated_at, row.source].filter(Boolean).map(literal).join(" · ")}</text></Show>
        <For each={row.metadata}>{field => <text fg={theme.text.muted}>  {literal(field.label)}: {literal(field.value)}</text>}</For>
        <Show when={row.input?.help}><text fg={theme.text.muted}>  {literal(row.input?.help)}</text></Show>
      </box>}</For>
      <For each={props.panel.blocks}>{block => <box marginTop={1}><text fg={theme.accent}>{literal(block.title)}</text><For each={block.fields}>{f => <text>{literal(f.label)}: {literal(f.value)}</text>}</For><For each={block.lines ?? []}>{line => <text>{literal(line)}</text>}</For></box>}</For>
      <For each={props.panel.links}>{link => <text fg={theme.accent}>{literal(link.label)}: {literal(link.url)}</text>}</For>
      <Show when={props.panel.notice}><text fg={props.panel.notice_tone === "error" ? theme.text.feedback.error.base : theme.text.muted}>{literal(props.panel.notice)}</text></Show>
      <Show when={props.panel.usage}><text>{JSON.stringify(props.panel.usage, null, 2)}</text></Show>
    </scrollbox>
    <text fg={theme.text.muted}>{footer()}</text>
    <Show when={data.state?.notice}><text fg={theme.text.feedback.warning.base}>{literal(data.state?.notice)}</text></Show>
    <Show when={formSubmit()}>{submit => <text fg={theme.text.muted}>F2 or Ctrl+R: {submit().label} form · Enter edits the selected field · Ctrl+Enter also works where supported.</text>}</Show>
  </box>
}

function MultilineField(props: { title: string; value: string; limit: number; save(value: string): void }) {
  const theme = useTheme(), client = useClient(); let editor: TextareaRenderable | undefined
  Keymap.createLayer(() => ({ mode: "modal", priority: 3, commands: [{ bind: "ctrl+s", title: "Keep field value", run: () => { const value = editor?.plainText ?? ""; if (value.length > props.limit) client.fail(new Error("This value exceeds the field limit.")); else props.save(value) } }] }))
  return <box padding={3} gap={1}><text fg={theme.accent}>{props.title}</text><textarea ref={value => { editor = value }} focused initialValue={props.value} height={8} wrapMode="word" textColor={theme.text.base} onSubmit={() => { editor?.newLine() }} /><text fg={theme.text.muted}>Enter adds a line · Ctrl+S keeps this field · Esc cancels</text></box>
}
