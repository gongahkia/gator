import { createHash } from "node:crypto"
import type { TimelineItem } from "./timeline"
import type { ReadingAnchor } from "@gator/application-host/contracts/frontend"
export type { ReadingAnchor }
export type ReadingItem = { key: string; version: string; node: string; item: TimelineItem }
export function readingSummary(count: number, shortcut: string, notice = "", width = Infinity) {
  if (notice) return notice
  const base = `${count ? `${count > 999 ? "999+" : count} new` : "Older"} · /catch-up · ${shortcut}`
  return base.length + 7 <= width ? base + " latest" : base
}
const hash = (value: unknown) => createHash("sha256").update(JSON.stringify(value)).digest("hex")
const retained = new WeakMap<TimelineItem, { base: string; version: string; node: string; occurrence?: number; reading?: ReadingItem }>()

// These identities are presentation-only. Backend message indexes, run IDs,
// update IDs and task owners remain the authority for contextual actions.
export function readingItems(items: TimelineItem[]): ReadingItem[] {
  const occurrences = new Map<string, number>()
  return items.map(item => {
    let cached = retained.get(item)
    if (!cached) {
      let identity: unknown, content: unknown, node: string
      if (item.kind === "message") {
        const message = item.message
      identity = [item.kind, message.role, message.run_id || message.revision_id, message.created_at,
          message.run_id || message.revision_id || message.created_at ? undefined : message.text]
        content = [message.text, message.result, message.bundle, message.activity, message.inputs]
        node = `message-${item.index}`
      } else if (item.kind === "update") {
        identity = [item.kind, item.update.id]
        const { title, summary, status, evidence, next_step } = item.update
        content = [title, summary, status, evidence, next_step]
        node = `update-${item.update.id}`
      } else {
        identity = [item.kind, item.turn.id]
        content = [item.turn.text, item.turn.response, item.turn.issue, item.turn.state]
        node = `dialogue-${item.turn.id}`
      }
      cached = { base: hash(identity), version: hash(content), node }
      retained.set(item, cached)
    }
    // The timeline reconciler already retains immutable unchanged items. Hash
    // each version once, rather than rehashing thousands on every append/tick.
    const occurrence = occurrences.get(cached.base) ?? 0
    occurrences.set(cached.base, occurrence + 1)
    if (!cached.reading || cached.occurrence !== occurrence) {
      cached.occurrence = occurrence
      cached.reading = { key: hash([cached.base, occurrence]), version: cached.version, node: cached.node, item }
    }
    return cached.reading
  })
}

export function unreadStart(items: ReadingItem[], seen?: Pick<ReadingAnchor, "seen" | "seen_version">): number {
  if (!seen || !items.length) return items.length
  const index = items.findIndex(item => item.key === seen.seen)
  if (index < 0) return 0
  return items[index].version === seen.seen_version ? index + 1 : index
}
