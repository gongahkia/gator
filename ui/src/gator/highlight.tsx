import { For, createMemo } from "solid-js"
import { literal } from "./text"

export function Highlight(props: { text?: string; query?: string }) {
  const parts = createMemo(() => {
    const text = literal(props.text), query = literal(props.query)
    if (!query) return [{ text, match: false }]
    const pattern = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "giu")
    const parts: { text: string; match: boolean }[] = []; let previous = 0
    for (const match of text.matchAll(pattern)) {
      parts.push({ text: text.slice(previous, match.index), match: false }, { text: match[0], match: true }); previous = match.index + match[0].length
    }
    parts.push({ text: text.slice(previous), match: false }); return parts
  })
  return <For each={parts()}>{part => part.match ? <b>{part.text}</b> : <span>{part.text}</span>}</For>
}
