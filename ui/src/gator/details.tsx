import { useTerminalDimensions } from "@opentui/solid"
import { DialogHeader, DialogLayout } from "../ui/dialog"
import { Keymap } from "../context/keymap"
import { useTheme } from "../context/theme"
import type { ScrollBoxRenderable } from "@opentui/core"
import { literal } from "./text"

// Retained evidence is selectable and scrollable, with no editable/submit field.
export function Details(props: { title: string; text: string }) {
  const dimensions = useTerminalDimensions(), theme = useTheme()
  let scroll: ScrollBoxRenderable | undefined
  const page = () => Math.max(1, (scroll?.viewport.height ?? 4) - 1)
  Keymap.createLayer(() => ({ mode: "modal", commands: [
    { bind: "up", title: "Earlier line", run: () => scroll?.scrollBy(-1) },
    { bind: "down", title: "Later line", run: () => scroll?.scrollBy(1) },
    { bind: "pageup", title: "Earlier page", run: () => scroll?.scrollBy(-page()) },
    { bind: "pagedown", title: "Later page", run: () => scroll?.scrollBy(page()) },
  ] }))
  return <DialogLayout header={<DialogHeader title={literal(props.title)} />}>
    <scrollbox ref={node => { scroll = node }} paddingRight={1} height={Math.max(4, dimensions().height - 14)}><text fg={theme.text.base}>{literal(props.text)}</text></scrollbox>
    <text fg={theme.text.muted}>Read only · ↑/↓ or PgUp/PgDn scroll · Esc close · Use “copy-message” in commands to copy chat text.</text>
  </DialogLayout>
}
