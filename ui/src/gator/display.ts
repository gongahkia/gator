// Display-only labels: retained timestamps and paths are never rewritten.
export function displayDate(value?: string, now = new Date(), includeTime = false): string {
  if (!value || /^0001-/.test(value)) return ""
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ""
  const time = date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
  if (date.toDateString() === now.toDateString()) return includeTime ? `Today · ${time}` : time
  const day = date.toLocaleDateString(undefined, { month: "short", day: "numeric", ...(date.getFullYear() !== now.getFullYear() ? { year: "numeric" as const } : {}) })
  return includeTime ? `${day} · ${time}` : day
}

export function folderLabel(source?: string): string {
  if (!source) return ""
  return source.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || source
}
