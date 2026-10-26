// Composition adapted from pinned OpenCode app.tsx: renderer-scoped providers,
// keymap, dialogs, toast, responsive session frame and bottom prompt. The Gator
// client and data contexts replace OpenCode's server/client/plugin composition.
import { createSignal, Show, For } from "solid-js"
import { useRenderer } from "@opentui/solid"
import { ClientProvider, useClient } from "./context/client"
import { useData } from "./context/data"
import { ConfigProvider, terminalShortcuts } from "./config"
import { ThemeProvider, useTheme } from "./context/theme"
import { Keymap } from "./context/keymap"
import { ClipboardProvider } from "./context/clipboard"
import { DialogProvider, useDialog } from "./ui/dialog"
import { ToastProvider } from "./ui/toast"
import { DialogConfirm } from "./ui/dialog-confirm"
import { DialogSelect } from "./ui/dialog-select"
import { CommandPaletteDialog } from "./component/command-palette"
import { SessionFrame } from "./component/session-frame"
import { Prompt } from "./component/prompt"
import { Transcript } from "./gator/transcript"
import { Panel } from "./gator/panel"
import { literal } from "./gator/text"
import { Hub } from "./gator/hub"
import { useActions } from "./gator/actions"
import { useNative } from "./gator/native"
import type { GatorClient } from "./gator/client"
import type { ClipboardService } from "./context/clipboard"

function Shell(props: { quit(): Promise<void> }) {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog()
  const controls = useActions()
  const voice = useNative(controls)
  const [sidebar, setSidebar] = createSignal(true)
  const attempt = (operation: Promise<unknown>) => { void operation.catch(error => client.fail(error)) }
  const palette = () => dialog.replace(() => <CommandPaletteDialog />)
  const navigate = (command: string) => attempt(client.navigate(command))
  const quit = async () => {
    if (!await client.mayLeave()) return
    if (data.snapshot.allChatsBusy || data.snapshot.allChatsQueued) dialog.replace(() => <DialogConfirm title="Stop work and close Gator?" message="Tasks in all open chats will stop. Queues remain paused for review after restart." onConfirm={() => attempt(props.quit())} />)
    else attempt(props.quit())
  }
  function approve(approved: boolean) {
    const pending = client.state?.pending_approval
    if (!pending) return
    const decide = () => attempt(client.send({ type: "approval", interaction_id: pending.id, approval_token: pending.token, approved }))
    if (!approved) { decide(); return }
    dialog.replace(() => <DialogConfirm title="Approve this exact action?" message="Confirm only after reviewing the current request in this chat. This decision applies to its live token." onConfirm={decide} />)
  }
  Keymap.createLayer(() => ({ mode: "global", commands: [
    { id: "command.palette.show", title: "Search commands", bind: "ctrl+p", run: palette },
    { id: "gator.sidebar", title: "Toggle navigation", bind: "ctrl+b", palette: true, run: () => { setSidebar(v => !v) } },
    { id: "gator.quit", title: "Quit Gator", bind: "ctrl+q", palette: true, run: quit },
    { id: "gator.cancel", title: "Stop active work", bind: "ctrl+c", palette: true, run: () => data.state?.running ? attempt(client.send({ type: "key", key: "ctrl+c" })) : quit() },
    { id: "gator.new", title: "New independent chat", bind: "ctrl+n", palette: true, run: () => navigate("/new") },
    { id: "gator.search", title: "Search current transcript", bind: "ctrl+f", palette: true, enabled: !data.state?.panel, run: () => controls.trigger("search") },
    { id: "gator.help", title: "Gator help", bind: "f1", palette: true, enabled: !data.state?.panel, run: () => navigate("/help") },
    { bind: "ctrl+p", title: "Command palette fallback", run: palette },
    { id: "gator.voice", title: "Voice controls", bind: "ctrl+shift+v", palette: true, run: () => controls.trigger("voice") },
    { id: "gator.approve", title: "Review current approval", bind: "ctrl+y", palette: true, run: () => approve(true) },
    { id: "gator.decline", title: "Decline current approval", bind: "ctrl+d", palette: true, run: () => approve(false) },
    { id: "gator.reconnect", title: "Reconnect selected chat", palette: true, run: () => attempt(client.host.sessions.restart()) },
    ...Object.keys(controls.actions).map(name => ({ id: `gator.local.${name}`, title: name, group: "Gator workspace", palette: true as const, run: () => controls.trigger(name) })),
    ...["following", "saved", "settings"].map(name => ({ id: `gator.${name}`, title: name[0].toUpperCase() + name.slice(1), palette: true as const, run: () => navigate(`/${name}`) })),
    ...(data.state?.commands ?? []).map(command => ({ id: `gator.command.${encodeURIComponent(command.name)}`, title: `/${command.name}`, description: literal(command.description), group: "Gator", palette: true as const,
      run: () => { if (command.needs_input) attempt(client.edit(`/${command.name} `)); else navigate(`/${command.name}`) } })),
  ] }))
  return <box width="100%" height="100%" backgroundColor={theme.background.base}>
    <box height={1} paddingX={1} flexDirection="row" justifyContent="space-between"><text fg={theme.accent}><b>Gator</b> · {literal(data.state?.title) || "Opening chat…"}</text><text fg={theme.text.muted}>{literal(data.snapshot.connection)}</text></box>
    <SessionFrame sidebar={sidebar()} search={palette}>
      <Show when={data.route === "chat"} fallback={<Hub />}><Show when={data.state?.panel} fallback={<><Transcript /><Prompt /></>}>{panel => <Panel panel={panel()} />}</Show></Show>
    </SessionFrame>
    <Show when={data.issue || data.snapshot.error || data.snapshot.sessionIssue}><text fg={theme.text.feedback.error.base}>{literal(data.issue || data.snapshot.error || data.snapshot.sessionIssue)}</text></Show>
    <Show when={data.voicePhase !== "idle"}><text fg={theme.text.feedback.warning.base}>Voice: {data.voicePhase} · Ctrl+Shift+V opens stop/cancel controls.</text></Show>
    <Show when={data.state?.external}><text fg={theme.accent}>A reviewed external step is ready. Use “continue-external” in Ctrl+P to continue and return to this chat.</text></Show>
    <Show when={data.state?.status_line_enabled}><text fg={theme.text.muted}>{data.state?.status_line.map(literal).join(" · ")}</text></Show>
  </box>
}
function Themed(props: { client: GatorClient; quit(): Promise<void>; clipboard: ClipboardService }) {
  const data = useData()
  return <ThemeProvider theme={data.state?.theme ?? "dark"}><ConfigProvider bindings={terminalShortcuts(data.state?.desktop?.shortcuts)}>
    <ClipboardProvider value={props.clipboard}><Keymap.Provider><ToastProvider><DialogProvider><Shell quit={props.quit} /></DialogProvider></ToastProvider></Keymap.Provider></ClipboardProvider>
  </ConfigProvider></ThemeProvider>
}
export function App(props: { client: GatorClient; quit(): Promise<void>; clipboard: ClipboardService }) {
  return <ClientProvider client={props.client}><Themed {...props} /></ClientProvider>
}
