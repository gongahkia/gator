import { createSignal, onCleanup, Show, type Accessor } from "solid-js"
import type { ScrollBoxRenderable } from "@opentui/core"
import { useTerminalDimensions } from "@opentui/solid"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"
import { DialogHeader, DialogLayout, useDialog } from "../ui/dialog"
import { Spinner } from "../component/spinner"
import { errorMessage } from "../util/error"
import { literal } from "./text"

type ReadViewOptions<T> = {
  title: string
  context: string
  description: string
  read(): Promise<T>
  show(value: T): void
  back?: () => unknown
}

// Closing a read view retires its presentation, not an already dispatched host
// request. Its completion stays handled and cannot reopen an overlay or report
// a failure in another view. Execution, permissions and durable outcomes remain
// owned by the host; this helper cannot cancel or replay an external action.
export function useReadView() {
  const client = useClient(), dialog = useDialog(), pending = new Set<() => void>()
  onCleanup(() => { for (const close of pending) close(); pending.clear() })
  return function readView<T>(options: ReadViewOptions<T>): Promise<void> {
    if (client.context !== options.context) return Promise.reject(new Error("Return to the original conversation and select this item again."))
    const token = Symbol(options.title), [busy, setBusy] = createSignal(true), [issue, setIssue] = createSignal("")
    let closed = false, completed = false, finish!: () => void
    const done = new Promise<void>(resolve => { finish = () => { if (!completed) { completed = true; resolve() } } })
    const close = () => { closed = true; pending.delete(close); finish() }
    const current = () => !closed && dialog.key === token && client.context === options.context
    const leave = (back: boolean) => {
      if (!current()) return
      dialog.clear()
      if (!back || !options.back) return
      try { const value = options.back(); if (value instanceof Promise) void client.track("Opening item actions…", value).catch(error => client.fail(error)) }
      catch (error) { client.fail(error) }
    }
    const load = async () => {
      if (!current()) return
      setBusy(true); setIssue("")
      try {
        const value = await options.read()
        if (!current()) return
        options.show(value)
        if (current()) dialog.clear()
      } catch (error) {
        if (current()) { setBusy(false); setIssue(errorMessage(error)) }
      } finally { finish() }
    }
    pending.add(close)
    dialog.replace(() => <ReadView title={options.title} description={options.description} busy={busy} issue={issue} retry={() => { void load() }} leave={leave} retired={close} />, close, { key: token, size: "large" })
    // Start after the dialog owns its token; synchronous host failures use the
    // same recovery view and a replaced view never starts a late request.
    void Promise.resolve().then(load)
    return done
  }
}

function ReadView(props: { title: string; description: string; busy: Accessor<boolean>; issue: Accessor<string>; retry(): void; leave(back: boolean): void; retired(): void }) {
  const theme = useTheme(), dimensions = useTerminalDimensions()
  let scroll: ScrollBoxRenderable | undefined
  onCleanup(props.retired)
  const page = () => Math.max(1, (scroll?.viewport.height ?? 2) - 1)
  Keymap.createLayer(() => ({ mode: "modal", priority: 10, commands: [
    { bind: "escape", title: "Back from read", run: () => props.leave(true) },
    { bind: "ctrl+c", title: "Close read view", run: () => props.leave(false) },
    { bind: "ctrl+r", title: "Retry this read", enabled: !props.busy(), run: props.retry },
    { bind: "up", title: "Earlier detail", run: () => scroll?.scrollBy(-1) },
    { bind: "down", title: "Later detail", run: () => scroll?.scrollBy(1) },
    { bind: "pageup", title: "Earlier detail page", run: () => scroll?.scrollBy(-page()) },
    { bind: "pagedown", title: "Later detail page", run: () => scroll?.scrollBy(page()) },
  ] }))
  return <DialogLayout id="item-read-view" header={<DialogHeader titleView={<text height={1} wrapMode="none" truncate>{literal(props.title)}</text>} onClose={() => props.leave(true)} closeLabel="back" />}>
    <scrollbox ref={node => { scroll = node }} paddingRight={1} height={Math.max(2, dimensions().height - (dimensions().height >= 24 ? Math.floor(dimensions().height / 4) : 0) - 8)}>
      <Show when={props.busy()} fallback={<text fg={theme.text.feedback.error.base}>{literal(props.issue())}</text>}><Spinner>Reading…</Spinner></Show>
      <text fg={theme.text.muted}>{literal(props.description)}</text>
    </scrollbox>
    <text fg={theme.text.muted}>{props.busy() ? "Esc back · Ctrl+C close" : "Ctrl+R retry read · Esc back · Ctrl+C close"}</text>
  </DialogLayout>
}
