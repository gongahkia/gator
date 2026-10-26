import { createContext, useContext, type ParentProps } from "solid-js"
import type { Binding } from "@opentui/keymap"
import type { KeyEvent, Renderable } from "@opentui/core"

const defaults: Record<string, string> = {
  "input.submit": "return", "input.newline": "shift+return,ctrl+j",
  "dialog.select.prev": "up", "dialog.select.next": "down", "dialog.select.submit": "return",
  "dialog.select.page_up": "pageup", "dialog.select.page_down": "pagedown",
  "dialog.select.home": "ctrl+home", "dialog.select.end": "ctrl+end", "dialog.prompt.submit": "return",
}
export type Preferences = { animations: boolean; terminal: { copy: string }; cursor: { style: "block" }; scroll: { speed: number }; keybinds: { get(id: string): readonly Binding<Renderable, KeyEvent>[] } }
const Context = createContext<Preferences>()
export function ConfigProvider(props: ParentProps<{ bindings?: Record<string, string> }>) {
  const data: Preferences = { animations: !process.env.NO_COLOR && !process.env.GATOR_REDUCED_MOTION,
    terminal: { copy: "manual" }, cursor: { style: "block" }, scroll: { speed: 3 },
    keybinds: { get(id) { const key = props.bindings?.[id] || defaults[id]; return key ? [{ key, cmd: id }] : [] } } }
  return <Context.Provider value={data}>{props.children}</Context.Provider>
}
export function useConfig() { const data = useContext(Context); if (!data) throw new Error("Missing Gator configuration"); return { data } }
export const useNewDesigns = () => () => true
