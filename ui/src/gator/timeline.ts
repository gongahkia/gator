import type { BackendMessage, ChatUpdate } from "@gator/application-host/contracts/protocol"

export type TimelineItem = { kind: "message"; message: BackendMessage; index: number } | { kind: "update"; update: ChatUpdate }
const time = (value?: string) => Math.max(0, Date.parse(value ?? "") || 0)
// Preserve Gator's message indexes and put an explicitly replied-to card before
// its reply, including after clocks change. This ports the reference timeline.
export function chatTimeline(messages: BackendMessage[], updates: ChatUpdate[]): TimelineItem[] {
  const remaining = [...updates].sort((a, b) => time(a.created_at) - time(b.created_at) || a.id.localeCompare(b.id)), result: TimelineItem[] = []
  let at = 0
  messages.forEach((message, index) => {
    at = Math.max(at, time(message.created_at), time(message.activity?.finished_at || message.activity?.started_at))
    while (remaining.length && time(remaining[0].created_at) <= at) result.push({ kind: "update", update: remaining.shift()! })
    const replied = remaining.findIndex(update => update.id === message.reply_to_update_id)
    if (replied >= 0) result.push({ kind: "update", update: remaining.splice(replied, 1)[0] })
    result.push({ kind: "message", message, index })
  })
  result.push(...remaining.map(update => ({ kind: "update" as const, update })))
  return result
}
export const updateStatus = (status: string) => ({ failed: "Couldn’t check", interrupted: "Check interrupted", limited: "Check limit reached", recovered: "Checks resumed", completed: "Update" })[status] || status
export const updateWarning = (status: string) => ({ failed: "This check didn’t complete successfully.", interrupted: "This check stopped before it finished.", limited: "This check could not run within its request limits." })[status]
