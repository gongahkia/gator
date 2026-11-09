import { SyntaxStyle, type MarkdownRenderable } from "@opentui/core"
import { useRenderer } from "@opentui/solid"
import { Show, createMemo, onCleanup } from "solid-js"
import { useTheme } from "../context/theme"
import { literal } from "./text"

export function Markdown(props: { text?: string; streaming?: boolean }) {
  const theme = useTheme(), renderer = useRenderer()
  const style = createMemo(() => { const value = SyntaxStyle.fromStyles({
    default: { fg: theme.text.base },
    "markup.heading": { fg: theme.text.base, bold: true },
    ...Object.fromEntries([1, 2, 3, 4, 5, 6].map(level => [`markup.heading.${level}`, { fg: theme.text.base, bold: true }])),
    "markup.strong": { bold: true },
    "markup.italic": { italic: true },
    "markup.raw": { fg: theme.accent },
    "markup.link": { fg: theme.accent, underline: true },
    "markup.link.label": { fg: theme.accent, underline: true },
    "markup.link.url": { fg: theme.text.muted, underline: true },
    "markup.list": { fg: theme.text.muted },
    "markup.quote": { fg: theme.text.muted, italic: true },
    conceal: { fg: theme.text.muted },
    keyword: { fg: theme.accent, bold: true },
    string: { fg: theme.text.base },
    comment: { fg: theme.text.muted, italic: true },
    number: { fg: theme.accent },
  }, renderer.nativeScene)
  return value })
  return <Show when={style()} keyed>{value => {
    let node: MarkdownRenderable | undefined
    onCleanup(() => {
      // Solid can defer native removal until the next render pass. Highlighting
      // still borrows this style in the meantime, so invalidate/destroy its
      // native consumers before releasing the scene-owned style.
      try { if (node && !node.isDestroyed) node.destroyRecursively() }
      finally { value.destroy() }
    })
    return <markdown ref={value => { node = value }} content={literal(props.text)} syntaxStyle={value} fg={theme.text.base} conceal streaming={props.streaming ?? false} tableOptions={{ style: "columns", wrapMode: "word", cellPaddingX: 1 }} />
  }}</Show>
}
