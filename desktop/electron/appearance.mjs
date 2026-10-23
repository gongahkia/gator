// Use the host-owned profile theme for window chrome and native dialogs.
// The renderer cannot set native appearance through an arbitrary IPC value.
export function nativeAppearance(theme) {
  return theme === "light"
    ? { themeSource: "light", background: "#fafbf8" }
    : { themeSource: "dark", background: "#101010" };
}
