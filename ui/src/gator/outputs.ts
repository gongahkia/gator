import type { BackendBundle, BackendState } from "@gator/application-host/contracts/protocol"

// Include the newest retained result even when it is not a transcript message.
// A repeated bundle is one result; the current Go projection supplies its fields.
export function conversationBundles(state: BackendState | undefined): BackendBundle[] {
  const bundles = new Map<string, BackendBundle>()
  for (const message of state?.messages ?? []) if (message.bundle?.Path) bundles.set(message.bundle.Path, message.bundle)
  const latest = state?.work_details?.latest_bundle
  if (latest?.Path) bundles.set(latest.Path, latest)
  return [...bundles.values()]
}
