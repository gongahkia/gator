// Gator adaptation of OpenCode component/prompt/index.tsx. Retains the bottom
// textarea/composer structure; replaces upstream prompt parts, API submission,
// agent variants and local stash with Gator's acknowledged drafts and queues.
import { TextareaRenderable } from "@opentui/core"
import { createEffect, createMemo, createSignal, For, on, Show } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import { useClient } from "../../context/client"
import { useData } from "../../context/data"
import { useTheme } from "../../context/theme"
import { useDialog } from "../../ui/dialog"
import { Keymap } from "../../context/keymap"
import { literal } from "../../gator/text"

export function Prompt() {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog()
  let textarea: TextareaRenderable | undefined
  const dimensions = useTerminalDimensions()
  const [selection, setSelection] = createSignal(0), [dismissed, setDismissed] = createSignal<string>()
  const commands = createMemo(() => {
    const local = [
      ["following", "Goals, checks and updates"], ["saved", "Knowledge, history and retained files"],
      ["settings", "Configure Gator"], ["models", "Choose model, effort or provider setup"],
      ["files", "Open files and working versions"], ["chats", "Switch conversations"],
      ["drafts", "Recover unsent messages"], ["queue", "Manage queued messages"],
      ["voice", "Dictation and spoken replies"], ["attach-file", "Choose a private attachment"],
      ["updates", "Review this chat's updates"], ["memories", "Review saved memory receipts"],
      ["receipts", "Inspect connected requests"], ["about", "Version and installation"],
      ["prompt-history", "Reuse a retained message"],
    ].map(([name, description]) => ({ name: name!, description: description!, needs_input: false }))
    return [...local, ...(data.state?.commands ?? []).filter(command => !local.some(item => item.name === command.name))]
      .sort((a, b) => a.name.localeCompare(b.name))
  })
  const matches = createMemo(() => {
    const text = data.draft.text
    if (dialog.stack.length || dismissed() === text || !/^\/[\w-]*$/.test(text)) return []
    return commands().filter(command => command.name.startsWith(text.slice(1).toLowerCase()))
  })
  createEffect(on(() => data.draft.text, () => { setSelection(0); setDismissed(undefined) }))
  const selected = () => Math.min(selection(), Math.max(0, matches().length - 1))
  const count = () => Math.max(1, Math.min(6, dimensions().height - 12))
  const start = () => Math.max(0, selected() - count() + 1)
  async function complete(execute: boolean) {
    const command = matches()[selected()]
    if (!command) return
    // Tab only completes text. Enter opens a command or leaves its arguments
    // editable; neither path bypasses Go's ordinary submission guards.
    const owner = client.context
    const saving = client.edit(`/${command.name} `), draft = client.draft
    await saving
    if (client.context !== owner || client.draft !== draft) return
    textarea?.gotoLineEnd()
    if (execute && !command.needs_input) await client.submit()
  }
  Keymap.createLayer(() => ({ priority: 3, enabled: matches().length > 0 && dialog.stack.length === 0, commands: [
    { bind: "up", run: () => setSelection((selected() + matches().length - 1) % matches().length) },
    { bind: "down", run: () => setSelection((selected() + 1) % matches().length) },
    { bind: "tab", run: () => { void complete(false).catch(error => client.fail(error)) } },
    { bind: "return", run: () => { void complete(true).catch(error => client.fail(error)) } },
    { bind: "escape", run: () => { setDismissed(data.draft.text) } },
  ] }))
  createEffect(() => { const text = data.draft.text; if (textarea && textarea.plainText !== text) textarea.setText(text) })
  createEffect(() => { if (dialog.stack.length || data.state?.panel) textarea?.blur(); else textarea?.focus() })
  const guide = () => { void client.guide().catch(error => client.fail(error)) }
  Keymap.createLayer(() => ({ priority: 2, enabled: dialog.stack.length === 0, commands: [
    { id: "gator.send", bind: "return", title: "Send or queue message", run: () => { void client.submit().catch(e => client.fail(e)) } },
    { id: "gator.newline", bind: "shift+return", title: "New line", run: () => { textarea?.newLine() } },
    { bind: "ctrl+j", title: "New line fallback", run: () => { textarea?.newLine() } },
    { id: "gator.guide", bind: "ctrl+g", title: "Guide active work with draft", palette: true, run: guide },
    { bind: "ctrl+g", title: "Guide work fallback", run: guide },
  ] }))
  return <box flexShrink={0} border={["top"]} borderColor={theme.accent} paddingX={1} paddingY={1}>
    <Show when={matches().length}>
      <box id="slash-completions" paddingX={1} backgroundColor={theme.background.raised.high}>
        <For each={matches().slice(start(), start() + count())}>{(command, index) => <box flexDirection="row" backgroundColor={start() + index() === selected() ? theme.background.action.primary.focused : undefined}
          onMouseUp={() => { setSelection(start() + index()); void complete(false).catch(error => client.fail(error)) }}>
          <text fg={theme.text.base} wrapMode="none" overflow="hidden">{start() + index() === selected() ? "› " : "  "}/{literal(command.name)}  {literal(command.description)}</text>
        </box>}</For>
        <text fg={theme.text.muted}>↑/↓ choose · Tab complete · Enter select · Esc dismiss ({selected() + 1}/{matches().length})</text>
      </box>
    </Show>
    <text fg={theme.text.muted}>{literal(data.state?.provider)} / {literal(data.state?.model)} · {data.state?.composer?.effort ?? "standard"} · {data.state?.running ? "Enter queues · Ctrl+G guides · Ctrl+C stops" : "Enter sends · Shift+Enter or Ctrl+J adds a line"}</text>
    <Show when={data.draft.uploads?.length}><text fg={theme.accent}>Attached: {data.draft.uploads?.map(f => literal(f.name)).join(" · ")}</text></Show>
    <Show when={data.draft.reply_to_update_id}><text fg={theme.accent}>Replying to: {literal(data.state?.chat_updates?.find(update => update.id === data.draft.reply_to_update_id)?.title || data.draft.reply_to_update_id)} · “cancel-update-reply” in commands clears the link.</text></Show>
    <textarea ref={value => { textarea = value }} focused height={3} minHeight={1} maxHeight={8}
      placeholderColor={theme.text.muted} initialValue={data.draft.text} placeholder="Message Gator… / for commands"
      textColor={theme.text.base} focusedTextColor={theme.text.base} backgroundColor={theme.background.base}
      keyBindings={[{ name: "return", action: "submit" }, { name: "return", shift: true, action: "newline" }, { name: "j", ctrl: true, action: "newline" }]}
      onContentChange={() => { if (textarea && textarea.plainText !== client.draft.text) void client.edit(textarea.plainText).catch(e => client.fail(e)) }}
      onSubmit={() => { if (!dialog.stack.length) void client.submit().catch(e => client.fail(e)) }} />
  </box>
}
