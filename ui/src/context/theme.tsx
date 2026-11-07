import { RGBA } from "@opentui/core"
import { createContext, createMemo, useContext, type ParentProps } from "solid-js"

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
export function presetPalette(name: string, noColor = !!process.env.NO_COLOR): ReturnType<typeof terminalPalette> {
  if (name === "terminal" || noColor) return terminalPalette(noColor)
  const light = name === "light", contrast = name === "contrast", mono = name === "mono"
  const base = RGBA.fromHex(light ? "#202124" : "#f4f4f5"), bg = RGBA.fromHex(light ? "#fafafa" : contrast ? "#000000" : "#111113")
  const accent = RGBA.fromHex(mono ? (light ? "#202124" : "#f4f4f5") : contrast ? "#ffff00" : light ? "#5b21b6" : "#ae8bfa")
  const muted = RGBA.fromHex(light ? "#53545b" : "#b1b1bb"), raised = RGBA.fromHex(light ? "#ececf0" : "#242428")
  const action = { base, focused: accent, disabled: muted, hover: raised }
  const color = (value: string) => mono ? base : RGBA.fromHex(value)
  return { text: { base, muted, faint: muted, action: { primary: action, secondary: action }, formfield: { base, focused: base, disabled: muted, selected: accent },
    feedback: { info: { base: accent }, success: { base: color(light ? "#166534" : "#86efac") }, warning: { base: color(light ? "#854d0e" : "#fde047") }, error: { base: color(light ? "#b91c1c" : "#fca5a5") } } },
    background: { base: bg, raised: { base: raised, high: raised }, action: { primary: { focused: raised, hovered: raised }, secondary: { focused: raised, hovered: raised } }, formfield: { focused: raised, disabled: bg } },
    hue: { accent: { 200: accent } }, accent }
}
type Theme = ReturnType<typeof terminalPalette>
const Context = createContext<Theme>()
export function ThemeProvider(props: ParentProps<{ theme?: string }>) {
  const palette = createMemo(() => presetPalette(props.theme || "terminal"))
  const value: Theme = { get text() { return palette().text }, get background() { return palette().background }, get hue() { return palette().hue }, get accent() { return palette().accent } }
  return <Context.Provider value={value}>{props.children}</Context.Provider>
}
export function useTheme() {
  const theme = useContext(Context); if (!theme) throw new Error("Missing Gator theme")
  return Object.assign(theme, { surface: (_name: string) => theme })
}
export function ThemeContextProvider(props: ParentProps<{ context: string }>) { return <>{props.children}</> }
