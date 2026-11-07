import { SyntaxStyle } from "@opentui/core"
import { useRenderer } from "@opentui/solid"
import { onCleanup } from "solid-js"
import { useTheme } from "../context/theme"
import { literal } from "./text"

export function Markdown(props: { text?: string; streaming?: boolean }) {
  const theme = useTheme(), renderer = useRenderer()
  const style = SyntaxStyle.fromStyles({
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
  onCleanup(() => style.destroy())
  return <markdown content={literal(props.text)} syntaxStyle={style} fg={theme.text.base} conceal streaming={props.streaming ?? false} tableOptions={{ style: "columns", wrapMode: "word", cellPaddingX: 1 }} />
}
