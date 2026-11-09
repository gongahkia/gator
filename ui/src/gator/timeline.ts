import type { BackendMessage, ChatUpdate, DialogueMessage } from "@gator/application-host/contracts/protocol"

export type TimelineItem = { kind: "message"; message: BackendMessage; index: number } | { kind: "update"; update: ChatUpdate } | {kind:"dialogue"; turn:DialogueMessage}

// Go sends complete JSON snapshots, so unchanged messages arrive as new objects.
// Preserve identity for Solid's keyed list instead of remounting every Markdown
// view on each status tick, acknowledgement, update receipt or appended message.
export function createTimelineReconciler() {
  let context = "", previous = new Map<string, { serialized: string; item: TimelineItem }>()
  return (owner: string, items: TimelineItem[]): TimelineItem[] => {
    if (owner !== context) { previous.clear(); context = owner }
    const next = new Map<string, { serialized: string; item: TimelineItem }>()
    const retained = items.map(item => {
      const key = item.kind === "message" ? `message:${item.index}` : item.kind === "update" ? `update:${item.update.id}` : `dialogue:${item.turn.id}`
      const serialized = JSON.stringify(item), before = previous.get(key)
      const entry = before?.serialized === serialized ? before : { serialized, item }
      next.set(key, entry)
      return entry.item
    })
    previous = next
    return retained
  }
}
const time = (value?: string) => Math.max(0, Date.parse(value ?? "") || 0)
// Preserve Gator's message indexes and put an explicitly replied-to card before
// its reply, including after clocks change. This ports the reference timeline.
export function chatTimeline(messages: BackendMessage[], updates: ChatUpdate[], dialogue:DialogueMessage[] = []): TimelineItem[] {
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
  const pending=[...dialogue].sort((a,b)=>time(a.created_at)-time(b.created_at)||a.sequence-b.sequence), combined:TimelineItem[]=[]
  for(const item of result) {
    const at=item.kind==="message"?Math.max(time(item.message.created_at),time(item.message.activity?.finished_at||item.message.activity?.started_at)):item.kind==="update"?time(item.update.created_at):time(item.turn.created_at)
    while(pending.length && time(pending[0].created_at)<=at) combined.push({kind:"dialogue",turn:pending.shift()!})
    combined.push(item)
  }
  combined.push(...pending.map(turn=>({kind:"dialogue" as const,turn})))
  return combined
}
export const updateStatus = (status: string) => ({ failed: "Couldn’t check", interrupted: "Check interrupted", limited: "Check limit reached", recovered: "Checks resumed", completed: "Update" })[status] || status
export const updateWarning = (status: string) => ({ failed: "This check didn’t complete successfully.", interrupted: "This check stopped before it finished.", limited: "This check could not run within its request limits." })[status]
