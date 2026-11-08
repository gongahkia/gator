// Navigation labels are frontend concerns; availability and execution stay in Go.
export const commandGroups = ["Chats", "Following", "Saved", "Files", "Settings", "Help and application"] as const
const groups: Record<string, string[]> = {
  Following: ["following", "goals", "followup", "jobs", "inbox", "overview", "updates", "pilot", "plan", "notifications", "check-health", "cancel-update-reply"],
  Saved: ["saved", "knowledge", "history", "drafts", "memories", "receipts", "note", "skill", "learnings", "suggestions", "prompt-history"],
  Files: ["files", "outputs", "attach", "attach-file", "attachments", "detach", "draft-context", "review", "save", "apply", "retry", "artifact", "paste-image", "source", "source-refresh", "source ignore", "source unignore", "source ignored"],
  Settings: ["settings", "models", "model", "mode", "effort", "sources", "connections", "connector", "browser", "memory", "appearance", "theme", "usage", "data", "recovery", "voice", "web-origin", "permissions", "statusline"],
  "Help and application": ["help", "about", "doctor", "exit", "quit", "sidebar", "reconnect", "links", "continue-external"],
}
export function commandAliases(name: string): string[] {
  return ({ mode: ["permissions", "work mode", "read only", "authority"], theme: ["color scheme", "dark", "light", "contrast"],
    model: ["provider", "local model", "API key"], memory: ["remember", "privacy", "recall"],
    followup: ["watch", "recurring checks", "schedule"], sources: ["accounts", "context"],
    "source ignore": ["ignore instructions"], "source unignore": ["restore instructions"] } as Record<string, string[]>)[name] ?? []
}
export function commandGroup(name: string): string {
  return Object.entries(groups).find(([, names]) => names.includes(name))?.[0] ?? "Chats"
}
