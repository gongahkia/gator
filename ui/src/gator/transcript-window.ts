import { createEffect, createMemo, createSignal, onCleanup, type Accessor } from "solid-js"
import { useRenderer } from "@opentui/solid"
import type { ScrollBoxRenderable } from "@opentui/core"
import { useData } from "../context/data"
import type { TimelineItem } from "./timeline"
import { readingItems } from "./transcript-reading"

type Target = { key: string; index: number; offset: number } | "bottom"
const size = 80, overscan = 24, threshold = 160
const margin = (item: TimelineItem) => item.kind === "update" ? 0 : 1

// Keep the complete immutable timeline for search/actions. Only the native
// content window mounts; measured/estimated spacers retain continuous scrolling.
export function useTranscriptWindow(timeline: Accessor<TimelineItem[]>, getScroll: () => ScrollBoxRenderable | undefined) {
  const renderer = useRenderer(), data = useData()
  const all = createMemo(() => readingItems(timeline()))
  const positions = createMemo(() => new Map(all().map((item, index) => [item.key, index])))
  const [range, setRange] = createSignal({ start: 0, end: 0 })
  const [before, setBefore] = createSignal(0), [after, setAfter] = createSignal(0)
  const visible = createMemo(() => timeline().slice(range().start, range().end))
  let owner = "", width = 0, prefix = [0], heights = new Map<string, number>()
  let pending: Target | undefined, settling = false, oldItems: ReturnType<typeof all> | undefined
  let stableTarget: Target | undefined, lastTop = 0
  const rebuild = () => {
    prefix = [0]
    for (const entry of all()) prefix.push(prefix.at(-1)! + (heights.get(entry.key) ?? 8))
  }
  const publishSpacers = () => {
    const count = all().length
    setBefore(prefix[Math.min(count, range().start)] ?? 0)
    setAfter(Math.max(0, (prefix.at(-1) ?? 0) - (prefix[Math.min(count, range().end)] ?? 0)))
  }
  const include = (index: number) => {
    const total = all().length
    const start = total <= threshold ? 0 : Math.max(0, Math.min(total - size, index - overscan))
    const end = total <= threshold ? total : Math.min(total, start + size)
    // Native text selection borrows its renderables. Keep those consumers alive
    // and expand when selecting across a boundary; prune after selection clears.
    const selected = !!renderer.getSelection()
    const next = selected ? { start: Math.min(start, range().start), end: Math.max(end, range().end) } : { start, end }
    if (next.start === range().start && next.end === range().end) return false
    setRange(next); publishSpacers(); settling = true; renderer.requestRender(); return true
  }
  const ensure = (index: number) => {
    index = Math.max(0, Math.min(index, all().length - 1))
    if (index >= range().start && index < range().end) return
    const entry = all()[index]
    if (entry) { pending = { key: entry.key, index, offset: 0 }; include(index) }
  }
  createEffect(() => {
    const next = data.context, entries = all()
    if (owner !== next) {
      owner = next; heights.clear(); width = 0; oldItems = undefined; pending = "bottom"; stableTarget = undefined; lastTop = 0
      setRange({ start: 0, end: 0 })
    }
    if (entries === oldItems) return
    oldItems = entries
    const retained = new Set(entries.map(item => item.key))
    for (const key of heights.keys()) if (!retained.has(key)) heights.delete(key)
    rebuild()
    if (range().end === 0) include(Math.max(0, entries.length - 1))
    else {
      // Preserve the current window through ordinary appends. A latest reader
      // moves to the new tail on the following native frame.
      if (entries.length <= threshold) include(0)
      publishSpacers()
    }
  })
  const indexAt = (row: number) => {
    let low = 0, high = Math.max(0, prefix.length - 1)
    while (low < high) { const middle = (low + high) >>> 1; if (prefix[middle + 1] <= row) low = middle + 1; else high = middle }
    return Math.min(low, Math.max(0, all().length - 1))
  }
  const frame = () => {
    const scroll = getScroll(), entries = all()
    if (!scroll || scroll.isDestroyed || !entries.length || scroll.viewport.height < 1) { settling = false; return }
    // Short histories retain the established full native scrollbox behavior;
    // there is no estimated geometry to reconcile until a window is needed.
    if (entries.length <= threshold) { pending = undefined; stableTarget = undefined; settling = false; return }
    const nodes = new Map(scroll.content.getChildren().map(node => [node.id, node]))
    const viewport = scroll.viewport
    const first = nodes.get("transcript-before"), origin = first ? first.screenY - viewport.screenY + scroll.scrollTop : 0
    const capture = (): Target => {
      if (scroll.scrollTop + viewport.height >= scroll.scrollHeight - 1) return "bottom"
      for (let i = range().start; i < range().end; i++) {
        const entry = entries[i], node = entry && nodes.get(entry.node)
        if (node && node.height > 0 && node.screenY + node.height > viewport.screenY && node.screenY < viewport.screenY + viewport.height) return { key: entry.key, index: i, offset: viewport.screenY - node.screenY }
      }
      const index = indexAt(scroll.scrollTop - origin), entry = entries[index]
      return { key: entry.key, index, offset: scroll.scrollTop - origin - prefix[index] - margin(entry.item) }
    }
    const movedByUser = scroll.scrollTop !== lastTop
    const target = pending ?? (movedByUser ? capture() : stableTarget ?? capture())
    let changed = false
    if (width !== viewport.width) { heights.clear(); width = viewport.width; changed = true }
    for (let i = range().start; i < range().end; i++) {
      const entry = entries[i], node = entry && nodes.get(entry.node)
      if (!node || node.height < 1) continue
      const height = node.height + margin(entry.item) * 2
      if (heights.get(entry.key) !== height) { heights.set(entry.key, height); changed = true }
    }
    if (changed) { rebuild(); publishSpacers() }
    const index = target === "bottom" ? entries.length - 1 : positions().get(target.key) ?? Math.min(target.index, entries.length - 1)
    const movedWindow = include(index)
    if (changed || movedWindow) {
      pending = target; settling = true; lastTop = scroll.scrollTop; renderer.requestRender(); return
    }
    if (pending) {
      if (target === "bottom") scroll.scrollTo(scroll.scrollHeight)
      else {
        const node = nodes.get(entries[index].node)
        if (!node || node.height < 1) { settling = true; renderer.requestRender(); return }
        const delta = node.screenY - viewport.screenY + Math.min(target.offset, Math.max(0, node.height - 1))
        if (delta) scroll.scrollBy(delta)
      }
      pending = undefined
    }
    stableTarget = capture(); lastTop = scroll.scrollTop
    settling = false
  }
  renderer.on("frame", frame)
  onCleanup(() => { renderer.off("frame", frame); heights.clear() })
  return { visible, before, after, ensure, active: () => all().length > threshold, ready: () => !settling }
}

export type TranscriptWindow = ReturnType<typeof useTranscriptWindow>
