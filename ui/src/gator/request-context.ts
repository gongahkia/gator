import type { BackendState } from "@gator/application-host/contracts/protocol"
import type { Draft } from "@gator/application-host/contracts/frontend"

// Display the host's current selections; neither this summary nor model setup
// grants accounts, tools, permissions, or approval to a future request.
export function requestContext(state: BackendState | undefined, draft: Draft) {
  const composer = state?.composer, readiness = composer?.readiness, budget = composer?.budget
  const selected = state?.sources?.selected ?? state?.work_details?.connector_ids
  const accounts = selected?.map(id => {
    const source = state?.sources?.choices.find(source => source.id === id)
    return source ? `${source.label} (${source.service}; ${source.status}; ${source.access})` : `${id} (details unavailable; inspect Sources)`
  })
  return [
    `Model: ${state?.provider || "Not selected"}${state?.model ? " / " + state.model : ""}`,
    readiness?.issue || (readiness?.state === "configured" ? "Configured; endpoint connectivity and provider acceptance are checked when you send." : "Open Models to inspect configuration."),
    `Processing destination: ${readiness?.processing.destination || "Not established"}`,
    readiness?.processing.description || "Inspect the selected provider before sending private context.",
    "",
    `Workspace: ${state?.source || "None selected"}`,
    "Workspace context and relevant saved memory may also be supplied to the model. Use /context to inspect supplied context.",
    `Additional workspace attachments: ${state?.work_details?.attachments.join(", ") || "None"}`,
    `Private draft files: ${draft.uploads?.map(file => file.name).join(", ") || "None"}`,
    `Connected accounts selected: ${accounts === undefined ? "Unavailable until the host loads this selection" : accounts.length ? accounts.join("; ") : "None"}`,
    "Accounts are optional until a task needs a connected service. Selection alone does not prove live access or approve changes. Public web tools can contact reviewed services independently of connected-account selection.",
    "",
    `Work mode: ${state?.work_details?.mode || "Not available"}`,
    budget ? `Work limit: ${budget.work_steps} steps. Each Code specialist call: up to ${budget.code_steps} steps.` : "Work limits are unavailable until the host loads its settings.",
    "Conversation, planning and provider retries can use additional model calls. Step limits are not token or monetary spending caps. Cloud usage follows your provider's API billing.",
    draft.plan_first ? "Plan first: review the proposal before implementation. Later external actions still need their own approval." : "Plan first is off. You can enable it in Draft details before sending.",
    "Future selection changes do not change running tasks, queued requests or retained evidence.",
  ].filter(value => value !== undefined).join("\n")
}
