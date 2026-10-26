import { RGBA } from "@opentui/core"
import { createContext, useContext, type ParentProps } from "solid-js"
const color = (hex: string) => RGBA.fromHex(hex)
function palette(name: string) {
  const light = name === "light", contrast = name === "contrast", mono = name === "mono"
  const base = color(light ? "#202523" : contrast ? "#ffffff" : "#e5e9e5"), muted = color(light ? "#4e5c56" : contrast ? "#dadada" : "#a5b1aa")
  const bg = color(light ? "#f6f8f5" : contrast ? "#000000" : "#101612"), raised = color(light ? "#e4ebe5" : mono ? "#242424" : "#1b2920")
  const accent = color(light ? "#246540" : mono ? "#dedede" : contrast ? "#baffca" : "#8ace9c")
  const action = { base, focused: base, disabled: muted, hover: raised }
  const text = { base, muted, faint: muted, action: { primary: action, secondary: action },
    formfield: { base, focused: base, disabled: muted, selected: accent },
    feedback: { info: { base: accent }, success: { base: accent }, warning: { base: color("#d4b76a") }, error: { base: color("#e98585") } } }
  const background = { base: bg, raised: { base: raised, high: raised },
    action: { primary: { focused: raised, hovered: raised }, secondary: { focused: raised, hovered: raised } },
    formfield: { focused: raised, disabled: bg } }
  return { text, background, hue: { accent: { 200: accent } }, accent }
}
type Theme = ReturnType<typeof palette>
const Context = createContext<() => Theme>()
export function ThemeProvider(props: ParentProps<{ theme: string }>) {
  return <Context.Provider value={() => palette(props.theme)}>{props.children}</Context.Provider>
}
export function useTheme() {
  const theme = useContext(Context); if (!theme) throw new Error("Missing Gator theme")
  const value = { get text() { return theme().text }, get background() { return theme().background },
    get hue() { return theme().hue }, get accent() { return theme().accent } }
  return Object.assign(value, { surface: (_name: string) => value })
}
export function ThemeContextProvider(props: ParentProps<{ context: string }>) { return <>{props.children}</> }
