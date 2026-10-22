export const defaultShortcuts = {
  new_chat: "Mod+N", command_search: "Mod+K", conversation_search: "Mod+F", settings: "Mod+,",
  send_message: "Enter", newline: "Shift+Enter", guide_work: "Mod+Enter",
  voice_input: "Mod+Shift+V", help: "F1",
} as const;

export type ShortcutAction = keyof typeof defaultShortcuts;
type KeyboardInput = { key: string; code?: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean; isComposing?: boolean };
const currentPlatform = () => typeof window !== "undefined" ? window.gator?.platform ?? "linux" : "linux";

export function shortcutBinding(action: ShortcutAction, bindings?: Record<string, string>) {
  return bindings?.[action] ?? defaultShortcuts[action];
}

export function shortcutMatches(event: KeyboardInput, action: ShortcutAction, bindings?: Record<string, string>, platform = currentPlatform()) {
  if (event.isComposing) return false;
  const parts = shortcutBinding(action, bindings).split("+");
  const key = parts.pop()!;
  const mods = new Set(parts.map(part => part === "Mod" ? platform === "darwin" ? "Meta" : "Ctrl" : part));
  const pressed = event.code === "Comma" ? "," : event.key.toLowerCase();
  return pressed === (key === "Space" ? " " : key.toLowerCase()) &&
    event.metaKey === mods.has("Meta") && event.ctrlKey === mods.has("Ctrl") &&
    event.altKey === mods.has("Alt") && event.shiftKey === mods.has("Shift");
}

export function shortcutLabel(action: ShortcutAction, bindings?: Record<string, string>, platform = currentPlatform()) {
  const parts = shortcutBinding(action, bindings).split("+");
  const mac = platform === "darwin";
  return parts.map(part => ({ Mod: mac ? "⌘" : "Ctrl", Meta: mac ? "⌘" : "Meta", Ctrl: mac ? "⌃" : "Ctrl", Alt: mac ? "⌥" : "Alt", Shift: mac ? "⇧" : "Shift" }[part] ?? part)).join(mac ? "" : "+");
}
