// Adapted from upstream SessionFrame: conversation and right-hand page/file
// composition, responsive pane widths and focus live in this frontend. Gator
// replaces OpenCode's server-backed terminal panes and plugin panel host.
import { Show, type ParentProps } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import { sessionTabsFitVertically } from "../ui/layout"
import { SessionTabs } from "./session-tabs"
export function SessionFrame(props: ParentProps<{ sidebar: boolean; search(): void }>) {
  const dimensions = useTerminalDimensions()
  return <box flexGrow={1} minHeight={0} flexDirection="row">
    <Show when={props.sidebar && sessionTabsFitVertically(dimensions().width, 24)}><SessionTabs width={24} search={props.search} /></Show>
    <box flexGrow={1} flexBasis={0} minWidth={0} minHeight={0}>{props.children}</box>
  </box>
}
