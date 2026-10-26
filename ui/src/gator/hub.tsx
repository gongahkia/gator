import { For, Show, createSignal, createEffect, onCleanup } from "solid-js"
import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/solid"
import type { ScrollBoxRenderable } from "@opentui/core"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"
import { useDialog } from "../ui/dialog"
import { literal } from "./text"
export function Hub() {
  const client = useClient(), data = useData(), theme = useTheme(), dialog = useDialog()
  const [selected, setSelected] = createSignal(0)
  const renderer = useRenderer(), dimensions = useTerminalDimensions()
  let scroll: ScrollBoxRenderable | undefined, reveal: (() => void) | undefined, previousRoute = ""
  createEffect(() => {
    const route = data.route
    selected(); dimensions()
    if (route !== previousRoute) { previousRoute = route; setSelected(0) }
    if (reveal) renderer.off("frame", reveal)
    reveal = () => { if (scroll && !scroll.isDestroyed) scroll.scrollChildIntoView(`hub-item-${selected()}`); reveal = undefined }
    renderer.once("frame", reveal)
  })
  onCleanup(() => { if (reveal) renderer.off("frame", reveal) })
  const items = () => data.route === "following" ? [
    ["Ongoing goals", "Saved goals and plans; recurring checks have their own status", "/goals"],
    ["Manage recurring checks", "Sources, timing, permissions and check health", "/followup"],
    ["Inbox", "Retained results of configured checks", "/inbox"],
    ["Activity and attention", "Changes, next actions and blocked work", "/overview"],
    ["This chat's updates", "Read, done, reply and usefulness are separate actions", "/updates"],
  ] : data.route === "saved" ? [
    ["Knowledge", "Notes, decisions, references and saved guidance", "/knowledge"],
    ["All conversations", "Search indexed history, pin, rename and archive", "/conversations"],
    ["Task runs", "Retained results and individual run details", "/history"],
    ["Unsent drafts", "Recover drafts and manage retention", "/drafts"],
    ["Files in this chat", "Edit, preview, version, export and Ask Gator", "/files"],
    ["Memory receipts", "Guarded corrections, forgetting and undo", "/memories"],
    ["Connected request receipts", "Inspect outcomes and uncertain sends", "/receipts"],
  ] : [
    ["Models and providers", "Choose model, effort, credentials or local installation", "/models"],
    ["Connections", "Sign in, verify, reconnect and disconnect accounts", "/connections"],
    ["Memory and privacy", "Personal memory, learning and earlier-chat recall", "/memory"],
    ["Appearance and shortcuts", "Gator theme, keys and display preferences", "/appearance"],
    ["Voice", "Dictation, spoken replies and provider preferences", "/voice"],
    ["Usage and limits", "Accounting, limits, pause and resume", "/usage"],
    ["Data and backups", "Retention, reviewed deletion, export, restore and relink", "/data"],
    ["Recovery", "Offline checks, supervisor and start-at-login reviews", "/recovery"],
    ["About and updates", "Version, dependencies and reviewed update path", "/about"],
  ]
  const open = (index: number) => { const item = items()[index]; if (item) void client.navigate(item[2]).catch(e => client.fail(e)) }
  useKeyboard(event => {
    if (dialog.stack.length || event.ctrl || event.meta) return
    if (event.name === "up" || event.name === "down") { event.preventDefault(); setSelected(v => (v + (event.name === "up" ? -1 : 1) + items().length) % items().length) }
    if (event.name === "return") { event.preventDefault(); open(selected()) }
    if (event.name === "escape") { event.preventDefault(); void client.navigate("/chat") }
  })
  return <box flexGrow={1} minHeight={0} padding={2} gap={1}><text fg={theme.accent}><b>{data.route[0].toUpperCase() + data.route.slice(1)}</b></text>
    <scrollbox id="hub-items" ref={value => { scroll = value }} flexGrow={1} minHeight={0}>
      <For each={items()}>{(item, index) => <box id={`hub-item-${index()}`} paddingY={1} backgroundColor={selected() === index() ? theme.background.raised.high : undefined} onMouseUp={() => open(index())}><text fg={theme.text.base}>{selected() === index() ? "› " : "  "}{item[0]}</text><text fg={theme.text.muted}>  {item[1]}</text></box>}</For>
      <Show when={data.route === "following"}>
        <Show when={data.state?.following_issue}><text fg={theme.text.feedback.warning.base}>{literal(data.state?.following_issue)}</text></Show>
        <Show when={data.state?.checks_issue}><text fg={theme.text.feedback.warning.base}>{literal(data.state?.checks_issue)}</text></Show>
        <Show when={!data.state?.checks_issue && !data.state?.checks?.length}><text fg={theme.text.muted}>No recurring checks configured.</text></Show>
      </Show>
      <For each={data.route === "following" ? data.state?.following_goals : []}>{goal => <text fg={theme.text.muted}>{literal(goal.title)} · {literal(goal.status)}{data.state?.checks?.some(check => check.goal_id === goal.id) ? "" : data.state?.checks_issue ? " · Check status unavailable" : " · No recurring check attached"}</text>}</For>
      <For each={data.route === "following" ? data.state?.checks : []}>{check => <text fg={theme.text.muted}>{literal(check.title)} · {literal(check.state)} · {literal(check.health)}</text>}</For>
    </scrollbox><text fg={theme.text.muted}>↑/↓ select · Enter open · Esc chat · Your message draft is kept.</text>
  </box>
}
