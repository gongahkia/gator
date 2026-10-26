// Gator adaptation of OpenCode component/prompt/index.tsx. Retains the bottom
// textarea/composer structure; replaces upstream prompt parts, API submission,
// agent variants and local stash with Gator's acknowledged drafts and queues.
import { TextareaRenderable } from "@opentui/core"
import { createEffect, Show } from "solid-js"
import { useClient } from "../../context/client"
import { useData } from "../../context/data"
import { useTheme } from "../../context/theme"
import { useDialog } from "../../ui/dialog"
import { Keymap } from "../../context/keymap"
import { literal } from "../../gator/text"

export function Prompt() {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog()
  let textarea: TextareaRenderable | undefined
  createEffect(() => { const text = data.draft.text; if (textarea && textarea.plainText !== text) textarea.setText(text) })
  createEffect(() => { if (dialog.stack.length || data.state?.panel) textarea?.blur(); else textarea?.focus() })
  const guide = async () => { if (client.state?.running) await client.send({ type: "key", key: "ctrl+g", text: client.draft.text }) }
  Keymap.createLayer(() => ({ priority: 2, enabled: dialog.stack.length === 0, commands: [
    { id: "gator.send", bind: "return", title: "Send or queue message", run: () => { void client.submit().catch(e => client.fail(e)) } },
    { id: "gator.newline", bind: "shift+return", title: "New line", run: () => { textarea?.newLine() } },
    { bind: "ctrl+j", title: "New line fallback", run: () => { textarea?.newLine() } },
    { id: "gator.guide", bind: "ctrl+g", title: "Guide active work with draft", palette: true, run: guide },
    { bind: "ctrl+g", title: "Guide work fallback", run: guide },
  ] }))
  return <box flexShrink={0} border={["top"]} borderColor={theme.accent} paddingX={1} paddingY={1}>
    <text fg={theme.text.muted}>{literal(data.state?.provider)} / {literal(data.state?.model)} · {data.state?.composer?.effort ?? "standard"} · {data.state?.running ? "Enter queues · Ctrl+G guides · Ctrl+C stops" : "Enter sends · Shift+Enter or Ctrl+J adds a line"}</text>
    <Show when={data.draft.uploads?.length}><text fg={theme.accent}>Attached: {data.draft.uploads?.map(f => literal(f.name)).join(" · ")}</text></Show>
    <textarea ref={value => { textarea = value }} focused height={3} minHeight={1} maxHeight={8}
      initialValue={data.draft.text} placeholder="Message Gator… / for commands"
      textColor={theme.text.base} focusedTextColor={theme.text.base} backgroundColor={theme.background.base}
      keyBindings={[{ name: "return", action: "submit" }, { name: "return", shift: true, action: "newline" }, { name: "j", ctrl: true, action: "newline" }]}
      onContentChange={() => { if (textarea && textarea.plainText !== client.draft.text) void client.edit(textarea.plainText).catch(e => client.fail(e)) }}
      onSubmit={() => { if (!dialog.stack.length) void client.submit().catch(e => client.fail(e)) }} />
  </box>
}
