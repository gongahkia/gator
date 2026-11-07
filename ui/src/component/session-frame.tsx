import { Show, createSignal, type ParentProps } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import type { BoxRenderable, MouseEvent } from "@opentui/core"
import { sessionTabsFitVertically } from "../ui/layout"
import { SessionTabs } from "./session-tabs"
import { useTheme } from "../context/theme"
import { Keymap } from "../context/keymap"

export function SessionFrame(props: ParentProps<{ sidebar: boolean; search(): void }>) {
  const dimensions = useTerminalDimensions(), theme = useTheme()
  const [preferred, setPreferred] = createSignal(24), [dragging, setDragging] = createSignal(false), [hovered, setHovered] = createSignal(false)
  const visible = () => props.sidebar && sessionTabsFitVertically(dimensions().width, 24)
  const clamp = (width: number) => Math.max(18, Math.min(48, dimensions().width - 44, Math.round(width)))
  const width = () => clamp(preferred())
  let rail: BoxRenderable | undefined
  const resize = (event: MouseEvent) => { if (!dragging()) return; event.stopPropagation(); setPreferred(clamp(event.x - (rail?.screenX ?? 0) + 1)) }
  Keymap.createLayer(() => ({ enabled: visible(), commands: [
    { bind: "ctrl+shift+left", title: "Narrow sidebar", run: () => { setPreferred(clamp(width() - 2)) } },
    { bind: "ctrl+shift+right", title: "Widen sidebar", run: () => { setPreferred(clamp(width() + 2)) } },
  ] }))
  return <box flexGrow={1} minHeight={0} flexDirection="row">
    <Show when={visible()}><box ref={value => { rail = value }} id="navigation-rail" width={width()} flexShrink={0} flexDirection="row">
      <SessionTabs width={width() - 1} search={props.search} />
      <box id="sidebar-resize" width={1} flexShrink={0} border={["left"]} borderColor={dragging() || hovered() ? theme.accent : theme.text.muted}
        onMouseOver={() => setHovered(true)} onMouseOut={() => setHovered(false)}
        onMouseDown={event => { if (event.button === 0) { event.stopPropagation(); setDragging(true) } }}
        onMouseDrag={resize} onMouseDragEnd={event => { resize(event); setDragging(false) }}
        onMouseUp={event => { resize(event); setDragging(false) }} />
    </box></Show>
    <box flexGrow={1} flexBasis={0} minWidth={0} minHeight={0}>{props.children}</box>
  </box>
}
