// Composition adapted from pinned OpenCode app.tsx: renderer-scoped providers,
// keymap, dialogs, toast, responsive session frame and bottom prompt. The Gator
// client and data contexts replace OpenCode's server/client/plugin composition.
import { Show, For, onCleanup } from "solid-js"
import { useRenderer } from "@opentui/solid"
import { ClientProvider, useClient } from "./context/client"
import { DataProvider, useData } from "./context/data"
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
import { Progress } from "./gator/progress"
import { ConnectionStatus } from "./gator/connection-status"
import { Transcript } from "./gator/transcript"
import { Panel } from "./gator/panel"
import { literal } from "./gator/text"
import { Hub } from "./gator/hub"
import { Following } from "./gator/following"
import { useActions } from "./gator/actions"
import { useProductActions } from "./gator/product-actions"
import { useNative } from "./gator/native"
import { commandGroup, commandAliases } from "./gator/command-groups"
import { ApprovalReview } from "./gator/approval-review"
import { runBackendCommand } from "./gator/command-actions"
import { RecoveryNotice } from "./gator/recovery-notice"
import { VoiceNotice } from "./gator/voice-notice"
import type { VoiceSession } from "@gator/application-host/voice-session"
import type { GatorClient } from "./gator/client"
import type { ClipboardService } from "./context/clipboard"

function Shell(props: { quit(): Promise<void>; voice?: VoiceSession }) {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog()
  const controls = useActions()
  const product = useProductActions(controls)
  const voice = useNative(controls, props.voice)
  const attempt = (operation: Promise<unknown>) => { void operation.catch(error => client.fail(error)) }
  const palette = () => dialog.replace(() => <CommandPaletteDialog />)
  const navigate = (command: string) => attempt(client.navigate(command))
  const close = async () => {
    if (/^\/(exit|quit)$/.test(client.draft.text.trim())) await client.edit("")
    await client.track("Closing Gator…", props.quit())
  }
  const quit = async () => {
    if (!await client.mayLeave()) return
    if (data.snapshot.allChatsBusy || data.snapshot.allChatsQueued) dialog.replace(() => <DialogConfirm title="Stop work and close Gator?" message="Tasks in all open chats will stop. Queues remain paused for review after restart." onConfirm={() => attempt(close())} />)
    else await close()
  }
  client.requestQuit = quit
  onCleanup(() => { if (client.requestQuit === quit) client.requestQuit = undefined })
  function approve(approved: boolean) {
    const pending = client.state?.pending_approval
    if (!pending) return
    const decide = () => attempt(client.send({ type: "approval", interaction_id: pending.id, approval_token: pending.token, approved }))
    if (!approved) { decide(); return }
    const context = client.context, message = client.state?.messages[pending.message_index]
    dialog.replace(() => <ApprovalReview pending={pending} message={message} context={context} />, undefined, { size: "xlarge" })
  }
  Keymap.createLayer(() => ({ mode: "global", commands: [
    { id: "command.palette.show", title: "Search commands", bind: "ctrl+p", run: palette },
    { id: "gator.sidebar", title: "Toggle navigation", bind: "ctrl+b", palette: true, run: () => attempt(client.configurePreferences({ sidebarCollapsed: !client.preferences.sidebarCollapsed })) },
    { id: "gator.quit", title: "Quit Gator", bind: "ctrl+q", palette: true, run: quit },
    { id: "gator.cancel", title: "Cancel dialog or stop active work", bind: "ctrl+c", palette: true, run: () => {
      if (dialog.stack.length) dialog.clear()
      else if (data.voicePhase !== "idle") voice.cancel()
      else if (data.state?.running) attempt(client.send({ type: "key", key: "esc" }))
    } },
    { id: "gator.new", title: "New independent chat", bind: "ctrl+n", palette: true, run: () => navigate("/new") },
    { id: "gator.search", title: "Search current transcript", bind: "ctrl+f", palette: true, enabled: !data.state?.panel, run: () => controls.trigger("search") },
    { id: "gator.help", title: "Gator help", bind: "ctrl+o", palette: true, enabled: !data.state?.panel, run: () => navigate("/help") },
    { id: "gator.voice", title: "Start or finish dictation", bind: "ctrl+v", palette: true, run: () => controls.trigger("toggle-dictation") },
    { id: "gator.approve", title: "Review current approval", bind: "ctrl+y", palette: true, enabled: dialog.stack.length === 0, run: () => approve(true) },
    { id: "gator.decline", title: "Decline current approval", bind: "ctrl+d", palette: true, enabled: dialog.stack.length === 0, run: () => approve(false) },
    { id: "gator.reconnect", title: "Reconnect selected chat", palette: true, run: () => attempt(client.reconnect()) },
    ...Object.keys(controls.actions).map(name => ({ id: `gator.local.${name}`, title: name, group: commandGroup(name), palette: true as const, run: () => controls.trigger(name) })),
    ...["following", "saved", "settings"].map(name => ({ id: `gator.${name}`, title: name[0].toUpperCase() + name.slice(1), palette: true as const, run: () => navigate(`/${name}`) })),
    // Only collapse known equivalent actions. Go's goal activity and the local
    // public-work trace share a name but are distinct capabilities.
    ...(data.state?.commands ?? []).filter(command => !["help", "queue", "following", "saved", "settings", "new", "exit", "cancel", "approve", "deny"].includes(command.name)).map(command => ({ id: `gator.command.${encodeURIComponent(command.name)}`, title: literal(command.description), group: command.name === "activity" ? "Following" : commandGroup(command.name), palette: true as const,
      slash: { name: command.name, aliases: commandAliases(command.name), ...(command.needs_input ? { arguments: true as const } : {}) },
      run: () => { const operation = runBackendCommand(client, controls, command); if (operation instanceof Promise) attempt(operation) } })),
  ] }))
  return <box width="100%" height="100%" backgroundColor={theme.background.base}>
    <box height={1} marginY={1} paddingX={2} gap={2} flexDirection="row" justifyContent="space-between"><text flexGrow={1} flexShrink={1} minWidth={0} height={1} wrapMode="none" truncate fg={theme.text.muted}><b style={{ fg: theme.text.base }}>🐊 Gator</b>  {literal(data.state?.title) || "Opening chat…"}</text><ConnectionStatus connection={data.snapshot.connection} error={!!data.snapshot.error} /></box>
    <SessionFrame sidebar={!data.preferences.sidebarCollapsed} search={palette}>
      <Show when={data.route === "chat"} fallback={<Show when={data.route === "following"} fallback={<Hub />}><Following controls={controls} inspect={product.commitment} refresh={product.refreshChecks} /></Show>}><Show when={data.state?.panel} fallback={<><Transcript /><Prompt /></>}>{panel => <Panel panel={panel()} />}</Show></Show>
    </SessionFrame>
    <RecoveryNotice open={() => controls.trigger("recover")} />
    <VoiceNotice />
    <Show when={data.state?.external}><text fg={theme.accent}>A reviewed external step is ready. Use “continue-external” in Ctrl+P to continue and return to this chat.</text></Show>
    <Progress />
    <Show when={data.state?.status_line_enabled}><text fg={theme.text.muted}>{data.state?.status_line.map(literal).join(" · ")}</text></Show>
  </box>
}
function Themed(props: { client: GatorClient; quit(): Promise<void>; clipboard: ClipboardService; voice?: VoiceSession }) {
  const data = useData()
  return <ThemeProvider theme={data.preferences.theme}><ConfigProvider bindings={terminalShortcuts(data.state?.desktop?.shortcuts)}>
    <ClipboardProvider value={props.clipboard}><Keymap.Provider><ToastProvider><DialogProvider><Shell quit={props.quit} voice={props.voice} /></DialogProvider></ToastProvider></Keymap.Provider></ClipboardProvider>
  </ConfigProvider></ThemeProvider>
}
export function App(props: { client: GatorClient; quit(): Promise<void>; clipboard: ClipboardService; voice?: VoiceSession }) {
  return <ClientProvider client={props.client}><DataProvider><Themed {...props} /></DataProvider></ClientProvider>
}
