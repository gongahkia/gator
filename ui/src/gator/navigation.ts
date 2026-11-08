import type { BackendPanel } from "@gator/application-host/contracts/protocol"

export type NavigationPage = "chat" | "chats" | "following" | "saved" | "settings"

export function navigationPage(route: string, panel?: BackendPanel): NavigationPage {
  if (route === "following" || route === "saved" || route === "settings") return route
  if (panel?.navigation) return panel.navigation
  if (panel?.catalog || panel?.kind === "conversations") return "chats"
  if (/^(goals?|agenda|plan|suggestions?|followup|inbox|overview|jobs)(-|$)/.test(panel?.kind || "")) return "following"
  if (/^(context|knowledge|history|learnings?)(-|$)/.test(panel?.kind || "")) return "saved"
  if (/^(models?|providers?|appearance|memory|settings|connections|browser|usage|data|recovery|status-line)(-|$)/.test(panel?.kind || "")) return "settings"
  return "chat"
}
