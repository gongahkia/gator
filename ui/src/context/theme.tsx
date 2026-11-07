import { RGBA } from "@opentui/core"
import { createContext, useContext, type ParentProps } from "solid-js"

// Keep terminal color intent, not RGB approximations: SGR 39/49 preserve the
// user's palette, light/dark changes and background transparency even when
// OSC palette queries are unsupported (for example inside a multiplexer).
export function terminalPalette(noColor = !!process.env.NO_COLOR) {
  const base = RGBA.defaultForeground(), bg = RGBA.defaultBackground()
  const indexed = (slot: number) => noColor ? base : RGBA.fromIndex(slot)
  const accent = indexed(6), muted = base, raised = bg
  const action = { base, focused: accent, disabled: muted, hover: raised }
  const text = { base, muted, faint: muted, action: { primary: action, secondary: action },
    formfield: { base, focused: base, disabled: muted, selected: accent },
    feedback: { info: { base: accent }, success: { base: indexed(2) }, warning: { base: indexed(3) }, error: { base: indexed(1) } } }
  const background = { base: bg, raised: { base: raised, high: raised },
    action: { primary: { focused: raised, hovered: raised }, secondary: { focused: raised, hovered: raised } },
    formfield: { focused: raised, disabled: bg } }
  return { text, background, hue: { accent: { 200: accent } }, accent }
}
type Theme = ReturnType<typeof terminalPalette>
const Context = createContext<Theme>()
// The shared backend theme remains a desktop preference. Native terminal
// surfaces always use the terminal's own colors.
export function ThemeProvider(props: ParentProps<{ theme?: string }>) {
  return <Context.Provider value={terminalPalette()}>{props.children}</Context.Provider>
}
export function useTheme() {
  const theme = useContext(Context); if (!theme) throw new Error("Missing Gator theme")
  return Object.assign({}, theme, { surface: (_name: string) => theme })
}
export function ThemeContextProvider(props: ParentProps<{ context: string }>) { return <>{props.children}</> }
