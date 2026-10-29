import { createSignal } from "solid-js"
import { TextAttributes } from "@opentui/core"
import { Keymap } from "../context/keymap"
import { useTheme } from "../context/theme"
import type { SessionTabsController } from "./session-tabs"

export function SessionTabsRailControls(props: { width: number; tabs: SessionTabsController; belowHighlighted: boolean }) {
  const theme = useTheme(), keymap = Keymap.use()
  const [hovered, setHovered] = createSignal(false)
  let pressed = false
  return <text height={1} width={props.width} selectable={false} fg={hovered() ? theme.accent : theme.text.base} attributes={hovered() ? TextAttributes.BOLD : undefined}
    onMouseOver={() => setHovered(true)} onMouseOut={() => { setHovered(false); pressed = false }}
    onMouseDown={event => { pressed = event.button === 0; event.stopPropagation() }}
    onMouseUp={event => {
      event.stopPropagation()
      if (event.button !== 0 || !pressed) return
      pressed = false
      if (props.tabs.search) props.tabs.search(); else keymap.dispatch("session.list")
    }} onMouseDragEnd={() => { pressed = false }}>⌕ Search</text>
}
