import { For, Show } from "solid-js"
import type { BackendActivity, BackendMessage, ActivityEntry } from "@gator/application-host/contracts/protocol"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import { useTheme } from "../context/theme"
import { literal } from "./text"

export const activityKey = (context: string, message: BackendMessage, index: number) => `${context}:${message.run_id || message.created_at || index}`
export function activitySummary(trace: BackendActivity) {
  const milliseconds = Date.parse(trace.finished_at || "") - Date.parse(trace.started_at)
  const seconds = Number.isFinite(milliseconds) && milliseconds >= 0 ? Math.floor(milliseconds / 1000) : undefined
  const duration = seconds === undefined ? "" : seconds < 1 ? "<1s" : seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  const label = trace.state === "stopped" ? "Stopped" : trace.state === "failed" ? "Work ended" : trace.state === "running" ? "Working" : "Worked"
  const failed = trace.entries.filter(entry => entry.kind === "tool" && entry.state === "failed").length
  return label + (duration ? `${trace.state === "completed" ? " for" : " after"} ${duration}` : "") + (failed ? ` · ${failed} failed ${failed === 1 ? "tool" : "tools"}` : "")
}
export function activityText(trace: BackendActivity) {
  return [activitySummary(trace), ...trace.entries.map(entry => entry.kind === "tool" ? `${entry.tool} · ${entry.state === "stopped" ? "unfinished" : entry.state}` : entry.text || ""), trace.truncated ? "Some activity was shortened to keep this conversation responsive." : ""].filter(Boolean).join("\n")
}
export function searchableMessage(message: BackendMessage, expanded: boolean) { return [message.text, expanded && message.activity ? activityText(message.activity) : ""].filter(Boolean).join("\n") }
export function Activity(props: { trace: BackendActivity; identity: string; live?: boolean }) {
  const client = useClient(), data = useData(), theme = useTheme()
  const hasEntries = () => props.trace.entries.some(entry => entry.kind !== "text" || entry.text?.trim()) || props.trace.truncated
  const open = () => props.live || data.activityExpanded.has(props.identity)
  const toggle = (key: string) => { if (client.activityExpanded.has(key)) client.activityExpanded.delete(key); else client.activityExpanded.add(key); client.emit("change") }
  const groups = () => {
    const result: ActivityEntry[][] = []
    for (const entry of props.trace.entries) {
      if (entry.kind === "text" && !entry.text?.trim()) continue
      if (entry.kind === "tool" && result.at(-1)?.[0].kind === "tool") result.at(-1)!.push(entry)
      else result.push([entry])
    }
    return result
  }
  return <Show when={hasEntries() || props.trace.state !== "completed"}><box>
    <Show when={!props.live}><text fg={theme.text.muted} onMouseUp={() => toggle(props.identity)}>{open() ? "▾ " : "▸ "}{activitySummary(props.trace)}{hasEntries() ? " · activity in Ctrl+P inspects/toggles" : ""}</text></Show>
    <Show when={open()}><For each={groups()}>{entries => <Show when={entries[0].kind === "tool"} fallback={<text fg={theme.text.muted}>{literal(entries[0].text)}</text>}>
      <text fg={theme.text.muted} onMouseUp={() => toggle(props.identity + ":" + entries[0].id)}>{entries.some(entry => entry.state === "running") ? "Using" : "Used"} {entries.length} {entries.length === 1 ? "tool" : "tools"} · {entries.filter(entry => entry.state === "failed").length} failed · activity in Ctrl+P inspects</text>
      <Show when={data.activityExpanded.has(props.identity + ":" + entries[0].id)}><For each={entries}>{entry => <text fg={entry.state === "failed" ? theme.text.feedback.warning.base : theme.text.muted}>  {literal(entry.tool)} · {entry.state === "stopped" ? "unfinished" : entry.state}</text>}</For></Show>
    </Show>}</For><Show when={props.trace.truncated}><text fg={theme.text.muted}>Some activity was shortened to keep this conversation responsive.</text></Show></Show>
  </box></Show>
}
