import { createEffect, createMemo, createSignal, onCleanup, type Accessor } from "solid-js"
import { useRenderer } from "@opentui/solid"
import type { Renderable, ScrollBoxRenderable } from "@opentui/core"
import { useClient } from "../context/client"
import { useData } from "../context/data"
import { useDialog } from "../ui/dialog"
import type { TimelineItem } from "./timeline"
import { readingItems, unreadStart, type ReadingAnchor } from "./transcript-reading"

// The native scrollbox owns scrolling/selection. This layer retains an item and
// its line offset, so changes above it do not move the user's reading position.
export function useTranscriptReader(timeline: Accessor<TimelineItem[]>, getScroll: () => ScrollBoxRenderable | undefined) {
  const client = useClient(), data = useData(), dialog = useDialog(), renderer = useRenderer()
  const items = createMemo(() => readingItems(timeline()))
  const indexes = createMemo(() => new Map(items().map((item, index) => [item.node, index])))
  const keys = createMemo(() => new Map(items().map((item, index) => [item.key, index])))
  const [following, setFollowing] = createSignal(true), [newCount, setNewCount] = createSignal(0), [notice, setNotice] = createSignal("")
  let context = "", conversation = "", generation = 0, alive = true, ready = false, restoring = false
  let anchor: ReadingAnchor | undefined, seen: Pick<ReadingAnchor, "seen" | "seen_version"> | undefined
  let lastTop = 0, shape = "", manual = false, pending: "latest" | "catch-up" | undefined
  const available = () => !dialog.stack.length && client.route === "chat" && !client.state?.panel && !client.state?.external && !client.snapshot.terminalBusy && !client.snapshot.sessionSwitching && client.snapshot.connection === "connected"
  const remember = (children?: Renderable[]) => {
    const scroll = getScroll(), all = items()
    if (!ready || !context || client.context !== context || !scroll || scroll.isDestroyed || !all.length) return
    const viewport = scroll.viewport
    children ??= scroll.content.getChildren()
    // Native slot/Show placeholders can sit outside document order. Only the
    // actual timeline boxes participate in this geometry search.
    const entries = children.filter(node => indexes().has(node.id))
    // Binary-search native geometry rather than reading thousands of
    // FFI-backed bounds per frame.
    let low = 0, high = entries.length
    while (low < high) {
      const middle = (low + high) >>> 1, node = entries[middle]
      if (node.screenY + node.height <= viewport.screenY) low = middle + 1
      else high = middle
    }
    let visible: Renderable | undefined
    for (let i = low; i < entries.length; i++) {
      const node = entries[i]
      if (node.screenY >= viewport.screenY + viewport.height) break
      if (indexes().has(node.id) && node.height > 0) { visible = node; break }
    }
    const index = visible ? indexes().get(visible.id)! : Math.max(0, all.length - 1), entry = all[index], tail = all.at(-1)!
    seen ??= { seen: tail.key, seen_version: tail.version }
    // A viewport can begin in the margin before an item. Preserve that small
    // negative offset too, or every append snaps the reader past the gap.
    anchor = { key: entry.key, index, offset: visible ? viewport.screenY - visible.screenY : 0, following: following(), ...seen }
    client.readingPositions.set(conversation, scroll.scrollTop)
    client.rememberReading(context, anchor)
  }
  createEffect(() => {
    const next = data.context, id = data.state?.conversation_id ?? data.state?.source ?? ""
    if (!next || next === context) return
    remember(); context = next; conversation = id; ready = false; restoring = true; anchor = undefined; seen = undefined; shape = ""
    setNotice(""); setFollowing(true); setNewCount(0)
    const token = ++generation
    void client.loadReading(next).then(() => {
      if (!alive || token !== generation) return
      anchor = client.readingAnchors.get(next); seen = anchor
      setFollowing(anchor?.following ?? true); ready = true; renderer.requestRender()
    })
  })
  const jump = (intent: "latest" | "catch-up") => { pending = intent; manual = true; renderer.requestRender() }
  const latest = () => jump("latest"), catchUp = () => jump("catch-up")
  const moved = () => { manual = true; renderer.requestRender() }
  const frame = () => {
    const scroll = getScroll(), all = items()
    if (!ready || client.context !== context || !scroll || scroll.isDestroyed || scroll.viewport.height < 1) return
    const children = scroll.content.getChildren(), nodes = new Map(children.map(node => [node.id, node]))
    const currentIndex = anchor && keys().get(anchor.key), currentNode = currentIndex === undefined ? undefined : nodes.get(all[currentIndex].node)
    const nextShape = `${scroll.viewport.width}:${scroll.viewport.height}:${scroll.scrollHeight}:${currentNode ? currentNode.screenY - scroll.viewport.screenY + scroll.scrollTop : 0}:${currentNode?.height ?? 0}`
    const changedLayout = nextShape !== shape
    if (pending) {
      const intent = pending; pending = undefined; restoring = false; setNotice("")
      const first = intent === "catch-up" ? unreadStart(all, seen) : all.length
      const node = all[first] && nodes.get(all[first].node)
      if (node) scroll.scrollBy(node.screenY - scroll.viewport.screenY)
      else scroll.scrollTo(scroll.scrollHeight)
    } else if (restoring || !manual && changedLayout && anchor && !following()) {
      if (following()) scroll.scrollTo(scroll.scrollHeight)
      else if (anchor && all.length) {
        const match = keys().get(anchor.key)
        const target = match ?? Math.min(anchor.index, all.length - 1), node = nodes.get(all[target].node)
        if (node) {
          const delta = node.screenY - scroll.viewport.screenY + Math.min(anchor.offset, Math.max(0, node.height - 1))
          if (delta) scroll.scrollBy(delta)
        }
        if (match === undefined) setNotice("Earlier item unavailable · /latest")
      }
      restoring = false
    }
    const atLatest = scroll.scrollTop + scroll.viewport.height >= scroll.scrollHeight - 1
    if (manual || !changedLayout && scroll.scrollTop !== lastTop || !atLatest && following()) setFollowing(atLatest)
    if (atLatest && available()) {
      setFollowing(true)
      const tail = all.at(-1)
      if (tail) seen = { seen: tail.key, seen_version: tail.version }
    }
    setNewCount(all.length - unreadStart(all, seen))
    client.setReadingStatus({ context, following: following(), count: newCount(), notice: notice() })
    shape = nextShape; lastTop = scroll.scrollTop; manual = false
    remember(children)
  }
  renderer.on("frame", frame)
  client.on("transcript-latest", latest); client.on("transcript-catch-up", catchUp); client.on("remember-reading", remember)
  onCleanup(() => { remember(); alive = false; renderer.off("frame", frame); client.off("transcript-latest", latest); client.off("transcript-catch-up", catchUp); client.off("remember-reading", remember); if (client.readingStatus?.context === context) client.setReadingStatus() })
  return { following, newCount, notice, latest, catchUp, moved, remember, restoring: () => !ready || restoring }
}
