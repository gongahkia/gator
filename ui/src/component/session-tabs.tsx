// Gator adaptation of upstream session tabs: the rail and narrow-layout rules
// are retained; session identity/activity comes exclusively from Gator's host.
import { For } from "solid-js"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { SessionTabsRailControls } from "./session-tabs-rail"
import { literal } from "../gator/text"
export type SessionTabsController = { search?: () => void }
export function SessionTabs(props: { width: number; search(): void }) {
  const data = useData(), client = useClient(), theme = useTheme()
  return <box width={props.width} flexShrink={0} backgroundColor={theme.background.base} paddingY={1} paddingX={1} gap={1}>
    <SessionTabsRailControls width={props.width - 2} tabs={{ search: props.search }} belowHighlighted={false} />
    <For each={[["Chat", "/chat"], ["Chats", "/conversations"], ["Following", "/following"], ["Saved", "/saved"], ["Settings", "/settings"]]}>{([title, command]) => <text fg={theme.text.base} onMouseUp={() => void client.navigate(command).catch(e => client.fail(e))}>{title}</text>}</For>
    <scrollbox flexGrow={1}>
      <For each={data.snapshot.chatActivity}>{chat => <box marginBottom={1} onMouseUp={() => void client.openSession(chat.id).catch(e => client.fail(e))}>
        <text fg={chat.id === data.state?.conversation_id ? theme.accent : theme.text.base}>{literal(chat.title)}</text><text fg={theme.text.muted}>{literal(chat.status)}</text>
      </box>}</For>
    </scrollbox>
    <text fg={theme.text.muted}>Ctrl+P commands</text><text fg={theme.text.muted}>Ctrl+B hide rail</text>
  </box>
}
