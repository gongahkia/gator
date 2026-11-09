import { Keymap } from "../context/keymap"

// Use the registered keymap rather than a second set of shortcut defaults.
export function useKeyHints() {
  const shortcuts = Keymap.useShortcuts()
  return (id: string) => (shortcuts.get(id) || "Unbound")
    .replace(/\bctrl\b/gi, "Ctrl").replace(/\bshift\b/gi, "Shift")
    .replace(/\balt\b/gi, "Alt").replace(/\bmeta\b/gi, "Meta")
    .replace(/\btab\b/gi, "Tab")
    .replace(/\b(return|enter)\b/gi, "Enter").replace(/\+([a-z])\b/g, (_, key: string) => "+" + key.toUpperCase())
}
