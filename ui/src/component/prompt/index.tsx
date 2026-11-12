// Gator adaptation of OpenCode component/prompt/index.tsx. Retains the bottom
// textarea/composer structure; replaces upstream prompt parts, API submission,
// agent variants and local stash with Gator's acknowledged drafts and queues.
import { TextareaRenderable, TextAttributes } from "@opentui/core"
import { createEffect, createMemo, createSignal, For, on, onCleanup, Show } from "solid-js"
import { useRenderer, useTerminalDimensions } from "@opentui/solid"
import { useClient } from "../../context/client"
import { useData } from "../../context/data"
import { useTheme } from "../../context/theme"
import { useDialog } from "../../ui/dialog"
import { Keymap } from "../../context/keymap"
import { literal } from "../../gator/text"
import { useKeyHints } from "../../gator/key-hints"

export function Prompt() {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog()
  let textarea: TextareaRenderable | undefined
  const dimensions = useTerminalDimensions()
  const renderer = useRenderer()
  const hint = useKeyHints()
  const compact = () => dimensions().height < 26
  const short = () => dimensions().height < 18
  const model = () => data.state?.composer?.models.find(model => model.provider === data.state?.provider && model.model === data.state?.model)?.label || data.state?.model || data.state?.provider || "Choose a model"
  const modelState = () => data.state?.composer?.readiness?.state
  const modelSummary = () => modelState() && modelState() !== "configured" ? "Model setup needed" : model()
  const contextualHint = () => {
    const hasItems = !!data.state?.messages.length || !!data.state?.chat_updates?.length || !!data.state?.dialogue?.length
    const actions = `${hint("gator.context-actions")} actions`, context = `${hint("gator.draft-context")} context`, commands = `/ ${hint("command.palette.show")}`
    if (dimensions().width < 60) return hasItems ? actions : "/ commands"
    if (!hasItems) return `${context} · ${commands}`
    return dimensions().width < 100 ? `${actions} · ${commands}` : `${actions} · ${context} · ${commands}`
  }
  const [selection, setSelection] = createSignal(0), [dismissed, setDismissed] = createSignal<string>()
  const commands = createMemo(() => {
    const local = [
      ["following", "Goals, checks and updates"], ["saved", "Knowledge, history and retained files"],
      ["settings", "Configure Gator"], ["models", "Choose model, effort or provider setup"],
      ["files", "Open files and working versions"], ["chats", "Switch conversations"],
      ["actions", "Open, edit, copy or inspect a conversation item"],
      ["drafts", "Recover unsent messages"], ["queue", "Manage queued messages"],
      ["voice", "Dictation and spoken replies"], ["attach-file", "Choose a private attachment"],
      ["updates", "Review this chat's updates"], ["memories", "Review saved memory receipts"],
      ["receipts", "Inspect connected requests"], ["about", "Version and installation"],
      ["prompt-history", "Reuse a retained message"], ["exit", "Exit Gator"], ["quit", "Exit Gator"],
    ].map(([name, description]) => ({ name: name!, description: description!, needs_input: false }))
    const additional = Object.keys(client.actions).filter(name => name !== "activity" && !local.some(item => item.name === name)).map(name => ({ name, description: name.replaceAll("-", " "), needs_input: false }))
    return [...local, ...additional, ...(data.state?.commands ?? []).filter(command => !local.some(item => item.name === command.name) && !additional.some(item => item.name === command.name))]
      .sort((a, b) => a.name.localeCompare(b.name))
  })
  const matches = createMemo(() => {
    const text = data.draft.text
    if (dialog.stack.length || ["recording", "transcribing"].includes(data.voicePhase) || dismissed() === text || !/^\/[\w-]*$/.test(text)) return []
    return commands().filter(command => command.name.startsWith(text.slice(1).toLowerCase()))
  })
  createEffect(on(() => data.draft.text, () => { setSelection(0); setDismissed(undefined) }))
  const selected = () => Math.min(selection(), Math.max(0, matches().length - 1))
  const count = () => Math.max(1, Math.min(6, dimensions().height - 18))
  const metadata = () => [data.draft.plan_first ? "Plan first" : "",
    data.draft.uploads?.length ? `${data.draft.uploads.length} ${data.draft.uploads.length === 1 ? "file" : "files"}` : "",
    data.draft.reply_to_update_id ? `Reply: ${literal(data.state?.chat_updates?.find(update => update.id === data.draft.reply_to_update_id)?.title || "update")}` : ""].filter(Boolean).join(" · ")
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
  // OpenTUI may emit intermediate content changes while replacing the buffer.
  // Those are synchronization, not edits; feeding them back into the client
  // can alternate old/new drafts indefinitely during live state updates.
  let syncingDraft = false
  let pendingDraft: string | undefined, synchronize: (() => void) | undefined
  const scheduleDraft = () => {
    if (synchronize) return
    synchronize = () => {
      synchronize = undefined
      if (!textarea || textarea.isDestroyed || pendingDraft === undefined) return
      // The editor's wrapping/cursor operations need a computed native layout.
      // Draft replacement must not reenter a Solid/native layout mutation.
      if (textarea.width < 1 || textarea.height < 1) { scheduleDraft(); return }
      const text = pendingDraft; pendingDraft = undefined
      if (textarea.plainText === text) return
      syncingDraft = true
      try { textarea.setText(text); textarea.gotoLineEnd() } finally { syncingDraft = false }
    }
    renderer.once("frame", synchronize)
    renderer.requestRender()
  }
  createEffect(() => { const text = data.draft.text; pendingDraft = text; scheduleDraft() })
  onCleanup(() => { if (synchronize) renderer.off("frame", synchronize); synchronize = undefined; pendingDraft = undefined })
  createEffect(() => { if (dialog.stack.length || data.state?.panel) textarea?.blur(); else textarea?.focus() })
  const guide = () => { void client.guide().catch(error => client.fail(error)) }
  Keymap.createLayer(() => ({ priority: 2, enabled: dialog.stack.length === 0, commands: [
    { id: "gator.send", bind: "return", title: "Talk, correct or start work", run: () => { void client.submit().catch(e => client.fail(e)) } },
    { id: "gator.queue", bind: "tab", title: "Queue draft after the current task", palette: true,
      enabled: () => !!data.state?.running && !data.state?.cancelling && !data.draft.text.trimStart().startsWith("/") && !!(data.draft.text.trim() || data.draft.uploads?.length),
      run: () => { void client.submit(true).catch(e => client.fail(e)) } },
    { id: "gator.newline", bind: "shift+return", title: "New line", run: () => { textarea?.newLine() } },
    { bind: "ctrl+j", title: "New line fallback", run: () => { textarea?.newLine() } },
    { id: "gator.guide", bind: "ctrl+g", title: "Guide active work with draft", palette: true, run: guide },
    { id: "gator.draft-context", bind: "ctrl+t", title: "Inspect draft, processing, sources and work limits", palette: true, run: () => { void client.navigate("/draft-context").catch(error => client.fail(error)) } },
  ] }))
  return <box id="composer" flexShrink={0} marginX={dimensions().width < 60 ? 1 : 2} marginTop={short() ? 0 : 1} marginBottom={short() ? 0 : 1}>
    <Show when={matches().length}>
      <box id="slash-completions" paddingX={1} backgroundColor={theme.background.raised.high}>
        <For each={matches().slice(start(), start() + count())}>{(command, index) => <box flexDirection="row" backgroundColor={start() + index() === selected() ? theme.background.action.primary.focused : undefined}
          onMouseUp={() => { setSelection(start() + index()); void complete(false).catch(error => client.fail(error)) }}>
          <text fg={theme.text.base} wrapMode="none" overflow="hidden">{start() + index() === selected() ? "› " : "  "}/{literal(command.name)}  {literal(command.description)}</text>
        </box>}</For>
        <text fg={theme.text.muted} height={1} wrapMode="none" truncate>{dimensions().width < 60 ? "Tab complete · Enter select · Esc close" : `↑/↓ choose · Tab complete · Enter select · Esc dismiss (${selected() + 1}/${matches().length})`}</text>
      </box>
    </Show>
    <box border={["left"]} borderColor={theme.accent} backgroundColor={theme.background.raised.base} paddingX={short() ? 1 : 2} paddingY={compact() ? 0 : 1} gap={compact() ? 0 : 1}>
    <Show when={metadata()}><text id="draft-context-summary" fg={theme.accent} height={1} wrapMode="none" truncate onMouseUp={() => void client.navigate("/draft-context").catch(error => client.fail(error))}>{hint("gator.draft-context")} · {metadata()}</text></Show>
    <textarea id="message-input" ref={value => { textarea = value }} focused width="100%" minHeight={1} maxHeight={short() ? 2 : Math.max(2, Math.min(8, Math.floor(dimensions().height / 4)))}
      placeholderColor={theme.text.muted} initialValue={client.draft.text} placeholder="Ask anything…"
      textColor={theme.text.base} focusedTextColor={theme.text.base} backgroundColor={theme.background.raised.base} focusedBackgroundColor={theme.background.raised.base}
      keyBindings={[{ name: "return", action: "submit" }, { name: "return", shift: true, action: "newline" }, { name: "j", ctrl: true, action: "newline" }]}
      onContentChange={() => { if (!syncingDraft && textarea && textarea.plainText !== client.draft.text) void client.edit(textarea.plainText).catch(e => client.fail(e)) }}
      onSubmit={() => { if (!dialog.stack.length) void client.submit().catch(e => client.fail(e)) }} />
    <text id="composer-model" height={1} wrapMode="none" truncate selectable={false} fg={theme.text.muted} onMouseDown={event => event.stopPropagation()}
      onMouseUp={event => { if (event.button !== 0) return; event.stopPropagation(); void client.navigate(modelState() && modelState() !== "configured" ? "/setup" : "/models").catch(e => client.fail(e)) }}><b style={{ fg: theme.accent }}>Gator</b>  {literal(modelSummary())}{(!modelState() || modelState() === "configured") && data.state?.composer?.effort ? ` · ${data.state.composer.effort} effort` : ""}</text>
    </box>
    <box paddingX={1} paddingTop={compact() ? 0 : 1} flexDirection="row" justifyContent="space-between" gap={2}>
      <text fg={theme.text.muted} attributes={TextAttributes.DIM} flexGrow={1} flexBasis={0} flexShrink={1} minWidth={0} height={1} wrapMode="none" truncate>{data.snapshot.connection !== "connected" ? "Reconnect before sending" : ["recording", "transcribing"].includes(data.voicePhase) ? "Finish dictation before sending" : data.state?.running ? `${hint("gator.send")} ${dimensions().width < 60 ? "talk" : "talk / steer"} · ${hint("gator.queue")} queue` : data.state?.dialogue_active ? `${hint("gator.send")} send · Talk, correct or start work` : `${hint("gator.send")} send · ${hint("gator.newline")} new line`}</text>
      <text fg={theme.text.muted} attributes={TextAttributes.DIM} flexShrink={0}>{contextualHint()}</text>
    </box>
  </box>
}
