// Gator adaptation of upstream session tabs: the rail and narrow-layout rules
// are retained; session identity/activity comes exclusively from Gator's host.
import { createMemo, For, Index, Show } from "solid-js"
import { TextAttributes } from "@opentui/core"
import { useData } from "../context/data"
import { useClient } from "../context/client"
import { useTheme } from "../context/theme"
import { SessionTabsRailControls } from "./session-tabs-rail"
import { Spinner } from "./spinner"
import { literal } from "../gator/text"
import { navigationPage } from "../gator/navigation"
export type SessionTabsController = { search?: () => void }
export function SessionTabs(props: { width: number; search(): void }) {
  const data = useData(), client = useClient(), theme = useTheme()
  const current = createMemo(() => navigationPage(data.route, data.state?.panel))
  return <box width={props.width} flexShrink={0} backgroundColor={theme.background.base} paddingY={1} paddingX={1} gap={1}>
    <SessionTabsRailControls width={props.width - 2} tabs={{ search: props.search }} belowHighlighted={false} />
    <For each={[["Chat", "/chat", "chat"], ["Chats", "/conversations", "chats"], ["Following", "/following", "following"], ["Saved", "/saved", "saved"], ["Settings", "/settings", "settings"]]}>{([title, command, page]) => <box id={`rail-page-${page}`} backgroundColor={current() === page ? theme.background.raised.high : undefined}
      onMouseUp={() => void client.navigate(command).catch(e => client.fail(e))}>
      <text fg={current() === page ? theme.accent : theme.text.base} attributes={current() === page ? TextAttributes.BOLD : undefined}>{current() === page ? "› " : "  "}{title}</text>
    </box>}</For>
    <scrollbox flexGrow={1}>
      <Index each={data.snapshot.chatActivity}>{chat => <box marginBottom={1} onMouseUp={() => void client.openSession(chat().id).catch(e => client.fail(e))}>
        <text fg={chat().id === data.state?.conversation_id ? theme.accent : theme.text.base}>{literal(chat().title)}</text>
        <Show when={chat().running && chat().status === "Working" && chat().id !== data.state?.conversation_id} fallback={<text fg={theme.text.muted}>{literal(chat().status)}</text>}><Spinner delay={150}>Working</Spinner></Show>
      </box>}</Index>
    </scrollbox>
    <text fg={theme.text.muted}>Ctrl+P commands</text><text fg={theme.text.muted}>Ctrl+B hide rail</text>
  </box>
}
