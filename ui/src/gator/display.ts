// Display-only labels: retained timestamps and paths are never rewritten.
export function displayDate(value?: string, now = new Date()): string {
  if (!value || /^0001-/.test(value)) return ""
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ""
  if (date.toDateString() === now.toDateString()) return date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric", ...(date.getFullYear() !== now.getFullYear() ? { year: "numeric" as const } : {}) })
}

export function folderLabel(source?: string): string {
  if (!source) return ""
  return source.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || source
}
